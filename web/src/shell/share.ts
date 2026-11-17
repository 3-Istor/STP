// shareWidths : largeur des barres fines de la Synthèse, en % entier de la largeur, pour chaque part du
// total. Géométrie d'affichage seulement : les montants restent des chaînes décimales, la division se fait
// en entiers exacts (BigInt), comme le graphe de prix (suppliers/graph.ts). Aucun pourcentage n'est affiché.
// null : barre absente (part ou total manquant, devises différentes, total nul ou négatif).
import type { Money } from '../money.ts'

function scaled(amount: string, scale: number): bigint | null {
  const m = /^(-?)(\d+)(?:\.(\d+))?$/.exec(amount)
  if (!m) return null
  const frac = (m[3] ?? '').padEnd(scale, '0').slice(0, scale)
  return BigInt(`${m[1]}${m[2]}${frac}`)
}

export function shareWidths(total: Money | null, parts: (Money | null)[]): (number | null)[] {
  if (!total) return parts.map(() => null)
  const scale = Math.max(...[total, ...parts].map((p) => (p ? (p.amount.split('.')[1] ?? '').length : 0)))
  const t = scaled(total.amount, scale)
  if (t === null || t <= 0n) return parts.map(() => null)
  return parts.map((p) => {
    if (!p || p.currency !== total.currency) return null
    const v = scaled(p.amount, scale)
    if (v === null || v < 0n) return null
    const pct = Number((v * 100n) / t)
    return Math.min(100, pct)
  })
}
