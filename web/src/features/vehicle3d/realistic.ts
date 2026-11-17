// Rendu Réaliste (ADR 0035), porté de docs/v2/prototype-3d/car-realiste.html : modèle Khronos « Car Concept »
// (CC-BY 4.0) relié aux sous-ensembles par data/realistic-map.json. Fichier absent ou illisible → erreur
// explicite, le moteur se replie sur le rendu Simple. GLTFLoader est importé à la demande (chunk séparé).
import * as THREE from 'three'
import mapJson from './data/realistic-map.json' with { type: 'json' }
import type { VehicleIndex } from './mapping.ts'
import { rememberMaterial, type Built, type Item } from './scene.ts'
import type { RealisticLoader } from './viewer.ts'

type Entry = { slot: string | null; name: string; nodes: string; axial?: number }
type RealisticMap = {
  model: { url: string; attribution: string }
  hide: { nodes: string; materials: string }
  parts: Entry[]
}
const map = mapJson as RealisticMap

const MISSING = `Modèle réaliste absent (web/public${map.model.url}) : lancez « make models ». Rendu Simple affiché.`

export const loadRealistic: RealisticLoader = async index => {
  const res = await fetch(map.model.url).catch(() => null)
  // Vite et nginx renvoient index.html (200, text/html) pour un fichier inexistant : on le traite comme absent.
  if (!res || !res.ok || (res.headers.get('content-type') ?? '').includes('text/html')) throw new Error(MISSING)
  const buf = await res.arrayBuffer()
  if (buf.byteLength < 12 || new TextDecoder().decode(new Uint8Array(buf, 0, 4)) !== 'glTF') {
    throw new Error(`Modèle réaliste illisible (${map.model.url} n'est pas un fichier .glb) : rendu Simple affiché.`)
  }
  const [{ GLTFLoader }, { MeshoptDecoder }] = await Promise.all([
    import('three/addons/loaders/GLTFLoader.js'),
    import('three/addons/libs/meshopt_decoder.module.js'), // si le modèle a été compressé par gltfpack
  ])
  let scene: THREE.Group
  try {
    scene = (await new GLTFLoader().setMeshoptDecoder(MeshoptDecoder).parseAsync(buf, '')).scene
  } catch (err) {
    throw new Error(`Modèle réaliste illisible (${err instanceof Error ? err.message : String(err)}) : rendu Simple affiché.`)
  }
  return { ...buildRealistic(scene, index), attribution: map.model.attribution }
}

const NOT_PAINTED = /glass|tire|disc|light|mirror|license|signal/i

/** Peinture neutre et logos masqués : seule la coloration budget / système doit porter un sens. */
function neutralize(scene: THREE.Object3D) {
  const hideNodes = new RegExp(map.hide.nodes)
  const hideMaterials = new RegExp(map.hide.materials)
  const originals = new Set<THREE.Material>()
  scene.traverse(o => {
    if (hideNodes.test(o.name)) o.visible = false
    const mesh = o as THREE.Mesh
    if (!mesh.isMesh || Array.isArray(mesh.material)) return
    const orig = mesh.material as THREE.MeshPhysicalMaterial
    originals.add(orig)
    const m = orig.clone() // un matériau par maillage : coloration et transparence propres à chaque item
    const name = orig.name ?? ''
    if (hideMaterials.test(name)) mesh.visible = false // plaque d'immatriculation avec logo
    if ('iridescence' in m) m.iridescence = 0 // reflets colorés de la peinture d'origine
    if ('sheen' in m) m.sheen = 0 // velours carmin des sièges
    if (/^Paint/.test(name)) {
      m.color.set('#c3c8cf'); m.map = null; m.metalness = 0.65; m.roughness = 0.28
    } else if (/Carmine/.test(name)) {
      m.color.set('#3b3f46'); m.map = null
    } else if (/^Interior [12]$/.test(name)) {
      m.color.set('#4a4e55')
    } else if (name === 'Tireside') {
      m.map = null; m.color.set('#1b1c1e') // flanc de pneu : texture avec logo retirée
    }
    mesh.material = m
    mesh.castShadow = true
    mesh.receiveShadow = true
    rememberMaterial(mesh, !NOT_PAINTED.test(name) && !m.transparent)
  })
  originals.forEach(m => m.dispose()) // les textures restent : elles sont partagées avec les clones
}

/** Avant vers -x, roues au sol, centré : même repère que le rendu Simple. */
function orient(scene: THREE.Object3D) {
  let box = new THREE.Box3().setFromObject(scene)
  const size = box.getSize(new THREE.Vector3())
  if (size.z > size.x) scene.rotation.y = Math.PI / 2
  scene.updateMatrixWorld(true)
  box = new THREE.Box3().setFromObject(scene)
  const hood = scene.getObjectByName('BodyHood')
  if (hood) {
    const h = new THREE.Box3().setFromObject(hood).getCenter(new THREE.Vector3())
    if (h.x > box.getCenter(new THREE.Vector3()).x) scene.rotation.y += Math.PI
  }
  scene.updateMatrixWorld(true)
  box = new THREE.Box3().setFromObject(scene)
  const c = box.getCenter(new THREE.Vector3())
  scene.position.set(-c.x, -box.min.y, -c.z)
  scene.updateMatrixWorld(true)
}

export function buildRealistic(scene: THREE.Object3D, index: VehicleIndex): Built {
  neutralize(scene)
  orient(scene)
  const root = new THREE.Group()
  root.name = 'vehicle3d-realistic'
  root.add(scene)

  const patterns = map.parts.map(p => new RegExp(p.nodes))
  const entryOf = new Map<THREE.Object3D, number>()
  scene.traverse(o => {
    const i = patterns.findIndex(re => o.name && re.test(o.name))
    if (i >= 0) entryOf.set(o, i)
  })
  const itemOf = new Map<THREE.Object3D, Item>()
  const items: Item[] = []
  const newItem = (node: THREE.Object3D, e: Entry | null): Item => ({
    obj: e?.slot ? index.byKey.get(e.slot) ?? null : null,
    part: e?.name ?? 'reste du modèle', axial: e?.axial ?? 1, node, meshes: [], lines: [],
    baseLocal: new THREE.Vector3(), baseWorld: new THREE.Vector3(), depth: 0,
  })
  for (const [node, i] of entryOf) {
    let inherited = false // un nœud dont un ancêtre relève de la même entrée appartient à cet ancêtre
    for (let p = node.parent; p; p = p.parent) if (entryOf.get(p) === i) { inherited = true; break }
    if (inherited) continue
    const it = newItem(node, map.parts[i])
    itemOf.set(node, it)
    items.push(it)
  }
  const rest = newItem(scene, null) // maillages hors de toute entrée : neutres, jamais déplacés
  // Chaque maillage appartient à l'item de son ancêtre le plus proche.
  scene.traverse(o => {
    const mesh = o as THREE.Mesh
    if (!mesh.isMesh) return
    let owner: Item | undefined
    for (let p: THREE.Object3D | null = mesh; p && !owner; p = p.parent) owner = itemOf.get(p)
    ;(owner ?? rest).meshes.push(mesh)
  })
  if (rest.meshes.length > 0) items.push(rest)
  const shown = new Set(items.flatMap(i => (i.obj ? [i.obj.key] : [])))
  return { root, items, missing: index.objects.filter(o => !shown.has(o.key)) }
}
