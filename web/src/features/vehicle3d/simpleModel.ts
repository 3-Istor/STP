// Rendu Simple (ADR 0035) : formes générées par slot, portées de docs/v2/prototype-3d/car.html.
// Positions et formes : data/simple-layout.json ; nom inconnu → cube « position estimée » dans la zone du système.
import * as THREE from 'three'
import { RoundedBoxGeometry } from 'three/addons/geometries/RoundedBoxGeometry.js'
import layoutJson from './data/simple-layout.json' with { type: 'json' }
import { estimatedPosition, zoneFor, type SlotTable, type VehicleIndex } from './mapping.ts'
import { rememberMaterial, type Built, type Item } from './scene.ts'

type V3 = [number, number, number]
type Shape = {
  box?: V3; radius?: number
  profile?: [number, number][]; depth?: number
  cylinder?: [number, number]
  torus?: [number, number]
  tube?: V3[]
  spring?: [number, number]
  quad?: V3[]
  at?: V3; rot?: V3; scale?: V3
}
type Part = { name: string; material: string; axial?: number; instances?: V3[]; shapes: Shape[] }
const layout = layoutJson as unknown as { slots: Record<string, Part[]> }

type MaterialDef = { color: string; metal: number; rough: number; paint: boolean; opacity?: number }
const MATERIALS: Record<string, MaterialDef> = {
  body: { color: '#365b68', metal: 0.55, rough: 0.3, paint: true },
  structure: { color: '#d6dae0', metal: 0.55, rough: 0.35, paint: true },
  dark: { color: '#2b2f35', metal: 0.05, rough: 0.7, paint: true },
  plastic: { color: '#4a5059', metal: 0.1, rough: 0.7, paint: true },
  metal: { color: '#8d939b', metal: 0.7, rough: 0.35, paint: true },
  chrome: { color: '#9aa0a6', metal: 0.8, rough: 0.3, paint: true },
  seat: { color: '#3a3f47', metal: 0, rough: 0.8, paint: true },
  fabric: { color: '#6b7079', metal: 0, rough: 0.9, paint: true },
  accent: { color: '#c8322b', metal: 0.4, rough: 0.4, paint: true },
  disc: { color: '#9097a0', metal: 0.9, rough: 0.3, paint: true },
  rim: { color: '#c4c9cf', metal: 0.9, rough: 0.25, paint: true },
  harness: { color: '#30343a', metal: 0.1, rough: 0.6, paint: true },
  estimated: { color: '#b8bfc8', metal: 0.1, rough: 0.6, paint: true },
  tyre: { color: '#24272b', metal: 0, rough: 0.85, paint: false },
  glass: { color: '#9db4c8', metal: 0.1, rough: 0.05, paint: false, opacity: 0.35 },
  light: { color: '#f3f4f6', metal: 0.2, rough: 0.1, paint: false },
  taillight: { color: '#b22a2a', metal: 0.2, rough: 0.2, paint: false },
}

function makeMaterial(name: string): { mat: THREE.MeshStandardMaterial; paint: boolean } {
  const d = MATERIALS[name] ?? MATERIALS.body
  const parameters: THREE.MeshStandardMaterialParameters = {
    color: d.color, metalness: d.metal, roughness: d.rough,
    transparent: d.opacity !== undefined, opacity: d.opacity ?? 1,
    side: d.opacity !== undefined ? THREE.DoubleSide : THREE.FrontSide,
  }
  const mat = name === 'body'
    ? new THREE.MeshPhysicalMaterial({ ...parameters, clearcoat: 1, clearcoatRoughness: 0.18 })
    : new THREE.MeshStandardMaterial(parameters)
  mat.name = name
  if (d.opacity !== undefined) mat.depthWrite = false
  return { mat, paint: d.paint }
}

