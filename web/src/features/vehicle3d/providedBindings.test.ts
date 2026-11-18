import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { parseManifest, resolveManifest } from './manifest.ts'

test('Fiat : deux emplacements réels pour les arbres à cames et renforts de quantité deux', () => {
  const manifest = parseManifest(JSON.parse(readFileSync(new URL('../../../public/models/manifests/draft-1.json', import.meta.url), 'utf8')))
  const nodes = [
    { id: 1, parent_id: null, part_id: null }, { id: 2, parent_id: 1, part_id: null },
    { id: 3, parent_id: 2, part_id: null }, { id: 8, parent_id: 3, part_id: 60, quantity: '2' },
    { id: 12502, parent_id: 1, part_id: null }, { id: 12503, parent_id: 12502, part_id: null },
    { id: 12928, parent_id: 12503, part_id: null }, { id: 12932, parent_id: 12928, part_id: 104, quantity: '2' },
  ].map(n => ({ ...n, name: 'Fixture numérique' }))
  const resolved = resolveManifest(manifest, { kind: 'draft', id: 1 }, nodes)
  for (const id of [8, 12932]) {
    const instances = resolved.instances.filter(i => i.node.id === id)
    assert.equal(instances.length, 2, 'Two explicitly placed instances for occurrence ' + id)
    assert.notEqual(instances[0].key, instances[1].key)
    assert.notDeepEqual(instances[0].placement.position, instances[1].placement.position)
    assert.deepEqual(instances[0].placement.explode_vector, instances[1].placement.explode_vector)
    assert.equal(resolved.unavailable.has(id), false)
  }
})
