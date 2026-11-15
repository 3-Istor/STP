import type { Explanation, Totals as NodeTotals } from '../api.ts'
import { formatDate, formatDecimal, formatMoney } from '../money.ts'

// Totals : coût, CO2 et délai de la racine, avec la source des prix en une ligne (détail au survol).
export function Totals({ totals, explanation }: { totals: NodeTotals; explanation: Explanation }) {
  const source =
    explanation.price_snapshot_id === null ? 'prix en direct' : `prix figés (snapshot de prix #${explanation.price_snapshot_id})`
  const rates = explanation.rates.map((r) => `1 ${r.from} = ${r.rate} ${r.to} (${r.source}, ${formatDate(r.quoted_at)})`).join('\n')
  return (
    <div className="totals" title={rates || undefined}>
      <div>
        <span className="muted small">Coût / véhicule</span>
        <strong className={totals.cost_error_code ? 'over' : undefined}>
          {totals.cost ? formatMoney(totals.cost) : 'incomplet'}
        </strong>
      </div>
      <div>
        <span className="muted small">CO2</span>
        <strong>{formatDecimal(totals.co2_kg, 1)} kg</strong>
      </div>
      <div>
        <span className="muted small">Délai</span>
        <strong>{totals.lead_time_days} j</strong>
      </div>
      <div title={totals.labor_error ?? (totals.labor_hours ? `${formatDecimal(totals.labor_hours, 2)} h par véhicule` : undefined)}>
        <span className="muted small">ETP</span>
        <strong className={totals.labor_error ? 'over' : undefined}>{totals.fte ? formatDecimal(totals.fte, 2) : 'n/d'}</strong>
      </div>
      <p className="muted small">
        {explanation.currency}, {source}
        {totals.missing_cost.length > 0 && <span className="over"> · {totals.missing_cost.length} nœud(s) sans prix</span>}
      </p>
    </div>
  )
}
