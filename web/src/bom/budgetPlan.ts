import type { NodePatch, PlanAction } from '../api.ts'

// Libellé d'une action du plan : « Porte avant nue : Arden → Fonderia (2 nœuds) » (même pièce, autre fournisseur)
// ou « Porte A → Porte B · Fonderia » (pièce alternative).
export function planLabel(a: PlanAction): string {
  const base = a.from.part_id === a.to.part_id ? `${a.from.part} : ${a.from.supplier} → ${a.to.supplier}` : `${a.from.part} → ${a.to.part} · ${a.to.supplier}`
  return a.nodes.length > 1 ? `${base} (${a.nodes.length} nœuds)` : base
}

// Matière avant → après, seulement si elle change (« n/d » : non renseignée) ; null sinon.
export function materialChange(a: PlanAction): string | null {
  if (a.from.material === a.to.material) return null
  return `${a.from.material ?? 'n/d'} → ${a.to.material ?? 'n/d'}`
}

// Écart signé (U+2212 pour le moins) d'une chaîne décimale ; « 0 » reste sans signe.
export function signed(value: string, format: (v: string) => string): string {
  if (/^-?0(\.0+)?$/.test(value)) return format('0')
  return value.startsWith('-') ? `−${format(value.slice(1))}` : `+${format(value)}`
}

// Patch d'un nœud pour l'action : même pièce → l'offre seule, sinon pièce et offre (un seul champ par PATCH, contrat T8).
export function planPatch(a: PlanAction): NodePatch {
  return a.from.part_id === a.to.part_id ? { offer_id: a.to.offer_id } : { part_id: a.to.part_id, offer_id: a.to.offer_id }
}
