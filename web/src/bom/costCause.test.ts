import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { TreeNode } from '../api.ts'
import { dominantCause, missingSummary } from './costCause.ts'

const leaf = (id: number, code: string | null) => [id, { id, totals: { cost_error_code: code } } as unknown as TreeNode] as const
const byID = new Map([leaf(1, 'no_budget_rate'), leaf(2, 'no_budget_rate'), leaf(3, 'no_quote'), leaf(4, 'missing_cost'), leaf(5, null)])

test('cause principale : la plus fréquente parmi les causes propres des pièces', () => {
  const c = dominantCause([1, 2, 3, 4, 5, 99], byID)
  assert.deepEqual(c, { code: 'no_budget_rate', label: 'taux budget absent', count: 2 })
})

test('égalité : ordre alphabétique du code ; aucune cause propre : null', () => {
  assert.equal(dominantCause([1, 3], byID)?.code, 'no_budget_rate')
  assert.equal(dominantCause([4, 5, 99], byID), null)
})

test('résumé : singulier, pluriel, part de la cause principale', () => {
  assert.equal(missingSummary([1], null), '1 pièce sans coût')
  assert.equal(missingSummary([1, 2, 3], dominantCause([1, 2, 3], byID)), '3 pièces sans coût · cause principale : taux budget absent (2)')
  assert.equal(missingSummary([1, 2], dominantCause([1, 2], byID)), '2 pièces sans coût · cause principale : taux budget absent')
})
