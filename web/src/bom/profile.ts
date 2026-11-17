// Profils de véhicule (ADR 0036) : libellés et préréglages lus dans l'API (GET /vehicle-profiles), une fois
// par page. Les poids sont des entiers (somme 100) validés par l'API ; le front ne classe rien.
import { useEffect, useState } from 'react'
import { api, errorMessage, type ProfilePreset, type Weights } from '../api.ts'

export const WEIGHT_KEYS: { key: keyof Weights; label: string }[] = [
  { key: 'cost', label: 'Coût' },
  { key: 'co2', label: 'CO₂' },
  { key: 'lead_time', label: 'Délai' },
  { key: 'mass', label: 'Masse' },
  { key: 'quality', label: 'Qualité' },
]

let cached: Promise<ProfilePreset[]> | null = null

export function usePresets(): { presets: ProfilePreset[] | null; error: string | null } {
  const [state, setState] = useState<{ presets: ProfilePreset[] | null; error: string | null }>({ presets: null, error: null })
  useEffect(() => {
    let cancelled = false
    cached ??= api.profilePresets()
    cached.then(
      (presets) => {
        if (!cancelled) setState({ presets, error: null })
      },
      (e: unknown) => {
        cached = null // relu à la prochaine demande
        if (!cancelled) setState({ presets: null, error: errorMessage(e) })
      },
    )
    return () => {
      cancelled = true
    }
  }, [])
  return state
}

/** « coût 40 · CO₂ 25 · délai 20 · masse 5 · qualité 10 » (critères à poids nul omis). */
export function weightsText(w: Weights): string {
  return WEIGHT_KEYS.filter((k) => w[k.key] > 0)
    .map((k) => `${k.label.toLowerCase()} ${w[k.key]}`)
    .join(' · ')
}
