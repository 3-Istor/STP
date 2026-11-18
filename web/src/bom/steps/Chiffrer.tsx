import { api } from '../../api.ts'
import { partName } from '../../catalog.ts'
import { FxSensitivityCard, PartAnomaliesCard, PriceDriversCard } from '../analysis/ChiffrerAnalyses.tsx'
import { AnomalySettings } from '../Anomalies.tsx'
import { LandedPanel } from '../LandedPanel.tsx'
import { MakeOrBuyPanel } from '../MakeOrBuyPanel.tsx'
import { PlanningPanel } from '../PlanningPanel.tsx'
import type { BomData } from '../useBomData.ts'
import { Workforce } from '../Workforce.tsx'

// Chiffrer (maquette I4-4) : « Combien, et pourquoi ? ». En tête, les analyses du brouillon (lot E) :
// sensibilité au change, pièces dont le prix pèse le plus, anomalies €/kg par impact. Dessous, plus
// discrètes (repliables), les fonctions existantes : coût rendu d'une pièce, make-or-buy d'un sous-ensemble,
// disponibilité et planning, main-d'œuvre (ETP), anomalies par médiane de catégorie (ADR 0027).
export function Chiffrer({ data, selectedID, onSelect }: { data: BomData; selectedID: number | null; onSelect: (id: number) => void }) {
  const node = selectedID !== null ? data.byID.get(selectedID) : undefined
  const bomID = data.target.id
  const title = node ? (node.part_id !== null ? partName(data.catalog, node.part_id) : node.name) : null
  return (
    <>
      {data.readOnly ? (
        <p className="notice">Snapshot publié : les analyses (change, pièces influentes, anomalies €/kg) se calculent sur un brouillon.</p>
      ) : (
        <>
          <FxSensitivityCard bomID={bomID} version={data.version} />
          <PriceDriversCard bomID={bomID} version={data.version} />
          <PartAnomaliesCard bomID={bomID} version={data.version} onSelect={onSelect} />
        </>
      )}

      <details className="block fold" aria-label="Élément sélectionné">
        <summary>Élément sélectionné{node ? ` · ${title}` : ''} · coût rendu, make-or-buy</summary>
        <div className="in">
          {!node && <p className="muted">Sélectionnez une pièce ou un ensemble (étape Structurer ou barres de la Synthèse).</p>}
          {node && data.readOnly && <p className="muted">Snapshot publié : coût rendu et make-or-buy se simulent sur un brouillon.</p>}
          {node && !data.readOnly && node.part_id !== null && data.bom && (
            <LandedPanel key={`landed-${node.id}-${data.version}`} bomID={bomID} node={node} catalog={data.catalog} destination={data.bom.destination_country} />
          )}
          {node && !data.readOnly && node.part_id === null && (
            <MakeOrBuyPanel key={`mob-${node.id}-${data.version}`} bomID={bomID} nodeID={node.id} currency={data.explanation.currency} />
          )}
          {node && <p className="muted small">« Pourquoi ce coût ? » : colonne de droite, onglet Détail.</p>}
        </div>
      </details>

      <details className="block fold" aria-label="Disponibilité">
        <summary>Disponibilité et planning</summary>
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
      </details>

      <details className="block fold" aria-label="Main-d’œuvre">
        <summary>Main-d’œuvre</summary>
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
      </details>

      {!data.readOnly && (
        <details className="block fold" aria-label="Anomalies de coût">
          <summary>Anomalies par médiane de catégorie (ADR 0027)</summary>
          <div className="in">
            <AnomalySettings open result={data.anomalies.data} error={data.anomalies.error} threshold={data.threshold} onApply={data.applyThreshold} />
          </div>
        </details>
      )}
    </>
  )
}
