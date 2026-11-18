import assert from 'node:assert/strict'
import { test } from 'node:test'
import profile from './data/powertrain-valhalla.json' with { type: 'json' }
import { powertrainRequest } from './powertrain.ts'
import type { TreeNode } from '../api.ts'

const nodes = () => profile.guards.map(g => ({ ...g, quantity: '1', name: 'Nom libre' })) as TreeNode[]
test('profil explicite : identité de brouillon, parents et pièces vérifiés sans rapprochement par nom', () => {
  const request = powertrainRequest({ kind: 'draft', id: 2 }, nodes())
  assert.equal(request?.root_id, 29978)
  assert.deepEqual(request?.scenarios.find(s => s.key === 'thermal')?.excluded_roots, [34537])
  for (const target of [{ kind: 'snapshot' as const, id: 2 }, { kind: 'draft' as const, id: 1 }]) assert.equal(powertrainRequest(target, nodes()), null)
  for (const field of ['part_id', 'parent_id'] as const) {
    const changed = nodes(); changed[1] = { ...changed[1], [field]: 999 }
    assert.equal(powertrainRequest({ kind: 'draft', id: 2 }, changed), null)
  }
  assert.equal(powertrainRequest({ kind: 'draft', id: 2 }, nodes().slice(1)), null)
  assert.deepEqual(powertrainRequest({ kind: 'draft', id: 2 }, nodes().map(n => ({ ...n, name: 'Renommé', offer_id: 900 }))), request)
})
