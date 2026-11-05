import type { Quote } from './types'

export type GraphPoint = { x: number; y: number; quote: Quote }

// Seules des coordonnées visuelles bornées deviennent des number. Aucun prix/taux n'est converti.
export function graphPoints(history: Quote[], width: number, height: number): GraphPoint[] {
  if (!history.length) return []
  const prices = history.map((quote) => {
    const [integer, fraction = ''] = quote.price.amount.split('.')
    return { coefficient: BigInt(integer + fraction), scale: fraction.length }
  })
  const scale = Math.max(...prices.map((price) => price.scale))
  const values = prices.map((price) => price.coefficient * 10n ** BigInt(scale - price.scale))
  const min = values.reduce((a, b) => a < b ? a : b)
  const max = values.reduce((a, b) => a > b ? a : b)
  const padding = 8
  const drawable = BigInt(height - padding * 2)
  const times = history.map((quote) => new Date(quote.quoted_at).getTime())
  const firstTime = Math.min(...times)
  const lastTime = Math.max(...times)
  return history.map((quote, index) => ({
    x: firstTime === lastTime ? width / 2 : padding + (times[index] - firstTime) * (width - padding * 2) / (lastTime - firstTime),
    y: min === max ? height / 2 : height - padding - Number((values[index] - min) * drawable / (max - min)),
    quote,
  }))
}
