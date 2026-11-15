import { useState } from 'react'
import type { Explanation, NewNode, NodeAnomaly, NodePatch, StockLine, TreeNode } from '../api.ts'
import { partName, type Catalog } from '../catalog.ts'
import { formatDate, formatDecimal, formatMoney, multiplyDecimal } from '../money.ts'
import { AddNodeDialog } from './AddNodeDialog.tsx'
import { AnomalyFacts } from './Anomalies.tsx'
import { LandedPanel } from './LandedPanel.tsx'
import { MakeOrBuyPanel } from './MakeOrBuyPanel.tsx'
import { StockPanel } from './StockPanel.tsx'
import { StrategyPanel } from './StrategyPanel.tsx'

type Props = {
  node: TreeNode
  explanation: Explanation
  volume: number
  catalog: Catalog
  readOnly: boolean
  isRoot: boolean
  onAdd: (n: NewNode) => Promise<boolean>
  onPatch: (p: NodePatch) => Promise<boolean>
  onStartMove: () => void
  onDelete: () => void
  stock: StockLine[]
  bomID: number
  anomaly?: NodeAnomaly
  treeVersion?: number
  // destination : pays de destination de la BOM ; undefined = pas de comparaison (snapshot en lecture seule).
  destination?: string | null
  onSetStock?: (partID: number, quantity: string) => Promise<boolean>
}

