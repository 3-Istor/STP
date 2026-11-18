import assert from 'node:assert/strict'
import { test } from 'node:test'
import { missingPairs } from './snapshotCoverage.ts'

test('un lot figé vers EUR ne couvre pas une BOM en GBP ; l’inverse suffit', () => {
  const eurOnly = [{ base_currency: 'GBP', quote_currency: 'EUR' }, { base_currency: 'USD', quote_currency: 'EUR' }]
  const gbpBom = [{ from: 'EUR', to: 'GBP' }, { from: 'USD', to: 'GBP' }]
  assert.deepEqual(missingPairs(eurOnly, gbpBom), [{ from: 'USD', to: 'GBP' }]) // EUR→GBP couvert par GBP→EUR
  assert.deepEqual(missingPairs(eurOnly, [{ from: 'USD', to: 'EUR' }]), [])
  assert.deepEqual(missingPairs([], [{ from: 'EUR', to: 'EUR' }]), [])
})
