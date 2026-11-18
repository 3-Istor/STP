import assert from 'node:assert/strict'
import { test } from 'node:test'
import { ManifestError, parseManifest, resolveManifest, type SceneManifest } from './manifest.ts'

const placement = (key: string, x = 0) => ({ key, position: [x, 0, 0], rotation: [0, 0, 0], scale: [1, 1, 1], explode_vector: [0, 1, 0] })
const asset = (key: string) => ({ key, path: `/models/${key}.glb`, sha256: 'a'.repeat(64), unit_scale: 1,
  source_url: 'https://example.test/original-test-geometry', author: 'Test fixture', license: 'CC0-1.0',
  license_url: 'https://creativecommons.org/publicdomain/zero/1.0/', geometry_status: 'schematic' })
function fixture() {
  return { schema_version: 1, revision: 'test-r1', scope: { kind: 'draft', id: 7 }, vehicle_label: 'Géométrie de test',
    assets: [asset('wheel'), asset('replacement')], assemblies: [{ node_id: 2, explode_vector: [0, 2, 0] }],
    bindings: [
      { node_id: 3, parent_id: 2, part_id: 40, asset_key: 'wheel', mesh_path: [], system: 'other', position_status: 'approximation', instances: [placement('left', 1), placement('second', 2)] },
      { node_id: 4, parent_id: 2, part_id: 40, asset_key: 'wheel', mesh_path: [], system: 'other', position_status: 'approximation', instances: [placement('right', -1)] },
      { node_id: 3, parent_id: 2, part_id: 41, asset_key: 'replacement', mesh_path: [], system: 'other', position_status: 'approximation', instances: [placement('left', 1)] },
    ] }
}
const nodes = [
  { id: 1, parent_id: null, name: 'Test', part_id: null, quantity: '1' },
  { id: 2, parent_id: 1, name: 'Assembly', part_id: null, quantity: '1' },
  { id: 3, parent_id: 2, name: 'Same name', part_id: 40, quantity: '1' },
  { id: 4, parent_id: 2, name: 'Same name', part_id: 40, quantity: '1' },
]
const scope = { kind: 'draft' as const, id: 7 }

test('occurrences répétées : mêmes noms/pièces, clés, positions et picking distincts', () => {
  const result = resolveManifest(parseManifest(fixture()), scope, nodes)
  assert.deepEqual(result.instances.map(i => [i.key, i.node.id, i.placement.position[0]]), [
    ['node:3/instance:left', 3, 1], ['node:4/instance:right', 4, -1],
  ])
  assert.deepEqual(result.instances[0].assembly_explode, [0, 2, 0])
  assert.equal(result.physical_nodes, 2)
  assert.equal(result.unavailable.size, 0)
})

test('édition de la BOM : variantes, offre, déplacement, ajout, suppression et instances', () => {
  const manifest = parseManifest(fixture())
  const cases = [
    { name: 'variante physique connue', changed: nodes.map(n => n.id === 3 ? { ...n, part_id: 41 } : n), asset: 'replacement', count: 2 },
    { name: 'offre seule', changed: nodes.map(n => ({ ...n, offer_id: 900 })), asset: 'wheel', count: 2 },
    { name: 'deux positions explicitement fournies', changed: nodes.map(n => n.id === 3 ? { ...n, quantity: '2.00' } : n), asset: 'wheel', count: 3 },
    { name: 'suppression occurrence', changed: nodes.filter(n => n.id !== 4), asset: 'wheel', count: 1 },
  ]
  for (const c of cases) {
    const result = resolveManifest(manifest, scope, c.changed)
    assert.equal(result.instances.find(i => i.node.id === 3)?.asset.key, c.asset, c.name)
    assert.equal(result.instances.length, c.count, c.name)
  }
  const missingCases = [
    { patch: { part_id: 99 }, reason: 'part_changed' },
    { patch: { parent_id: 1 }, reason: 'parent_changed' },
    { patch: { quantity: '3' }, reason: 'positions_missing' },
    { patch: { quantity: '1.5' }, reason: 'invalid_quantity' },
    { patch: { quantity: '9007199254740993' }, reason: 'positions_missing' },
  ]
  for (const c of missingCases) {
    const result = resolveManifest(manifest, scope, nodes.map(n => n.id === 3 ? { ...n, ...c.patch } : n))
    assert.equal(result.unavailable.get(3), c.reason)
    assert.equal(result.instances.some(i => i.node.id === 3), false)
  }
  const added = resolveManifest(manifest, scope, [...nodes, { id: 5, parent_id: 2, name: 'Same name', part_id: 40, quantity: '1' }])
  assert.equal(added.unavailable.get(5), 'no_binding')
})

