import { useEffect, useState } from 'react'
import { api, errorMessage, type PriceSnapshotRef } from '../api.ts'
import { priceSnapshotName } from './priceSnapshots.ts'

type Props = {
  bomID: number
  priceSnapshotID: number | null
  onChange: (action: () => Promise<unknown>) => void
}

// PriceSource : source des prix du brouillon (features.md, bom-draft-and-snapshots). Soit en direct, soit
// un snapshot de prix choisi dans la liste des snapshots existants (nommés par leur instant à la seconde),
// soit « Figer maintenant » qui crée un nouveau snapshot des offres de la BOM. L'API refuse un snapshot qui
// ne couvre pas la BOM (offres manquantes listées) : l'erreur est affichée par BomView, la BOM ne bouge pas.
export function PriceSource({ bomID, priceSnapshotID, onChange }: Props) {
  const [snapshots, setSnapshots] = useState<PriceSnapshotRef[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    // Relue à chaque changement de source : un gel vient d'ajouter un snapshot à la liste.
    let cancelled = false
    api
      .listPriceSnapshots()
      .then((list) => {
        if (!cancelled) setSnapshots(list)
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(errorMessage(e))
      })
    return () => {
      cancelled = true
    }
  }, [priceSnapshotID])

  // Un snapshot attaché mais absent de la liste (liste en échec) reste sélectionnable : jamais caché.
  const current = priceSnapshotID !== null && !snapshots?.some((s) => s.id === priceSnapshotID) ? priceSnapshotID : null
  return (
    <div className="row small price-source">
      <label>
        Prix{' '}
        <select
          aria-label="Source des prix"
          value={priceSnapshotID ?? ''}
          disabled={snapshots === null && error === null}
          onChange={(e) => {
            const value = e.target.value
            onChange(() => (value === '' ? api.unfreezePrices(bomID) : api.usePriceSnapshot(bomID, Number.parseInt(value, 10))))
          }}
        >
          <option value="">en direct</option>
          {current !== null && <option value={current}>snapshot de prix #{current}</option>}
          {(snapshots ?? []).map((s) => (
            <option key={s.id} value={s.id}>
              {priceSnapshotName(s)}
            </option>
          ))}
        </select>
      </label>
      <button type="button" onClick={() => onChange(() => api.freezePrices(bomID))}>
        Figer maintenant
      </button>
      {error && <span className="error">Snapshots de prix indisponibles : {error}</span>}
    </div>
  )
}
