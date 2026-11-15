import type { NodeAnomaly } from '../api.ts'
import { formatDecimal, formatMoney } from '../money.ts'

// describeAnomaly : « 200,00 € l'unité, 4× la médiane de « Freins » (50,00 €, 5 pièces, 1 sans prix) ».
export function describeAnomaly(a: NodeAnomaly): string {
  const missing = a.missing > 0 ? `, ${a.missing} sans prix` : ''
  return `${formatMoney(a.unit_cost)} l'unité, ${a.ratio ? formatDecimal(a.ratio, 2) : 'n/d'}× la médiane de « ${a.category} » (${formatMoney(a.median)}, ${a.peers} pièces${missing})`
}
