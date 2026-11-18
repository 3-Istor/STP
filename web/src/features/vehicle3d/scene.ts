// Modèle de scène commun aux rendus Simple et Réaliste (ADR 0035) : des « items » (pièces visuelles)
// rattachés à un objet de la BOM, que le moteur déplace, colore et rend transparents de la même façon.
import * as THREE from 'three'
import type { VehicleIndex, VehicleObject } from './mapping.ts'
import type { AssetDefinition } from './manifest.ts'

export type Item = {
  obj: VehicleObject | null // null : pièce du modèle non reliée à la BOM (neutre, non cliquable)
  part: string
  axial: number // rang intérieur → extérieur pour l'éclatement dans l'axe (focus)
  node: THREE.Object3D // nœud déplacé
  meshes: THREE.Mesh[] // maillages possédés (pas ceux des items descendants)
  lines: THREE.LineSegments[]
  baseLocal: THREE.Vector3
  baseWorld: THREE.Vector3
  depth: number
  explodeVector?: [number, number, number] // déplacement explicite du manifeste en repère véhicule
  displayRole?: 'exterior' | 'interior'
}

export type Built = {
  root: THREE.Object3D
  items: Item[]
  /** Objets de la BOM sans aucune géométrie dans ce rendu (signalés, jamais cachés). */
  missing: VehicleObject[]
  index?: VehicleIndex
  manifestInfo?: {
    revision: string; vehicleLabel: string; unavailable: Map<number, string>
    physicalNodes: number; representedNodes: number; assets: AssetDefinition[]
    approximatePlacements: number
  }
  dispose?: () => void // le chargeur lié possède les géométries/textures partagées via des baux
}

export function disposeBuilt(built: Built): void {
  if (built.dispose) built.dispose()
  else disposeTree(built.root)
}

/** État d'origine d'un matériau, pour revenir d'une coloration, d'une sélection ou d'un focus. */
export type MaterialBase = {
  color: THREE.Color
  map: THREE.Texture | null
  opacity: number
  transparent: boolean
  depthWrite: boolean
  emissive: THREE.Color | null
  emissiveIntensity: number
  transmission: number
  castShadow: boolean
  paint: boolean // la coloration budget / système s'applique-t-elle à ce maillage ?
}

type Colored = THREE.Material & { color: THREE.Color; map?: THREE.Texture | null; emissive?: THREE.Color; emissiveIntensity?: number; transmission?: number }

/** Mémorise l'état d'origine du matériau (propre à ce maillage) dans userData.v3d. */
export function rememberMaterial(mesh: THREE.Mesh, paint: boolean): void {
  const base = (m: Colored): MaterialBase => ({
    color: m.color.clone(),
    map: m.map ?? null,
    opacity: m.opacity,
    transparent: m.transparent,
    depthWrite: m.depthWrite,
    emissive: m.emissive ? m.emissive.clone() : null,
    emissiveIntensity: m.emissiveIntensity ?? 1,
    transmission: m.transmission ?? 0,
    castShadow: mesh.castShadow,
    paint,
  })
  mesh.userData.v3d = Array.isArray(mesh.material) ? mesh.material.map(m => base(m as Colored)) : base(mesh.material as Colored)
}

// La sélection n'altère pas la couleur (un rouge teinté de bleu se lirait comme une autre information) :
// elle est montrée par un cadre autour de l'objet (viewer.ts).
export type MeshStyle = { tint: string | null; ghost: boolean; estimated: boolean }

export function styleMesh(mesh: THREE.Mesh, s: MeshStyle): void {
  const stored = mesh.userData.v3d as MaterialBase | MaterialBase[] | undefined
  if (!stored) return
  const bases = Array.isArray(stored) ? stored : [stored]
  const materials = Array.isArray(mesh.material) ? mesh.material : [mesh.material]
  materials.forEach((material, i) => {
    const base = bases[i]
    if (base) styleMaterial(material as Colored, base, s)
  })
  mesh.castShadow = !s.ghost && bases.some(base => base.castShadow)
}

function styleMaterial(m: Colored, base: MaterialBase, s: MeshStyle) {
  let dirty = false
  if (base.paint && s.tint) {
    m.color.set(s.tint)
    if (m.map) { m.map = null; dirty = true }
  } else {
    m.color.copy(base.color)
    if ((m.map ?? null) !== base.map) { m.map = base.map; dirty = true }
  }
  if (m.emissive && base.emissive) {
    m.emissive.copy(base.emissive)
    m.emissiveIntensity = base.emissiveIntensity
  }
  const transparent = s.ghost || base.transparent || s.estimated
  if (m.transparent !== transparent) { m.transparent = transparent; dirty = true }
  m.opacity = s.ghost ? Math.min(base.opacity, 0.12) : s.estimated ? 0.82 : base.opacity
  m.depthWrite = s.ghost ? false : base.depthWrite
  if (m.transmission !== undefined) m.transmission = s.ghost ? 0 : base.transmission
  if (dirty) m.needsUpdate = true
}

/** Boîte englobante monde des seuls maillages d'un item (pas de ses descendants appartenant à d'autres items). */
export function itemBox(item: Item, target = new THREE.Box3()): THREE.Box3 {
  target.makeEmpty()
  const b = new THREE.Box3()
  for (const mesh of item.meshes) {
    if (!mesh.visible) continue
    if (mesh instanceof THREE.InstancedMesh) {
      if (!mesh.boundingBox) mesh.computeBoundingBox()
      b.copy(mesh.boundingBox!)
    } else {
      if (!mesh.geometry.boundingBox) mesh.geometry.computeBoundingBox()
      b.copy(mesh.geometry.boundingBox!)
    }
    target.union(b.applyMatrix4(mesh.matrixWorld))
  }
  return target
}

/** Libère géométries, matériaux et textures d'un sous-graphe (partagés ou non, chacun une seule fois). */
export function disposeTree(root: THREE.Object3D): void {
  const geometries = new Set<THREE.BufferGeometry>()
  const materials = new Set<THREE.Material>()
  const textures = new Set<THREE.Texture>()
  const skeletons = new Set<THREE.Skeleton>()
  root.traverse(o => {
    const m = o as THREE.Mesh
    if ((m as THREE.SkinnedMesh).isSkinnedMesh) skeletons.add((m as THREE.SkinnedMesh).skeleton)
    if (m.geometry) geometries.add(m.geometry)
    const mats = m.material ? (Array.isArray(m.material) ? m.material : [m.material]) : []
    for (const mat of mats) {
      materials.add(mat)
      for (const v of Object.values(mat)) if (v instanceof THREE.Texture) textures.add(v)
    }
    const stored = o.userData.v3d as MaterialBase | MaterialBase[] | undefined
    for (const base of Array.isArray(stored) ? stored : stored ? [stored] : []) if (base.map) textures.add(base.map)
  })
  geometries.forEach(g => g.dispose())
  materials.forEach(m => m.dispose())
  textures.forEach(t => t.dispose())
  skeletons.forEach(s => s.dispose())
}
