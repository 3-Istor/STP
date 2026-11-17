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
  mass_score?: string | null
  quality_score?: string | null
  excluded: string | null // raison (prix, taux ou attribut manquant)
}

export type Suggestions = {
  // profile : profil de véhicule qui classe quand aucune stratégie de nœud ne s'applique (ADR 0036) ;
  // weights : poids appliqués (stratégie, profil, ou null).
  profile?: VehicleProfile | null
  weights?: Weights | null
  node_id: number
  strategy: Strategy | null
  strategy_from: number | null
  real_quantity: string
  currency: string
  price_snapshot_id: number | null
  candidates: Candidate[]
  excluded: Candidate[]
}

// Méthode de calcul du coût (ADR 0031) : base (achat, rendu, complet) × change (taux budget, dernier taux).
// program_volume : volume programme pour amortir l'outillage, exigé par la base « full ».
export type CostBasis = 'purchase' | 'landed' | 'full'
export type FXPolicy = 'budget' | 'latest'
export type CostMethod = { basis: CostBasis; fx: FXPolicy; program_volume?: number | null }

// cost_method et cost_as_of (date de calcul : contrats, année du taux budget) sont absents d'un snapshot
// publié avant ADR 0031 ; budget_rates : taux budget utilisés.
export type Explanation = {
  currency: string
  price_snapshot_id: number | null
  rates: Rate[]
  cost_method?: CostMethod
  cost_as_of?: string | null
  budget_rates?: Rate[]
}

// Budget cible d'un nœud comparé à son coût (ADR 0018, 0031). État et pourcentage sont calculés par l'API :
// ok (vert), warn (orange, coût ≥ 90 % du budget), over (rouge, coût > budget), unavailable (coût ou
// conversion manquant, jamais vert). percent : coût ÷ budget × 100, 1 décimale, null si indisponible.
export type BudgetState = 'ok' | 'warn' | 'over' | 'unavailable'
export type NodeBudget = {
  node_id: number
  target: Money
  cost: Money | null
  delta: Money | null
  percent: string | null
  state: BudgetState
  error: string | null
  // ADR 0044 : budget dans une autre devise que la BOM, converti au taux du jour (taux utilisé) ; null sinon.
  converted?: Money | null
  rate?: Rate | null
  // Les sous-budgets dépassent ce budget au taux courant (change) : signalé, jamais bloquant ; null sinon.
  allocation?: string | null
}

// Ventilation du coût d'un nœud (« Pourquoi ce coût ? », ADR 0031) : lignes additives dans la devise de la
// BOM, Σ amount = total. Un sous-ensemble agrège ses descendants par nature (as_of null).
export type CostLineKind = 'price' | 'fx' | 'discount' | 'customs' | 'freight' | 'packaging' | 'tooling'
export type CostBreakdown = {
  node_id: number
  currency: string
  method: CostMethod
  cost_as_of: string | null
  lines: { kind: CostLineKind; label: string; amount: Money; source: string; as_of: string | null }[]
  total: Money
}

// État de la source de prix (GET /api/v1/pricing/status).
export type PricingStatus = { dataset_version?: string; tick?: number; as_of?: string; status: string }

// En-tête d'un snapshot de prix (GET /api/v1/price-snapshots) : périmètre et instant figé.
export type PriceSnapshotRef = {
  id: number
  scope: 'offer' | 'supplier' | 'bom' | 'catalog'
  scope_id?: number // absent pour le catalogue entier
  taken_at: string
}

// budgets : budgets cibles évalués sur les totaux de cette même réponse (absent d'une API antérieure au lot F).
export type Tree = { bom: BOM; nodes: TreeNode[]; explanation: Explanation; stock: StockLine[]; planning: Planning; budgets?: NodeBudget[] }

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
  // Trace de la publication (ADR 0033) ; null si non saisie ou publiée avant.
  note?: string | null
  proposed_by?: string | null
  validated_by?: string | null
}

// Contrôle figé à la publication, avec la justification de chaque avertissement (ADR 0033).
export type FrozenCheck = { code: string; level: CheckLevel; title: string; detail: string; justification: string | null }

