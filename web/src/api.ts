// Client de l'API /api/v1 (contrat : internal/httpapi/boms_json.go). Champs en snake_case comme l'API ;
// montants, quantités, CO2, masses et taux en chaînes décimales, jamais en `number`.
import type { Money } from './money.ts'

export type BOM = {
  id: number
  name: string
  currency: string
  volume: number
  annual_hours: string // base horaire annuelle d'un ETP (ADR 0024)
  destination_country: string | null // pays de l'usine (ADR 0025) ; null : non renseigné
  price_snapshot_id: number | null // null : prix en direct
  source_snapshot_id: number | null
  created_at: string
}

export type Rate = { from: string; to: string; rate: string; source: string; quoted_at: string }
export type Quote = { offer_id: number; price: Money; source: string; quoted_at: string }

export type Totals = {
  real_quantity: string
  cost: Money | null // null quand cost_error est renseigné, jamais zéro
  cost_error_code: 'no_quote' | 'no_rate' | 'missing_cost' | null
  cost_error: string | null
  missing_cost: number[]
  co2_kg: string
  mass_kg: string
  lead_time_days: number
  own_cost: Money | null
  own_lead_time_days: number
  quote: Quote | null
  rate: Rate | null
  labor_hours: string | null // heures par véhicule du sous-arbre ; null quand labor_error est renseigné
  labor_error: string | null
  missing_labor: number[]
  fte: string | null // ETP arrondi à 2 décimales par l'API ; null sans heures ou sans base horaire
  stock_covered: string | null // quantité couverte par le stock de la BOM ; null si la pièce n'a pas de stock
  in_stock: boolean // couverture totale : délai propre nul (ADR 0022)
}

export type StockLine = { part_id: number; quantity: string }

// Anomalies de coût (ADR 0027) : status jamais « normal » par défaut ; reason dit pourquoi non évaluable.
export type NodeAnomaly = {
  node_id: number
  part_id: number
  category: string
  status: 'anomaly' | 'normal' | 'not_evaluable'
  unit_cost: Money | null
  median: Money | null
  ratio: string | null // coût ÷ médiane, 2 décimales
  peers: number
  missing: number // pièces de la catégorie écartées faute de prix ou de taux
  reason: string | null
}

export type CostAnomalies = {
  threshold: string
  currency: string
  price_snapshot_id: number | null
  rates: Rate[]
  nodes: NodeAnomaly[]
}

// Coût total rendu (ADR 0025) : amount null quand error est renseignée, jamais zéro.
export type LandedErrorCode =
  | 'no_destination'
  | 'no_customs_duty'
  | 'no_freight_rate'
  | 'no_packaging'
  | 'no_mass'
  | 'no_quote'
  | 'no_rate'
  | 'other'
export type LandedComponent = { amount: Money | null; error: string | null; error_code: LandedErrorCode | null }
export type OfferLanded = {
  offer_id: number
  origin_country: string
  destination_country: string | null
  purchase: LandedComponent
  customs: LandedComponent
  freight: LandedComponent
  packaging: LandedComponent
  total: Money | null
  error: string | null
  duty_percent: string | null
  freight_per_kg: Money | null
  packaging_per_unit: Money | null
  rates: Rate[]
  logistics_tables: 'live'
}
export type LandedComparison = { part_id: number; currency: string; price_snapshot_id: number | null; offers: OfferLanded[] }

// Make or buy (ADR 0026) : simulation sans stockage ; difference = fabriquer − acheter par voiture.
export type CostLine = LandedComponent & { label: string }
export type MakeOrBuyResult = {
  node_id: number
  real_quantity: string
  volume: number
  currency: string
  price_snapshot_id: number | null
  make: CostLine[]
  buy: CostLine[]
  fixed_per_car: Money | null
  make_per_car: Money | null
  buy_per_car: Money | null
  difference: Money | null
  break_even_volume: number | null
  never_profitable: boolean
  rates: Rate[]
  error: string | null
}
export type MakeOrBuyRequest = {
  fixed_costs: Money
  logistics_per_unit: Money
  tier1_price: Money
  tier1_origin: string
  tier1_packaging: Money
}

export type Schedule = { start_on: string; target_on: string | null } // dates "AAAA-MM-JJ"

// Planning (ADR 0023) : available_on null sans date de départ ; target_met et allowed_days null sans cible.
export type Planning = {
  schedule: Schedule | null
  lead_time_days: number
  available_on: string | null
  critical: number[]
  allowed_days: number | null
  target_met: boolean | null
  late: { node_id: number; chain_days: number; overrun_days: number }[]
}

export type TreeNode = {
  id: number
  parent_id: number | null
  name: string
  part_id: number | null
  offer_id: number | null
  quantity: string
  production_days: number
  strategy: Strategy | null // stratégie attachée à ce nœud (ADR 0028) ; null = héritée ou aucune
  totals: Totals | null
}

export type Strategy = 'cost' | 'lead_time' | 'co2' | 'balanced'

export type Candidate = {
  part_id: number
  offer_id: number
  current: boolean
  cost: Money | null
  lead_time_days: number | null // null : attribut de la pièce manquant
  co2_kg: string | null
  quote: Quote | null // cotation et taux utilisés pour le coût (explicabilité)
  rate: Rate | null
  score: string | null // 0 = meilleur, 1 = pire, sur les candidates comparées
  cost_score: string | null
  lead_time_score: string | null
  co2_score: string | null
  excluded: string | null // raison (prix, taux ou attribut manquant)
}

export type Suggestions = {
  node_id: number
  strategy: Strategy | null
  strategy_from: number | null
  real_quantity: string
  currency: string
  price_snapshot_id: number | null
  candidates: Candidate[]
  excluded: Candidate[]
}

