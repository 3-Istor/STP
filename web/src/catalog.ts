// Catalogue vu par l'UI de la BOM : noms des pièces et offres proposées au changement d'offre.
// Source provisoire : GET /api/v1/demo/catalog, servi seulement avec BOM_DEMO=1 (internal/demo).
// Feature toggle — propriétaire : Arthur.presle ; raison : tester l'UI avant NLPF-67/68 ;
// suppression : à la livraison de NLPF-67/68, remplacer cet appel par l'API catalogue réelle.
// Sans démo (404), le catalogue est vide : l'arbre affiche « Pièce #id » et l'offre par son id.
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
  co2_kg: string
  mass_kg: string
  production_days: number
  offers: Offer[]
}

export type Catalog = { parts: Part[]; byPart: Map<number, Part>; available: boolean }

export const emptyCatalog: Catalog = { parts: [], byPart: new Map(), available: false }

export async function loadCatalog(): Promise<Catalog> {
  const res = await fetch('/api/v1/demo/catalog')
  if (res.status === 404) return emptyCatalog
  if (!res.ok) throw new Error(`catalogue : ${res.status} ${res.statusText}`)
  const parts = (await res.json()) as Part[]
  return { parts, byPart: new Map(parts.map((p) => [p.id, p])), available: true }
}

export function partName(c: Catalog, partID: number): string {
  return c.byPart.get(partID)?.name ?? `Pièce #${partID}`
}
