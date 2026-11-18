import { useEffect, useState } from 'react'
import { api, errorMessage, type BudgetPlan, type PlanAction } from '../api.ts'
import { formatDecimal, formatMoney } from '../money.ts'
import { useAppDialog } from '../useAppDialog.tsx'
import { materialChange, planLabel, planPatch, signed } from './budgetPlan.ts'
import type { BomData } from './useBomData.ts'

// Revenir dans le budget (ADR 0052) : pour un nœud au-dessus de son budget, les substitutions d'offre ou de pièce
// proposées par l'API, de la plus rentable à la moins rentable, avec le gain cumulé. Rien n'est appliqué d'office :
// chaque action s'applique au brouillon après confirmation, puis le plan est relu avec les nouveaux coûts.
export function BudgetPlanPanel({ data, nodeID }: { data: BomData; nodeID: number }) {
  const bomID = data.target.id
  const [plan, setPlan] = useState<BudgetPlan | null>(null)
  const [error, setError] = useState<string | null>(null)
  const dialogs = useAppDialog()
  // Relu à chaque relecture de l'arbre (data.version) : après une application, les coûts et le plan changent.
  useEffect(() => {
    let cancelled = false
    api
      .budgetPlan(bomID, nodeID)
      .then((p) => {
        if (cancelled) return
        setPlan(p)
        setError(null)
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(errorMessage(e))
      })
    return () => {
      cancelled = true
    }
  }, [bomID, nodeID, data.version])

  const apply = (a: PlanAction) => {
    const n = a.nodes.length
    void dialogs
      .confirm({
        title: `Appliquer : ${planLabel(a)} ?`,
        message: `Remplace l’offre sur ${n} nœud${n > 1 ? 's' : ''} du brouillon. Gain attendu ${formatMoney(a.gain)} par véhicule.`,
        confirmLabel: 'Appliquer',
      })
      .then((ok) => {
        if (!ok) return
        // Un PATCH par nœud, dans l'ordre ; le premier refus arrête la série et s'affiche tel quel.
        void data.run(async () => {
          for (const node of a.nodes) await api.patchNode(bomID, node.id, planPatch(a))
        })
      })
  }

  return (
    <section className="block" aria-label="Revenir dans le budget">
      {dialogs.element}
      <h3>Revenir dans le budget</h3>
      <div className="in">
        {error && <p className="error">{error}</p>}
        {!error && plan === null && <p className="muted">Calcul du plan…</p>}
        {plan && <PlanBody plan={plan} readOnly={data.readOnly} busy={data.busy} onApply={apply} />}
      </div>
    </section>
  )
}

function PlanBody({ plan, readOnly, busy, onApply }: { plan: BudgetPlan; readOnly: boolean; busy: boolean; onApply: (a: PlanAction) => void }) {
  if (plan.actions.length === 0 && plan.covered) return <p className="muted">Ce nœud est dans son budget ({formatMoney(plan.cost)} pour {formatMoney(plan.budget)}).</p>
  return (
    <>
      <p>
        Coût {formatMoney(plan.cost)} pour un budget de {formatMoney(plan.budget)} : <strong className="over">dépassement de {formatMoney(plan.overrun)}</strong> par véhicule.
      </p>
      {plan.actions.length === 0 ? (
        <p className="muted">Aucune offre ni pièce alternative moins chère : le dépassement reste entier.</p>
      ) : (
        <>
          <p className="muted small">
            Propositions classées par gain par véhicule (prix d’achat × quantité réelle) : le plan ne modifie rien, chaque action s’applique au brouillon.
          </p>
          <table className="compact">
            <thead>
              <tr>
                <th>Action</th>
                <th>Gain</th>
                <th>Cumulé</th>
                <th>CO₂</th>
                <th>Masse</th>
                <th>Délai</th>
                <th>Qualité</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {plan.actions.map((a, i) => {
                const material = materialChange(a)
                return (
                  <tr key={`${a.from.offer_id}-${a.to.offer_id}`}>
                    <td title={a.nodes.map((n) => `nœud #${n.id}`).join(', ')}>
                      {planLabel(a)}
                      {material && <div className="muted small">matière {material}</div>}
                    </td>
                    <td>{formatMoney(a.gain)}</td>
                    <td className={a.covers ? 't-ok' : undefined}>
                      {formatMoney(a.cumulative_gain)}
                      {a.covers && <div className="small">couvre le dépassement</div>}
                    </td>
                    <td>{signed(a.co2_delta_kg, (v) => `${formatDecimal(v, 2)} kg`)}</td>
                    <td>{signed(a.mass_delta_kg, (v) => `${formatDecimal(v, 2)} kg`)}</td>
                    <td>{signed(String(a.lead_time_delta_days), (v) => `${v} j`)}</td>
                    <td>{a.from.quality !== null || a.to.quality !== null ? `${a.from.quality ?? 'n/d'} → ${a.to.quality ?? 'n/d'}` : 'n/d'}</td>
                    <td>
                      {!readOnly && (
                        <button type="button" disabled={busy} onClick={() => onApply(a)} aria-label={`Appliquer l’action ${i + 1}`}>
                          Appliquer
                        </button>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
          {!plan.covered && (
            <p className="over">
              Même en appliquant toutes les actions, il reste {formatMoney(plan.remaining)} par véhicule à trouver (autre fournisseur, quantité, conception).
            </p>
          )}
        </>
      )}
    </>
  )
}
