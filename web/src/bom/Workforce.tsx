import { useState } from 'react'
import type { TreeNode } from '../api.ts'
import { partName, type Catalog } from '../catalog.ts'
import { decimalInput, formatDecimal, sameDecimal } from '../money.ts'

type Props = {
  nodes: TreeNode[]
  annualHours: string | null // null : snapshot publié avant ADR 0024
  catalog: Catalog
  readOnly: boolean
  onApplyAnnualHours?: (hours: string) => void
  open?: boolean // déplié d'emblée (carte de l'étape Chiffrer)
}

// Workforce : ETP par sous-système (enfants directs de la racine, ADR 0024), à la demande, et base horaire.
// Les ETP viennent de l'API : aucun calcul côté front.
export function Workforce({ nodes, annualHours, catalog, readOnly, onApplyAnnualHours, open = false }: Props) {
  const [hours, setHours] = useState(annualHours ?? '')
  const [hoursError, setHoursError] = useState<string | null>(null)
  const root = nodes.find((n) => n.parent_id === null)
  const subsystems = root ? nodes.filter((n) => n.parent_id === root.id) : []
  return (
    <details className="small" open={open}>
      <summary>ETP par sous-système</summary>
      {readOnly || !onApplyAnnualHours ? (
        <p className="muted">Base horaire : {annualHours ? `${formatDecimal(annualHours, 2)} h/an` : 'non figée'}</p>
      ) : (
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault()
            const h = decimalInput(hours)
            if (!/^\d+(\.\d+)?$/.test(h) || Number(h) <= 0) return setHoursError('La base horaire est un nombre positif d’heures par an (ex. 1600).')
            onApplyAnnualHours(h)
          }}
        >
          <label>
            Base horaire (h/an){' '}
            <input aria-label="Base horaire annuelle" inputMode="decimal" size={6} value={hours} aria-invalid={hoursError !== null} onChange={(e) => { setHours(e.target.value); setHoursError(null) }} />
          </label>
          {(annualHours === null || !sameDecimal(decimalInput(hours), annualHours)) && <button type="submit">Appliquer</button>}
          {hoursError && <span role="alert" className="over">{hoursError}</span>}
        </form>
      )}
      {subsystems.length === 0 ? (
        <p className="muted">Aucun sous-système.</p>
      ) : (
        <table className="compact">
          <thead>
            <tr>
              <th>Sous-système</th>
              <th>h / véhicule</th>
              <th>ETP</th>
            </tr>
          </thead>
          <tbody>
            {subsystems.map((n) => (
              <tr key={n.id}>
                <td>{n.part_id !== null ? partName(catalog, n.part_id) : n.name}</td>
                <td>{n.totals?.labor_hours ? formatDecimal(n.totals.labor_hours, 2) : 'n/d'}</td>
                <td className={n.totals?.labor_error ? 'over' : undefined} title={n.totals?.labor_error ?? undefined}>
                  {n.totals?.fte ? formatDecimal(n.totals.fte, 2) : 'n/d'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </details>
  )
}
