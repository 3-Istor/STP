import { Component, useMemo, useState, type ReactNode } from 'react'
import { api } from '../../api.ts'
import { isStaleChunkError } from '../../features/vehicle3d/chunkError.ts'
import { Vehicle3D, type ColorBy, type Vehicle3DNode } from '../../features/vehicle3d/index.tsx'
import { BomTree } from '../BomTree.tsx'
import type { BomData } from '../useBomData.ts'
import { boundVehicle3dKey, vehicle3dNodes } from '../vehicle3dNodes.ts'

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

// Structurer (maquettes I4-1, I4-2, I4-3) : l'explorateur de la BOM en grand. Arbre virtualisé (NLPF-69),
// changement d'offre en ligne, ajout avec choix de l'offre, déplacement, suppression ; bascule Arbre /
// Arbre + 3D / 3D. La vue 3D (ADR 0035, chargée à la demande avec `three`) partage la sélection de l'arbre
// par l'URL (?node=) : clic dans l'arbre → objet encadré dans la 3D, clic dans la 3D → ligne de l'arbre.
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
      onCancelMove={p.onCancelMove}
      onAddUnder={data.readOnly ? undefined : p.onAddUnder}
      onStartMove={p.onStartMove}
      onDelete={p.onDelete}
    />
  )
  const view3d = p.view !== 'arbre' ? <Vehicle3DPane key={'vehicle-' + data.target.kind + data.target.id} data={data} selectedID={p.selectedID} onSelect={p.onSelect} /> : null
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
        {data.readOnly && <span className="muted small">{data.simulation ? 'Scénario de conception : lecture seule' : 'Snapshot publié : lecture seule'}</span>}
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
        {view3d}
      </div>
      <div className="tfoot muted small">
        {data.readOnly
          ? data.simulation ? 'Quittez la simulation pour modifier la BOM de référence.' : 'Pour modifier : nouveau brouillon depuis ce snapshot (étape Publier ou Toutes les BOM).'
          : 'Sélectionnez une ligne : + ajoute dessous, ⇅ puis clic sur le nouveau parent déplace, × supprime (avec confirmation) · totaux recalculés par le serveur'}
      </div>
    </section>
  )
}

const colorModes: { id: ColorBy; label: string }[] = [
  { id: 'material', label: 'Matériaux' },
  { id: 'budget', label: 'Budget' },
  { id: 'system', label: 'Système' },
]

// Vehicle3DPane : contrôles de la maquette I4-2 (Rendu, Colorer, Éclatement) au-dessus de la vue 3D. Focus
// (I4-3) : double-clic sur un sous-ensemble, « Sortir du focus » ou Échap.
function Vehicle3DPane({ data, selectedID, onSelect }: { data: BomData; selectedID: number | null; onSelect: (id: number) => void }) {
  const [colorBy, setColorBy] = useState<ColorBy>('material')
  const [explode, setExplode] = useState(0)
  const budgets = data.budgets.unavailable === null ? data.budgets.byNode : null
  const next = useMemo(() => vehicle3dNodes(data.nodes, budgets, data.catalog.byPart), [data.nodes, budgets, data.catalog.byPart])
  const key = useMemo(() => boundVehicle3dKey(next), [next])
  // Même scène tant que la structure et les statuts ne changent pas (les totaux, eux, changent à chaque relecture).
  const [stable, setStable] = useState<{ key: string; nodes: Vehicle3DNode[] }>({ key, nodes: next })
  if (stable.key !== key) setStable({ key, nodes: next })
  return (
    <div className="v3d-pane" role="region" aria-label="Vue 3D">
      <div className="v3d-controls">
        <span className="muted small">Colorer</span>
        <div className="seg" role="radiogroup" aria-label="Colorer">
          {colorModes.map((m) => (
            <button key={m.id} type="button" role="radio" aria-checked={colorBy === m.id} className={colorBy === m.id ? 'on' : ''} onClick={() => setColorBy(m.id)}>
              {m.label}
            </button>
          ))}
        </div>
        <label className="v3d-explode small">
          <span className="muted">Éclatement</span>
          <input type="range" min={0} max={100} step={5} value={Math.round(explode * 100)} onChange={(e) => setExplode(Number(e.target.value) / 100)} aria-label="Éclatement" />
        </label>
        <span className="muted small v3d-hint">Double-clic : focus sur un sous-ensemble</span>
      </div>
      <div className="v3d-stage">
        <Vehicle3DBoundary>
          <Vehicle3D bomId={data.target.id} sceneScope={data.target} nodes={stable.nodes} selectedNodeId={selectedID} onSelectNode={onSelect} mode="bound" simulationMode={data.simulation?.pending ? undefined : data.simulation?.mode} colorBy={colorBy} explode={explode} onResetView={() => setExplode(0)} />
        </Vehicle3DBoundary>
      </div>
    </div>
  )
}

// Vehicle3DBoundary : une panne de la 3D (pas de WebGL, chunk `three` non chargé) reste dans son panneau ;
// l'arbre, la Synthèse et le reste de l'étape continuent de fonctionner.
class Vehicle3DBoundary extends Component<{ children: ReactNode }, { error: string | null }> {
  state = { error: null as string | null }
  static getDerivedStateFromError(e: unknown) {
    return { error: e instanceof Error ? e.message : String(e) }
  }
  render() {
    if (this.state.error === null) return this.props.children
    if (isStaleChunkError(this.state.error)) {
      return (
        <div className="v3d-failed" role="alert">
          <p><strong>Une nouvelle version de l’application est disponible.</strong> Rechargez la page pour afficher la vue 3D.</p>
          <button type="button" className="v3d-button" onClick={() => window.location.reload()}>Recharger la page</button>
        </div>
      )
    }
    return (
      <div className="v3d-failed" role="alert">
        <p>
          <strong>Vue 3D indisponible</strong> : {this.state.error}
        </p>
        <p className="muted small">WebGL est peut-être désactivé dans ce navigateur. L’arbre et les totaux restent utilisables.</p>
      </div>
    )
  }
}
