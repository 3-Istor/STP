import { compareAbsDecimal, decimalInput, type Money } from '../money.ts'
import { errorMessage, request } from './api'
import type { Offer, Supplier } from './types'

// Attributs catalogue d'une pièce (GET/PATCH /parts/{id}) : décimaux en chaînes, null = non renseigné.
export type PartAttrs = {
  id: number; name: string
  mass_kg: string | null; co2_kg_per_unit: string | null; recycled_percent: string | null; recyclability_percent: string | null
  manufacturing_cost: Money | null; production_days: number | null; labor_hours: string | null; hourly_rate: Money | null
}
export type PartPatch = Partial<Omit<PartAttrs, 'id' | 'name'>>
export type OfferPatch = { price_min?: string; price_max?: string; lead_time_days?: number; quality_score?: string; origin_country?: string }
export type SupplierPatch = { name?: string; quality_score?: string; trust_score?: string }

const patch = <T,>(path: string, body: unknown) => request<T>(path, { method: 'PATCH', body: JSON.stringify(body) })
export const getPart = (id: number) => request<PartAttrs>(`/parts/${id}`)
export const updatePart = (id: number, body: PartPatch) => patch<PartAttrs>(`/parts/${id}`, body)
export const getOffer = (id: number) => request<Offer>(`/offers/${id}`)
export const updateOffer = (id: number, body: OfferPatch) => patch<Offer>(`/offers/${id}`, body)
export const updateSupplier = (id: number, body: SupplierPatch) => patch<Supplier>(`/suppliers/${id}`, body)
// Archivée si l'offre est utilisée par une BOM ou un contrat, supprimée sinon (ADR 0009) : l'API le dit.
export const archiveOffer = (id: number) => request<{ id: number; archived: boolean }>(`/offers/${id}`, { method: 'DELETE' })

// attempt : le message d'erreur à afficher dans la fenêtre, null si l'appel a réussi.
export async function attempt(call: () => Promise<unknown>): Promise<string | null> {
  try { await call(); return null } catch (e) { return errorMessage(e) }
}

// Validation au champ : chaîne vide si valide, sinon le message affiché sous le champ.
export const decimalError = (text: string, max?: string) => {
  const v = decimalInput(text)
  if (!/^\d+(\.\d+)?$/.test(v)) return 'Nombre positif attendu (ex. 12,5).'
  return max !== undefined && compareAbsDecimal(v, max) > 0 ? `Au plus ${max}.` : ''
}
export const daysError = (text: string) => (/^\d+$/.test(text.trim()) ? '' : 'Entier de jours, 0 ou plus.')
export const scoreError = (text: string) => decimalError(text, '5')
