import assert from 'node:assert/strict'
import { test } from 'node:test'
import * as THREE from 'three'
import { BoundAssetStore, buildBoundScene, validateGLB } from './boundModel.ts'
import { isStaleChunkError } from './chunkError.ts'
import { parseManifest, resolveManifest } from './manifest.ts'
import { disposeBuilt, styleMesh } from './scene.ts'
import type { ComponentSystem } from './manifest.ts'

function glb(json: object = { asset: { version: '2.0' } }): ArrayBuffer {
  const bytes = new TextEncoder().encode(JSON.stringify(json)), length = Math.ceil(bytes.length / 4) * 4
  const buffer = new ArrayBuffer(20 + length), view = new DataView(buffer)
  view.setUint32(0, 0x46546c67, true); view.setUint32(4, 2, true); view.setUint32(8, buffer.byteLength, true)
  view.setUint32(12, length, true); view.setUint32(16, 0x4e4f534a, true)
  new Uint8Array(buffer, 20).fill(0x20); new Uint8Array(buffer, 20, bytes.length).set(bytes)
  return buffer
}
async function fixture() {
  const buffer = glb()
  const sha = [...new Uint8Array(await crypto.subtle.digest('SHA-256', buffer))].map(v => v.toString(16).padStart(2, '0')).join('')
  const nodes = [
    { id: 1, parent_id: null, name: 'Test', part_id: null, quantity: '1' },
    { id: 2, parent_id: 1, name: 'Assembly', part_id: null, quantity: '1' },
    { id: 3, parent_id: 2, name: 'Same name', part_id: 40, quantity: '1' },
    { id: 4, parent_id: 2, name: 'Same name', part_id: 40, quantity: '1' },
  ]
  const manifest = parseManifest({ schema_version: 1, revision: 'fixture', vehicle_label: 'Test', scope: { kind: 'draft', id: 1 },
    assets: [{ key: 'test', path: '/models/test.glb', sha256: sha, unit_scale: 0.1, source_url: 'https://example.test/fixture', author: 'Test', license: 'CC0', license_url: 'https://creativecommons.org/publicdomain/zero/1.0/', geometry_status: 'schematic' }],
    bindings: [3, 4].map(node_id => ({ node_id, parent_id: 2, part_id: 40, asset_key: 'test', mesh_path: ['Parts', 'Mesh'], system: 'body', position_status: 'approximation',
      instances: [{ key: 'one', position: [node_id === 3 ? 1 : -1, 0, 0], rotation: [0, 0, 0], scale: [1, 1, 1], explode_vector: [0, 1, 0] }] })) })
  return { buffer, manifest, nodes, resolution: resolveManifest(manifest, { kind: 'draft', id: 1 }, nodes) }
}
function sourceScene() {
  const root = new THREE.Group(), parts = new THREE.Group()
  parts.name = 'Parts'; parts.position.x = 10; parts.rotation.z = Math.PI / 2
  const texture = new THREE.Texture(), geometry = new THREE.BoxGeometry()
  const material = new THREE.MeshStandardMaterial({ color: '#335577', map: texture })
  const mesh = new THREE.Mesh(geometry, material); mesh.name = 'Mesh'; mesh.position.y = 1
  parts.add(mesh); root.add(parts)
  return { root, geometry, material, texture }
}

test('instances : mêmes géométries, matériaux indépendants, bon node_id et transforms parents conservés', async () => {
  const f = await fixture(), source = sourceScene()
  let requests = 0, parsed = 0
  const store = new BoundAssetStore(async () => { requests++; return new Response(f.buffer) }, async () => { parsed++; return source.root })
  const built = await buildBoundScene(f.resolution, f.nodes, store, new AbortController().signal)
  built.root.updateMatrixWorld(true)
  assert.deepEqual(built.items.map(i => i.obj?.nodeId), [3, 4])
  assert.equal(built.items[0].meshes[0].geometry, built.items[1].meshes[0].geometry)
  assert.notEqual(built.items[0].meshes[0].material, built.items[1].meshes[0].material)
  const at = built.items[0].meshes[0].getWorldPosition(new THREE.Vector3())
  assert.ok(Math.abs(at.x - 1.9) < 1e-9)
  const other = built.items[1].meshes[0].material as THREE.Material
  styleMesh(built.items[0].meshes[0], { tint: null, ghost: true, estimated: false })
  assert.equal(other.opacity, 1)
  assert.equal(source.material.opacity, 1)
  assert.equal(requests, 1); assert.equal(parsed, 1)
  disposeBuilt(built); store.dispose()
})

