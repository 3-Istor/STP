// Affichage des budgets cibles (ADR 0018, 0031). État et pourcentage viennent de l'API : ici, seulement leur
// mise en forme (texte, classe de couleur --ok / --warn / --over, largeur de barre). Aucun calcul de budget.
import type { BudgetState, NodeBudget } from '../api.ts'
import { formatDecimal, formatMoney } from '../money.ts'

export const budgetStateLabel: Record<BudgetState, string> = {
  ok: 'dans le budget',
  warn: 'proche du budget (≥ 90 %)',
  over: 'budget dépassé',
  unavailable: 'comparaison indisponible',
}

/** Classe de couleur du texte : vert, orange, rouge ; rien pour une comparaison indisponible. */
export function budgetClass(b: NodeBudget | undefined): string {
  if (!b || b.state === 'unavailable') return ''
  return `t-${b.state}`
}

/** « 97,2 % », « n/d » si la comparaison est indisponible. */
export function budgetPercentText(b: NodeBudget): string {
  return b.percent !== null ? `${formatDecimal(b.percent, 1)} %` : 'n/d'
}

/** Infobulle : budget, coût comparé, écart, état ; l'erreur de l'API quand la comparaison est indisponible. */
export function budgetTitle(b: NodeBudget): string {
  const parts = [`Budget ${formatMoney(b.target)}`]
  if (b.cost) parts.push(`coût ${formatMoney(b.cost)}`)
  if (b.delta) parts.push(`écart ${b.delta.amount.startsWith('-') ? '' : '+'}${formatMoney(b.delta)}`)
  parts.push(budgetStateLabel[b.state])
  if (b.error) parts.push(b.error)
  return parts.join(' · ')
}

/**
 * Largeur d'une barre « % du budget », en % entier de la piste (plafonnée à 100). Géométrie d'affichage
 * seulement : on lit la partie entière de la chaîne décimale, sans arithmétique sur le montant.
 */
export function budgetBarWidth(percent: string | null): number | null {
  if (percent === null) return null
  const m = /^(\d+)(?:\.\d+)?$/.exec(percent)
  if (!m) return null
  return Math.min(100, Number.parseInt(m[1], 10))
}
