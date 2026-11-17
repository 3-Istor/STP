import { useState } from 'react'
import { api } from '../../api.ts'
import { decimalSign, formatDecimal, formatMoney, formatPercent, formatSignedMoney } from '../../money.ts'
import { shareWidths } from '../../shell/share.ts'
import { useLoad } from '../useLoad.ts'
import { Show } from './Loading.tsx'

// Analyses de l'étape Chiffrer (maquette I4-4, lot E, ADR 0037). Tout est calculé par l'API sur le
// brouillon (méthode de calcul de la BOM) : le front met en forme, rien de plus.

const SHOCKS = ['5', '10'] as const

export function FxSensitivityCard({ bomID, version }: { bomID: number; version: number }) {
  const [shock, setShock] = useState<(typeof SHOCKS)[number]>('5')
  const value = useLoad(`${bomID}-${version}-${shock}`, () => api.fxSensitivity(bomID, shock))
  return (
    <section className="block" aria-label="Sensibilité au change">
      <h3>
        Sensibilité au change · ce que coûte un choc, par véhicule
        <span className="sp" />
        <span className="seg" role="radiogroup" aria-label="Choc de change">
          {SHOCKS.map((s) => (
            <button key={s} type="button" role="radio" aria-checked={shock === s} className={shock === s ? 'on' : ''} onClick={() => setShock(s)}>
              +{s} %
            </button>
          ))}
        </span>
      </h3>
      <Show value={value} label="Sensibilité au change">
        {(rows) =>
          rows.length === 0 ? (
            <p className="in muted">Aucune devise étrangère dans ce brouillon : pas d’exposition au change.</p>
          ) : (
            <div className="scroll">
              <table>
                <thead>
                  <tr>
                    <th>Hypothèse</th>
                    <th className="n">Part du coût exposée</th>
                    <th className="n">Impact par véhicule</th>
                    <th className="n">Coût après choc</th>
                    <th className="n">Budget après choc</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr key={r.currency} title={r.rates.map((x) => `1 ${x.from} = ${x.rate} ${x.to} (${x.source})`).join('\n')}>
                      <td>
                        {r.currency} +{formatDecimal(r.shock_percent, 2)} % contre {r.cost_before.currency}
                      </td>
                      <td className="n">{formatPercent(r.share_percent, 1)}</td>
                      <td className={`n ${decimalSign(r.impact.amount) > 0 ? 't-over' : decimalSign(r.impact.amount) < 0 ? 't-ok' : ''}`}>{formatSignedMoney(r.impact)}</td>
                      <td className="n">{formatMoney(r.cost_after)}</td>
                      <td className="n" title={r.budget_error ?? (r.budget ? `budget ${formatMoney(r.budget)}` : undefined)}>
                        {r.budget_percent_after !== null ? formatPercent(r.budget_percent_after, 1) : <span className="muted">{r.budget_error ? 'n/d' : 'pas de budget'}</span>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <p className="in muted small">
                Rechiffrage par le moteur avec le dernier taux et le taux budget de la devise relevés du choc ; part exposée = ce qui est converti depuis cette devise.
              </p>
            </div>
          )
        }
      </Show>
    </section>
  )
}

export function PriceDriversCard({ bomID, version }: { bomID: number; version: number }) {
  const value = useLoad(`${bomID}-${version}`, () => api.priceDrivers(bomID, 8))
  return (
    <section className="block" aria-label="Pièces dont le prix pèse le plus">
      <h3>Pièces dont le prix pèse le plus · amplitude des cotations × quantité</h3>
      <Show value={value} label="Pièces influentes">
        {(rows) => {
          const ranked = rows.filter((r) => r.rank !== null && r.amplitude)
          const others = rows.filter((r) => r.rank === null)
          // Barres : part de la plus grande amplitude (géométrie, entiers exacts) ; l'API trie déjà.
          const widths = shareWidths(ranked[0]?.amplitude ?? null, ranked.map((r) => r.amplitude))
          return (
            <div className="in">
              {ranked.length === 0 && <p className="muted">Aucune pièce évaluable.</p>}
              <ul className="bars">
                {ranked.map((r, i) => (
                  <li key={r.part_id} title={`fenêtre ${r.window_start.slice(0, 10)} → ${r.window_end.slice(0, 10)} · ${r.offers.map((o) => `offre #${o.offer_id} : ${o.quotes} cotations, ${o.min ? formatMoney(o.min) : 'n/d'} → ${o.max ? formatMoney(o.max) : 'n/d'} × ${o.quantity}`).join(' ; ')}`}>
                    <span className="bar-name">{r.name}</span>
                    <span className="bar" aria-hidden="true">
                      {widths[i] !== null && <i style={{ width: `${widths[i]}%` }} />}
                    </span>
                    <span className="n">{formatMoney(r.amplitude)}</span>
                  </li>
                ))}
              </ul>
              {others.length > 0 && (
                <ul className="explain">
                  {others.map((r) => (
                    <li key={r.part_id} className="muted">
                      {r.name} : non évaluable ({r.error})
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )
        }}
      </Show>
    </section>
  )
}

const SHOWN_ANOMALIES = 6

export function PartAnomaliesCard({ bomID, version, onSelect }: { bomID: number; version: number; onSelect: (id: number) => void }) {
  const value = useLoad(`${bomID}-${version}`, () => api.partAnomalies(bomID))
  const [all, setAll] = useState(false)
  return (
    <section className="block" aria-label="Anomalies en euros par kilo">
      <h3>Anomalies · comparaison en €/kg par famille</h3>
      <Show value={value} label="Anomalies €/kg">
        {(a) => {
          const anomalies = a.parts.filter((p) => p.status === 'anomaly')
          const rows = all ? anomalies : anomalies.slice(0, SHOWN_ANOMALIES)
          return (
            <div className="scroll">
              <p className="in muted small">
                {formatDecimal(String(a.anomaly_count), 0)} pièce(s) au-delà de {formatDecimal(a.threshold, 2)} × la médiane €/kg de leur famille, classées par surcoût estimé par véhicule.
              </p>
              {rows.length > 0 && (
                <table>
                  <thead>
                    <tr>
                      <th>Pièce</th>
                      <th>Famille</th>
                      <th className="n">€/kg</th>
                      <th className="n">Médiane famille</th>
                      <th className="n">Ratio</th>
                      <th className="n">Surcoût estimé</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((p) => (
                      <tr key={`${p.part_id}-${p.offer_id}`} className="clickable" onClick={() => p.node_ids[0] !== undefined && onSelect(p.node_ids[0])} title={`${p.peers} pièces comparées · ${formatDecimal(p.quantity)} u. par véhicule`}>
                        <td>{p.name}</td>
                        <td>{p.category}</td>
                        <td className="n">{p.per_kg ? formatDecimal(p.per_kg.amount, 1) : 'n/d'}</td>
                        <td className="n">{p.median_per_kg ? formatDecimal(p.median_per_kg.amount, 1) : 'n/d'}</td>
                        <td className="n">{p.ratio ? `×${formatDecimal(p.ratio, 2)}` : 'n/d'}</td>
                        <td className="n t-over">{p.impact ? formatSignedMoney(p.impact) : 'n/d'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              {anomalies.length > SHOWN_ANOMALIES && (
                <p className="in">
                  <button type="button" className="link small" onClick={() => setAll((v) => !v)}>
                    {all ? 'Moins d’anomalies' : `Toutes les anomalies (${anomalies.length})…`}
                  </button>
                </p>
              )}
            </div>
          )
        }}
      </Show>
    </section>
  )
}
