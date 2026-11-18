import { decimalInput, sameDecimal } from '../money.ts'

// productionDaysInput lit les jours de production saisis : un entier ≥ 0, sinon null (bouton désactivé).
export function productionDaysInput(text: string): number | null {
  const t = text.trim()
  return /^\d{1,6}$/.test(t) ? Number.parseInt(t, 10) : null
}

// quantityChanged dit si la saisie (virgule ou point) diffère de la quantité enregistrée.
export function quantityChanged(input: string, stored: string): boolean {
  const q = decimalInput(input)
  return q !== '' && !sameDecimal(q, stored)
}
