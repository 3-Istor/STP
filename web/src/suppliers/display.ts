import type { ConvertedQuote, FXQuote, Money, OfferView, Quote, Statistics } from './types'

// Devise d'affichage des historiques fournisseur (ADR 0030) : conversion faite par l'API, au taux du
// jour de chaque cotation. Le front ne convertit jamais lui-même.
export const DISPLAY_CURRENCY = 'EUR'

// Historique prêt à afficher : converti quand l'API a fourni le bloc `converted`, sinon en devise
// d'origine (source de taux absente). `origin` retrouve la cotation d'origine et le taux d'un point.
export type DisplayStats = {
  converted: boolean
  history: Quote[]
  current: Quote | null
  reference: Quote | null
  minimum: Money | null
  maximum: Money | null
  variation: string | null
  trend: Statistics['trend']
  missing: number
  origin: Map<number, { quote: Quote; rate?: FXQuote }>
}

export function displayStats(view: OfferView): DisplayStats | null {
  const stats = view.statistics
  if (!stats) return null
  const byID = new Map<number, Quote>()
  for (const q of [...stats.history, stats.current_quote, stats.reference_quote]) if (q) byID.set(q.id, q)
  const conv = view.converted
  if (!conv) {
    return {
      converted: false, history: stats.history, current: stats.current_quote, reference: stats.reference_quote, minimum: stats.minimum, maximum: stats.maximum,
      variation: stats.variation_percent, trend: stats.trend, missing: 0,
      origin: new Map([...byID].map(([id, quote]) => [id, { quote }])),
    }
  }
  // Une cotation non convertie (aucun taux du jour) n'est pas tracée : elle est comptée dans `missing`.
  const asQuote = (c: ConvertedQuote | null): Quote | null => {
    const source = c ? byID.get(c.quote_id) : undefined
    return c?.price && source ? { ...source, price: c.price } : null
  }
  const origin = new Map<number, { quote: Quote; rate?: FXQuote }>()
  for (const c of [...conv.history, conv.current_quote, conv.reference_quote]) {
    const source = c ? byID.get(c.quote_id) : undefined
    if (c && source) origin.set(c.quote_id, { quote: source, rate: c.fx_quote })
  }
  return {
    converted: true,
    history: conv.history.map(asQuote).filter((q): q is Quote => q !== null),
    current: asQuote(conv.current_quote),
    reference: asQuote(conv.reference_quote),
    minimum: conv.minimum, maximum: conv.maximum, variation: conv.variation_percent, trend: conv.trend,
    missing: conv.missing, origin,
  }
}
