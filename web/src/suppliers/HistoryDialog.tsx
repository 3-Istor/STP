import { useEffect, useRef, useState } from 'react'
import { formatMoney, formatUTC, formatVariation } from './format'
import { graphPoints } from './graph'
import type { OfferView } from './types'

export function HistoryDialog({ view, onClose }: { view: OfferView; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const stats = view.statistics
  const history = stats?.history ?? []
  const points = graphPoints(history, 720, 220)
  const selected = history.find((quote) => quote.id === selectedID) ?? history.at(-1) ?? stats?.current_quote
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
        <div><span className="field-caption">Prix actuel</span><strong>{formatMoney(stats?.current_quote?.price)}</strong></div>
        <div><span className="field-caption">Minimum de la période</span><strong>{stats?.minimum ? formatMoney(stats.minimum) : 'Indisponible'}</strong></div>
        <div><span className="field-caption">Maximum de la période</span><strong>{stats?.maximum ? formatMoney(stats.maximum) : 'Indisponible'}</strong></div>
        <div><span className="field-caption">Variation</span><strong className={`trend trend-${stats?.trend ?? 'unknown'}`}>{formatVariation(stats?.variation_percent, stats?.trend)}</strong></div>
      </div>
      {view.error && <p className="notice" role="status">{view.error}</p>}
      {history.length ? <>
        <p className="muted">Cotations du {formatUTC(stats?.window_start)} au {formatUTC(stats?.as_of)}. Les prix restent dans leur devise d’origine.</p>
        <svg viewBox="0 0 720 220" className="detail-chart" aria-hidden="true">
          <line x1="8" y1="212" x2="712" y2="212" className="chart-grid" />
          <line x1="8" y1="110" x2="712" y2="110" className="chart-grid" />
          <polyline points={points.map((point) => `${point.x},${point.y}`).join(' ')} className="chart-line" />
          {points.map((point) => <circle key={point.quote.id} cx={point.x} cy={point.y} r={selected?.id === point.quote.id ? 6 : 4}
            className={selected?.id === point.quote.id ? 'chart-dot selected' : 'chart-dot'}
            onPointerEnter={() => setSelectedID(point.quote.id)} onPointerDown={() => setSelectedID(point.quote.id)}>
            <title>{formatUTC(point.quote.quoted_at)} : {formatMoney(point.quote.price)}</title>
          </circle>)}
        </svg>
        {selected && <div className="quote-detail" data-testid="quote-detail" role="status" aria-live="polite"><strong>{formatMoney(selected.price)}</strong><span>{formatUTC(selected.quoted_at)}</span><span className="muted">Source : {selected.source} · cotation #{selected.id}</span></div>}
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
      {stats?.reference_quote && <p className="reference-note">Référence de la variation : {formatMoney(stats.reference_quote.price)}, cotée le {formatUTC(stats.reference_quote.quoted_at)}. Période réelle jusqu’au {formatUTC(stats.period_end)}.</p>}
    </dialog>
  )
}
