// Client de l'API /api/v1 (contrat : internal/httpapi/boms_json.go). Champs en snake_case comme l'API ;
// montants, quantités, CO2, masses et taux en chaînes décimales, jamais en `number`.
import type { Money } from './money.ts'

export type BOM = {
  id: number
  name: string
  currency: string
  volume: number
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
}

export type TreeNode = {
  id: number
  parent_id: number | null
  name: string
  part_id: number | null
  offer_id: number | null
  quantity: string
  production_days: number
  totals: Totals | null
}

export type Explanation = { currency: string; price_snapshot_id: number | null; rates: Rate[] }

export type Tree = { bom: BOM; nodes: TreeNode[]; explanation: Explanation }

export type Snapshot = {
  id: number
  bom_id: number
  price_snapshot_id: number
  name: string
  currency: string
  volume: number
  published_at: string
}

export type Published = { snapshot: Snapshot; nodes: TreeNode[]; explanation: Explanation }

export type NewNode =
  | { parent_id: number; name: string; quantity: string; production_days: number }
  | { parent_id: number; part_id: number; offer_id: number; quantity: string }

export type NodePatch =
  | { offer_id: number }
  | { quantity: string }
  | { parent_id: number }
  | { production_days: number }

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
  deleteBOM: (id: number) => call<void>('DELETE', `/boms/${id}`),
  tree: (id: number) => call<Tree>('GET', `/boms/${id}/tree`),
  addNode: (bomID: number, n: NewNode) => call<TreeNode>('POST', `/boms/${bomID}/nodes`, n),
  // Un seul champ par PATCH (contrat T8) : le type NodePatch l'impose.
  patchNode: (bomID: number, nodeID: number, p: NodePatch) =>
    call<TreeNode>('PATCH', `/boms/${bomID}/nodes/${nodeID}`, p),
  deleteNode: (bomID: number, nodeID: number) => call<void>('DELETE', `/boms/${bomID}/nodes/${nodeID}`),
  freezePrices: (id: number) => call<BOM>('POST', `/boms/${id}/freeze-prices`),
  unfreezePrices: (id: number) => call<BOM>('DELETE', `/boms/${id}/freeze-prices`),
  publish: (id: number) => call<Snapshot>('POST', `/boms/${id}/publish`),
  snapshot: (id: number) => call<Published>('GET', `/snapshots/${id}`),
  newDraft: (snapshotID: number) => call<BOM>('POST', `/snapshots/${snapshotID}/drafts`),
}

export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}
