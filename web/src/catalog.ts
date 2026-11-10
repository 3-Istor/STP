// Catalogue vu par l'UI de la BOM : noms des pièces et offres proposées au changement d'offre.
// Source : GET /api/v1/parts (part-catalog) et GET /api/v1/suppliers pour les noms des fournisseurs.
import type { Money } from './money.ts'

export type Offer = {
  id: number
  supplier: string
  currency: string
  lead_time_days: number
  origin_country: string
  price: Money | null // cotation à la source des totaux de la BOM (offerPrices.ts) ; null : aucune
  price_source: string | null // provenance du prix (« en direct au … », « snapshot de prix #N … »)
}

export type Part = {
  id: number
  name: string
  category: string
  co2_kg: string | null // null : attribut absent du catalogue, jamais remplacé par zéro
  mass_kg: string | null
  production_days: number | null
  offers: Offer[]
}

export type Catalog = { parts: Part[]; byPart: Map<number, Part>; available: boolean }

export const emptyCatalog: Catalog = { parts: [], byPart: new Map(), available: false }

// Forme de GET /api/v1/parts (catalog.PartWithOffers) : attributs optionnels, pas de prix.
type CatalogPart = {
  id: number
  name: string
  category: string
  co2_kg_per_unit: string | null
  mass_kg: string | null
  production_days: number | null
  offers: { id: number; supplier_id: number; currency: string; lead_time_days: number; origin_country: string }[]
}

function fromCatalog(p: CatalogPart, supplierNames: Map<number, string>): Part {
  return {
    id: p.id,
    name: p.name,
    category: p.category,
    co2_kg: p.co2_kg_per_unit,
    mass_kg: p.mass_kg,
    production_days: p.production_days,
    // Le catalogue ne porte pas de prix : withOfferPrices (offerPrices.ts) les ajoute depuis la source
    // des totaux de la BOM (direct ou snapshot de prix).
    offers: p.offers.map((o) => ({
      id: o.id,
      supplier: supplierNames.get(o.supplier_id) ?? `Fournisseur #${o.supplier_id}`,
      currency: o.currency,
      lead_time_days: o.lead_time_days,
      origin_country: o.origin_country,
      price: null,
      price_source: null,
    })),
  }
}

export async function loadCatalog(): Promise<Catalog> {
  const [res, sup] = await Promise.all([fetch('/api/v1/parts'), fetch('/api/v1/suppliers')])
  if (!res.ok) throw new Error(`catalogue : ${res.status} ${res.statusText}`)
  // Sans la liste des fournisseurs, l'offre affiche « Fournisseur #id » : le catalogue reste utilisable.
  const suppliers = sup.ok ? ((await sup.json()) as { id: number; name: string }[]) : []
  const names = new Map(suppliers.map((s) => [s.id, s.name]))
  const parts = ((await res.json()) as CatalogPart[]).map((p) => fromCatalog(p, names))
  return { parts, byPart: new Map(parts.map((p) => [p.id, p])), available: true }
}

export function partName(c: Catalog, partID: number): string {
  return c.byPart.get(partID)?.name ?? `Pièce #${partID}`
}
