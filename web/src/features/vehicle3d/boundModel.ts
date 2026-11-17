// Géométries et placements explicites (voir ADR 0041) ; aucun nom BOM ni offre ne choisit un asset.
import * as THREE from 'three'
import { clone as cloneSkeleton } from 'three/addons/utils/SkeletonUtils.js'
import type { VehicleIndex, VehicleObject } from './mapping.ts'
import { ManifestError, type AssetDefinition, type ManifestResolution } from './manifest.ts'
import { disposeTree, rememberMaterial, type Built, type Item } from './scene.ts'
import type { Vehicle3DNode } from './types.ts'

type Entry = { users: number; controller: AbortController; scene: THREE.Object3D | null; promise: Promise<THREE.Object3D> }
type Lease = { scene: THREE.Object3D; release: () => void }
type Parser = (buffer: ArrayBuffer) => Promise<THREE.Object3D>
const MAX_ASSET_BYTES = 128 * 1024 * 1024

async function parseGLB(buffer: ArrayBuffer): Promise<THREE.Object3D> {
  const [{ GLTFLoader }, { MeshoptDecoder }] = await Promise.all([
    import('three/addons/loaders/GLTFLoader.js'), import('three/addons/libs/meshopt_decoder.module.js'),
  ])
  return (await new GLTFLoader().setMeshoptDecoder(MeshoptDecoder).parseAsync(buffer, '')).scene
}

/** Le fichier doit être autonome : son empreinte couvre aussi les données et textures utilisées. */
export function validateGLB(buffer: ArrayBuffer): void {
  if (buffer.byteLength < 20) throw new ManifestError('invalid_asset', 'GLB tronqué.')
  const view = new DataView(buffer)
  if (view.getUint32(0, true) !== 0x46546c67 || view.getUint32(4, true) !== 2 || view.getUint32(8, true) !== buffer.byteLength || view.getUint32(16, true) !== 0x4e4f534a) {
    throw new ManifestError('invalid_asset', 'En-tête GLB invalide.')
  }
  const length = view.getUint32(12, true)
  if (length > buffer.byteLength - 20) throw new ManifestError('invalid_asset', 'Bloc JSON GLB tronqué.')
  const json = JSON.parse(new TextDecoder().decode(new Uint8Array(buffer, 20, length)).replace(/\0+$/, '')) as {
    buffers?: { uri?: unknown }[]; images?: { uri?: unknown }[]
  }
  if ([...(json.buffers ?? []), ...(json.images ?? [])].some(resource => resource.uri !== undefined)) {
    throw new ManifestError('external_resource', 'GLB non autonome : ressources externes non couvertes par l’empreinte.')
  }
}

function abortable<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) {
    void promise.catch(() => {}) // la ressource peut encore finir/rejeter après l'annulation du consommateur
    return Promise.reject(new DOMException('Chargement annulé', 'AbortError'))
  }
  return new Promise((resolve, reject) => {
    const onAbort = () => { signal.removeEventListener('abort', onAbort); reject(new DOMException('Chargement annulé', 'AbortError')) }
    signal.addEventListener('abort', onAbort, { once: true })
    promise.then(value => { signal.removeEventListener('abort', onAbort); resolve(value) }, error => { signal.removeEventListener('abort', onAbort); reject(error) })
  })
}

/** Un asset actif est partagé ; le dernier bail libère ses géométries, matériaux et textures. */
export class BoundAssetStore {
  private entries = new Map<string, Entry>()
  private closed = false
  private readonly fetcher: typeof fetch
  private readonly parser: Parser

  constructor(fetcher: typeof fetch = (...args) => fetch(...args), parser: Parser = parseGLB) { this.fetcher = fetcher; this.parser = parser }

