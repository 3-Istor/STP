import { useState } from 'react'
import type { Totals } from '../api.ts'
import { decimalSign, formatDecimal, formatSignedMoney } from '../money.ts'
import { computeImpact, isNoImpact, type Impact } from './impact.ts'

const signed = (value: string, digits: number, unit: string) =>
  decimalSign(value) === 0 ? `0 ${unit}` : `${decimalSign(value) < 0 ? '−' : '+'} ${formatDecimal(value.replace(/^-/, ''), digits)} ${unit}`

// useLastImpact : écart des totaux de la racine entre deux relectures de l'arbre (version). Une relecture suit une
// modification de l'utilisateur (ajout, quantité, offre, attributs d'une pièce) ; les ticks de prix ne relisent pas.
export function useLastImpact(version: number, totals: Totals | null | undefined): Impact | null {
  const [state, setState] = useState<{ version: number; totals: Totals | null | undefined; impact: Impact | null }>({ version, totals, impact: null })
  if (version !== state.version) {
    setState({ version, totals, impact: state.totals && totals ? computeImpact(state.totals, totals) : state.impact })
  }
  return state.impact
}

export function LastImpact({ impact }: { impact: Impact | null }) {
  if (!impact) return null
  return (
    <p className="last-impact small" role="status">
      <b>Impact de la dernière modification</b> :{' '}
      {isNoImpact(impact) ? (
        'aucun changement des totaux.'
      ) : (
        <>
          coût {impact.cost ? `${formatSignedMoney(impact.cost)} / véhicule` : 'indisponible (coût manquant ou devises différentes)'} · CO₂ {signed(impact.co2_kg, 2, 'kg')} · masse{' '}
          {signed(impact.mass_kg, 2, 'kg')} · délai {impact.lead_time_days === 0 ? '0 j' : `${impact.lead_time_days < 0 ? '−' : '+'} ${Math.abs(impact.lead_time_days)} j`}
        </>
      )}
    </p>
  )
}
