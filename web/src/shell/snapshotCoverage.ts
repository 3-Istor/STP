export type Pair = { from: string; to: string }
export type FrozenRate = { base_currency: string; quote_currency: string }

// missingPairs : required pairs a price lot cannot convert; the inverse pair is enough (ADR 0017).
export function missingPairs(rates: FrozenRate[], required: Pair[]): Pair[] {
  const have = new Set(rates.map((r) => `${r.base_currency}>${r.quote_currency}`))
  return required.filter((p) => p.from !== p.to && !have.has(`${p.from}>${p.to}`) && !have.has(`${p.to}>${p.from}`))
}
