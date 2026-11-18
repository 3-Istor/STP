import SuppliersPage from '../../suppliers/SuppliersPage.tsx'
import { SourcingOverview } from '../analysis/SourcingOverview.tsx'

type Props = {
  tab: string | null
  onTab: (tab: string) => void
  supplier: number | null
  onSupplier: (id: number) => void
  bomID: number
  version: number
  onOpenSupplier: (id: number) => void // onglet Fournisseurs, fournisseur choisi (une seule navigation)
  readOnly: boolean // snapshot publié : les analyses se calculent sur un brouillon (E12)
  onSaved: () => void // contrat ou offre enregistré : la BOM est relue (totaux, contrôles, badges)
}

const SOURCER_TABS = [
  { id: 'apercu', label: 'Vue d’ensemble' },
  { id: 'fournisseurs', label: 'Fournisseurs, offres et contrats' },
]

// Sourcer (maquettes I4-5, I4-5b) : « Chez qui, à quelles conditions ? ». L'onglet Fournisseurs reprend
// toute la page Fournisseurs de la V1 (création, offres, contrats et remises, historique des prix).
export function Sourcer({ tab, onTab, supplier, onSupplier, onOpenSupplier, bomID, version, readOnly, onSaved }: Props) {
  const current = tab === 'fournisseurs' ? 'fournisseurs' : 'apercu'
  return (
    <>
      <div className="seg tabs-seg" role="tablist" aria-label="Sourcer">
        {SOURCER_TABS.map((t) => (
          <button key={t.id} type="button" role="tab" aria-selected={current === t.id} className={current === t.id ? 'on' : ''} onClick={() => onTab(t.id)}>
            {t.label}
          </button>
        ))}
      </div>
      {current === 'apercu' ? (
        readOnly ? (
          <p className="notice">Snapshot publié : la vue d’ensemble du sourcing se calcule sur un brouillon.</p>
        ) : (
          <SourcingOverview
            bomID={bomID}
            version={version}
            onSupplier={onOpenSupplier}
          />
        )
      ) : (
        <SuppliersPage selectedID={supplier} onSelect={onSupplier} onSaved={onSaved} />
      )}
    </>
  )
}