test('quantités héritées : multiplication exacte, aucune conversion flottante', () => {
  const result = resolveManifest(parseManifest(fixture()), scope, nodes.map(n => n.id === 2 ? { ...n, quantity: '0.5' } : n.id > 2 ? { ...n, quantity: '2' } : n))
  assert.equal(result.instances.length, 2)
  assert.equal(result.unavailable.size, 0)
})

test('portée et révision de snapshot : aucun rapprochement implicite', () => {
  const manifest = parseManifest(fixture())
  assert.throws(() => resolveManifest(manifest, { kind: 'draft', id: 8 }, nodes), (e: unknown) => e instanceof ManifestError && e.code === 'scope_mismatch')
  const snapshot: SceneManifest = { ...manifest, scope: { kind: 'snapshot', id: 7 } }
  assert.throws(() => resolveManifest(snapshot, { kind: 'snapshot', id: 7 }, nodes), /révision/)
  assert.throws(() => resolveManifest(snapshot, { kind: 'snapshot', id: 7, revision: 'other' }, nodes), /révision/)
  assert.equal(resolveManifest(snapshot, { kind: 'snapshot', id: 7, revision: 'test-r1' }, nodes).instances.length, 2)
})

test('manifeste invalide : provenance, identité, chemins, valeurs et preuves', () => {
  const cases = [
    (m: ReturnType<typeof fixture>) => { m.assets[0].path = 'https://example.test/model.glb' },
    (m: ReturnType<typeof fixture>) => { m.assets[0].path = '/models/../secret.glb' },
    (m: ReturnType<typeof fixture>) => { m.assets[0].sha256 = 'not-a-checksum' },
    (m: ReturnType<typeof fixture>) => { m.assets[0].author = '' },
    (m: ReturnType<typeof fixture>) => { m.assets[0].unit_scale = 0 },
    (m: ReturnType<typeof fixture>) => { m.assets[0].geometry_status = 'verified_exact' },
    (m: ReturnType<typeof fixture>) => { m.bindings[0].position_status = 'verified' },
    (m: ReturnType<typeof fixture>) => { m.bindings[0].node_id = Number.MAX_SAFE_INTEGER + 1 },
    (m: ReturnType<typeof fixture>) => { m.bindings[0].instances[0].position[0] = NaN },
    (m: ReturnType<typeof fixture>) => { m.bindings[0].instances.push(placement('left')) },
    (m: ReturnType<typeof fixture>) => { m.bindings.push(structuredClone(m.bindings[0])) },
    (m: ReturnType<typeof fixture>) => { m.bindings[0].asset_key = 'unknown' },
  ]
  for (const corrupt of cases) { const input = fixture(); corrupt(input); assert.throws(() => parseManifest(input), ManifestError) }
})

test('arbre invalide et arbre profond : pas de boucle ni de débordement de pile', () => {
  const manifest = parseManifest(fixture())
  assert.throws(() => resolveManifest(manifest, scope, [...nodes, nodes[0]]), /Identifiant/)
  assert.throws(() => resolveManifest(manifest, scope, nodes.map(n => n.id === 2 ? { ...n, parent_id: 3 } : n)), /Cycle/)
  const deep = Array.from({ length: 30_000 }, (_, i) => ({ id: i + 1, parent_id: i === 0 ? null : i, name: '', part_id: null, quantity: '1' }))
  assert.equal(resolveManifest(manifest, scope, deep).instances.length, 0)
})
