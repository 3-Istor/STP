// Catalogue vu par l'UI de la BOM : noms des pièces et offres proposées au changement d'offre.
// Source : GET /api/v1/parts (part-catalog). Avec BOM_DEMO=1, GET /api/v1/demo/catalog le remplace
// (feature toggle — propriétaire : Arthur.presle ; à supprimer avec le seed sample-dataset).
import type { Money } from './money.ts'

export type Offer = {
  id: number
  supplier: string
  currency: string
  lead_time_days: number
  origin_country: string
  price: Money | null // dernière cotation ; null : aucune
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

function fromCatalog(p: CatalogPart): Part {
  return {
    id: p.id,
    name: p.name,
    category: p.category,
    co2_kg: p.co2_kg_per_unit,
    mass_kg: p.mass_kg,
    production_days: p.production_days,
    // Le prix vient des totaux de la BOM (cotation de live-price-feed), pas du catalogue.
    offers: p.offers.map((o) => ({
      id: o.id,
      supplier: `Fournisseur #${o.supplier_id}`,
      currency: o.currency,
      lead_time_days: o.lead_time_days,
      origin_country: o.origin_country,
      price: null,
    })),
  }
}

export async function loadCatalog(): Promise<Catalog> {
  let parts: Part[]
  const demo = await fetch('/api/v1/demo/catalog')
  if (demo.ok) {
    parts = (await demo.json()) as Part[]
  } else if (demo.status === 404) {
    const res = await fetch('/api/v1/parts')
    if (!res.ok) throw new Error(`catalogue : ${res.status} ${res.statusText}`)
    parts = ((await res.json()) as CatalogPart[]).map(fromCatalog)
  } else {
    throw new Error(`catalogue : ${demo.status} ${demo.statusText}`)
  }
  return { parts, byPart: new Map(parts.map((p) => [p.id, p])), available: true }
}

export function partName(c: Catalog, partID: number): string {
  return c.byPart.get(partID)?.name ?? `Pièce #${partID}`
}
