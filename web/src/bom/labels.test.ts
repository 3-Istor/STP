import assert from 'node:assert/strict'
import { test } from 'node:test'
import { roundedLabel } from './labels.ts'

const plain = (s: string) => s.replace(/[  ]/g, ' ')

test('libellés de cascade : nombres longs arrondis à l’affichage, le reste intact', () => {
  const cases: [string, string][] = [
    ['Prix coté 124.74059645460442 GBP × 1, au dernier taux GBP→EUR 1.163666652231798', 'Prix coté 124,74 GBP × 1, au dernier taux GBP→EUR 1,1637'],
    ['Transport (origine GB) : 0.28 EUR/kg × 13.5585 kg × 1', 'Transport (origine GB) : 0,28 EUR/kg × 13,5585 kg × 1'],
    ['Outillage 84000.00 EUR ÷ 60000 véhicules × part du nœud 1 / 2', 'Outillage 84 000,00 EUR ÷ 60000 véhicules × part du nœud 1 / 2'],
    ['Remise de contrat 9.7 % (palier ≥ 500 u.)', 'Remise de contrat 9,7 % (palier ≥ 500 u.)'],
    ['Prix coté 0.125 EUR', 'Prix coté 0,12 EUR'], // half-even
    ['Écart de change : budget 2026 GBP→EUR 1.15 au lieu du dernier taux 1.1458828780', 'Écart de change : budget 2026 GBP→EUR 1,15 au lieu du dernier taux 1,1459'],
    ['Contrat expiré : remise non appliquée', 'Contrat expiré : remise non appliquée'],
  ]
  for (const [input, want] of cases) assert.equal(plain(roundedLabel(input)), want, input)
})
