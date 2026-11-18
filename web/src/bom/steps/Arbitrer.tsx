import { partName } from '../../catalog.ts'
import { api } from '../../api.ts'
import { ComparisonPanel } from '../Comparison.tsx'
import { BomTree } from '../BomTree.tsx'
import { StrategyPanel } from '../StrategyPanel.tsx'
import { BudgetPlanPanel } from '../BudgetPlanPanel.tsx'
import type { BomData } from '../useBomData.ts'

// Arbitrer (maquette I4-6) : « Que garder ? ». Comparaison de deux versions (par défaut la dernière publiée →
// la version ouverte) avec les causes de l'écart (ADR 0032) et les pièces modifiées ; puis la stratégie d'un
// nœud (ADR 0028) et ses alternatives classées (stratégie, sinon profil de véhicule, ADR 0036), appliquées au
// brouillon après confirmation. Les pistes de réduction sont dans la colonne de droite.
export function Arbitrer({ data, selectedID, onSelect, onGoStructurer }: { data: BomData; selectedID: number | null; onSelect: (id: number) => void; onGoStructurer: () => void }) {
  const node = selectedID !== null ? data.byID.get(selectedID) : undefined
  return (
    <>
      <ComparisonPanel data={data} onSelect={onSelect} />
      <section className="block" aria-label="Arbre de la BOM">
        <h3>Arbre · choisir un nœud</h3>
        <div className="in" style={{ height: 320, display: 'flex' }}>
          {/* Arbre en sélection seule (readOnly) : on y choisit le nœud dont on règle la stratégie plus bas. */}
          <BomTree
            key={data.target.kind + data.target.id}
            nodes={data.nodes}
            catalog={data.catalog}
            readOnly
            selectedID={selectedID}
            onSelect={onSelect}
            anomalies={data.anomalyByNode}
            query=""
            onChangeOffer={() => {}}
            moving={null}
            onPickParent={() => {}}
          />
        </div>
      </section>
      {node && !data.readOnly && data.budgets.byNode.get(node.id)?.state === 'over' && <BudgetPlanPanel key={node.id} data={data} nodeID={node.id} />}
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
            <p className="muted">
              Aucun nœud n’est sélectionné : sa stratégie ou le profil de véhicule classe les offres de ses pièces. Choisissez un nœud dans les causes
              ci-dessus, ou{' '}
              <button type="button" className="link" onClick={onGoStructurer}>
                ouvrez l’arbre dans Structurer
              </button>
              .
            </p>
          )}
        </div>
      </section>
    </>
  )
}