  async acquire(asset: AssetDefinition, signal: AbortSignal): Promise<Lease> {
    if (this.closed || signal.aborted) throw new DOMException('Chargement annulé', 'AbortError')
    const key = `${asset.path}/${asset.sha256}`
    let entry = this.entries.get(key)
    if (!entry) {
      const controller = new AbortController()
      entry = { users: 0, controller, scene: null, promise: Promise.resolve(new THREE.Group()) }
      const created = entry
      entry.promise = this.read(asset, controller.signal).then(scene => {
        created.scene = scene
        if (created.users === 0) { disposeTree(scene); created.scene = null }
        return scene
      })
      this.entries.set(key, entry)
    }
    const held = entry
    held.users++
    let released = false
    const release = () => {
      if (released) return
      released = true
      held.users = Math.max(0, held.users - 1)
      if (held.users === 0) {
        held.controller.abort()
        if (held.scene) { disposeTree(held.scene); held.scene = null }
        if (this.entries.get(key) === held) this.entries.delete(key)
      }
    }
    try { return { scene: await abortable(held.promise, signal), release } }
    catch (error) { release(); throw error }
  }

  private async read(asset: AssetDefinition, signal: AbortSignal) {
    const response = await this.fetcher(asset.path, { signal })
    if (!response.ok || (response.headers.get('content-type') ?? '').includes('text/html')) throw new ManifestError('asset_missing', `Modèle introuvable : ${asset.path}.`)
    if (Number(response.headers.get('content-length')) > MAX_ASSET_BYTES) throw new ManifestError('asset_too_large', 'Modèle supérieur à 128 Mo.')
    const buffer = await response.arrayBuffer()
    if (buffer.byteLength > MAX_ASSET_BYTES) throw new ManifestError('asset_too_large', 'Modèle supérieur à 128 Mo.')
    const digest = await crypto.subtle.digest('SHA-256', buffer)
    const hash = [...new Uint8Array(digest)].map(n => n.toString(16).padStart(2, '0')).join('')
    if (hash !== asset.sha256) throw new ManifestError('integrity_mismatch', `Empreinte incorrecte pour ${asset.key}.`)
    validateGLB(buffer)
    return this.parser(buffer)
  }

  get residentAssets() { return this.entries.size }
  dispose() {
    this.closed = true
    for (const entry of this.entries.values()) {
      entry.users = 0; entry.controller.abort()
      if (entry.scene) { disposeTree(entry.scene); entry.scene = null }
    }
    this.entries.clear()
  }
}

function findMeshRoot(scene: THREE.Object3D, path: string[]): THREE.Object3D {
  let current = scene
  for (const name of path) {
    const matches = current.children.filter(child => child.name === name)
    if (matches.length !== 1) throw new ManifestError('mesh_missing', `Chemin glTF absent ou ambigu : ${path.join(' / ')}.`)
    current = matches[0]
  }
  return current
}

