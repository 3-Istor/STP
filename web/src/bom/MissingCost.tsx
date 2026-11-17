import { useState } from 'react'
import type { TreeNode } from '../api.ts'
import { partName, type Catalog } from '../catalog.ts'
import { dominantCause, missingSummary } from './costCause.ts'

const LIST_MAX = 20

// MissingCost : « N pièces sans coût » et leur cause principale ; la liste (20 au plus, cliquables) vient à la demande.
export function MissingCost({ ids, byID, catalog, onSelect }: { ids: readonly number[]; byID: ReadonlyMap<number, TreeNode>; catalog: Catalog; onSelect: (id: number) => void }) {
  const [open, setOpen] = useState(false)
  const label = (id: number) => {
    const n = byID.get(id)
    if (!n) return `#${id}`
    return n.part_id !== null ? partName(catalog, n.part_id) : n.name
  }
  return (
    <>
      {missingSummary(ids, dominantCause(ids, byID))}{' '}
      <button type="button" className="link" aria-expanded={open} onClick={() => setOpen(!open)}>
        {open ? 'Masquer la liste' : 'Voir la liste'}
      </button>
      {open && (
        <ul className="small">
          {ids.slice(0, LIST_MAX).map((id) => (
            <li key={id}>
              <button type="button" className="link" onClick={() => onSelect(id)}>
                {label(id)}
              </button>
            </li>
          ))}
          {ids.length > LIST_MAX && <li className="muted">… et {ids.length - LIST_MAX} autres</li>}
        </ul>
      )}
    </>
  )
}
