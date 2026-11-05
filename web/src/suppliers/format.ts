import type { Money } from './types'

// Intl reçoit la chaîne décimale exacte. Le cast ne convertit jamais la valeur en number.
function exactFormat(formatter: Intl.NumberFormat, value: string): string {
  return (formatter.format as unknown as (decimal: string) => string)(value)
}
export function formatMoney(money: Money | null | undefined): string {
  if (!money || typeof money.amount !== 'string') return 'Prix indisponible'
  const options = { style: 'currency', currency: money.currency, roundingMode: 'halfEven' } as Intl.NumberFormatOptions
  return exactFormat(new Intl.NumberFormat('fr-FR', options), money.amount)
}
export function formatDecimal(value: string | null | undefined): string {
  if (value === null || value === undefined) return 'Non renseignée'
  const match = /^(-?)(\d+)(?:\.(\d+))?$/.exec(value)
  if (!match) return value
  const integer = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 0 }).format(BigInt(match[1] + match[2]))
  return integer + (match[3] ? `,${match[3]}` : '')
}
export function formatVariation(value: string | null | undefined, trend: string | null | undefined): string {
  if (value === null || value === undefined) return 'Variation indisponible'
  if (/^-?0\.0$/.test(value) && trend === 'up') return 'Hausse < 0,1 %'
  if (/^-?0\.0$/.test(value) && trend === 'down') return 'Baisse < 0,1 %'
  const formatter = new Intl.NumberFormat('fr-FR', { minimumFractionDigits: 1, maximumFractionDigits: 1, signDisplay: trend === 'stable' ? 'never' : 'exceptZero' })
  return `${exactFormat(formatter, value)} %`
}
export function formatUTC(value: string | null | undefined): string {
  if (!value) return 'Date indisponible'
  return new Intl.DateTimeFormat('fr-FR', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' }).format(new Date(value)) + ' UTC'
}
export function formatDay(value: string): string { return value.split('-').reverse().join('/') }
