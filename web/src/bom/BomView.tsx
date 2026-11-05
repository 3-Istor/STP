import { useCallback, useEffect, useState } from 'react'
import { api, errorMessage, type NewNode, type NodePatch, type Snapshot, type Tree } from '../api.ts'
import type { Catalog } from '../catalog.ts'
import { formatMoney } from '../money.ts'
import { BomTree } from './BomTree.tsx'
import { NodePanel } from './NodePanel.tsx'
import { Totals } from './Totals.tsx'

type Props = {
  bomID: number
  catalog: Catalog
  onBOMChanged: () => void
  onOpenSnapshot: (id: number) => void
}

// BomView : un brouillon — réglages, totaux de la racine, arbre virtualisé et panneau latéral.
// Après chaque action, l'arbre et ses totaux sont relus (pas de recalcul côté front).
export function BomView({ bomID, catalog, onBOMChanged, onOpenSnapshot }: Props) {
  const [tree, setTree] = useState<Tree | null>(null)
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const [moving, setMoving] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [published, setPublished] = useState<Snapshot | null>(null)

  const reload = useCallback(async () => {
    const t = await api.tree(bomID)
    setTree(t)
    return t
  }, [bomID])

  useEffect(() => {
    // La vue est remontée (key) quand on change de BOM : pas d'état à remettre à zéro ici.
    let cancelled = false
    api
      .tree(bomID)
      .then((t) => {
        if (!cancelled) setTree(t)
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(errorMessage(e))
      })
    return () => {
      cancelled = true
    }
  }, [bomID])

  // run exécute une action de l'utilisateur puis relit l'arbre ; une erreur de l'API est affichée telle quelle.
  async function run(action: () => Promise<unknown>, opts: { bomChanged?: boolean } = {}): Promise<boolean> {
    setBusy(true)
    setError(null)
    try {
      await action()
      await reload()
      if (opts.bomChanged) onBOMChanged()
      return true
    } catch (e) {
      setError(errorMessage(e))
      return false
    } finally {
      setBusy(false)
    }
  }

  if (!tree) {
    return <section className="view">{error ? <p role="alert" className="error">{error}</p> : <p>Chargement…</p>}</section>
  }

  const { bom } = tree
  const root = tree.nodes.find((n) => n.parent_id === null)
  const selected = tree.nodes.find((n) => n.id === selectedID) ?? null
  const frozen = bom.price_snapshot_id !== null

  return (
    <section className="view" aria-busy={busy}>
      <header className="view-head">
        <div>
          <h1>
            {bom.name}{' '}
            <button
              type="button"
              className="link small"
              onClick={() => {
                const name = window.prompt('Nouveau nom de la BOM', bom.name)
                if (name && name.trim() !== bom.name) void run(() => api.renameBOM(bom.id, name.trim()), { bomChanged: true })
              }}
            >
              renommer
            </button>
          </h1>
          <Settings
            key={`${bom.currency}-${bom.volume}`}
            currency={bom.currency}
            volume={bom.volume}
            onApply={(c, v) => void run(() => api.updateSettings(bom.id, c, v), { bomChanged: true })}
          />
          {bom.source_snapshot_id !== null && (
            <p className="muted small">
              Créé depuis le{' '}
              <button type="button" className="link" onClick={() => onOpenSnapshot(bom.source_snapshot_id!)}>
                snapshot #{bom.source_snapshot_id}
              </button>
            </p>
          )}
        </div>
        {root?.totals && <Totals totals={root.totals} explanation={tree.explanation} />}
        <div className="actions">
          {frozen ? (
            <button type="button" onClick={() => void run(() => api.unfreezePrices(bom.id), { bomChanged: true })}>
              Repasser en prix en direct
            </button>
          ) : (
            <button type="button" onClick={() => void run(() => api.freezePrices(bom.id), { bomChanged: true })}>
              Figer les prix
            </button>
          )}
          <button
            type="button"
            className="primary"
            onClick={() =>
              void run(async () => {
                setPublished(await api.publish(bom.id))
              })
            }
          >
            Publier
          </button>
        </div>
      </header>

      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {published && (
        <p role="status" className="notice">
          Snapshot #{published.id} publié ({formatMoney(root?.totals?.cost)}).{' '}
          <button type="button" className="link" onClick={() => onOpenSnapshot(published.id)}>
            Ouvrir
          </button>
        </p>
      )}
      {moving !== null && (
        <p role="status" className="notice">
          Cliquez sur le nouveau parent du nœud #{moving}.{' '}
          <button type="button" className="link" onClick={() => setMoving(null)}>
            Annuler
          </button>
        </p>
      )}

      <div className="workspace">
        <BomTree
          nodes={tree.nodes}
          catalog={catalog}
          readOnly={false}
          selectedID={selectedID}
          onSelect={setSelectedID}
          onChangeOffer={(nodeID, offerID) => void run(() => api.patchNode(bom.id, nodeID, { offer_id: offerID }))}
          moving={moving}
          onPickParent={(parentID) => {
            const nodeID = moving!
            setMoving(null)
            void run(() => api.patchNode(bom.id, nodeID, { parent_id: parentID }))
          }}
        />
        {selected ? (
          <NodePanel
            key={selected.id}
            node={selected}
            explanation={tree.explanation}
            volume={bom.volume}
            catalog={catalog}
            readOnly={false}
            isRoot={selected.parent_id === null}
            onAdd={(n: NewNode) => run(() => api.addNode(bom.id, n))}
            onPatch={(p: NodePatch) => run(() => api.patchNode(bom.id, selected.id, p))}
            onStartMove={() => setMoving(selected.id)}
            onDelete={() => {
              if (!window.confirm('Supprimer ce nœud et tout son sous-arbre ?')) return
              setSelectedID(selected.parent_id)
              void run(() => api.deleteNode(bom.id, selected.id))
            }}
          />
        ) : (
          <aside className="panel muted">Sélectionnez un nœud pour voir son détail.</aside>
        )}
      </div>
    </section>
  )
}

function Settings({
  currency,
  volume,
  onApply,
}: {
  currency: string
  volume: number
  onApply: (currency: string, volume: number) => void
}) {
  const [c, setC] = useState(currency)
  const [v, setV] = useState(String(volume))
  const changed = c.trim().toUpperCase() !== currency || v !== String(volume)
  return (
    <form
      className="row small"
      onSubmit={(e) => {
        e.preventDefault()
        onApply(c.trim().toUpperCase(), Number.parseInt(v, 10))
      }}
    >
      <label>
        Devise <input aria-label="Devise de référence" size={4} pattern="[A-Za-z]{3}" value={c} onChange={(e) => setC(e.target.value)} />
      </label>
      <label>
        Volume <input aria-label="Volume" type="number" min={1} step={1} value={v} onChange={(e) => setV(e.target.value)} />
      </label>
      {changed && <button type="submit">Appliquer</button>}
    </form>
  )
}
