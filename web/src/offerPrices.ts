// Prix des offres affichés dans les sélecteurs de la BOM (NLPF-72). Même source que les totaux de la BOM
// (AGENTS §3) : dernière cotation en direct si la BOM n'est pas figée (GET /api/v1/quotes), sinon les
// cotations figées du snapshot de prix (GET /api/v1/price-snapshots/{id}). Endpoints existants : aucun
// changement de contrat /api/v1. Les montants restent des chaînes jusqu'à formatMoney.
import type { Catalog } from './catalog.ts'
import { formatDate, type Money } from './money.ts'

// byOffer : seulement les offres réellement cotées ; une offre absente s'affiche « sans prix ».
// source : provenance lisible des prix (survol de l'option), null si aucune lecture n'a abouti.
export type OfferPrices = { byOffer: ReadonlyMap<number, Money>; source: string | null }

export const noOfferPrices: OfferPrices = { byOffer: new Map(), source: null }

type PriceContext = { mode: string; as_of: string; snapshot_id?: number; reason?: string }
type HistoricalQuote = { offer_id: number; price: Money }
// GET /api/v1/quotes (pricing.PricePage) : une entrée en erreur a `quote: null`.
type PricePage = { context: PriceContext; quotes: { offer_id: number; quote: HistoricalQuote | null }[] }
// GET /api/v1/price-snapshots/{id} (pricing.SnapshotResult) : uniquement les cotations figées.
type SnapshotResult = { id: number; taken_at: string; quotes: HistoricalQuote[] }

function describe(c: PriceContext): string {
  switch (c.mode) {
    case 'live':
      return `en direct au ${formatDate(c.as_of)}`
    case 'snapshot_fallback':
      return `repli sur le snapshot de prix #${c.snapshot_id} du ${formatDate(c.as_of)} (source en direct indisponible)`
    default:
      return `snapshot de prix #${c.snapshot_id} du ${formatDate(c.as_of)}`
  }
}

async function readJSON<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) throw new Error(`prix des offres : ${res.status} ${res.statusText}`)
  return (await res.json()) as T
}

// loadOfferPrices lit les prix des offres à la source utilisée par les totaux de la BOM.
// priceSnapshotID : bom.price_snapshot_id (null = prix en direct).
export async function loadOfferPrices(priceSnapshotID: number | null): Promise<OfferPrices> {
  if (priceSnapshotID === null) {
    const page = await readJSON<PricePage>('/api/v1/quotes')
    const byOffer = new Map<number, Money>()
    for (const e of page.quotes) if (e.quote) byOffer.set(e.offer_id, e.quote.price)
    return { byOffer, source: describe(page.context) }
  }
  const snap = await readJSON<SnapshotResult>(`/api/v1/price-snapshots/${priceSnapshotID}`)
  return {
    byOffer: new Map(snap.quotes.map((q) => [q.offer_id, q.price])),
    source: describe({ mode: 'snapshot', as_of: snap.taken_at, snapshot_id: snap.id }),
  }
}

// withOfferPrices renvoie le catalogue dont chaque offre porte son prix et sa provenance (null : non cotée).
export function withOfferPrices(catalog: Catalog, prices: OfferPrices): Catalog {
  const parts = catalog.parts.map((p) => ({
    ...p,
    offers: p.offers.map((o) => {
      const price = prices.byOffer.get(o.id) ?? null
      return { ...o, price, price_source: price ? prices.source : null }
    }),
  }))
  return { ...catalog, parts, byPart: new Map(parts.map((p) => [p.id, p])) }
}
