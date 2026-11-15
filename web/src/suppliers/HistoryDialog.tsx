import { useEffect, useRef, useState } from 'react'
import { formatMoney, formatRate, formatUTC, formatVariation } from './format'
import { DISPLAY_CURRENCY, displayStats } from './display'
import { monthLabel, monthlyChart, monthlySeries, monthTitle } from './graph'
import type { FXQuote, OfferView, Quote } from './types'

const CHART = { width: 760, height: 250, padding: { top: 14, right: 24, bottom: 34, left: 104 } }

export function HistoryDialog({ view, onClose }: { view: OfferView; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const stats = view.statistics
  // Montants en devise d'affichage (EUR, ADR 0030) quand l'API les a convertis, sinon devise d'origine.
  const shown = displayStats(view)
  const history = shown?.history ?? []
  // Une année de cotations quotidiennes est illisible point par point : un point par mois (clôture) ;
  // la plus basse et la plus haute cotation du mois sont données au survol.
  const months = monthlySeries(history)
  const chart = monthlyChart(months, CHART.width, CHART.height, CHART.padding)
  const selected = history.find((quote) => quote.id === selectedID) ?? history.at(-1) ?? shown?.current ?? undefined
  const selectedMonth = months.find((month) => selected && month.key === selected.quoted_at.slice(0, 7))
  useEffect(() => {
    const opener = document.activeElement
    if (dialog.current && !dialog.current.open) dialog.current.showModal()
    return () => { if (opener instanceof HTMLElement && opener.isConnected) opener.focus() }
  }, [])
  return (
    <dialog ref={dialog} className="history-dialog" aria-labelledby="history-title" onCancel={(event) => { event.preventDefault(); onClose() }}>
      <div className="dialog-heading">
        <div><p className="eyebrow">Détail de l’offre #{view.offer.id}</p><h2 id="history-title">Historique : {view.part.name}</h2></div>
        <button type="button" className="icon-button" aria-label="Fermer l’historique" onClick={onClose}>×</button>
      </div>
      <div className="history-summary">
        <div><span className="field-caption">Prix actuel</span><strong>{formatMoney(shown?.current?.price)}</strong></div>
        <div><span className="field-caption">Minimum de la période</span><strong>{shown?.minimum ? formatMoney(shown.minimum) : 'Indisponible'}</strong></div>
        <div><span className="field-caption">Maximum de la période</span><strong>{shown?.maximum ? formatMoney(shown.maximum) : 'Indisponible'}</strong></div>
        <div><span className="field-caption">Variation</span><strong className={`trend trend-${shown?.trend ?? 'unknown'}`}>{formatVariation(shown?.variation, shown?.trend)}</strong></div>
      </div>
      {view.error && <p className="notice" role="status">{view.error}</p>}
      {history.length ? <>
        <p className="muted">Cotations du {formatUTC(stats?.window_start)} au {formatUTC(stats?.as_of)}. {shown?.converted
          ? `Prix convertis en ${DISPLAY_CURRENCY} au taux coté le jour de chaque cotation (devise d’origine : ${view.offer.currency}).`
          : 'Prix dans leur devise d’origine : conversion indisponible.'}</p>
        {!!shown?.missing && <p className="notice" role="status">{shown.missing} cotation{shown.missing > 1 ? 's' : ''} sans taux de change du jour, non tracée{shown.missing > 1 ? 's' : ''}.</p>}
        {chart && <svg viewBox={`0 0 ${CHART.width} ${CHART.height}`} className="detail-chart" aria-hidden="true">
          <line x1={CHART.padding.left} y1={chart.top} x2={CHART.width - CHART.padding.right} y2={chart.top} className="chart-grid" />
          <line x1={CHART.padding.left} y1={chart.bottom} x2={CHART.width - CHART.padding.right} y2={chart.bottom} className="chart-grid" />
          <text x={CHART.padding.left - 10} y={chart.top + 4} className="chart-axis" textAnchor="end">{formatMoney(chart.high.price)}</text>
          <text x={CHART.padding.left - 10} y={chart.bottom + 4} className="chart-axis" textAnchor="end">{formatMoney(chart.low.price)}</text>
          <polyline points={chart.points.map((p) => `${p.x},${p.close}`).join(' ')} className="chart-line" />
          {chart.points.map((p, index) => {
            const active = selectedMonth?.key === p.month.key
            const half = chart.points.length > 1 ? (chart.points[1].x - chart.points[0].x) / 2 : CHART.width / 2
            return <g key={p.month.key} className={active ? 'chart-month active' : 'chart-month'}
              onPointerEnter={() => setSelectedID(p.month.close.id)} onPointerDown={() => setSelectedID(p.month.close.id)}>
              <rect x={p.x - half} y={0} width={half * 2} height={CHART.height} className="chart-hit" />
              {active && <line x1={p.x} y1={chart.top} x2={p.x} y2={chart.bottom} className="chart-cursor" />}
              <circle cx={p.x} cy={p.close} r={active ? 6 : 4} className={active ? 'chart-dot selected' : 'chart-dot'}>
                <title>{monthTitle(p.month)} : clôture {formatMoney(p.month.close.price)}, min. {formatMoney(p.month.low.price)}, max. {formatMoney(p.month.high.price)}</title>
              </circle>
              <text x={p.x} y={CHART.height - 6} className="chart-axis" textAnchor="middle">{monthLabel(p.month, index)}</text>
            </g>
          })}
        </svg>}
        <p className="chart-legend"><span className="legend-line" aria-hidden="true" /> Dernière cotation du mois (survolez un mois pour sa plus basse et sa plus haute cotation)</p>
        {selected && <div className="quote-detail" data-testid="quote-detail" role="status" aria-live="polite"><strong>{formatMoney(selected.price)}</strong><span>{formatUTC(selected.quoted_at)}</span>{selectedMonth && <span>{monthTitle(selectedMonth)} : min. {formatMoney(selectedMonth.low.price)} · max. {formatMoney(selectedMonth.high.price)} · {selectedMonth.count} cotation{selectedMonth.count > 1 ? 's' : ''}</span>}{shown?.converted && <OriginPrice origin={shown.origin.get(selected.id)} />}<span className="muted">Source : {selected.source} · cotation #{selected.id}</span></div>}
        <details className="quotes-disclosure" open={history.length <= 7}>
          <summary>Consulter les {history.length} cotations (clavier et tactile)</summary>
          <p className="muted">Choisissez une cotation pour afficher sa date et son prix sur le graphique.</p>
          <div className="quote-list" role="list" aria-label="Cotations de la période">
            {history.map((quote) => <div key={quote.id} role="listitem"><button type="button" aria-pressed={selected?.id === quote.id}
              aria-label={`Cotation du ${formatUTC(quote.quoted_at)} : ${formatMoney(quote.price)}`} onClick={() => setSelectedID(quote.id)} onFocus={() => setSelectedID(quote.id)}>
              <time dateTime={quote.quoted_at}>{formatUTC(quote.quoted_at)}</time><strong>{formatMoney(quote.price)}</strong>
            </button></div>)}
          </div>
        </details>
      </> : <p className="empty-state">Aucune cotation dans la période affichée.</p>}
      {shown?.reference && <p className="reference-note">Référence de la variation : {formatMoney(shown.reference.price)}, cotée le {formatUTC(shown.reference.quoted_at)}. Période réelle jusqu’au {formatUTC(stats?.period_end)}.</p>}
    </dialog>
  )
}

// OriginPrice : le prix tel que coté par le fournisseur et le taux du jour qui l'a converti (ADR 0030).
function OriginPrice({ origin }: { origin: { quote: Quote; rate?: FXQuote } | undefined }) {
  if (!origin) return null
  const { quote, rate } = origin
  return <span>Coté {formatMoney(quote.price)}{rate ? ` · taux ${rate.base_currency}→${rate.quote_currency} ${formatRate(rate.rate)} du ${formatUTC(rate.quoted_at)}` : ''}</span>
}