export async function buildBoundScene(resolution: ManifestResolution, nodes: Vehicle3DNode[], store: BoundAssetStore, signal: AbortSignal): Promise<Built> {
  const root = new THREE.Group(), items: Item[] = [], materials = new Set<THREE.Material>(), skeletons = new Set<THREE.Skeleton>()
  root.name = 'vehicle3d-bound'
  const unavailable = new Map<number, string>(resolution.unavailable)
  const activeAssets = new Map(resolution.instances.map(instance => [instance.asset.key, instance.asset]))
  const leases = new Map<string, Lease>()
  const failures = new Map<string, string>()
  await Promise.all([...activeAssets].map(async ([key, asset]) => {
    try { leases.set(key, await store.acquire(asset, signal)) }
    catch (error) { failures.set(key, error instanceof Error ? error.message : String(error)) }
  }))
  const release = () => { skeletons.forEach(s => s.dispose()); materials.forEach(m => m.dispose()); leases.forEach(lease => lease.release()) }
  if (signal.aborted) { release(); throw new DOMException('Chargement annulé', 'AbortError') }
  const byId = new Map(nodes.map(n => [n.id, n]))
  const rootNode = nodes.find(n => n.parent_id === null)
  const systems = nodes.filter(n => n.parent_id === rootNode?.id && n.part_id === null).map((n, index) => ({ id: n.id, name: n.name, index }))
  const parentOf = new Map(nodes.map(n => [n.id, n.parent_id]))
  const systemFor = (nodeId: number) => {
    let n = byId.get(nodeId)
    while (n && n.parent_id !== null && n.parent_id !== rootNode?.id) n = byId.get(n.parent_id)
    return systems.find(s => s.id === n?.id)
  }
  const objects: VehicleObject[] = []
  const usedAssets = new Set<string>()
  try {
    for (const instance of resolution.instances) {
      const lease = leases.get(instance.asset.key)
      if (!lease) { unavailable.set(instance.node.id, failures.get(instance.asset.key) ?? 'Modèle indisponible.'); continue }
      try {
        lease.scene.updateMatrixWorld(true)
        const source = findMeshRoot(lease.scene, instance.binding.mesh_path)
        let meshCount = 0
        source.traverse(o => {
          const mesh = o as THREE.SkinnedMesh
          if (mesh.isMesh) meshCount++
          if (mesh.isSkinnedMesh && mesh.skeleton.bones.some(bone => !source.getObjectById(bone.id))) throw new ManifestError('mesh_missing', 'Squelette incomplet : sélectionner un sous-arbre glTF autonome.')
        })
        if (!meshCount) throw new ManifestError('mesh_missing', 'Aucun maillage dans le sous-arbre glTF.')
        const clone = cloneSkeleton(source)
        clone.matrix.copy(source.matrixWorld)
        clone.matrixAutoUpdate = false // conserve aussi une éventuelle matrice parent non décomposable sans perte
        const units = new THREE.Group()
        units.scale.setScalar(instance.asset.unit_scale); units.add(clone)
        const group = new THREE.Group(), p = instance.placement
        group.position.set(...p.position); group.rotation.set(...p.rotation); group.scale.set(...p.scale)
        group.name = instance.key; group.add(units)
        const system = systemFor(instance.node.id)
        const obj: VehicleObject = {
          key: instance.key, nodeId: instance.node.id,
          name: instance.node.name || `Pièce #${instance.node.part_id}`, slot: null,
          estimated: false, positionStatus: instance.binding.position_status, systemId: system?.id ?? null,
          systemName: system?.name ?? resolution.manifest.vehicle_label, systemIndex: system?.index ?? 0,
          budgetStatus: instance.node.budget_status ?? null, componentSystem: instance.binding.system,
        }
        const offset = new THREE.Vector3(...p.explode_vector).applyEuler(new THREE.Euler(...p.rotation)).add(new THREE.Vector3(...instance.assembly_explode))
        const item: Item = { obj, part: obj.name, axial: 1, node: group, meshes: [], lines: [],
          baseLocal: new THREE.Vector3(), baseWorld: new THREE.Vector3(), depth: 1, explodeVector: offset.toArray() }
        clone.traverse(o => {
          const mesh = o as THREE.Mesh
          if (!mesh.isMesh) return
          if ((mesh as THREE.SkinnedMesh).isSkinnedMesh) skeletons.add((mesh as THREE.SkinnedMesh).skeleton)
          const copies = (Array.isArray(mesh.material) ? mesh.material : [mesh.material]).map(m => { const copy = m.clone(); materials.add(copy); return copy })
          mesh.material = Array.isArray(mesh.material) ? copies : copies[0]
          mesh.castShadow = true; mesh.receiveShadow = true
          rememberMaterial(mesh, true) // la lecture budget/système concerne chaque composant ; le mode Matériaux restaure la source
          item.meshes.push(mesh)
        })
        root.add(group)
        objects.push(obj); items.push(item); usedAssets.add(instance.asset.key)
      } catch (error) { unavailable.set(instance.node.id, error instanceof Error ? error.message : String(error)) }
    }
    for (const [key, lease] of leases) if (!usedAssets.has(key)) { lease.release(); leases.delete(key) }
    const index: VehicleIndex = { representation: 'manifest', rootId: rootNode?.id ?? null, objects,
      byKey: new Map(objects.map(o => [o.key, o])), byNodeId: new Map(objects.map(o => [o.nodeId, o])), systems, parentOf }
    return { root, items, missing: [], index, dispose: release, manifestInfo: {
      revision: resolution.manifest.revision, vehicleLabel: resolution.manifest.vehicle_label, unavailable,
      physicalNodes: resolution.physical_nodes,
      representedNodes: new Set(objects.filter(o => byId.get(o.nodeId)?.part_id !== null).map(o => o.nodeId)).size,
      assets: [...activeAssets.values()].filter(a => usedAssets.has(a.key)),
      approximatePlacements: new Set(objects.filter(o => o.positionStatus === 'approximation').map(o => o.nodeId)).size,
    } }
  } catch (error) { release(); throw error }
}
