import { useCallback, useEffect, useState } from 'react'
import { api, errorMessage, type BOM } from './api.ts'
import { BomList } from './bom/BomList.tsx'
import { BomView } from './bom/BomView.tsx'
import { SnapshotView } from './bom/SnapshotView.tsx'
import { emptyCatalog, loadCatalog, type Catalog } from './catalog.ts'

type Open = { kind: 'bom'; id: number } | { kind: 'snapshot'; id: number } | null

export default function App() {
  const [boms, setBOMs] = useState<BOM[]>([])
  const [catalog, setCatalog] = useState<Catalog>(emptyCatalog)
  const [open, setOpen] = useState<Open>(null)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(() => {
    api
      .listBOMs()
      .then(setBOMs)
      .catch((e: unknown) => setError(errorMessage(e)))
  }, [])

  useEffect(() => {
    refresh()
    loadCatalog()
      .then(setCatalog)
      .catch((e: unknown) => setError(errorMessage(e)))
  }, [refresh])

  async function create(b: { name: string; currency: string; volume: number }) {
    setError(null)
    try {
      const created = await api.createBOM(b)
      refresh()
      setOpen({ kind: 'bom', id: created.id })
    } catch (e) {
      setError(errorMessage(e))
    }
  }

  async function remove(b: BOM) {
    if (!window.confirm(`Supprimer la BOM « ${b.name} » ?`)) return
    setError(null)
    try {
      await api.deleteBOM(b.id)
      if (open?.kind === 'bom' && open.id === b.id) setOpen(null)
      refresh()
    } catch (e) {
      setError(errorMessage(e))
    }
  }

  const openBOM = (id: number) => {
    refresh()
    setOpen({ kind: 'bom', id })
  }

  return (
    <div className="app">
      <BomList
        boms={boms}
        selectedID={open?.kind === 'bom' ? open.id : null}
        onSelect={openBOM}
        onCreate={create}
        onDelete={remove}
      />
      <main>
        {error && (
          <p role="alert" className="error">
            {error}
          </p>
        )}
        {!catalog.available && (
          <p className="muted small">
            Catalogue indisponible (NLPF-67) : seuls les sous-ensembles s'ajoutent. Lancer le serveur avec BOM_DEMO=1
            pour le catalogue de démo.
          </p>
        )}
        {open?.kind === 'bom' && (
          <BomView
            key={open.id}
            bomID={open.id}
            catalog={catalog}
            onBOMChanged={refresh}
            onOpenSnapshot={(id) => setOpen({ kind: 'snapshot', id })}
          />
        )}
        {open?.kind === 'snapshot' && (
          <SnapshotView key={open.id} snapshotID={open.id} catalog={catalog} onOpenBOM={openBOM} />
        )}
        {open === null && <p className="muted">Choisissez ou créez une BOM.</p>}
      </main>
    </div>
  )
}
