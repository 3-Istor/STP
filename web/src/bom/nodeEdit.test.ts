import { describe, expect, it } from 'vitest'
import { productionDaysInput, quantityChanged } from './nodeEdit.ts'

describe('productionDaysInput', () => {
  it.each([
    ['12', 12],
    [' 0 ', 0],
    ['', null],
    ['1,5', null],
    ['-3', null],
    ['abc', null],
  ])('%j → %j', (text, want) => expect(productionDaysInput(text)).toBe(want))
})

describe('quantityChanged', () => {
  it.each([
    ['1,5', '1', true],
    ['1.50', '1.5', false],
    ['', '1', false],
    ['2', '1', true],
  ])('%j vs %j → %j', (input, stored, want) => expect(quantityChanged(input, stored)).toBe(want))
})
