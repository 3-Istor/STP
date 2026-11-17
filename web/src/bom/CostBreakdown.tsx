import { useEffect, useState } from 'react'
import { api, errorMessage, type CostBasis, type CostBreakdown as Breakdown, type CostLineKind } from '../api.ts'
import { formatDate, formatMoney } from '../money.ts'
import type { Target } from '../router.ts'
import { methodLabel } from './costMethod.ts'
import { roundedLabel } from './labels.ts'

// « Pourquoi ce coût ? » (maquette I4-1, ADR 0031) : la ventilation du coût d'un nœud telle que l'API la
// renvoie — une ligne par nature (prix, change, remise, douane, transport, emballage, outillage) avec sa
// source et sa date, puis le total. Pour un sous-ensemble, l'API agrège les pièces par nature. Rien n'est
// additionné ici : le total affiché est celui de l'API (Σ des lignes exactes, arrondi à l'affichage).

const kindLabels: Record<CostLineKind, string> = {
  price: 'Prix',
  fx: 'Change',
  discount: 'Remise de contrat',
  customs: 'Douane',
  freight: 'Transport',
  packaging: 'Emballage',
  tooling: 'Outillage amorti',
}

// Composantes que chaque base de coût contient (ADR 0031) : une composante attendue sans ligne est montrée
// « absente » au lieu d'être tue ou affichée à 0. Le change n'y figure pas : sa ligne n'existe que si une
// conversion a eu lieu (pièce dans une autre devise, taux budget).
const expectedKinds: Record<CostBasis, CostLineKind[]> = {
  purchase: ['price'],
  landed: ['price', 'discount', 'customs', 'freight', 'packaging'],
  full: ['price', 'discount', 'customs', 'freight', 'packaging', 'tooling'],
}

type State = { key: string; status: 'loading' } | { key: string; status: 'ready'; data: Breakdown } | { key: string; status: 'error'; error: string }

/** version : change à chaque relecture de l'arbre (action, méthode, source des prix) : la cascade est relue. */
export function CostBreakdown({ target, nodeID, title, version }: { target: Target; nodeID: number; title: string; version: number }) {
  const key = `${target.kind}-${target.id}-${nodeID}-${version}`
  const [state, setState] = useState<State>({ key, status: 'loading' })
  useEffect(() => {
    let cancelled = false
    const load = target.kind === 'draft' ? api.costBreakdown(target.id, nodeID) : api.snapshotCostBreakdown(target.id, nodeID)
    load
      .then((data) => {
        if (!cancelled) setState({ key, status: 'ready', data })
      })
      .catch((e: unknown) => {
        if (!cancelled) setState({ key, status: 'error', error: errorMessage(e) })
      })
    return () => {
      cancelled = true
    }
  }, [key, target.kind, target.id, nodeID])
  const current = state.key === key ? state : { key, status: 'loading' as const }

  return (
    <section className="cascade-block" aria-label="Pourquoi ce coût ?">
      <h3>Pourquoi ce coût ? · {title}</h3>
      {current.status === 'loading' && (
        <p className="muted small" role="status">
          Chargement de la ventilation…
        </p>
      )}
      {current.status === 'error' && (
        <p className="error small" role="alert">
          Ventilation indisponible : {current.error}
        </p>
      )}
      {current.status === 'ready' && <Cascade data={current.data} />}
    </section>
  )
}

function Cascade({ data }: { data: Breakdown }) {
  const present = new Set(data.lines.map((l) => l.kind))
  const missing = (expectedKinds[data.method.basis] ?? []).filter((k) => !present.has(k))
  return (
    <>
      <p className="muted small cascade-method">
        {methodLabel(data.method)}
        {data.cost_as_of ? ` · calcul au ${formatDate(data.cost_as_of)}` : ''}
      </p>
      <div className="casc" role="table" aria-label="Ventilation du coût">
        {data.lines.map((l, i) => (
          <div className="casc-row" role="row" key={`${l.kind}-${i}`}>
            <div role="cell">
              <span className="casc-kind">{kindLabels[l.kind] ?? l.kind}</span>
              <div className="casc-label" title={l.label}>
                {roundedLabel(l.label)}
              </div>
              <div className="src">
                {l.source}
                {l.as_of ? ` · ${formatDate(l.as_of)}` : ''}
              </div>
            </div>
            <div role="cell" className={`casc-amount ${l.amount.amount.startsWith('-') ? 't-ok' : ''}`}>
              {formatMoney(l.amount)}
            </div>
          </div>
        ))}
        {missing.map((k) => (
          <div className="casc-row absent" role="row" key={`absent-${k}`}>
            <div role="cell">
              <span className="casc-kind">{kindLabels[k]}</span>
              <div className="src">aucune ligne renvoyée par l’API pour cette composante de la méthode</div>
            </div>
            <div role="cell" className="casc-amount muted">
              absente
            </div>
          </div>
        ))}
        <div className="casc-row tot" role="row">
          <div role="cell">Total ({data.currency})</div>
          <div role="cell" className="casc-amount">
            {formatMoney(data.total)}
          </div>
        </div>
      </div>
      <p className="muted small">Montants exacts additionnés par l’API, arrondis à l’affichage : la somme des lignes arrondies peut différer d’un centime.</p>
    </>
  )
}
