import { useEffect, useMemo, useRef, useState } from 'react'
import type { NodeAnomaly, NodeBudget, TreeNode } from '../api.ts'
import { partName, type Catalog } from '../catalog.ts'
import { formatDecimal, formatMoney } from '../money.ts'
import { flatten, initiallyCollapsed } from './flatten.ts'
import { describeAnomaly } from './anomaly.ts'
import { budgetClass, budgetPercentText, budgetTitle } from './budget.ts'
import { strategyLabels } from './strategy.ts'

// Virtualisation faite à la main (spec NLPF-69, aucune dépendance) : l'arbre est aplati en lignes de
// hauteur fixe et seules les lignes visibles (plus une marge) sont dans le DOM, quel que soit le nombre de nœuds.
export const ROW_HEIGHT = 30
const OVERSCAN = 8
// Niveaux dépliés à l'ouverture (racine, sous-systèmes, ensembles) : le détail s'ouvre à la demande.
const OPEN_DEPTH = 2

type Props = {
  nodes: TreeNode[]
  catalog: Catalog
  readOnly: boolean
  selectedID: number | null
  onSelect: (id: number) => void
  onChangeOffer: (nodeID: number, offerID: number) => void
  // Déplacement en cours : la ligne cliquée devient le nouveau parent du nœud `moving`.
  moving: number | null
  onPickParent: (parentID: number) => void
  // Anomalies de coût par nœud (ADR 0027) : surbrillance des pièces suspectes, raison au survol.
  anomalies?: ReadonlyMap<number, NodeAnomaly>
  // Budgets cibles par nœud (API) ; budgetsUnavailable : pourquoi la colonne est vide (snapshot publié…).
  budgets?: ReadonlyMap<number, NodeBudget>
  budgetsUnavailable?: string | null
  // Recherche (champ dans la barre d'outils de l'étape Structurer).
  query: string
  // Actions en ligne sur le nœud sélectionné (brouillon) : ajouter dessous, déplacer, supprimer.
  onAddUnder?: (id: number) => void
  onStartMove?: (id: number) => void
  onDelete?: (id: number) => void
}

