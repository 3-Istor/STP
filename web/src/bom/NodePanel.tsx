import { useState } from 'react'
import type { Explanation, NewNode, NodeAnomaly, NodeBudget, NodePatch, StockLine, TreeNode } from '../api.ts'
import { partName, type Catalog } from '../catalog.ts'
import { decimalInput, formatDate, formatDecimal, formatMoney, multiplyDecimal, sameDecimal } from '../money.ts'
import { AddNodeDialog } from './AddNodeDialog.tsx'
import { AnomalyFacts } from './Anomalies.tsx'
import { budgetClass, budgetPercentText, budgetTitle } from './budget.ts'
import { CostBreakdown } from './CostBreakdown.tsx'
import type { Target } from '../router.ts'
import { StockPanel } from './StockPanel.tsx'

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
  anomaly?: NodeAnomaly
  parentName?: string // fil du nœud (parent direct), comme la maquette I4-1
  onSetStock?: (partID: number, quantity: string) => Promise<boolean>
  target: Target // brouillon ou snapshot : source de la ventilation « Pourquoi ce coût ? »
  version: number // relecture de l'arbre : la ventilation est relue
  budget?: NodeBudget // budget cible propre au nœud (API), s'il en a un
  currency: string // devise de la BOM : celle de tout budget
  // onSetBudget : saisie du budget (onglet Structurer, brouillon) ; amount null retire le budget (ADR 0040).
  onSetBudget?: (amount: string | null) => Promise<boolean>
}

// NodePanel : détail d'un nœud à la demande (AGENTS §5), dans la colonne de droite de toutes les étapes —
// totaux, sur quoi ils reposent, quantités commandées, stock — et les actions du brouillon sur ce nœud.
// V2 (lot C) : la stratégie est dans l'étape Arbitrer, le coût rendu et le make-or-buy dans Chiffrer.
export function NodePanel(p: Props) {
  const { node, explanation, volume, catalog, readOnly, isRoot } = p
  const t = node.totals
  const [adding, setAdding] = useState(false)
  const isPart = node.part_id !== null
  const title = isPart ? partName(catalog, node.part_id!) : node.name
  const offer = isPart ? catalog.byPart.get(node.part_id!)?.offers.find((o) => o.id === node.offer_id) : undefined

  return (
    <section className="node-detail" aria-label="Détail du nœud">
      {p.parentName && <div className="muted small">{p.parentName} ›</div>}
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
          {p.budget && (
            <>
              <dt>Budget</dt>
              <dd title={budgetTitle(p.budget)}>
                {formatMoney(p.budget.target)} · <b className={budgetClass(p.budget)}>{budgetPercentText(p.budget)}</b>
              </dd>
            </>
          )}
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
          <dt>CO₂</dt>
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

      <CostBreakdown target={p.target} nodeID={node.id} title={title} version={p.version} />

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

      {!readOnly && !isPart && p.onSetBudget && (
        <BudgetField key={`budget-${node.id}-${p.budget?.target.amount ?? ''}`} current={p.budget?.target.amount ?? null} currency={p.currency} onSet={p.onSetBudget} />
      )}

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
    </section>
  )
}

// BudgetField : budget cible de l'ensemble, dans la devise de la BOM. La règle « Σ des sous-ensembles ≤ budget de
// l'ensemble » est vérifiée par l'API ; un refus s'affiche tel quel (data.run), le champ garde la saisie.
function BudgetField({ current, currency, onSet }: { current: string | null; currency: string; onSet: (amount: string | null) => Promise<boolean> }) {
  const [value, setValue] = useState(current ?? '')
  const amount = decimalInput(value)
  const valid = /^\d+(\.\d+)?$/.test(amount)
  return (
    <form
      className="row"
      onSubmit={(e) => {
        e.preventDefault()
        if (valid) void onSet(amount)
      }}
    >
      <label>
        Budget ({currency}){' '}
        <input aria-label="Budget de l'ensemble" inputMode="decimal" size={10} placeholder="aucun" value={value} onChange={(e) => setValue(e.target.value)} />
      </label>
      <button type="submit" disabled={!valid || (current !== null && sameDecimal(amount, current))}>
        OK
      </button>
      {current !== null && (
        <button type="button" className="link" onClick={() => void onSet(null)}>
          retirer
        </button>
      )}
    </form>
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
