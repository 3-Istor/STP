import assert from 'node:assert/strict'
import { test } from 'node:test'
import { decimalSign, formatPercent, formatSignedMoney } from './money.ts'

const eur = (amount: string) => ({ amount, currency: 'EUR' })
// Intl sépare milliers et devise par des espaces insécables : on compare le texte lisible.
const plain = (s: string) => s.replace(/[\u00a0\u202f]/g, ' ')

test('écart signé : signe lu dans la chaîne, arrondi half-even à l’affichage', () => {
  assert.equal(plain(formatSignedMoney(eur('331.9463198037682312408505'))), '+ 331,95 €')
  assert.equal(plain(formatSignedMoney(eur('-510.5984148019521432432258333335'))), '− 510,60 €')
  assert.equal(plain(formatSignedMoney(eur('0.000'))), '0,00 €')
  assert.equal(plain(formatSignedMoney(eur('0.125'))), '+ 0,12 €') // half-even
  assert.equal(formatSignedMoney(null), 'n/d')
})

test('signe d’une chaîne décimale', () => {
  assert.equal(decimalSign('-0.01'), -1)
  assert.equal(decimalSign('0.00'), 0)
  assert.equal(decimalSign('-0'), 0)
  assert.equal(decimalSign('12'), 1)
})

test('pourcentage de l’API mis en forme, « n/d » absent', () => {
  assert.equal(plain(formatPercent('43.16')), '43,2 %')
  assert.equal(plain(formatPercent('-0.09', 2)), '-0,09 %')
  assert.equal(formatPercent(null), 'n/d')
})

test('tri par valeur absolue exacte (causes d’écart), sans conversion en nombre', async () => {
  const { compareAbsDecimal } = await import('./money.ts')
  const amounts = ['73.3191773', '-170.035', '28.299', '60.29310132295939616885', '-0.5']
  const sorted = [...amounts].sort((a, b) => compareAbsDecimal(b, a))
  assert.deepEqual(sorted, ['-170.035', '73.3191773', '60.29310132295939616885', '28.299', '-0.5'])
  // Au-delà de la précision d'un double : 0.10000000000000000001 > 0.1
  assert.equal(compareAbsDecimal('0.10000000000000000001', '-0.1'), 1)
})
