import { api } from '../../api.ts'
import { partName } from '../../catalog.ts'
import { Pending } from '../../shell/Pending.tsx'
import { AnomalySettings } from '../Anomalies.tsx'
import { LandedPanel } from '../LandedPanel.tsx'
import { MakeOrBuyPanel } from '../MakeOrBuyPanel.tsx'
import { PlanningPanel } from '../PlanningPanel.tsx'
import type { BomData } from '../useBomData.ts'
import { Workforce } from '../Workforce.tsx'

// Chiffrer (maquette I4-4) : « Combien, et pourquoi ? ». Regroupe les fonctions de chiffrage existantes —
// anomalies de coût, disponibilité et planning, main-d'œuvre (ETP), coût rendu d'une pièce, make-or-buy
// d'un sous-ensemble. Sensibilité au change, pièces qui pèsent le plus et anomalies €/kg : lot E.
export function Chiffrer({ data, selectedID, onSelect }: { data: BomData; selectedID: number | null; onSelect: (id: number) => void }) {
  const node = selectedID !== null ? data.byID.get(selectedID) : undefined
  const bomID = data.target.id
  const title = node ? (node.part_id !== null ? partName(data.catalog, node.part_id) : node.name) : null
  return (
    <>
      <section className="block" aria-label="Élément sélectionné">
        <h3>{node ? `Pourquoi ce coût ? · ${title}` : 'Élément sélectionné'}</h3>
        <div className="in">
          {!node && <p className="muted">Sélectionnez une pièce ou un ensemble (étape Structurer ou barres de la Synthèse).</p>}
          {node && data.readOnly && <p className="muted">Snapshot publié : coût rendu et make-or-buy se simulent sur un brouillon.</p>}
          {node && !data.readOnly && node.part_id !== null && data.bom && (
            <LandedPanel key={`landed-${node.id}-${data.version}`} bomID={bomID} node={node} catalog={data.catalog} destination={data.bom.destination_country} />
          )}
          {node && !data.readOnly && node.part_id === null && (
            <MakeOrBuyPanel key={`mob-${node.id}-${data.version}`} bomID={bomID} nodeID={node.id} currency={data.explanation.currency} />
          )}
          {node && <p className="muted small">Cascade complète prix → change → remise → douane → transport → outillage : lot A.</p>}
        </div>
      </section>

      <div className="grid2">
        <section className="block" aria-label="Disponibilité">
          <h3>Disponibilité et planning</h3>
          <div className="in">
            {data.planning ? (
              <PlanningPanel
                key={`plan-${data.planning.schedule?.start_on ?? ''}-${data.planning.schedule?.target_on ?? ''}`}
                open
                planning={data.planning}
                nodes={data.nodes}
                catalog={data.catalog}
                readOnly={data.readOnly}
                onSetSchedule={(sch) => void data.run(() => api.setSchedule(bomID, sch))}
                onClearSchedule={() => void data.run(() => api.clearSchedule(bomID))}
                onSelect={onSelect}
              />
            ) : (
              <p className="muted">Planning absent de ce snapshot (publié avant ADR 0023).</p>
            )}
          </div>
        </section>
        <section className="block" aria-label="Main-d’œuvre">
          <h3>Main-d’œuvre</h3>
          <div className="in">
            <Workforce
              key={`fte-${data.annualHours}`}
              open
              nodes={data.nodes}
              annualHours={data.annualHours}
              catalog={data.catalog}
              readOnly={data.readOnly}
              onApplyAnnualHours={data.readOnly ? undefined : (h) => void data.run(() => api.setAnnualHours(bomID, h), { bomChanged: true })}
            />
          </div>
        </section>
      </div>

      {!data.readOnly && (
        <section className="block" aria-label="Anomalies de coût">
          <h3>Anomalies de coût</h3>
          <div className="in">
            <AnomalySettings open result={data.anomalies.data} error={data.anomalies.error} threshold={data.threshold} onApply={data.applyThreshold} />
          </div>
        </section>
      )}

      <Pending title="Sensibilité au change et aux matières · pièces dont le prix pèse le plus · anomalies €/kg par famille" lot="lot E" />
    </>
  )
}
