import assert from 'node:assert/strict'
import { test } from 'node:test'
import { buildVehicleIndex, type VehicleObject } from './mapping.ts'
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

test('manifeste : sélection par occurrence et descendants, jamais par pièce commune ou nom', () => {
  const nodes = [
    { id: 1, parent_id: null, name: 'Auto', part_id: null },
    { id: 2, parent_id: 1, name: 'Système', part_id: null },
    { id: 3, parent_id: 2, name: 'Identique', part_id: 40 },
    { id: 4, parent_id: 2, name: 'Identique', part_id: 40 },
    { id: 5, parent_id: 2, name: 'Absent', part_id: 40 },
  ]
  const objects = [
    { nodeId: 3, key: '3/left' }, { nodeId: 3, key: '3/right' }, { nodeId: 4, key: '4/only' },
  ] as VehicleObject[]
  const index = { ...buildVehicleIndex(nodes, slotTable), representation: 'manifest' as const, objects }
  for (const [node, keys, unavailable] of [
    [nodes[2], ['3/left', '3/right'], false], [nodes[3], ['4/only'], false],
    [nodes[1], ['3/left', '3/right', '4/only'], false], [nodes[4], [], true],
  ] as const) assert.deepEqual(sceneSelection(index, node), { keys, unavailable })
})