export type Published = {
  snapshot: Snapshot
  nodes: TreeNode[]
  explanation: Explanation
  stock: StockLine[]
  planning: Planning
  checks?: FrozenCheck[]
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

// --- Versions, comparaison, publication tracée, pistes (lot B, ADR 0032, 0033) -------------------------

export type VersionRef = string // « bom:3 » ou « snapshot:1 », à passer à /compare
export type Version = {
  kind: 'draft' | 'snapshot'
  id: number
  bom_id: number
  ref: VersionRef
  label: string // « V1 », « Brouillon B »
  name: string
  at: string // création (brouillon) ou publication (snapshot), à la seconde
  source_snapshot_id: number | null
  method: CostMethod
  cost: Money | null
  cost_error: string | null
  target_budget: Money | null
  budget_percent: string | null
  price_snapshot_id: number | null
  proposed_by: string | null
  validated_by: string | null
  note: string | null
  part_count: number
  node_count: number
}

export type VersionTotals = { cost: Money; co2_kg: string; mass_kg: string; lead_time_days: number }
export type CompareSide = { ref: VersionRef; label: string; name: string; method: CostMethod; price_snapshot_id: number | null; totals: VersionTotals }
export type CauseKind = 'structure' | 'offer_change' | 'price' | 'fx' | 'discount' | 'logistics' | 'tooling' | 'method'
// Σ amount des causes = delta.cost exactement (ADR 0032) ; node_ids : 20 plus gros écarts (côté « vers »),
// from_node_ids : nœuds du côté « de » (pièces retirées) ; node_count : total.
export type Cause = { kind: CauseKind; amount: Money; explanation: string; node_ids: number[]; from_node_ids: number[]; node_count: number }
export type ChangedPart = {
  change: string // added | removed | offer | quantity…
  path: string
  node_id: number | null
  from_node_id: number | null
  part_id: number
  before: string
  after: string
  delta: Money
}
export type Comparison = { from: CompareSide; to: CompareSide; delta: VersionTotals; causes: Cause[]; changed_parts: ChangedPart[] }

export type CheckLevel = 'ok' | 'warn' | 'block'
export type PublicationCheck = { code: string; level: CheckLevel; title: string; detail: string; node_ids: number[] }
export type PublishRequest = { note: string; proposed_by: string; validated_by: string; justifications: Record<string, string> }

export type IdeaStatus = 'idea' | 'in_progress' | 'to_validate' | 'done' | 'dropped'
export type Idea = { id: number; title: string; owner: string; estimated_gain: Money; status: IdeaStatus; draft_bom_id: number | null; created_at: string }
// open_total : pistes ouvertes (idée, en cours, à valider) ; gap_to_target = coût − budget global ;
// remaining = écart − pistes ouvertes. Signés : négatif = couvert (budget tenu).
export type IdeasSummary = { ideas: Idea[]; open_total: Money; gap_to_target: Money | null; remaining: Money | null; gap_error: string | null }
export type NewIdea = { title: string; owner: string; estimated_gain: Money; status: IdeaStatus; draft_bom_id: number | null }
export type IdeaPatch = Partial<Pick<Idea, 'title' | 'owner' | 'estimated_gain' | 'status'>>

// --- Analyses et profils (lot E, ADR 0036, 0037) --------------------------------------------------------

export type FxSensitivity = {
  currency: string
  share_percent: string // exposition ÷ total, 2 décimales
  impact: Money
  budget_percent_after: string | null
  shock_percent: string
  exposed: Money
  cost_before: Money
  cost_after: Money
  budget: Money | null
  budget_error: string | null
  rates: Rate[]
  cost_method: CostMethod
  price_snapshot_id: number | null
}

export type PriceDriver = {
  part_id: number
  name: string
  amplitude: Money | null
  rank: number | null // null : non évaluable (error)
  error: string | null
  offers: { offer_id: number; quantity: string; min: Money | null; max: Money | null; quotes: number; amplitude: Money | null; error: string | null }[]
  window_start: string
  window_end: string
}

export type CostShare = { key: string; cost: Money; share_percent: string; parts: number }
export type Sourcing = {
  currency: string
  as_of: string
  total: Money
  by_country: CostShare[]
  by_currency: CostShare[]
  supplier_count: number
  top5_share_percent: string
  suppliers: {
    supplier_id: number
    name: string
    countries: string[]
    currencies: string[]
    cost: Money
    share_percent: string
    parts: number
    contract: { status: 'active' | 'expired' | 'not_started' | 'none'; contract_id: number | null; ends_on: string | null }
  }[]
  mono_source_count: number
  mono_source: { part_id: number; name: string; active_offers: number; supplier_ids: number[]; cost: Money; share_percent: string }[]
  expired_contracts: {
    contract_id: number
    supplier_id: number
    supplier_name: string
    ends_on: string
    lost_amount: Money | null
    parts: { part_id: number; name: string; offer_id: number; ordered_quantity: string; discount_percent: string; tier_min: string | null; amount: Money | null; error: string | null }[]
  }[]
  rates: { currency: string; budget_rate: Rate | null; latest_rate: Rate | null; gap_percent: string | null; effect: Money | null; error: string | null }[]
  cost_method: CostMethod
  price_snapshot_id: number | null
}

export type PartAnomaly = {
  part_id: number
  name: string
  category: string
  offer_id: number
  node_ids: number[]
  quantity: string
  status: 'anomaly' | 'normal' | 'not_evaluable'
  unit_cost: Money | null
  per_kg: Money | null
  median_per_kg: Money | null
  reference_cost: Money | null
  ratio: string | null
  excess: Money | null
  impact: Money | null // surcoût estimé par véhicule
  peers: number
  missing: number
  reason: string | null
}
export type PartAnomalies = { method: string; threshold: string; currency: string; price_snapshot_id: number | null; rates: Rate[]; anomaly_count: number; parts: PartAnomaly[] }

export type Weights = { cost: number; co2: number; lead_time: number; mass: number; quality: number }
export type VehicleProfile = { preset: string | null; weights: Weights | null }
export type ProfilePreset = { preset: string; label: string; hint: string; weights: Weights | null; custom: boolean }

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
  // Budget cible d'un nœud (ADR 0042) : refusé (409) si les sous-budgets d'un ensemble dépasseraient son budget.
  setNodeBudget: (bomID: number, nodeID: number, target: Money) => call<unknown>('PUT', `/boms/${bomID}/nodes/${nodeID}/budget`, target),
  deleteNodeBudget: (bomID: number, nodeID: number) => call<void>('DELETE', `/boms/${bomID}/nodes/${nodeID}/budget`),
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
  // Publication tracée (ADR 0033) : 409 si un contrôle bloque, 422 si un avertissement n'est pas justifié.
  publish: (id: number, body?: PublishRequest) => call<Snapshot>('POST', `/boms/${id}/publish`, body),
  snapshot: (id: number) => call<Published>('GET', `/snapshots/${id}`),
  newDraft: (snapshotID: number) => call<BOM>('POST', `/snapshots/${snapshotID}/drafts`),
  pricingStatus: () => call<PricingStatus>('GET', '/pricing/status'),
  // Méthode de calcul (brouillon seulement en écriture) ; « full » exige program_volume.
  setCostMethod: (bomID: number, m: CostMethod) => call<CostMethod>('PUT', `/boms/${bomID}/cost-method`, m),
  costBreakdown: (bomID: number, nodeID: number) => call<CostBreakdown>('GET', `/boms/${bomID}/nodes/${nodeID}/cost-breakdown`),
  snapshotCostBreakdown: (snapshotID: number, nodeID: number) =>
    call<CostBreakdown>('GET', `/snapshots/${snapshotID}/nodes/${nodeID}/cost-breakdown`),
  versions: (bomID: number) => call<Version[]>('GET', `/boms/${bomID}/versions`),
  compare: (from: VersionRef, to: VersionRef) =>
    call<Comparison>('GET', `/compare?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`),
  publicationChecks: (bomID: number) => call<PublicationCheck[]>('GET', `/boms/${bomID}/publication-checks`),
  ideas: (bomID: number) => call<IdeasSummary>('GET', `/boms/${bomID}/ideas`),
  createIdea: (bomID: number, idea: NewIdea) => call<Idea>('POST', `/boms/${bomID}/ideas`, idea),
  patchIdea: (bomID: number, ideaID: number, p: IdeaPatch) => call<Idea>('PATCH', `/boms/${bomID}/ideas/${ideaID}`, p),
  deleteIdea: (bomID: number, ideaID: number) => call<void>('DELETE', `/boms/${bomID}/ideas/${ideaID}`),
  fxSensitivity: (bomID: number, shock: string) =>
    call<FxSensitivity[]>('GET', `/boms/${bomID}/analysis/fx-sensitivity?shock_percent=${encodeURIComponent(shock)}`),
  priceDrivers: (bomID: number, limit = 10) => call<PriceDriver[]>('GET', `/boms/${bomID}/analysis/price-drivers?limit=${limit}`),
  sourcing: (bomID: number) => call<Sourcing>('GET', `/boms/${bomID}/analysis/sourcing`),
  partAnomalies: (bomID: number) => call<PartAnomalies>('GET', `/boms/${bomID}/analysis/anomalies`),
  profilePresets: () => call<ProfilePreset[]>('GET', '/vehicle-profiles'),
  profile: (bomID: number) => call<VehicleProfile>('GET', `/boms/${bomID}/profile`),
  snapshotProfile: (snapshotID: number) => call<VehicleProfile>('GET', `/snapshots/${snapshotID}/profile`),
  setProfile: (bomID: number, p: { preset: string; weights: Weights }) => call<VehicleProfile>('PUT', `/boms/${bomID}/profile`, p),
}

export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}
