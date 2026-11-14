import type { Strategy } from '../api.ts'

// Libellés des stratégies de nœud (ADR 0028).
export const strategyLabels: Record<Strategy, string> = {
  cost: 'coût minimum',
  lead_time: 'délai minimum',
  co2: 'CO2 minimum',
  balanced: 'équilibré',
}
