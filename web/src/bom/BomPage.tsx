import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, errorMessage, type BOM } from '../api.ts'
import { BomList } from './BomList.tsx'
import { BomView } from './BomView.tsx'
import { SnapshotView } from './SnapshotView.tsx'
import { emptyCatalog, loadCatalog, type Catalog } from '../catalog.ts'
import { currencyChoices, loadCurrencies } from '../currencies.ts'
import './bom.css'

type Open = { kind: 'bom'; id: number } | { kind: 'snapshot'; id: number } | null

export default function BomPage() {
  const [boms, setBOMs] = useState<BOM[]>([])
  const [catalog, setCatalog] = useState<Catalog>(emptyCatalog)
  const [open, setOpen] = useState<Open>(null)
  const [error, setError] = useState<string | null>(null)
  const [fxCurrencies, setFxCurrencies] = useState<string[]>([])

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
    // Taux illisibles : le choix se limite aux devises des BOM existantes, l'erreur est montrée.
    loadCurrencies()
      .then(setFxCurrencies)
      .catch((e: unknown) => setError(errorMessage(e)))
  }, [refresh])

  const currencies = useMemo(() => currencyChoices(fxCurrencies, boms.map((b) => b.currency)), [fxCurrencies, boms])

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
    <div className="bom-space">
    <div className="app">
      <BomList
        boms={boms}
        currencies={currencies}
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
            Catalogue indisponible : seuls les sous-ensembles s'ajoutent.
          </p>
        )}
        {open?.kind === 'bom' && (
          <BomView
            key={open.id}
            bomID={open.id}
            catalog={catalog}
            currencies={currencies}
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
    </div>
  )
}
