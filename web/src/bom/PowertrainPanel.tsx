import { useState } from 'react'
import { formatDateTime, formatDecimal, formatMoney, formatSignedMoney } from '../money.ts'
import { partName } from '../catalog.ts'
import { powertrainAssumptions, powertrainLabels } from './powertrain.ts'
import type { PowertrainMode } from '../api.ts'
import type { BomData } from './useBomData.ts'
import type { usePowertrainSimulation } from './usePowertrainSimulation.ts'

export function PowertrainPanel({ state, source, onSelect }: { state: ReturnType<typeof usePowertrainSimulation>; source: BomData; onSelect: (mode: PowertrainMode) => void }) {
  const [open, setOpen] = useState(false), [limit, setLimit] = useState(100)
  if (!state.eligible) return null
  const active = state.comparison?.scenarios.find(s => s.key === state.activeMode)
  const reference = state.comparison?.scenarios.find(s => s.key === 'hybrid')
  const referenceNodes = new Map((reference?.tree.nodes ?? source.nodes).map(n => [n.id, n]))
  return <section className="block powertrain-panel" aria-label="Simulation de motorisation">
    <div className="in">
      <h3>Motorisation simulée</h3>
      <div className="row">
        <div className="seg" role="radiogroup" aria-label="Motorisation simulée">
          {(['thermal', 'electric', 'hybrid'] as const).map(mode => <button key={mode} type="button" role="radio" className={state.requestedMode && state.activeMode === mode ? 'on' : ''} aria-checked={state.requestedMode !== null && state.activeMode === mode}
            disabled={state.pending || source.busy} onClick={() => { setLimit(100); onSelect(mode) }}>{powertrainLabels[mode]}</button>)}
        </div>
        {state.requestedMode ? <button type="button" onClick={state.exit}>Quitter la simulation</button> : null}
        {state.comparison || state.error ? <button type="button" onClick={state.refresh} disabled={state.pending}>Actualiser la comparaison</button> : null}
      </div>
      {!state.requestedMode ? <p className="muted small">Choisissez une configuration pour comparer la BOM complète avec les mêmes prix et taux, sans modifier la référence hybride.</p> : null}
      {state.pending ? <p role="status">Calcul de la configuration {powertrainLabels[state.requestedMode!]}… La référence hybride reste affichée jusqu'au résultat.</p> : null}
      {state.error ? <p role="alert">Simulation indisponible : {state.error}. Aucun coût de scénario n'est inventé ; la référence reste affichée.</p> : null}
      {state.comparison ? <>
        <p className="small">Scénario actif : <strong>{powertrainLabels[state.activeMode]}</strong> · lecture seule · référence inchangée.
          {state.comparison.price_as_of ? ` Dernières cotations de l’entrée : ${formatDateTime(state.comparison.price_as_of)}.` : ''}</p>
        <div className="powertrain-table"><table aria-label="Comparaison économique des motorisations">
          <thead><tr><th>Configuration</th><th>Coût / véhicule</th><th>Écart / hybride</th><th>Écart %</th><th>Occurrences pièce</th><th>Masse kg</th><th>CO₂ kg</th><th>Délai</th></tr></thead>
          <tbody>{state.comparison.scenarios.map(s => {
            const root = s.tree.nodes.find(n => n.id === state.comparison!.root_id)?.totals
            const parts = s.tree.nodes.filter(n => n.part_id !== null).map(n => source.catalog.byPart.get(n.part_id!))
            const massMissing = parts.some(p => p?.mass_kg == null), co2Missing = parts.some(p => p?.co2_kg == null)
            return <tr key={s.key} data-scenario={s.key} aria-selected={s.key === state.activeMode}>
              <th>{powertrainLabels[s.key]}</th><td>{root?.cost ? formatMoney(root.cost) : 'Coût incomplet'}</td>
              <td>{s.delta ? formatSignedMoney(s.delta) : 'n/d'}</td><td>{s.delta_percent === null ? 'n/d' : `${formatDecimal(s.delta_percent, 2)} %`}</td>
              <td>{parts.length}</td><td>{root ? `${formatDecimal(root.mass_kg, 1)}${massMissing ? ' (partiel)' : ''}` : 'n/d'}</td>
              <td>{root ? `${formatDecimal(root.co2_kg, 1)}${co2Missing ? ' (partiel)' : ''}` : 'n/d'}</td><td>{root?.lead_time_days ?? 'n/d'} j</td>
            </tr>
          })}</tbody>
        </table></div>
        <p className="small muted">Masse et CO₂ : sommes des données catalogue disponibles ; « partiel » signale un attribut absent. Délai : approvisionnement et production de la BOM active, hors adaptations non conçues.</p>
        {state.activeMode === 'electric' ? <p className="small muted">Simulation électrique : scénario de démonstration avec hypothèses techniques.</p> : null}
        <details><summary>Hypothèses et limites techniques</summary><ul className="small muted">{powertrainAssumptions[state.activeMode].map(a => <li key={a}>{a}</li>)}</ul></details>
        {active ? <details onToggle={e => setOpen(e.currentTarget.open)}><summary>Composants retirés : {active.excluded_ids.length} · composants communs conservés</summary>
          {open ? <div className="powertrain-removed-body" role="region" aria-label="Liste des composants retirés" tabIndex={0}><ul>{active.excluded_ids.slice(0, limit).map(id => { const n = referenceNodes.get(id); return <li key={id}>#{id} · {n ? n.part_id === null ? n.name : partName(source.catalog, n.part_id) : 'Occurrence'} · quantité {n?.quantity ?? 'n/d'}</li> })}</ul>
            {limit < active.excluded_ids.length ? <button type="button" onClick={() => setLimit(n => n + 100)}>Afficher 100 suivantes</button> : null}</div> : null}
        </details> : null}
        {active?.tree.nodes.some(n => n.totals?.missing_cost.length) ? <p role="status">Des prix ou taux manquent : voir les erreurs nommées dans la BOM active. Les coûts de pièces disponibles restent consultables ; aucun total complet ni écart fiable n'est annoncé.</p> : null}
      </> : null}
    </div>
  </section>
}
