import assert from 'node:assert/strict'
import { test } from 'node:test'
import { computeImpact, isNoImpact, subDecimal } from './impact.ts'

test('soustraction décimale exacte, signe et échelle', () => {
  const cases: [string, string, string][] = [['10.5', '0.25', '10.25'], ['0.1', '0.3', '-0.2'], ['5', '5', '0'], ['1.50', '1.5', '0.00'], ['-2', '3.5', '-5.5']]
  for (const [a, b, want] of cases) assert.equal(subDecimal(a, b), want, `${a} - ${b}`)
})

test('impact : écarts sur les totaux de la racine ; coût inconnu ou devises différentes : pas d’écart inventé', () => {
  const before = { cost: { amount: '1000.10', currency: 'EUR' }, co2_kg: '500', mass_kg: '900.5', lead_time_days: 20 }
  const after = { cost: { amount: '1012.40', currency: 'EUR' }, co2_kg: '498.5', mass_kg: '900.5', lead_time_days: 22 }
  assert.deepEqual(computeImpact(before, after), { cost: { amount: '12.30', currency: 'EUR' }, co2_kg: '-1.5', mass_kg: '0.0', lead_time_days: 2 })
  assert.equal(computeImpact(before, { ...after, cost: null }).cost, null)
  assert.equal(computeImpact(before, { ...after, cost: { amount: '1', currency: 'USD' } }).cost, null)
  assert.equal(isNoImpact(computeImpact(before, { ...before })), true)
  assert.equal(isNoImpact(computeImpact(before, after)), false)
})
