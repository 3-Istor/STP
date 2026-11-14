import { useState } from 'react'
import type { CostAnomalies, NodeAnomaly } from '../api.ts'
import { decimalInput } from '../money.ts'
import { describeAnomaly } from './anomaly.ts'

// AnomalySettings : seuil des anomalies de coût (3× par défaut) et résumé ; le calcul est fait par l'API.
export function AnomalySettings({
  result,
  error,
  threshold,
  onApply,
}: {
  result: CostAnomalies | null
  error: string | null
  threshold: string
  onApply: (threshold: string) => void
}) {
  const [value, setValue] = useState(threshold)
  const count = result?.nodes.filter((n) => n.status === 'anomaly').length ?? 0
  const notEvaluable = result?.nodes.filter((n) => n.status === 'not_evaluable').length ?? 0
  return (
    <details className="small">
      <summary>
        Anomalies de coût :{' '}
        {error ? (
          <span className="muted">indisponibles</span>
        ) : (
          // Vert seulement si tout a été évalué : un « non évaluable » n'est jamais un succès (ADR 0027).
          <strong className={count > 0 ? 'over' : notEvaluable > 0 || !result ? undefined : 'ok'}>{count}</strong>
        )}
        {notEvaluable > 0 && <span className="muted"> · {notEvaluable} non évaluable(s)</span>}
      </summary>
      {error && <p className="error">{error}</p>}
      <form
        className="row"
        onSubmit={(e) => {
          e.preventDefault()
          onApply(decimalInput(value))
        }}
      >
        <label>
          Seuil (× médiane) <input aria-label="Seuil d'anomalie" inputMode="decimal" size={4} value={value} onChange={(e) => setValue(e.target.value)} />
        </label>
        <button type="submit">Appliquer</button>
      </form>
    </details>
  )
}

// AnomalyFacts : verdict d'un nœud pièce dans le panneau latéral, avec ce sur quoi il repose.
export function AnomalyFacts({ anomaly }: { anomaly: NodeAnomaly | undefined }) {
  if (!anomaly) return null
  return (
    <dl className="facts">
      <dt>Coût / catégorie</dt>
      <dd className={anomaly.status === 'anomaly' ? 'over' : undefined}>
        {anomaly.status === 'not_evaluable'
          ? `non évaluable : ${anomaly.reason ?? ''}`
          : `${describeAnomaly(anomaly)}${anomaly.status === 'anomaly' ? ' : anomalie' : ''}`}
      </dd>
    </dl>
  )
}
