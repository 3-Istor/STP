import { useState } from 'react'
import { api } from '../../api.ts'
import { BomTree } from '../BomTree.tsx'
import type { BomData } from '../useBomData.ts'
import { Vehicle3DSlot } from '../../shell/Vehicle3DSlot.tsx'

export type TreeView = 'arbre' | 'arbre-3d' | '3d'
const treeViews: { id: TreeView; label: string }[] = [
  { id: 'arbre', label: 'Arbre' },
  { id: 'arbre-3d', label: 'Arbre + 3D' },
  { id: '3d', label: '3D' },
]

type Props = {
  data: BomData
  selectedID: number | null
  onSelect: (id: number) => void
  view: TreeView
  onView: (v: TreeView) => void
  moving: number | null
  onCancelMove: () => void
  onPickParent: (parentID: number) => void
  onAddUnder: (parentID: number) => void
  onStartMove: (id: number) => void
  onDelete: (id: number) => void
}

// Structurer (maquettes I4-1, I4-2) : l'explorateur de la BOM en grand. Arbre virtualisé (NLPF-69), changement
// d'offre en ligne, ajout avec choix de l'offre, déplacement, suppression ; bascule Arbre / Arbre + 3D / 3D.
export function Structurer(p: Props) {
  const { data } = p
  const [query, setQuery] = useState('')
  // Ajout depuis la barre d'outils : sous l'ensemble sélectionné, sous le parent d'une pièce, sinon sous la racine.
  const selected = p.selectedID !== null ? data.byID.get(p.selectedID) : undefined
  const addParent = selected ? (selected.part_id === null ? selected.id : selected.parent_id) : (data.root?.id ?? null)
  const tree = (
    <BomTree
      key={data.target.kind + data.target.id}
      nodes={data.nodes}
      catalog={data.catalog}
      readOnly={data.readOnly}
      selectedID={p.selectedID}
      onSelect={p.onSelect}
      anomalies={data.anomalyByNode}
      budgets={data.budgets.byNode}
      budgetsUnavailable={data.budgets.unavailable}
      query={query}
      onChangeOffer={(nodeID, offerID) => void data.run(() => api.patchNode(data.target.id, nodeID, { offer_id: offerID }))}
      moving={p.moving}
      onPickParent={p.onPickParent}
      onAddUnder={data.readOnly ? undefined : p.onAddUnder}
      onStartMove={p.onStartMove}
      onDelete={p.onDelete}
    />
  )
  const slot = <Vehicle3DSlot bomId={data.target.id} nodes={data.nodes} selectedNodeId={p.selectedID} onSelectNode={p.onSelect} />
  return (
    <section className="block explorer" aria-label="Explorateur de la BOM">
      <div className="tbar">
        <div className="seg" role="radiogroup" aria-label="Affichage">
          {treeViews.map((v) => (
            <button key={v.id} type="button" role="radio" aria-checked={p.view === v.id} className={p.view === v.id ? 'on' : ''} onClick={() => p.onView(v.id)}>
              {v.label}
            </button>
          ))}
        </div>
        {p.view !== '3d' && (
          <input
            type="search"
            className="search"
            placeholder="Rechercher une pièce, un ensemble…"
            aria-label="Rechercher dans la BOM"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        )}
        {!data.readOnly && addParent !== null && (
          <button type="button" className="btn pri" onClick={() => p.onAddUnder(addParent)}>
            + Ajouter une pièce
          </button>
        )}
        {data.readOnly && <span className="muted small">Snapshot publié : lecture seule</span>}
      </div>
      {p.moving !== null && (
        <p role="status" className="notice moving-notice">
          Cliquez sur le nouveau parent du nœud #{p.moving}.{' '}
          <button type="button" className="link" onClick={p.onCancelMove}>
            Annuler
          </button>
        </p>
      )}
      <div className={`explorer-body view-${p.view}`}>
        {p.view !== '3d' && tree}
        {p.view !== 'arbre' && slot}
      </div>
      <div className="tfoot muted small">
        {data.readOnly
          ? 'Pour modifier : nouveau brouillon depuis ce snapshot (étape Publier ou Toutes les BOM).'
          : 'Sélectionnez une ligne : + ajoute dessous, ⇅ puis clic sur le nouveau parent déplace, × supprime (avec confirmation) · totaux recalculés par le serveur'}
      </div>
    </section>
  )
}
