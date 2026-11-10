import { useEffect, useMemo, useRef, useState } from 'react'
import type { TreeNode } from '../api.ts'
import { partName, type Catalog } from '../catalog.ts'
import { formatMoney } from '../money.ts'
import { flatten } from './flatten.ts'

// Virtualisation faite à la main (spec NLPF-69, aucune dépendance) : l'arbre est aplati en lignes de
// hauteur fixe et seules les lignes visibles (plus une marge) sont dans le DOM, quel que soit le nombre de nœuds.
export const ROW_HEIGHT = 30
const OVERSCAN = 8

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
}

export function BomTree({ nodes, catalog, readOnly, selectedID, onSelect, onChangeOffer, moving, onPickParent }: Props) {
  const [collapsed, setCollapsed] = useState<ReadonlySet<number>>(() => new Set())
  const rows = useMemo(() => flatten(nodes, collapsed), [nodes, collapsed])

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

  const first = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN)
  const last = Math.min(rows.length, Math.ceil((scrollTop + height) / ROW_HEIGHT) + OVERSCAN)

  function toggle(id: number) {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (!next.delete(id)) next.add(id)
      return next
    })
  }

  return (
    <div
      ref={viewport}
      className={moving !== null ? 'tree moving' : 'tree'}
      role="tree"
      aria-label="Arbre de la BOM"
      onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
    >
      <div style={{ height: rows.length * ROW_HEIGHT, position: 'relative' }}>
        {rows.slice(first, last).map((r, i) => {
          const n = r.node
          const t = n.totals
          const isPart = n.part_id !== null
          const offers = isPart ? (catalog.byPart.get(n.part_id!)?.offers ?? []) : []
          const classes = ['tree-row']
          if (n.id === selectedID) classes.push('selected')
          if (t?.cost_error_code) classes.push('has-error')
          return (
            <div
              key={n.id}
              role="treeitem"
              aria-level={r.depth + 1}
              aria-selected={n.id === selectedID}
              aria-expanded={r.hasChildren ? !collapsed.has(n.id) : undefined}
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
                {collapsed.has(n.id) ? '▸' : '▾'}
              </button>
              <span className="name" title={isPart ? partName(catalog, n.part_id!) : n.name}>
                {isPart ? partName(catalog, n.part_id!) : n.name}
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
                {t?.cost ? formatMoney(t.cost) : t?.cost_error_code ? 'prix manquant' : '—'}
              </span>
              <span className="lead">{t ? `${t.lead_time_days} j` : ''}</span>
            </div>
          )
        })}
      </div>
    </div>
  )
}
