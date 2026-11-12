import { useEffect, useState } from 'react'
import { api, errorMessage, type Published } from '../api.ts'
import type { Catalog } from '../catalog.ts'
import { formatDate } from '../money.ts'
import { BomTree } from './BomTree.tsx'
import { NodePanel } from './NodePanel.tsx'
import { Totals } from './Totals.tsx'

// SnapshotView : snapshot publié, en lecture seule (S2). Pour le modifier : nouveau brouillon (D3).
export function SnapshotView({
  snapshotID,
  catalog,
  onOpenBOM,
}: {
  snapshotID: number
  catalog: Catalog
  onOpenBOM: (id: number) => void
}) {
  const [data, setData] = useState<Published | null>(null)
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    api
      .snapshot(snapshotID)
      .then((d) => {
        if (!cancelled) setData(d)
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(errorMessage(e))
      })
    return () => {
      cancelled = true
    }
  }, [snapshotID])

  if (!data) {
    return <section className="view">{error ? <p role="alert" className="error">{error}</p> : <p>Chargement…</p>}</section>
  }
  const { snapshot: s } = data
  const root = data.nodes.find((n) => n.parent_id === null)
  const selected = data.nodes.find((n) => n.id === selectedID) ?? null

  return (
    <section className="view">
      <header className="view-head">
        <div>
          <h1>
            {s.name} <span className="badge">snapshot #{s.id}</span>
          </h1>
          <p className="muted small">
            Publié le {formatDate(s.published_at)} · {s.currency} · {s.volume} u. · lecture seule
          </p>
        </div>
        {root?.totals && <Totals totals={root.totals} explanation={data.explanation} />}
        <div className="actions">
          <button
            type="button"
            className="primary"
            onClick={() => {
              setError(null)
              api
                .newDraft(s.id)
                .then((b) => onOpenBOM(b.id))
                .catch((e: unknown) => setError(errorMessage(e)))
            }}
          >
            Nouveau brouillon depuis ce snapshot
          </button>
        </div>
      </header>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      <div className="bom-workspace">
        <BomTree
          nodes={data.nodes}
          catalog={catalog}
          readOnly
          selectedID={selectedID}
          onSelect={setSelectedID}
          onChangeOffer={() => {}}
          moving={null}
          onPickParent={() => {}}
        />
        {selected ? (
          <NodePanel
            key={selected.id}
            node={selected}
            explanation={data.explanation}
            volume={s.volume}
            catalog={catalog}
            readOnly
            isRoot={selected.parent_id === null}
            onAdd={async () => false}
            onPatch={async () => false}
            onStartMove={() => {}}
            onDelete={() => {}}
          />
        ) : (
          <aside className="panel muted">Sélectionnez un nœud pour voir son détail.</aside>
        )}
      </div>
    </section>
  )
}
