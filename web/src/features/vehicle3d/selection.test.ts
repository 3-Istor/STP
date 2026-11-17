import assert from 'node:assert/strict'
import { test } from 'node:test'
import { buildVehicleIndex } from './mapping.ts'
import { sceneSelection } from './selection.ts'
import { slotTable } from './tables.ts'

test('absence de modèle : aucune substitution automatique par un ancêtre ou une autre occurrence', () => {
  const nodes = [
    { id: 1, parent_id: null, name: 'Auto', part_id: null },
    { id: 2, parent_id: 1, name: 'Liaison au sol', part_id: null },
    { id: 3, parent_id: 2, name: 'Roue avant gauche', part_id: null },
    { id: 4, parent_id: 3, name: 'Pneu', part_id: 40 },
    { id: 5, parent_id: 2, name: 'Roue avant droite', part_id: null },
    { id: 6, parent_id: 5, name: 'Pneu', part_id: 40 },
  ]
  const index = buildVehicleIndex(nodes, slotTable)
  const cases = [
    { node: nodes[3], keys: [], missing: true },
    { node: nodes[5], keys: [], missing: true },
    { node: nodes[2], keys: ['wheel_fl'], missing: false },
    { node: nodes[0], keys: ['wheel_fl', 'wheel_fr'], missing: false },
    { node: undefined, keys: [], missing: false },
  ]
  for (const c of cases) {
    const got = sceneSelection(index, c.node)
    assert.deepEqual(got.keys, c.keys)
    assert.equal(got.unavailable, c.missing)
  }
})

test('sous-ensemble absent du GLB chargé : aucune commande vers une boîte vide', () => {
  const nodes = [
    { id: 1, parent_id: null, name: 'Auto', part_id: null },
    { id: 2, parent_id: 1, name: 'Liaison au sol', part_id: null },
    { id: 3, parent_id: 2, name: 'Roue avant gauche', part_id: null },
  ]
  const index = buildVehicleIndex(nodes, slotTable)
  const got = sceneSelection(index, nodes[2], new Set(['wheel_fl']))
  assert.deepEqual(got.keys, [])
  assert.equal(got.unavailable, true)
})