test('remplacement de scène : réutilisation active et libération au dernier propriétaire seulement', async () => {
  const f = await fixture(), source = sourceScene()
  let geometryDisposed = 0, textureDisposed = 0, requests = 0
  source.geometry.addEventListener('dispose', () => geometryDisposed++)
  source.texture.addEventListener('dispose', () => textureDisposed++)
  const store = new BoundAssetStore(async () => { requests++; return new Response(f.buffer) }, async () => source.root)
  const first = await buildBoundScene(f.resolution, f.nodes, store, new AbortController().signal)
  const second = await buildBoundScene(f.resolution, f.nodes, store, new AbortController().signal)
  assert.equal(requests, 1)
  disposeBuilt(first)
  assert.equal(geometryDisposed, 0); assert.equal(textureDisposed, 0)
  disposeBuilt(second)
  assert.equal(geometryDisposed, 1); assert.equal(textureDisposed, 1)
  assert.equal(store.residentAssets, 0)
})

test('empreinte, modèle absent et mesh absent : aucune géométrie étrangère conservée', async () => {
  const f = await fixture()
  for (const failure of ['hash', 'http', 'mesh']) {
    const store = new BoundAssetStore(async () => failure === 'http' ? new Response('', { status: 404 }) : new Response(f.buffer), async () => new THREE.Group())
    const resolution = failure === 'hash' ? resolveManifest({ ...f.manifest, assets: f.manifest.assets.map(a => ({ ...a, sha256: 'b'.repeat(64) })) }, { kind: 'draft', id: 1 }, f.nodes) : f.resolution
    const built = await buildBoundScene(resolution, f.nodes, store, new AbortController().signal)
    assert.equal(built.root.children.length, 0, failure)
    assert.equal(built.items.length, 0, failure)
    assert.equal(built.manifestInfo?.unavailable.size, 2, failure)
    assert.deepEqual(built.manifestInfo?.missingAssets, failure === 'http' ? ['/models/test.glb'] : [], failure)
    assert.equal(store.residentAssets, 0, failure)
    disposeBuilt(built)
  }
})

test('GLB autonome : refuse HTML, version erronée et ressources hors empreinte', () => {
  validateGLB(glb())
  const cases = [new ArrayBuffer(3), glb({ buffers: [{ uri: 'other.bin' }] }), glb({ images: [{ uri: 'https://example.test/image.jpg' }] })]
  for (const data of cases) assert.throws(() => validateGLB(data))
})

test('couleurs budget/système puis matériaux natifs, pour chaque classification du composant', async () => {
  const f = await fixture()
  for (const system of ['body', 'thermal', 'electric', 'shared', 'other'] as ComponentSystem[]) {
    const source = sourceScene()
    const manifest = { ...f.manifest, bindings: f.manifest.bindings.map(b => ({ ...b, system })) }
    const store = new BoundAssetStore(async () => new Response(f.buffer), async () => source.root)
    const built = await buildBoundScene(resolveManifest(manifest, { kind: 'draft', id: 1 }, f.nodes), f.nodes, store, new AbortController().signal)
    const mesh = built.items[0].meshes[0], material = mesh.material as THREE.MeshStandardMaterial
    const original = material.color.getHexString(), map = material.map
    styleMesh(mesh, { tint: '#b83240', ghost: false, estimated: false })
    assert.equal(material.color.getHexString(), 'b83240', system)
    assert.equal(material.map, null, system)
    styleMesh(mesh, { tint: null, ghost: false, estimated: false })
    assert.equal(material.color.getHexString(), original, system)
    assert.equal(material.map, map, system)
    disposeBuilt(built); store.dispose()
  }
})

test('GLB articulé : squelettes indépendants et textures de peau libérées à la destruction', async () => {
  const f = await fixture(), source = sourceScene()
  const parts = source.root.children[0], original = parts.children[0] as THREE.Mesh
  const bone = new THREE.Bone(), skeleton = new THREE.Skeleton([bone])
  const mesh = new THREE.SkinnedMesh(original.geometry, original.material)
  mesh.name = 'Mesh'; mesh.bind(skeleton)
  parts.remove(original); parts.add(bone, mesh)
  const manifest = { ...f.manifest, bindings: f.manifest.bindings.map(b => ({ ...b, mesh_path: ['Parts'] })) }
  const store = new BoundAssetStore(async () => new Response(f.buffer), async () => source.root)
  const built = await buildBoundScene(resolveManifest(manifest, { kind: 'draft', id: 1 }, f.nodes), f.nodes, store, new AbortController().signal)
  const skeletons = built.items.map(i => (i.meshes[0] as THREE.SkinnedMesh).skeleton)
  assert.notEqual(skeletons[0], skeletons[1])
  assert.notEqual(skeletons[0], skeleton)
  let disposed = 0
  for (const own of skeletons) { own.computeBoneTexture(); own.boneTexture!.addEventListener('dispose', () => disposed++) }
  disposeBuilt(built); store.dispose()
  assert.equal(disposed, 2)
  assert.ok(skeletons.every(own => own.boneTexture === null))
})

test('échec de chargement du chunk reconnu, WebGL non', () => {
  assert.ok(isStaleChunkError('Failed to fetch dynamically imported module: https://x/assets/Vehicle3DView-ab.js'))
  assert.ok(isStaleChunkError('Importing a module script failed.'))
  assert.ok(!isStaleChunkError('Error creating WebGL context.'))
})
