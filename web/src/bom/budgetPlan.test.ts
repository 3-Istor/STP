import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { PlanAction } from '../api.ts'
import { materialChange, planLabel, planPatch, signed } from './budgetPlan.ts'

const m = (amount: string) => ({ amount, currency: 'EUR' })
const action = (over: Partial<PlanAction> & { fromPart?: number; toPart?: number } = {}): PlanAction => ({
  nodes: [{ id: 1, name: '' }],
  from: { part_id: over.fromPart ?? 10, offer_id: 20, part: 'Porte avant nue', supplier: 'Arden', material: 'acier', quality: null },
  to: { part_id: over.toPart ?? 10, offer_id: 21, part: 'Porte avant nue', supplier: 'Fonderia', material: 'acier', quality: null },
  gain: m('10'), cumulative_gain: m('10'), co2_delta_kg: '0', mass_delta_kg: '0', lead_time_delta_days: 0, covers: false,
  ...over,
})

test('libellé : même pièce, nombre de nœuds, pièce alternative', () => {
  assert.equal(planLabel(action()), 'Porte avant nue : Arden → Fonderia')
  assert.equal(planLabel(action({ nodes: [{ id: 1, name: '' }, { id: 2, name: '' }] })), 'Porte avant nue : Arden → Fonderia (2 nœuds)')
  assert.equal(planLabel(action({ toPart: 11 })), 'Porte avant nue → Porte avant nue · Fonderia')
})

test('matière : seulement si elle change', () => {
  assert.equal(materialChange(action()), null)
  const a = action()
  assert.equal(materialChange({ ...a, to: { ...a.to, material: 'aluminium' } }), 'acier → aluminium')
  assert.equal(materialChange({ ...a, from: { ...a.from, material: null } }), 'n/d → acier')
})

test('écart signé et patch', () => {
  const id = (v: string) => v
  assert.deepEqual([signed('-1.5', id), signed('2', id), signed('0.00', id), signed('-0', id)], ['−1.5', '+2', '0', '0'])
  assert.deepEqual(planPatch(action()), { offer_id: 21 })
  assert.deepEqual(planPatch(action({ toPart: 11 })), { part_id: 11, offer_id: 21 })
})
