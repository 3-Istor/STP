import { createHash } from 'node:crypto'
import type { Page } from '@playwright/test'

/** Small synthetic boxes for integration tests; no vehicle fidelity is claimed. */
export async function mockBoundAssemblies(page: Page) {
  const positions = new Float32Array([-0.4,0,-0.4, 0.4,0,-0.4, 0.4,0.8,-0.4, -0.4,0.8,-0.4, -0.4,0,0.4, 0.4,0,0.4, 0.4,0.8,0.4, -0.4,0.8,0.4])
  const indices = new Uint16Array([0,2,1,0,3,2,4,5,6,4,6,7,0,1,5,0,5,4,3,7,6,3,6,2,1,2,6,1,6,5,0,4,7,0,7,3])
  const bin = Buffer.concat([Buffer.from(positions.buffer), Buffer.from(indices.buffer)])
  const json = Buffer.from(JSON.stringify({ asset: { version: '2.0' }, scene: 0, scenes: [{ nodes: [0] }], nodes: [{ name: 'Fixture', mesh: 0 }],
    meshes: [{ primitives: [{ attributes: { POSITION: 0 }, indices: 1 }] }], buffers: [{ byteLength: bin.length }],
    bufferViews: [{ buffer: 0, byteLength: positions.byteLength }, { buffer: 0, byteOffset: positions.byteLength, byteLength: indices.byteLength }],
    accessors: [{ bufferView: 0, componentType: 5126, count: 8, type: 'VEC3', min: [-0.4,0,-0.4], max: [0.4,0.8,0.4] }, { bufferView: 1, componentType: 5123, count: indices.length, type: 'SCALAR' }] }))
  const padded = Buffer.concat([json, Buffer.alloc((4 - json.length % 4) % 4, 0x20)])
  const glb = Buffer.alloc(28 + padded.length + bin.length)
  glb.write('glTF'); glb.writeUInt32LE(2,4); glb.writeUInt32LE(glb.length,8)
  glb.writeUInt32LE(padded.length,12); glb.writeUInt32LE(0x4e4f534a,16); padded.copy(glb,20)
  glb.writeUInt32LE(bin.length,20+padded.length); glb.writeUInt32LE(0x004e4942,24+padded.length); bin.copy(glb,28+padded.length)
  await page.route('**/models/test-assembly.glb', route => route.fulfill({ body: glb, contentType: 'model/gltf-binary' }))
  await page.route('**/models/manifests/draft-1.json', route => route.fulfill({ json: {
    schema_version: 1, revision: 'integration-assemblies', scope: { kind: 'draft', id: 1 }, vehicle_label: 'Fixture synthétique', assemblies: [],
    assets: [{ key: 'fixture', path: '/models/test-assembly.glb', sha256: createHash('sha256').update(glb).digest('hex'), unit_scale: 1,
      source_url: 'https://example.test/fixture', license: 'CC0', license_url: 'https://creativecommons.org/publicdomain/zero/1.0/', author: 'Test', geometry_status: 'schematic' }],
    bindings: [[3,2,[-1.4,0,0]], [5,2,[0,0,-2]], [6,1,[1.4,0,0]]].map(([id,parent,position]) => ({
      node_id: id, parent_id: parent, part_id: null, asset_key: 'fixture', mesh_path: ['Fixture'], system: 'body', position_status: 'approximation',
      instances: [{ key: 'schematic', position, rotation: [0,0,0], scale: [1,1,1], explode_vector: [0,1,0] }],
    })),
  } }))
}
