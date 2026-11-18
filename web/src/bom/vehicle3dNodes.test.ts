import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { NodeBudget, TreeNode } from '../api.ts'
import { boundVehicle3dKey, vehicle3dKey, vehicle3dNodes } from './vehicle3dNodes.ts'

const node = (id: number, parent_id: number | null, name: string, part_id: number | null = null): TreeNode => ({
  id, parent_id, name, part_id, offer_id: null, quantity: '1', production_days: 0, strategy: null, totals: null,
})
const budget = (node_id: number, state: NodeBudget['state']): NodeBudget => ({
  node_id, target: { amount: '100.00', currency: 'EUR' }, cost: null, delta: null, percent: null, state, error: null,
})
const nodes = [node(1, null, 'Multipla'), node(2, 1, 'Carrosserie'), node(3, 2, 'Porte avant gauche'), node(4, 3, '', 40), node(5, 2, 'Capot')]

test('statut budget : celui du nœud, sinon du sous-ensemble budgété le plus proche (hors racine)', () => {
  const budgets = new Map([[1, budget(1, 'ok')], [2, budget(2, 'over')], [3, budget(3, 'warn')], [5, budget(5, 'unavailable')]])
  const got = new Map(vehicle3dNodes(nodes, budgets).map((n) => [n.id, n.budget_status]))
  assert.equal(got.get(1), 'ok')
  assert.equal(got.get(2), 'over')
  assert.equal(got.get(3), 'warn')
  assert.equal(got.get(5), null) // budget propre indisponible : gris, pas vert, pas celui du parent
  assert.equal(got.get(4), 'warn') // pièce de la porte : statut de la porte, pas de la carrosserie
})

test('statut budget : la racine ne colore pas ses descendants (pas de vert ou rouge global)', () => {
  const got = new Map(vehicle3dNodes(nodes, new Map([[1, budget(1, 'over')]])).map((n) => [n.id, n.budget_status]))
  assert.equal(got.get(1), 'over')
  for (const id of [2, 3, 4, 5]) assert.equal(got.get(id), null, `nœud ${id}`)
})

test('statut budget : un budget indisponible masque celui de l’ancêtre ; un niveau 2 hérite de son système', () => {
  const deep = [...nodes, node(6, 5, 'Charnière de capot'), node(7, 5, '', 41)]
  const got = new Map(vehicle3dNodes(deep, new Map([[2, budget(2, 'over')], [5, budget(5, 'unavailable')]])).map((n) => [n.id, n.budget_status]))
  assert.equal(got.get(3), 'over') // porte sans budget propre : celui de la carrosserie
  assert.equal(got.get(4), 'over')
  assert.equal(got.get(6), null) // sous le capot indisponible : gris, pas le rouge de la carrosserie
  assert.equal(got.get(7), null)
})

test('statut budget : parent absent de la liste, la remontée continue jusqu’au dernier connu', () => {
  const orphan = [node(3, 2, 'Porte'), node(4, 3, '', 40)] // 2 (et la racine) absents
  const got = new Map(vehicle3dNodes(orphan, new Map([[3, budget(3, 'warn')]])).map((n) => [n.id, n.budget_status]))
  assert.equal(got.get(4), 'warn')
})

test('sans budgets dans l’API : champ absent (la 3D colore par système et le dit)', () => {
  for (const n of vehicle3dNodes(nodes, null)) assert.ok(!('budget_status' in n), `nœud ${n.id}`)
})

test('clé de scène : stable si seuls les totaux changent, change avec la structure ou un statut', () => {
  const budgets = new Map([[3, budget(3, 'warn')]])
  const k = vehicle3dKey(vehicle3dNodes(nodes, budgets))
  const withTotals = nodes.map((n) => ({ ...n, quantity: '2' }))
  assert.equal(vehicle3dKey(vehicle3dNodes(withTotals, budgets)), k)
  const moved = nodes.map((n) => (n.id === 4 ? { ...n, parent_id: 5 } : n))
  assert.notEqual(vehicle3dKey(vehicle3dNodes(moved, budgets)), k)
  assert.notEqual(vehicle3dKey(vehicle3dNodes(nodes, new Map([[3, budget(3, 'over')]]))), k)
  assert.notEqual(vehicle3dKey(vehicle3dNodes(nodes, null)), k)
})

test('remplacement physique invalide la scène ; offre seule conserve sa géométrie', () => {
  const baseline = vehicle3dKey(vehicle3dNodes(nodes, null))
  const cases = [
    { patch: { part_id: 41 }, changed: true },
    { patch: { offer_id: 900 }, changed: false },
  ]
  for (const c of cases) {
    const next = nodes.map(n => n.id === 4 ? { ...n, ...c.patch } : n)
    assert.equal(vehicle3dKey(vehicle3dNodes(next, null)) !== baseline, c.changed)
  }
})

test('rendu lié : quantités transmises, scène renouvelée pour instances mais stable pour offres/prix', () => {
  const baseline = boundVehicle3dKey(vehicle3dNodes(nodes, null))
  for (const [patch, changed] of [
    [{ quantity: '2' }, true], [{ part_id: 41 }, true], [{ parent_id: 5 }, true],
    [{ offer_id: 900 }, false], [{ production_days: 10 }, false],
  ] as const) {
    const next = nodes.map(n => n.id === 4 ? { ...n, ...patch } : n)
    assert.equal(boundVehicle3dKey(vehicle3dNodes(next, null)) !== baseline, changed)
  }
  assert.equal(vehicle3dNodes(nodes, null)[3].quantity, '1')
})

test('libellé catalogue lisible sans changer l’identité de l’occurrence', () => {
  const parts = new Map([[40, { name: 'Bloc moteur fourni' }]])
  const result = vehicle3dNodes(nodes, null, parts)
  assert.equal(result[3].name, 'Bloc moteur fourni')
  assert.equal(result[3].id, nodes[3].id)
  assert.equal(result[3].part_id, 40)
  assert.equal(result[2].name, nodes[2].name)
})
