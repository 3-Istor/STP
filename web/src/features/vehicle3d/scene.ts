// Modèle de scène commun aux rendus Simple et Réaliste (ADR 0035) : des « items » (pièces visuelles)
// rattachés à un objet de la BOM, que le moteur déplace, colore et rend transparents de la même façon.
import * as THREE from 'three'
import type { VehicleObject } from './mapping.ts'

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
}

export type Built = {
  root: THREE.Object3D
  items: Item[]
  /** Objets de la BOM sans aucune géométrie dans ce rendu (signalés, jamais cachés). */
  missing: VehicleObject[]
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
  const m = mesh.material as Colored
  const base: MaterialBase = {
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
  }
  mesh.userData.v3d = base
}

export type MeshStyle = { tint: string | null; selected: boolean; ghost: boolean; estimated: boolean }

const SELECT_EMISSIVE = new THREE.Color('#2f6fd0')

export function styleMesh(mesh: THREE.Mesh, s: MeshStyle): void {
  const base = mesh.userData.v3d as MaterialBase | undefined
  if (!base || Array.isArray(mesh.material)) return
  const m = mesh.material as Colored
  let dirty = false
  if (base.paint && s.tint) {
    m.color.set(s.tint)
    if (m.map) { m.map = null; dirty = true }
  } else {
    m.color.copy(base.color)
    if ((m.map ?? null) !== base.map) { m.map = base.map; dirty = true }
  }
  if (m.emissive && base.emissive) {
    m.emissive.copy(s.selected ? SELECT_EMISSIVE : base.emissive)
    m.emissiveIntensity = s.selected ? 0.3 : base.emissiveIntensity
  }
  const transparent = s.ghost || base.transparent || s.estimated
  if (m.transparent !== transparent) { m.transparent = transparent; dirty = true }
  m.opacity = s.ghost ? Math.min(base.opacity, 0.12) : s.estimated ? 0.82 : base.opacity
  m.depthWrite = s.ghost ? false : base.depthWrite
  if (m.transmission !== undefined) m.transmission = s.ghost ? 0 : base.transmission
  mesh.castShadow = !s.ghost && base.castShadow
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
  root.traverse(o => {
    const m = o as THREE.Mesh
    if (m.geometry) geometries.add(m.geometry)
    const mats = m.material ? (Array.isArray(m.material) ? m.material : [m.material]) : []
    for (const mat of mats) {
      materials.add(mat)
      for (const v of Object.values(mat)) if (v instanceof THREE.Texture) textures.add(v)
    }
    const base = o.userData.v3d as MaterialBase | undefined
    if (base?.map) textures.add(base.map)
  })
  geometries.forEach(g => g.dispose())
  materials.forEach(m => m.dispose())
  textures.forEach(t => t.dispose())
}
