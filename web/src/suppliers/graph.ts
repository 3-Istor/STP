import type { Quote } from './types'

// Prix comparés en entiers exacts (BigInt) à une échelle commune : aucun prix n'est converti en number.
// Seules des coordonnées visuelles bornées deviennent des number.
function scaled(history: Quote[]): bigint[] {
  const prices = history.map((quote) => {
    const [integer, fraction = ''] = quote.price.amount.split('.')
    return { coefficient: BigInt(integer + fraction), scale: fraction.length }
  })
  const scale = Math.max(...prices.map((price) => price.scale))
  return prices.map((price) => price.coefficient * 10n ** BigInt(scale - price.scale))
}

// Un mois de cotations résumé par trois cotations réelles : la dernière du mois (clôture), la plus basse
// et la plus haute. Aucune moyenne n'est calculée : chaque valeur affichée est un prix réellement coté.
export type Month = { key: string; start: Date; close: Quote; low: Quote; high: Quote; count: number }

export function monthlySeries(history: Quote[]): Month[] {
  if (!history.length) return []
  const values = scaled(history)
  const months = new Map<string, { start: Date; close: number; low: number; high: number; count: number }>()
  history.forEach((quote, index) => {
    const at = new Date(quote.quoted_at)
    const key = `${at.getUTCFullYear()}-${String(at.getUTCMonth() + 1).padStart(2, '0')}`
    const month = months.get(key)
    if (!month) {
      months.set(key, { start: new Date(Date.UTC(at.getUTCFullYear(), at.getUTCMonth(), 1)), close: index, low: index, high: index, count: 1 })
      return
    }
    month.count++
    if (at.getTime() >= new Date(history[month.close].quoted_at).getTime()) month.close = index
    if (values[index] < values[month.low]) month.low = index
    if (values[index] > values[month.high]) month.high = index
  })
  return [...months.entries()]
    .sort(([a], [b]) => (a < b ? -1 : 1))
    .map(([key, m]) => ({ key, start: m.start, close: history[m.close], low: history[m.low], high: history[m.high], count: m.count }))
}

export type MonthPoint = { month: Month; x: number; close: number; low: number; high: number }
export type MonthChart = { points: MonthPoint[]; low: Quote; high: Quote; top: number; bottom: number }

// monthlyChart place chaque mois sur l'axe horizontal (régulièrement espacés) et ses trois cotations sur
// l'axe vertical, entre la plus basse et la plus haute cotation de la période.
export function monthlyChart(months: Month[], width: number, height: number, padding = { top: 12, right: 12, bottom: 12, left: 12 }): MonthChart | null {
  if (!months.length) return null
  const quotes = months.flatMap((m) => [m.close, m.low, m.high])
  const values = scaled(quotes)
  let lowIndex = 0
  let highIndex = 0
  values.forEach((value, index) => {
    if (value < values[lowIndex]) lowIndex = index
    if (value > values[highIndex]) highIndex = index
  })
  const min = values[lowIndex]
  const max = values[highIndex]
  const top = padding.top
  const bottom = height - padding.bottom
  const drawable = BigInt(bottom - top)
  const y = (value: bigint) => (min === max ? (top + bottom) / 2 : bottom - Number(((value - min) * drawable) / (max - min)))
  const step = months.length > 1 ? (width - padding.left - padding.right) / (months.length - 1) : 0
  return {
    points: months.map((month, index) => ({
      month,
      x: months.length > 1 ? padding.left + index * step : width / 2,
      close: y(values[index * 3]),
      low: y(values[index * 3 + 1]),
      high: y(values[index * 3 + 2]),
    })),
    low: quotes[lowIndex],
    high: quotes[highIndex],
    top,
    bottom,
  }
}

const monthFormat = new Intl.DateTimeFormat('fr-FR', { month: 'short', timeZone: 'UTC' })
const monthYearFormat = new Intl.DateTimeFormat('fr-FR', { month: 'long', year: 'numeric', timeZone: 'UTC' })

// Étiquette d'axe : « janv. 2033 » au premier mois et à chaque janvier, sinon le mois seul.
export function monthLabel(month: Month, index: number): string {
  const short = monthFormat.format(month.start)
  return index === 0 || month.start.getUTCMonth() === 0 ? `${short} ${month.start.getUTCFullYear()}` : short
}
export function monthTitle(month: Month): string { return monthYearFormat.format(month.start) }