export type Explanation = { currency: string; price_snapshot_id: number | null; rates: Rate[] }

// En-tête d'un snapshot de prix (GET /api/v1/price-snapshots) : périmètre et instant figé.
export type PriceSnapshotRef = {
  id: number
  scope: 'offer' | 'supplier' | 'bom' | 'catalog'
  scope_id?: number // absent pour le catalogue entier
  taken_at: string
}

export type Tree = { bom: BOM; nodes: TreeNode[]; explanation: Explanation; stock: StockLine[]; planning: Planning }

export type Snapshot = {
  id: number
  bom_id: number
  price_snapshot_id: number
  name: string
  currency: string
  volume: number
  annual_hours: string | null // null : snapshot publié avant ADR 0024
  destination_country: string | null
  published_at: string
}

export type Published = {
  snapshot: Snapshot
  nodes: TreeNode[]
  explanation: Explanation
  stock: StockLine[]
  planning: Planning
}

export type NewNode =
  | { parent_id: number; name: string; quantity: string; production_days: number }
  | { parent_id: number; part_id: number; offer_id: number; quantity: string }

export type NodePatch =
  | { offer_id: number }
  | { quantity: string }
  | { parent_id: number }
  | { production_days: number }
  | { strategy: Strategy | null }
  | { part_id: number; offer_id: number }

export class ApiError extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`
    try {
      const data = (await res.json()) as { error?: string }
      if (data.error) msg = data.error
    } catch {
      // corps non JSON : on garde le statut HTTP
    }
    throw new ApiError(res.status, msg)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  listBOMs: () => call<BOM[]>('GET', '/boms'),
  createBOM: (b: { name: string; currency: string; volume: number }) => call<BOM>('POST', '/boms', b),
  renameBOM: (id: number, name: string) => call<BOM>('PATCH', `/boms/${id}`, { name }),
  updateSettings: (id: number, currency: string, volume: number) =>
    call<BOM>('PATCH', `/boms/${id}`, { currency, volume }),
  setAnnualHours: (id: number, annualHours: string) => call<BOM>('PATCH', `/boms/${id}`, { annual_hours: annualHours }),
  setPartStock: (bomID: number, partID: number, quantity: string) =>
    call<StockLine>('PUT', `/boms/${bomID}/stock/${partID}`, { quantity }),
  alternatives: (partID: number) => call<{ id: number; name: string }[]>('GET', `/parts/${partID}/alternatives`),
  setSchedule: (bomID: number, s: Schedule) => call<Schedule>('PUT', `/boms/${bomID}/schedule`, s),
  clearSchedule: (bomID: number) => call<void>('DELETE', `/boms/${bomID}/schedule`),
  suggestions: (bomID: number, nodeID: number) => call<Suggestions>('GET', `/boms/${bomID}/nodes/${nodeID}/suggestions`),
  anomalies: (bomID: number, threshold: string) =>
    call<CostAnomalies>('GET', `/boms/${bomID}/anomalies?threshold=${encodeURIComponent(threshold)}`),
  setDestination: (id: number, country: string | null) => call<BOM>('PATCH', `/boms/${id}`, { destination_country: country }),
  landedCost: (bomID: number, partID: number, offerIDs: number[]) =>
    call<LandedComparison>('GET', `/parts/${partID}/landed-cost?bom_id=${bomID}&offer_ids=${offerIDs.join(',')}`),
  setCustomsDuty: (origin: string, destination: string, dutyPercent: string) =>
    call<unknown>('PUT', `/customs-duties/${origin}/${destination}`, { duty_percent: dutyPercent }),
  setFreightRate: (origin: string, destination: string, perKg: Money) =>
    call<unknown>('PUT', `/freight-rates/${origin}/${destination}`, { per_kg: perKg }),
  setPackaging: (offerID: number, packaging: Money) => call<unknown>('PUT', `/offers/${offerID}/packaging`, { packaging }),
  makeOrBuy: (bomID: number, nodeID: number, req: MakeOrBuyRequest) =>
    call<MakeOrBuyResult>('POST', `/boms/${bomID}/nodes/${nodeID}/make-or-buy`, req),
  deleteBOM: (id: number) => call<void>('DELETE', `/boms/${id}`),
  tree: (id: number) => call<Tree>('GET', `/boms/${id}/tree`),
  addNode: (bomID: number, n: NewNode) => call<TreeNode>('POST', `/boms/${bomID}/nodes`, n),
  // Un seul champ par PATCH (contrat T8) : le type NodePatch l'impose.
  patchNode: (bomID: number, nodeID: number, p: NodePatch) =>
    call<TreeNode>('PATCH', `/boms/${bomID}/nodes/${nodeID}`, p),
  deleteNode: (bomID: number, nodeID: number) => call<void>('DELETE', `/boms/${bomID}/nodes/${nodeID}`),
  freezePrices: (id: number) => call<BOM>('POST', `/boms/${id}/freeze-prices`),
  // Attache un snapshot de prix existant ; 409 avec les offres manquantes s'il ne couvre pas la BOM.
  usePriceSnapshot: (id: number, snapshotID: number) =>
    call<BOM>('PUT', `/boms/${id}/freeze-prices`, { price_snapshot_id: snapshotID }),
  listPriceSnapshots: () => call<PriceSnapshotRef[]>('GET', '/price-snapshots'),
  unfreezePrices: (id: number) => call<BOM>('DELETE', `/boms/${id}/freeze-prices`),
  publish: (id: number) => call<Snapshot>('POST', `/boms/${id}/publish`),
  snapshot: (id: number) => call<Published>('GET', `/snapshots/${id}`),
  newDraft: (snapshotID: number) => call<BOM>('POST', `/snapshots/${snapshotID}/drafts`),
}

export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}