// NodePanel : détail d'un nœud à la demande (AGENTS §5) — totaux, sur quoi ils reposent, quantités
// commandées — et les actions du brouillon sur ce nœud.
export function NodePanel(p: Props) {
  const { node, explanation, volume, catalog, readOnly, isRoot } = p
  const t = node.totals
  const [adding, setAdding] = useState(false)
  const isPart = node.part_id !== null
  const title = isPart ? partName(catalog, node.part_id!) : node.name
  const offer = isPart ? catalog.byPart.get(node.part_id!)?.offers.find((o) => o.id === node.offer_id) : undefined

  return (
    <aside className="panel" aria-label="Détail du nœud">
      <h2>{title}</h2>
      <p className="muted">
        {isPart ? `Pièce #${node.part_id}, offre #${node.offer_id}${offer ? ` (${offer.supplier})` : ''}` : 'Sous-ensemble'}
      </p>

      {t && (
        <dl className="facts">
          <dt>Coût total</dt>
          <dd className={t.cost_error_code ? 'over' : undefined}>
            {t.cost ? formatMoney(t.cost) : (t.cost_error ?? 'n/d')}
          </dd>
          {t.missing_cost.length > 0 && (
            <>
              <dt>Sans prix</dt>
              <dd className="over">nœuds {t.missing_cost.join(', ')}</dd>
            </>
          )}
          {t.own_cost && (
            <>
              <dt>Coût propre</dt>
              <dd>{formatMoney(t.own_cost)}</dd>
            </>
          )}
          <dt>Quantité réelle</dt>
          <dd>{formatDecimal(t.real_quantity)} par véhicule</dd>
          <dt>Quantité commandée</dt>
          {/* Produit fait par BigInt sur la chaîne décimale : pas de float (AGENTS §3). */}
          <dd>
            {formatDecimal(multiplyDecimal(t.real_quantity, volume))} pour {formatDecimal(String(volume))} véhicules
          </dd>
          <dt>CO2</dt>
          <dd>{formatDecimal(t.co2_kg, 2)} kg</dd>
          <dt>Masse</dt>
          <dd>{formatDecimal(t.mass_kg, 2)} kg</dd>
          <dt>Délai</dt>
          <dd>
            {t.lead_time_days} j (dont {t.own_lead_time_days} j propres)
          </dd>
          <dt>Main-d'œuvre</dt>
          <dd className={t.labor_error ? 'over' : undefined}>
            {t.labor_hours
              ? `${formatDecimal(t.labor_hours, 2)} h par véhicule${t.fte ? ` · ${formatDecimal(t.fte, 2)} ETP` : ''}`
              : `indisponible (pièces sans heures : ${t.missing_labor.join(', ')})`}
          </dd>
        </dl>
      )}

      <AnomalyFacts anomaly={p.anomaly} />

      <StrategyPanel
        key={`strategy-${node.id}-${p.treeVersion ?? 0}`}
        bomID={p.bomID}
        node={node}
        catalog={catalog}
        readOnly={readOnly}
        onPatch={p.onPatch}
      />

      {!isPart && !readOnly && p.destination !== undefined && (
        <MakeOrBuyPanel key={`mob-${node.id}-${p.treeVersion ?? 0}`} bomID={p.bomID} nodeID={node.id} currency={explanation.currency} />
      )}

      {isPart && !readOnly && p.destination !== undefined && (
        <LandedPanel key={`landed-${node.id}-${p.treeVersion ?? 0}`} bomID={p.bomID} node={node} catalog={catalog} destination={p.destination} />
      )}

      {isPart && (
        <StockPanel
          key={`stock-${node.id}-${p.stock.find((s) => s.part_id === node.part_id)?.quantity ?? '0'}`}
          node={node}
          stock={p.stock}
          volume={volume}
          readOnly={readOnly}
          onSetStock={p.onSetStock}
        />
      )}

      <details>
        <summary>Sur quoi reposent ces totaux</summary>
        <ul className="explain">
          <li>Devise : {explanation.currency}</li>
          <li>
            {explanation.price_snapshot_id === null
              ? 'Prix en direct (dernière cotation)'
              : `Prix figés : snapshot de prix #${explanation.price_snapshot_id}`}
          </li>
          {t?.quote && (
            <li>
              Cotation : {formatMoney(t.quote.price)} ({t.quote.source}, {formatDate(t.quote.quoted_at)})
            </li>
          )}
          {t?.rate && (
            <li>
              Taux : 1 {t.rate.from} = {t.rate.rate} {t.rate.to} ({t.rate.source}, {formatDate(t.rate.quoted_at)})
            </li>
          )}
          {!t?.quote &&
            explanation.rates.map((r) => (
              <li key={`${r.from}-${r.to}`}>
                Taux : 1 {r.from} = {r.rate} {r.to} ({r.source}, {formatDate(r.quoted_at)})
              </li>
            ))}
        </ul>
      </details>

      {!readOnly && (
        <>
          <EditFields key={`edit-${node.id}`} node={node} onPatch={p.onPatch} />
          <div className="row">
            <button type="button" className="primary" onClick={() => setAdding(true)}>
              + Ajouter dessous
            </button>
            {!isRoot && (
              <button type="button" onClick={p.onStartMove}>
                Déplacer…
              </button>
            )}
            {!isRoot && (
              <button type="button" className="danger" onClick={p.onDelete}>
                Supprimer
              </button>
            )}
          </div>
          {adding && <AddNodeDialog parentName={title} parentID={node.id} catalog={catalog} onAdd={p.onAdd} onClose={() => setAdding(false)} />}
        </>
      )}
    </aside>
  )
}

function EditFields({ node, onPatch }: { node: TreeNode; onPatch: (p: NodePatch) => Promise<boolean> }) {
  const [qty, setQty] = useState(node.quantity)
  const [days, setDays] = useState(String(node.production_days))
  const isPart = node.part_id !== null
  return (
    <div className="stack">
      {node.parent_id !== null && (
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault()
            void onPatch({ quantity: qty.trim() })
          }}
        >
          <label>
            Quantité <input aria-label="Quantité" inputMode="decimal" size={6} value={qty} onChange={(e) => setQty(e.target.value)} />
          </label>
          <button type="submit" disabled={qty.trim() === node.quantity}>
            OK
          </button>
        </form>
      )}
      {/* Les jours de production d'une pièce viennent du catalogue (choix T6) : seulement pour un sous-ensemble. */}
      {!isPart && (
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault()
            void onPatch({ production_days: Number.parseInt(days, 10) })
          }}
        >
          <label>
            Jours de production{' '}
            <input aria-label="Jours de production" type="number" min={0} step={1} value={days} onChange={(e) => setDays(e.target.value)} />
          </label>
          <button type="submit" disabled={days === String(node.production_days)}>
            OK
          </button>
        </form>
      )}
    </div>
  )
}
