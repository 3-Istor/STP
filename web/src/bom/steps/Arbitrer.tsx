import { partName } from '../../catalog.ts'
import { api } from '../../api.ts'
import { Pending } from '../../shell/Pending.tsx'
import { StrategyPanel } from '../StrategyPanel.tsx'
import type { BomData } from '../useBomData.ts'

// Arbitrer (maquette I4-6) : « Que garder ? ». Fonction existante : stratégie d'un nœud (ADR 0028) et
// alternatives classées, appliquées au brouillon après confirmation. Comparaison de versions, pont de coût
// et pistes de réduction : lots B et F.
export function Arbitrer({ data, selectedID }: { data: BomData; selectedID: number | null }) {
  const node = selectedID !== null ? data.byID.get(selectedID) : undefined
  return (
    <>
      <section className="block" aria-label="Stratégie et alternatives">
        <h3>Stratégie et alternatives{node ? ` · ${node.part_id !== null ? partName(data.catalog, node.part_id) : node.name}` : ''}</h3>
        <div className="in">
          {node ? (
            <StrategyPanel
              key={`strategy-${node.id}-${data.version}`}
              bomID={data.target.id}
              node={node}
              catalog={data.catalog}
              readOnly={data.readOnly}
              onPatch={(p) => data.run(() => api.patchNode(data.target.id, node.id, p))}
            />
          ) : (
            <p className="muted">Sélectionnez un nœud : sa stratégie (coût, délai, CO₂, équilibrée) classe les offres de ses pièces.</p>
          )}
        </div>
      </section>
      <Pending title="Comparer deux versions · pourquoi le coût a bougé" lot="lot B">
        Comparaison brouillon / snapshot avec les causes d’écart (change, contrats, hausses, structure, offres).
      </Pending>
      <Pending title="Pistes de réduction" lot="lot B">
        Idées, gain estimé, statut et brouillon associé.
      </Pending>
    </>
  )
}