function geometryFor(s: Shape, cache: Map<string, THREE.BufferGeometry>): THREE.BufferGeometry {
  const { at: _at, rot: _rot, scale: _scale, ...shape } = s
  const key = JSON.stringify(shape)
  const hit = cache.get(key)
  if (hit) return hit // géométrie partagée : roues, sièges, faisceaux identiques
  let g: THREE.BufferGeometry
  if (s.box) {
    const [w, h, d] = s.box
    const r = Math.min(s.radius ?? 0.02, Math.min(w, h, d) / 2 - 1e-4)
    g = new RoundedBoxGeometry(w, h, d, 2, Math.max(r, 0))
  } else if (s.profile) {
    const sh = new THREE.Shape()
    sh.moveTo(s.profile[0][0], s.profile[0][1])
    for (const p of s.profile.slice(1)) sh.lineTo(p[0], p[1])
    const depth = s.depth ?? 0.05
    const bevel = Math.min(0.03, depth / 2)
    g = new THREE.ExtrudeGeometry(sh, { depth, bevelEnabled: true, bevelThickness: bevel, bevelSize: bevel, bevelSegments: 3, steps: 1 })
    g.translate(0, 0, -depth / 2)
  } else if (s.cylinder) {
    g = new THREE.CylinderGeometry(s.cylinder[0], s.cylinder[0], s.cylinder[1], 32)
  } else if (s.torus) {
    g = new THREE.TorusGeometry(s.torus[0], s.torus[1], 18, 48)
  } else if (s.tube) {
    const curve = new THREE.CatmullRomCurve3(s.tube.map(p => new THREE.Vector3(...p)))
    g = new THREE.TubeGeometry(curve, 48, s.radius ?? 0.015, 8)
  } else if (s.spring) {
    const [r, h] = s.spring
    const pts: THREE.Vector3[] = []
    for (let i = 0; i <= 200; i++) {
      const a = (i / 200) * Math.PI * 2 * 6
      pts.push(new THREE.Vector3(Math.cos(a) * r, (i / 200) * h - h / 2, Math.sin(a) * r))
    }
    g = new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 300, 0.018, 8)
  } else if (s.quad) {
    const [a, b, c, d] = s.quad.map(p => new THREE.Vector3(...p))
    g = new THREE.BufferGeometry().setFromPoints([a, b, c, a, c, d])
    g.computeVertexNormals()
  } else {
    throw new Error(`simple-layout : forme inconnue ${key}`)
  }
  cache.set(key, g)
  return g
}

function shapeMatrix(s: Shape): THREE.Matrix4 {
  const q = new THREE.Quaternion().setFromEuler(new THREE.Euler(...(s.rot ?? [0, 0, 0])))
  return new THREE.Matrix4().compose(new THREE.Vector3(...(s.at ?? [0, 0, 0])), q, new THREE.Vector3(...(s.scale ?? [1, 1, 1])))
}

function newItem(obj: Item['obj'], part: string, axial: number, node: THREE.Object3D): Item {
  return { obj, part, axial, node, meshes: [], lines: [], baseLocal: new THREE.Vector3(), baseWorld: new THREE.Vector3(), depth: 1 }
}

export function buildSimple(index: VehicleIndex, table: SlotTable): Built {
  const root = new THREE.Group()
  root.name = 'vehicle3d-simple'
  const cache = new Map<string, THREE.BufferGeometry>()
  const items: Item[] = []
  const missing = []
  const estimatedPerZone = new Map<string, number>()

  for (const obj of index.objects) {
    const parts = obj.slot ? layout.slots[obj.slot] : undefined
    if (!parts) {
      // Nom inconnu : cube générique dans la zone de son système, marqué « position estimée ».
      const zone = zoneFor(obj.systemName, table)
      const k = estimatedPerZone.get(zone.join()) ?? 0
      estimatedPerZone.set(zone.join(), k + 1)
      const node = new THREE.Group()
      const { mat, paint } = makeMaterial('estimated')
      const geo = geometryFor({ box: [0.16, 0.16, 0.16], radius: 0.03 }, cache)
      const mesh = new THREE.Mesh(geo, mat)
      mesh.position.set(...estimatedPosition(zone, k))
      mesh.castShadow = true
      rememberMaterial(mesh, paint)
      const edges = new THREE.LineSegments(new THREE.EdgesGeometry(new THREE.BoxGeometry(0.17, 0.17, 0.17)),
        new THREE.LineDashedMaterial({ color: '#40464f', dashSize: 0.02, gapSize: 0.015, transparent: true }))
      edges.computeLineDistances()
      mesh.add(edges)
      node.add(mesh)
      root.add(node)
      const item = newItem(obj, 'position estimée', 0, node)
      item.meshes.push(mesh)
      item.lines.push(edges)
      items.push(item)
      continue
    }
    if (parts.length === 0) { missing.push(obj); continue }
    for (const part of parts) {
      const node = new THREE.Group()
      node.name = `${obj.key}:${part.name}`
      const { mat, paint } = makeMaterial(part.material)
      const item = newItem(obj, part.name, part.axial ?? 1, node)
      for (const s of part.shapes) {
        const geo = geometryFor(s, cache)
        let mesh: THREE.Mesh
        if (part.instances && part.instances.length > 0) {
          // Instancing : une géométrie, un appel de dessin pour les sièges d'un rang ou les supports.
          const inst = new THREE.InstancedMesh(geo, mat, part.instances.length)
          const local = shapeMatrix(s)
          part.instances.forEach((off, i) => inst.setMatrixAt(i, new THREE.Matrix4().makeTranslation(...off).multiply(local)))
          inst.instanceMatrix.needsUpdate = true
          inst.computeBoundingBox()
          mesh = inst
        } else {
          mesh = new THREE.Mesh(geo, mat)
          shapeMatrix(s).decompose(mesh.position, mesh.quaternion, mesh.scale)
        }
        mesh.castShadow = mat.opacity === 1
        mesh.receiveShadow = true
        rememberMaterial(mesh, paint)
        node.add(mesh)
        item.meshes.push(mesh)
      }
      root.add(node)
      items.push(item)
    }
  }
  return { root, items, missing }
}
