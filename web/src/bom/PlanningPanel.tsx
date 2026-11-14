import { useState } from 'react'
import type { Planning, Schedule, TreeNode } from '../api.ts'
import { partName, type Catalog } from '../catalog.ts'

type Props = {
  planning: Planning
  nodes: TreeNode[]
  catalog: Catalog
  readOnly: boolean
  onSetSchedule?: (s: Schedule) => void
  onClearSchedule?: () => void
  onSelect: (nodeID: number) => void
}

const dateFormat = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'medium', timeZone: 'UTC' })
const formatDay = (d: string) => dateFormat.format(new Date(`${d}T00:00:00Z`))

// PlanningPanel : date de disponibilité, date cible et pièces qui la mettent en danger (ADR 0023).
// Tout vient de l'API ; les alternatives plus rapides proposées sont les autres offres de la même pièce
// au délai de livraison plus court (suggestion seulement : l'utilisateur change l'offre dans l'arbre).
export function PlanningPanel({ planning, nodes, catalog, readOnly, onSetSchedule, onClearSchedule, onSelect }: Props) {
  const [start, setStart] = useState(planning.schedule?.start_on ?? '')
  const [target, setTarget] = useState(planning.schedule?.target_on ?? '')
  const byID = new Map(nodes.map((n) => [n.id, n]))
  const label = (id: number) => {
    const n = byID.get(id)
    return n?.part_id != null ? partName(catalog, n.part_id) : (n?.name ?? `#${id}`)
  }
  const status = planning.target_met === null ? undefined : planning.target_met ? 'ok' : 'over'

  return (
    <details className="small">
      <summary>
        Disponibilité :{' '}
        <strong className={status}>
          {planning.available_on ? formatDay(planning.available_on) : `${planning.lead_time_days} j (sans date de départ)`}
        </strong>
        {planning.target_met === false && <span className="over"> · cible impossible</span>}
      </summary>

      {readOnly || !onSetSchedule ? (
        <p className="muted">
          {planning.schedule
            ? `Départ ${formatDay(planning.schedule.start_on)}${planning.schedule.target_on ? `, cible ${formatDay(planning.schedule.target_on)}` : ''}`
            : 'Aucun planning figé.'}
        </p>
      ) : (
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault()
            onSetSchedule({ start_on: start, target_on: target === '' ? null : target })
          }}
        >
          <label>
            Départ <input aria-label="Date de départ" type="date" required value={start} onChange={(e) => setStart(e.target.value)} />
          </label>
          <label>
            Cible <input aria-label="Date cible" type="date" min={start || undefined} value={target} onChange={(e) => setTarget(e.target.value)} />
          </label>
          <button type="submit" disabled={start === ''}>
            Appliquer
          </button>
          {planning.schedule && onClearSchedule && (
            <button type="button" className="link" onClick={onClearSchedule}>
              retirer
            </button>
          )}
        </form>
      )}

      {planning.allowed_days !== null && (
        <p className={status}>
          {planning.lead_time_days} j nécessaires pour {planning.allowed_days} j disponibles.
        </p>
      )}
      {planning.late.length > 0 && (
        <>
          <p>Responsables :</p>
          <ul className="explain">
            {planning.late.map((l) => {
              const n = byID.get(l.node_id)
              const current = n?.part_id != null ? catalog.byPart.get(n.part_id)?.offers.find((o) => o.id === n.offer_id) : undefined
              const faster =
                n?.part_id != null && current && !n.totals?.in_stock
                  ? (catalog.byPart.get(n.part_id)?.offers ?? [])
                      .filter((o) => o.lead_time_days < current.lead_time_days)
                      .sort((a, b) => a.lead_time_days - b.lead_time_days)[0]
                  : undefined
              return (
                <li key={l.node_id}>
                  <button type="button" className="link" onClick={() => onSelect(l.node_id)}>
                    {label(l.node_id)}
                  </button>{' '}
                  <span className="over">+{l.overrun_days} j</span>
                  {faster && (
                    <span className="muted">
                      {' '}
                      · offre plus rapide : {faster.supplier} ({faster.lead_time_days} j au lieu de {current!.lead_time_days} j)
                    </span>
                  )}
                </li>
              )
            })}
          </ul>
        </>
      )}
      {planning.critical.length > 0 && (
        <p className="muted">
          Chemin critique :{' '}
          {planning.critical.map((id, i) => (
            <span key={id}>
              {i > 0 && ', '}
              <button type="button" className="link" onClick={() => onSelect(id)}>
                {label(id)}
              </button>
            </span>
          ))}
        </p>
      )}
    </details>
  )
}
