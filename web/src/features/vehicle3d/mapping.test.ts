import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import layout from './data/simple-layout.json' with { type: 'json' }
import { buildVehicleIndex, estimatedPosition, focusGroup, normalizeName, objectsForNode, slotFor, zoneFor } from './mapping.ts'
import { slotTable } from './tables.ts'
import type { Vehicle3DNode } from './types.ts'

const fixture = (file: string): Vehicle3DNode[] =>
  JSON.parse(readFileSync(new URL(`./demo/${file}`, import.meta.url), 'utf8')).nodes

test('normalizeName', () => {
  const tests = [
    { in: 'Côté de caisse gauche', want: 'cote de caisse gauche' },
    { in: '  Échappement ', want: 'echappement' },
    { in: 'Porte  arrière\tDROITE', want: 'porte arriere droite' },
  ]
  for (const tt of tests) assert.equal(normalizeName(tt.in), tt.want, tt.in)
})

test('slotFor : noms réels, variantes et inconnus', () => {
  const tests = [
    { name: 'Porte avant gauche', want: 'door_fl' },
    { name: 'Porte avant droite', want: 'door_fr' },
    { name: 'Porte dièdre gauche', want: 'door_fl' }, // Valhalla
    { name: 'Roue avant gauche', want: 'wheel_fl' },
    { name: 'Train avant gauche', want: 'corner_fl' },
    { name: 'Coin arrière droite', want: 'corner_rr' }, // Valhalla
    { name: 'Moteur 1.6 16V', want: 'engine' },
    { name: 'Moteur V8 4,0 l biturbo', want: 'engine' },
    { name: 'Moteur électrique arrière', want: null }, // pas le moteur thermique
    { name: 'Roue de secours', want: 'spare_wheel' }, // pas une roue « avant gauche »
    { name: 'Aileron actif', want: null },
  ]
  for (const tt of tests) assert.equal(slotFor(tt.name, slotTable)?.slot ?? null, tt.want, tt.name)
})

test('chaque slot a une forme Simple, chaque forme un slot', () => {
  const slots = new Set(slotTable.slots.map(s => s.slot))
  const shapes = new Set(Object.keys(layout.slots))
  assert.deepEqual([...slots].filter(s => !shapes.has(s)), [])
  assert.deepEqual([...shapes].filter(s => !slots.has(s)), [])
  for (const s of slotTable.slots) for (const f of s.focusWith ?? []) assert.ok(slots.has(f), `${s.slot} → ${f}`)
})

test('Multipla réelle : 56 sous-ensembles, aucun en position estimée', () => {
  const idx = buildVehicleIndex(fixture('multipla-nodes.json'), slotTable)
  assert.equal(idx.systems.length, 6)
  assert.equal(idx.objects.length, 56)
  assert.deepEqual(idx.objects.filter(o => o.estimated).map(o => o.name), [])
  assert.equal(new Set(idx.objects.map(o => o.key)).size, 56)
})

test('Valhalla réelle : noms inconnus en position estimée, jamais perdus', () => {
  const nodes = fixture('valhalla-nodes.json')
  const idx = buildVehicleIndex(nodes, slotTable)
  const est = idx.objects.filter(o => o.estimated)
  assert.ok(est.length > 0)
  assert.ok(est.some(o => o.name === 'Ponton gauche'))
  // tous les sous-ensembles de niveau 2 sont représentés, connus ou estimés
  const level2 = nodes.filter(n => n.part_id === null && idx.systems.some(s => s.id === n.parent_id))
  assert.equal(idx.objects.length, level2.length)
})

const tiny: Vehicle3DNode[] = [
  { id: 1, parent_id: null, name: 'Auto', part_id: null },
  { id: 2, parent_id: 1, name: 'Liaison au sol', part_id: null },
  { id: 3, parent_id: 2, name: 'Roue avant gauche', part_id: null, budget_status: 'over' },
  { id: 4, parent_id: 3, name: 'Pneu', part_id: 40 },
  { id: 5, parent_id: 2, name: 'Train avant gauche', part_id: null },
  { id: 6, parent_id: 1, name: 'Électricité', part_id: null }, // système sans sous-ensemble : un objet
  { id: 7, parent_id: 6, name: 'Boîtier BSI', part_id: 70 },
  { id: 8, parent_id: 2, name: 'Roue avant gauche', part_id: null }, // doublon de nom : estimé
  { id: 9, parent_id: 1, name: 'Pièce directe', part_id: 90 }, // pièce de niveau 1 : pas un système
]

test('buildVehicleIndex : systèmes, doublons, statut budget', () => {
  const idx = buildVehicleIndex(tiny, slotTable)
  assert.deepEqual(idx.systems.map(s => s.name), ['Liaison au sol', 'Électricité'])
  assert.deepEqual(idx.objects.map(o => o.key), ['wheel_fl', 'corner_fl', 'est:8', 'est:6'])
  assert.equal(idx.byKey.get('wheel_fl')?.budgetStatus, 'over')
  assert.equal(idx.byKey.get('corner_fl')?.budgetStatus, null)
})

test('objectsForNode : remonte au sous-ensemble ou au système', () => {
  const idx = buildVehicleIndex(tiny, slotTable)
  const tests = [
    { node: 4, want: ['wheel_fl'] }, // pièce profonde → son sous-ensemble
    { node: 3, want: ['wheel_fl'] },
    { node: 2, want: ['wheel_fl', 'corner_fl', 'est:8'] }, // système → tous ses objets
    { node: 7, want: ['est:6'] },
    { node: 1, want: [] }, // racine : rien
    { node: 999, want: [] }, // inconnu
    { node: null, want: [] },
  ]
  for (const tt of tests) assert.deepEqual(objectsForNode(idx, tt.node), tt.want, String(tt.node))
})

test('focusGroup : roue + train seulement si présents', () => {
  const idx = buildVehicleIndex(tiny, slotTable)
  assert.deepEqual(focusGroup('wheel_fl', idx, slotTable), ['wheel_fl', 'corner_fl'])
  const noCorner = buildVehicleIndex(tiny.filter(n => n.id !== 5), slotTable)
  assert.deepEqual(focusGroup('wheel_fl', noCorner, slotTable), ['wheel_fl'])
})

test('zones et positions estimées', () => {
  assert.deepEqual(zoneFor('Groupe motopropulseur', slotTable), [-1.5, 1.05, 0])
  assert.deepEqual(zoneFor('Inconnu', slotTable), slotTable.defaultZone)
  const tests = [
    { k: 0, want: [-0.33, 0, -0.22] },
    { k: 3, want: [0.33, 0, -0.22] },
    { k: 4, want: [-0.33, 0, 0] },
    { k: 12, want: [-0.33, 0.22, -0.22] },
  ]
  for (const tt of tests) {
    const got = estimatedPosition([0, 0, 0], tt.k)
    got.forEach((v, i) => assert.ok(Math.abs(v - tt.want[i]) < 1e-9, `k=${tt.k} : ${got}`))
  }
})
