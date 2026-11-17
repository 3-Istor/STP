import SuppliersPage from '../../suppliers/SuppliersPage.tsx'
import { Pending } from '../../shell/Pending.tsx'

type Props = {
  tab: string | null
  onTab: (tab: string) => void
  supplier: number | null
  onSupplier: (id: number) => void
}

const SOURCER_TABS = [
  { id: 'apercu', label: 'Vue d’ensemble' },
  { id: 'fournisseurs', label: 'Fournisseurs, offres et contrats' },
]

// Sourcer (maquettes I4-5, I4-5b) : « Chez qui, à quelles conditions ? ». L'onglet Fournisseurs reprend
// toute la page Fournisseurs de la V1 (création, offres, contrats et remises, historique des prix).
export function Sourcer({ tab, onTab, supplier, onSupplier }: Props) {
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
        <>
          <Pending title="Origine du coût par pays · poids des 5 premiers fournisseurs · pièces mono-source" lot="lot E">
            Parts par pays et par fournisseur, contrats expirés encore utilisés.{' '}
            <button type="button" className="link" onClick={() => onTab('fournisseurs')}>
              Voir les fournisseurs, offres et contrats
            </button>
          </Pending>
          <Pending title="Change · taux budget et taux courant" lot="lot E" />
        </>
      ) : (
        <SuppliersPage selectedID={supplier} onSelect={onSupplier} />
      )}
    </>
  )
}
