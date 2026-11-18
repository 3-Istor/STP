import type { TreeNode } from '../api.ts'

// Causes d'un coût absent, en mots courts (champ cost_error_code d'une pièce). « missing_cost » n'est pas une
// cause : c'est un descendant qui en a une (internal/httpapi/boms_json.go, costErrorCode).
const causeLabels: Record<string, string> = {
  no_quote: 'prix manquant',
  no_rate: 'taux de change manquant',
  no_budget_rate: 'taux budget absent',
  no_tooling: 'outillage non renseigné',
  no_program_volume: 'volume du programme non renseigné',
  no_cost_date: 'date de calcul absente',
  no_cost_facts: 'données de coût indisponibles',
  contract_selection_required: 'contrat à choisir',
  no_destination: 'destination non renseignée',
  no_customs_duty: 'droit de douane absent',
  no_freight_rate: 'tarif de fret absent',
  no_packaging: 'emballage non renseigné',
}

export type CostCause = { code: string; label: string; count: number }

export function causeLabel(code: string | null | undefined): string {
  return (code && causeLabels[code]) || 'coût indisponible'
}

// dominantCause : la cause la plus fréquente parmi les pièces sans coût `ids` (égalité : ordre alphabétique du
// code, donc stable). null quand aucune pièce n'a de cause propre connue.
export function dominantCause(ids: readonly number[], byID: ReadonlyMap<number, TreeNode>): CostCause | null {
  const counts = new Map<string, number>()
  for (const id of ids) {
    const code = byID.get(id)?.totals?.cost_error_code
    if (code && code !== 'missing_cost') counts.set(code, (counts.get(code) ?? 0) + 1)
  }
  let best: CostCause | null = null
  for (const [code, count] of [...counts].sort(([a], [b]) => a.localeCompare(b))) {
    if (!best || count > best.count) best = { code, label: causeLabel(code), count }
  }
  return best
}

export function missingSummary(ids: readonly number[], cause: CostCause | null): string {
  const n = ids.length
  const base = `${n} ${n > 1 ? 'pièces sans coût' : 'pièce sans coût'}`
  return cause ? `${base} · cause principale : ${cause.label}${cause.count < n ? ` (${cause.count})` : ''}` : base
}
