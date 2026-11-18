import assert from 'node:assert/strict'
import { test } from 'node:test'
import { productionDaysInput, quantityChanged } from './nodeEdit.ts'

test('jours de production : entier ≥ 0, sinon null', () => {
  const cases: [string, number | null][] = [['12', 12], [' 0 ', 0], ['', null], ['1,5', null], ['-3', null], ['abc', null]]
  for (const [text, want] of cases) assert.equal(productionDaysInput(text), want, JSON.stringify(text))
})

test('quantité : la virgule est acceptée, une saisie vide ou identique ne change rien', () => {
  const cases: [string, string, boolean][] = [['1,5', '1', true], ['1.50', '1.5', false], ['', '1', false], ['2', '1', true]]
  for (const [input, stored, want] of cases) assert.equal(quantityChanged(input, stored), want, input)
})
