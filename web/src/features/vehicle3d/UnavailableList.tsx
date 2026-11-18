import { useMemo, useState } from 'react'
import type { Vehicle3DNode } from './types.ts'

const reasons: Record<string, string> = {
  no_binding: 'liaison absente', part_changed: 'variante physique sans modèle', parent_changed: 'placement du nouveau parent absent',
  asset_missing: 'fichier modèle non installé', invalid_quantity: 'quantité non représentable', positions_missing: 'positions d’instances insuffisantes',
}

/** La vraie BOM contient 20000 pièces sans mesh : aucun DOM ni recherche quadratique au repos. */
export function UnavailableList({ unavailable, nodes, onSelect }: {
  unavailable: ReadonlyMap<number, string>; nodes: Vehicle3DNode[]; onSelect: (id: number) => void
}) {
  const [open, setOpen] = useState(false)
  const [limit, setLimit] = useState(100)
  const byId = useMemo(() => new Map(nodes.map(n => [n.id, n])), [nodes])
  // Seules les pièces physiques comptent, comme le badge « représentées » ; les sous-ensembles sans liaison sont exclus.
  const entries = useMemo(() => [...unavailable].filter(([id]) => byId.get(id)?.part_id !== null), [unavailable, byId])
  return <details onToggle={event => setOpen(event.currentTarget.open)}>
    <summary>Occurrences indisponibles ({entries.length})</summary>
    {open ? <>
      <ul>{entries.slice(0, limit).map(([id, reason]) => <li key={id}>
        <button type="button" onClick={() => onSelect(id)}>#{id} {byId.get(id)?.name || `Pièce #${byId.get(id)?.part_id}`}</button> : {reasons[reason] ?? reason}
      </li>)}</ul>
      {limit < entries.length ? <button type="button" className="v3d-button" onClick={() => setLimit(n => n + 100)}>Afficher 100 suivantes</button> : null}
    </> : null}
  </details>
}
