import { useEffect, useState } from 'react'
import { api, errorMessage, type NodePatch, type Strategy, type Suggestions, type TreeNode } from '../api.ts'
import { partName, type Catalog } from '../catalog.ts'
import { formatDecimal, formatMoney } from '../money.ts'
import { strategyLabels } from './strategy.ts'
import { usePresets, weightsText } from './profile.ts'
import { useAppDialog } from '../useAppDialog.tsx'

type Props = {
  bomID: number
  node: TreeNode
  catalog: Catalog
  readOnly: boolean
  onPatch: (p: NodePatch) => Promise<boolean>
}

// StrategyPanel : stratégie du nœud (ADR 0028) et, pour une pièce, les alternatives triées selon la stratégie
// héritée. Les suggestions viennent de l'API ; une suggestion ne s'applique qu'au brouillon, après confirmation.
// Un changement de stratégie relit tout l'arbre (totaux, anomalies) : tant que la relecture n'est pas
// revenue, le choix reste affiché, le sélecteur est verrouillé et « Recalcul en cours… » le dit. Le panneau
// est remonté par NodePanel (clé treeVersion) une fois l'arbre relu ; en cas d'échec, il reprend la valeur du nœud.
export function StrategyPanel({ bomID, node, catalog, readOnly, onPatch }: Props) {
  const isPart = node.part_id !== null
  // pending : valeur choisie dont le calcul est en cours ; null quand rien n'est en cours.
  const [pending, setPending] = useState<string | null>(null)
  return (
    <div className="stack">
      {!readOnly ? (
        <label className="row">
          Stratégie{' '}
          <select
            aria-label="Stratégie du nœud"
            value={pending ?? node.strategy ?? ''}
            disabled={pending !== null}
            onChange={(e) => {
              const value = e.target.value
              setPending(value)
              void onPatch({ strategy: value === '' ? null : (value as Strategy) }).finally(() => setPending(null))
            }}
          >
            <option value="">héritée</option>
            {(Object.keys(strategyLabels) as Strategy[]).map((s) => (
              <option key={s} value={s}>
                {strategyLabels[s]}
              </option>
            ))}
          </select>
          {pending !== null && (
            <span role="status" className="muted small recalc">
              Recalcul en cours…
            </span>
          )}
        </label>
      ) : (
        node.strategy && <p className="muted">Stratégie : {strategyLabels[node.strategy]}</p>
      )}
      {isPart && !readOnly && <SuggestionList bomID={bomID} node={node} catalog={catalog} onPatch={onPatch} />}
    </div>
  )
}

function SuggestionList({ bomID, node, catalog, onPatch }: Omit<Props, 'readOnly'>) {
  const [data, setData] = useState<Suggestions | null>(null)
  const [error, setError] = useState<string | null>(null)
  const dialogs = useAppDialog()
  const { presets } = usePresets()
  // Lu à l'ouverture du panneau (remonté à chaque relecture de l'arbre : stratégie ou profil changés).
  useEffect(() => {
    let cancelled = false
    api
      .suggestions(bomID, node.id)
      .then((d) => {
        if (!cancelled) setData(d)
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(errorMessage(e))
      })
    return () => {
      cancelled = true
    }
  }, [bomID, node.id])
  const offerLabel = (partID: number, offerID: number) => {
    const o = catalog.byPart.get(partID)?.offers.find((x) => x.id === offerID)
    return `${partName(catalog, partID)} · ${o ? o.supplier : `offre #${offerID}`}`
  }
  return (
    <>
    {dialogs.element}
    <details open>
      <summary>Alternatives suggérées</summary>
      {error && <p className="error">{error}</p>}
      {!error && data === null && <p className="muted">Chargement…</p>}
      {data && (
        <>
          <p className="muted small">
            {data.strategy
              ? `Triées par ${strategyLabels[data.strategy]} (stratégie du nœud #${data.strategy_from})`
              : data.profile?.preset
                ? `Classées selon le profil « ${presets?.find((p) => p.preset === data.profile!.preset)?.label ?? data.profile.preset} »${data.weights ? ` (${weightsText(data.weights)})` : ''}`
                : 'Aucune stratégie ni profil de véhicule : ordre par offre.'}{' '}
            · {formatDecimal(data.real_quantity)} u. par véhicule ·{' '}
            {data.price_snapshot_id === null ? 'prix en direct' : `prix figés #${data.price_snapshot_id}`}
          </p>
          <table className="compact">
            <thead>
              <tr>
                <th>Offre</th>
                <th>Coût</th>
                <th>Délai</th>
                <th>CO2</th>
                <th title="Score pondéré calculé par l’API : 0 = meilleur">Score</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {data.candidates.map((c) => (
                <tr
                  key={c.offer_id}
                  title={[
                    `scores (0 = meilleur) : coût ${c.cost_score} · délai ${c.lead_time_score} · CO2 ${c.co2_score}${c.mass_score ? ` · masse ${c.mass_score}` : ''}${c.quality_score ? ` · qualité ${c.quality_score}` : ''}`,
                    c.quote ? `cotation ${formatMoney(c.quote.price)} (${c.quote.source})` : '',
                    c.rate ? `taux 1 ${c.rate.from} = ${c.rate.rate} ${c.rate.to} (${c.rate.source})` : '',
                  ]
                    .filter(Boolean)
                    .join('\n')}
                >
                  <td>{offerLabel(c.part_id, c.offer_id)}</td>
                  <td>{formatMoney(c.cost)}</td>
                  <td>{c.lead_time_days === null ? 'n/d' : `${c.lead_time_days} j`}</td>
                  <td>{c.co2_kg === null ? 'n/d' : `${formatDecimal(c.co2_kg, 2)} kg`}</td>
                  <td>{c.score ? formatDecimal(c.score, 2) : 'n/d'}</td>
                  <td>
                    {c.current ? (
                      <span className="muted">actuelle</span>
                    ) : (
                      <button
                        type="button"
                        className="link"
                        onClick={() => {
                          void dialogs
                            .confirm({ title: `Remplacer par ${offerLabel(c.part_id, c.offer_id)} ?`, message: 'Le remplacement s’applique au brouillon.', confirmLabel: 'Remplacer' })
                            .then((ok) => {
                              if (ok) void onPatch(c.part_id === node.part_id ? { offer_id: c.offer_id } : { part_id: c.part_id, offer_id: c.offer_id })
                            })
                        }}
                      >
                        appliquer
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {data.excluded.length > 0 && (
            <ul className="explain">
              {data.excluded.map((c) => (
                <li key={c.offer_id} className="muted">
                  {offerLabel(c.part_id, c.offer_id)} exclue : {c.excluded}
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </details>
    </>
  )
}