export function BomTree(props: Props) {
  const { nodes, catalog, readOnly, selectedID, onSelect, onChangeOffer, moving, onPickParent, anomalies, query } = props
  const [collapsed, setCollapsed] = useState<ReadonlySet<number>>(() => initiallyCollapsed(nodes, OPEN_DEPTH))
  const rows = useMemo(
    () => flatten(nodes, collapsed, (n) => (n.part_id !== null ? partName(catalog, n.part_id) : n.name), query),
    [nodes, collapsed, catalog, query],
  )

  const viewport = useRef<HTMLDivElement>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [height, setHeight] = useState(600)
  useEffect(() => {
    const el = viewport.current
    if (!el) return
    const ro = new ResizeObserver(() => setHeight(el.clientHeight))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // Sélection venue d'ailleurs (lien profond, autre étape, Synthèse) : ses ancêtres sont dépliés et la
  // ligne est ramenée dans la zone visible. Un clic dans l'arbre ne fait rien défiler (ligne déjà visible).
  const byID = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes])
  const [revealedFor, setRevealedFor] = useState<number | null>(null)
  if (selectedID !== revealedFor) {
    // Mise à jour pendant le rendu (état dérivé de la sélection précédente), pas dans un effet.
    setRevealedFor(selectedID)
    const hidden: number[] = []
    for (let p = selectedID !== null ? (byID.get(selectedID)?.parent_id ?? null) : null; p !== null; p = byID.get(p)?.parent_id ?? null) {
      if (collapsed.has(p)) hidden.push(p)
    }
    if (hidden.length > 0) {
      const next = new Set(collapsed)
      for (const h of hidden) next.delete(h)
      setCollapsed(next)
    }
  }
  // Une fois par sélection : les lignes changent à chaque dépliage, on ne recentre pas à chaque fois.
  const revealed = useRef<number | null>(null)
  useEffect(() => {
    const el = viewport.current
    if (!el || selectedID === null || revealed.current === selectedID) return
    const index = rows.findIndex((r) => r.node.id === selectedID)
    if (index < 0) return
    revealed.current = selectedID
    const top = (index + 1) * ROW_HEIGHT // + en-tête
    if (top < el.scrollTop + ROW_HEIGHT || top + ROW_HEIGHT > el.scrollTop + el.clientHeight) {
      el.scrollTop = Math.max(0, top - el.clientHeight / 3)
    }
  }, [selectedID, rows])

  // L'en-tête de colonnes (une ligne, collant) précède les lignes de l'arbre dans la zone défilante.
  const first = Math.max(0, Math.floor((scrollTop - ROW_HEIGHT) / ROW_HEIGHT) - OVERSCAN)
  const last = Math.min(rows.length, Math.ceil((scrollTop + height) / ROW_HEIGHT) + OVERSCAN)

  function toggle(id: number) {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (!next.delete(id)) next.add(id)
      return next
    })
  }

  return (
    <div className="tree-pane">
    <div
      ref={viewport}
      className={moving !== null ? 'tree moving' : 'tree'}
      role="tree"
      aria-label="Arbre de la BOM"
      onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
    >
      <div className="tree-head" aria-hidden="true" style={{ height: ROW_HEIGHT }}>
        <span className="name">Ensemble / pièce</span>
        <span className="qty" title="Quantité par parent">Qté</span>
        <span className="offer" title="Offre retenue : fournisseur · prix unitaire · délai de livraison">Offre retenue</span>
        <span className="cost" title="Coût par véhicule du sous-arbre, quantités comprises">Coût / véh.</span>
        <span className="budget" title={props.budgetsUnavailable ?? 'Coût du sous-arbre ÷ budget cible du nœud (vert < 90 %, orange ≥ 90 %, rouge > 100 %) ; « — » : pas de budget sur ce nœud'}>
          Budget
        </span>
        <span className="co2" title="CO₂ du sous-arbre par véhicule, en kg">CO₂ kg</span>
        <span className="lead" title="Délai du sous-arbre en jours : le plus long des enfants plus le délai propre (livraison ou production)">Délai</span>
      </div>
      {query !== '' && rows.length === 0 && <p className="muted tree-empty">Aucun résultat pour « {query} ».</p>}
      <div style={{ height: rows.length * ROW_HEIGHT, position: 'relative' }}>
        {rows.slice(first, last).map((r, i) => {
          const n = r.node
          const t = n.totals
          const isPart = n.part_id !== null
          const offers = isPart ? (catalog.byPart.get(n.part_id!)?.offers ?? []) : []
          const classes = ['tree-row']
          if (n.id === selectedID) classes.push('selected')
          if (!isPart) classes.push('group')
          if (t?.cost_error_code) classes.push('has-error')
          const anomaly = anomalies?.get(n.id)
          if (anomaly?.status === 'anomaly') classes.push('anomaly')
          return (
            <div
              key={n.id}
              role="treeitem"
              aria-level={r.depth + 1}
              aria-selected={n.id === selectedID}
              aria-expanded={r.hasChildren ? r.expanded : undefined}
              className={classes.join(' ')}
              style={{ top: (first + i) * ROW_HEIGHT, height: ROW_HEIGHT, paddingLeft: 8 + r.depth * 16 }}
              onClick={() => (moving !== null ? onPickParent(n.id) : onSelect(n.id))}
            >
              <button
                type="button"
                className="toggle"
                tabIndex={-1}
                aria-hidden={!r.hasChildren}
                style={{ visibility: r.hasChildren ? 'visible' : 'hidden' }}
                onClick={(e) => {
                  e.stopPropagation()
                  toggle(n.id)
                }}
              >
                {r.expanded ? '▾' : '▸'}
              </button>
              <span className="name" title={isPart ? partName(catalog, n.part_id!) : n.name}>
                {isPart ? partName(catalog, n.part_id!) : n.name}
                {t?.in_stock && <span className="badge ok">en stock</span>}
                {n.strategy && <span className="badge">{strategyLabels[n.strategy]}</span>}
                {anomaly?.status === 'anomaly' && (
                  <span className="badge over" title={describeAnomaly(anomaly)}>
                    ×{anomaly.ratio ? formatDecimal(anomaly.ratio, 2) : '?'}
                  </span>
                )}
                {anomaly?.status === 'not_evaluable' && (
                  <span className="badge muted" title={`Coût non évaluable : ${anomaly.reason ?? ''}`}>
                    ?
                  </span>
                )}
                {n.id === selectedID && !readOnly && moving === null && props.onAddUnder && (
                  <span className="acts">
                    {!isPart && (
                      <button type="button" className="ib" aria-label="Ajouter dessous" title="Ajouter dessous" onClick={(e) => { e.stopPropagation(); props.onAddUnder!(n.id) }}>
                        +
                      </button>
                    )}
                    {n.parent_id !== null && (
                      <button type="button" className="ib" aria-label="Déplacer" title="Déplacer : cliquez ensuite le nouveau parent" onClick={(e) => { e.stopPropagation(); props.onStartMove?.(n.id) }}>
                        ⇅
                      </button>
                    )}
                    {n.parent_id !== null && (
                      <button type="button" className="ib" aria-label="Supprimer" title="Supprimer (avec confirmation)" onClick={(e) => { e.stopPropagation(); props.onDelete?.(n.id) }}>
                        ×
                      </button>
                    )}
                  </span>
                )}
              </span>
              <span className="qty">×{n.quantity}</span>
              {isPart && !readOnly && offers.length > 0 ? (
                <select
                  className="offer"
                  aria-label="Offre"
                  value={n.offer_id ?? ''}
                  onClick={(e) => e.stopPropagation()}
                  onChange={(e) => onChangeOffer(n.id, Number(e.target.value))}
                >
                  {offers.map((o) => (
                    <option key={o.id} value={o.id} title={o.price_source ?? undefined}>
                      {o.supplier} · {o.price ? formatMoney(o.price) : 'sans prix'} · {o.lead_time_days} j
                    </option>
                  ))}
                </select>
              ) : (
                <span className="offer muted">
                  {isPart ? (offers.find((o) => o.id === n.offer_id)?.supplier ?? `offre #${n.offer_id}`) : ''}
                </span>
              )}
              <span className="cost" title={t?.cost_error ?? undefined}>
                {t?.cost ? formatMoney(t.cost) : t?.cost_error_code ? 'prix manquant' : 'n/d'}
              </span>
              <BudgetCell budget={props.budgets?.get(n.id)} />
              <span className="co2">{t ? formatDecimal(t.co2_kg, 1) : ''}</span>
              <span className="lead">{t ? `${t.lead_time_days} j` : ''}</span>
            </div>
          )
        })}
      </div>
    </div>
    </div>
  )
}

function BudgetCell({ budget }: { budget: NodeBudget | undefined }) {
  if (!budget) return <span className="budget muted">—</span>
  return (
    <span className={`budget ${budgetClass(budget)}`} title={budgetTitle(budget)}>
      {budgetPercentText(budget)}
    </span>
  )
}
