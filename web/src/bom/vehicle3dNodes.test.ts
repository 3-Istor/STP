import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { NodeBudget, TreeNode } from '../api.ts'
import { vehicle3dKey, vehicle3dNodes } from './vehicle3dNodes.ts'

const node = (id: number, parent_id: number | null, name: string, part_id: number | null = null): TreeNode => ({
  id, parent_id, name, part_id, offer_id: null, quantity: '1', production_days: 0, strategy: null, totals: null,
})
const budget = (node_id: number, state: NodeBudget['state']): NodeBudget => ({
  node_id, target: { amount: '100.00', currency: 'EUR' }, cost: null, delta: null, percent: null, state, error: null,
})
const nodes = [node(1, null, 'Multipla'), node(2, 1, 'Carrosserie'), node(3, 2, 'Porte avant gauche'), node(4, 3, '', 40), node(5, 2, 'Capot')]

test('statut budget : propre au nœud, null sans budget ou comparaison indisponible, jamais hérité', () => {
  const budgets = new Map([[2, budget(2, 'over')], [3, budget(3, 'warn')], [5, budget(5, 'unavailable')]])
  const got = new Map(vehicle3dNodes(nodes, budgets).map((n) => [n.id, n.budget_status]))
  assert.equal(got.get(2), 'over')
  assert.equal(got.get(3), 'warn')
  assert.equal(got.get(5), null) // indisponible : gris, pas vert
  assert.equal(got.get(4), null) // pièce sous une porte « warn » : pas d'héritage
  assert.equal(got.get(1), null)
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
