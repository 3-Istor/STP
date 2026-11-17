import assert from 'node:assert/strict'
import { test } from 'node:test'
import { budgetBarWidth, budgetClass, budgetPercentText } from './budget.ts'
import type { NodeBudget } from '../api.ts'

const budget = (state: NodeBudget['state'], percent: string | null): NodeBudget => ({
  node_id: 1, target: { amount: '100.00', currency: 'EUR' }, cost: null, delta: null, percent, state, error: null,
})

test('largeur de barre : partie entière du pourcentage de l’API, plafonnée à 100', () => {
  const cases: [string | null, number | null][] = [
    ['97.2', 97],
    ['0.0', 0],
    ['113.4', 100],
    ['100.0', 100],
    [null, null],
    ['-3.0', null], // un pourcentage négatif n'est pas une largeur
    ['abc', null],
  ]
  for (const [input, want] of cases) assert.equal(budgetBarWidth(input), want, String(input))
})

test('couleur : état de l’API, jamais vert pour une comparaison indisponible', () => {
  assert.equal(budgetClass(budget('ok', '50.0')), 't-ok')
  assert.equal(budgetClass(budget('warn', '95.0')), 't-warn')
  assert.equal(budgetClass(budget('over', '120.0')), 't-over')
  assert.equal(budgetClass(budget('unavailable', null)), '')
  assert.equal(budgetClass(undefined), '')
})

test('pourcentage affiché tel que l’API le donne, une décimale, « n/d » sans comparaison', () => {
  assert.equal(budgetPercentText(budget('over', '104.0')), '104,0 %')
  assert.equal(budgetPercentText(budget('ok', '86.3')), '86,3 %')
  assert.equal(budgetPercentText(budget('unavailable', null)), 'n/d')
})
