// Libellés de la méthode de calcul du coût (ADR 0031). Aucune règle de calcul ici : la méthode est appliquée
// par l'API, le front ne fait que la nommer.
import type { CostBasis, CostMethod, FXPolicy } from '../api.ts'
import { formatDecimal } from '../money.ts'

export const basisLabels: Record<CostBasis, string> = { purchase: 'Achat', landed: 'Rendu', full: 'Complet' }
export const fxLabels: Record<FXPolicy, string> = { budget: 'Taux budget', latest: 'Dernier taux' }
const basisLong: Record<CostBasis, string> = { purchase: 'Coût d’achat', landed: 'Coût rendu', full: 'Coût complet' }
const basisHelp: Record<CostBasis, string> = {
  purchase: 'cotation × quantité, au change choisi',
  landed: 'achat − remise de contrat + douane + transport + emballage',
  full: 'rendu + outillage amorti sur le volume programme',
}

/** « Coût complet par véhicule » ; « Coût par véhicule » si l'API ne donne pas la méthode. */
export function costTitle(m: CostMethod | undefined): string {
  return m ? `${basisLong[m.basis] ?? m.basis} par véhicule` : 'Coût par véhicule'
}

export function basisHint(b: CostBasis): string {
  return basisHelp[b] ?? ''
}

/** « Coût complet · taux budget · volume programme 60 000 ». */
export function methodLabel(m: CostMethod): string {
  const basis = basisLong[m.basis] ?? m.basis
  const fx = (fxLabels[m.fx] ?? m.fx).toLowerCase()
  const vol = m.program_volume ? ` · volume programme ${formatDecimal(String(m.program_volume), 0)}` : ''
  return `${basis} · ${fx}${vol}`
}
