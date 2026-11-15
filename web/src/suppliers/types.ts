export type Money = { amount: string; currency: string }
export type Supplier = { id: number; name: string; quality_score: string | null; trust_score: string | null }
export type Quote = { id: number; offer_id: number; price: Money; source: string; quoted_at: string }
export type Statistics = {
  offer_id: number; as_of: string; window_start: string
  current_quote: Quote | null; reference_quote: Quote | null
  period_start: string | null; period_end: string | null
  minimum: Money | null; maximum: Money | null
  variation_percent: string | null; trend: 'up' | 'down' | 'stable' | null; history: Quote[]
}
export type Offer = {
  id: number; part_id: number; supplier_id: number; currency: string
  price_min: Money; price_max: Money; lead_time_days: number
  quality_score: string | null; origin_country: string
}
export type Part = { id: number; name: string; category: string; zone: string }
export type FXQuote = { id: number; base_currency: string; quote_currency: string; rate: string; source: string; quoted_at: string }
// Cotation convertie au taux de son jour (ADR 0030) : price null quand error est renseignée, jamais zéro.
export type ConvertedQuote = { quote_id: number; quoted_at: string; price: Money | null; direction?: string; fx_quote?: FXQuote; error?: string }
export type ConvertedStatistics = {
  currency: string; current_quote: ConvertedQuote | null; reference_quote: ConvertedQuote | null
  minimum: Money | null; maximum: Money | null; variation_percent: string | null; trend: Statistics['trend']
  history: ConvertedQuote[]; missing: number
}
export type OfferView = { offer: Offer; part: Part; statistics: Statistics | null; converted?: ConvertedStatistics; error?: string }
export type OffersPage = { as_of: string | null; offers: OfferView[]; context?: { mode: 'live' | 'snapshot' | 'snapshot_fallback'; as_of: string; tick?: number; snapshot_id?: number; reason?: string } }
export type Tier = { min_quantity: string; discount_percent: string }
export type Contract = {
  id: number; supplier_id: number; starts_on: string; ends_on: string; conditions: string
  offer_ids: number[]; tiers: Tier[]; status?: 'active' | 'not_started' | 'expired'
}
