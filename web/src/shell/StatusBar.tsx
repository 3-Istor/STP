import { useEffect, useState } from 'react'
import { api, errorMessage, type PricingStatus } from '../api.ts'
import type { Theme } from '../ui/theme.ts'

// Relecture de l'état de la source de prix : assez souvent pour voir une panne, sans charger l'API.
const STATUS_POLL_MS = 30_000

// usePricingStatus : état de l'API de prix (GET /api/v1/pricing/status) ; injoignable = erreur nommée.
function usePricingStatus() {
  const [state, setState] = useState<{ status: PricingStatus | null; error: string | null }>({ status: null, error: null })
  useEffect(() => {
    let cancelled = false
    const read = () =>
      api
        .pricingStatus()
        .then((status) => !cancelled && setState({ status, error: null }))
        .catch((e: unknown) => !cancelled && setState({ status: null, error: errorMessage(e) }))
    void read()
    const timer = window.setInterval(() => void read(), STATUS_POLL_MS)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [])
  return state
}

type Props = { theme: Theme; onToggleTheme: () => void; items: string[]; right: string | null }

// StatusBar : barre d'état du bas (maquettes I4) — bouton thème à gauche, informations de la BOM, état de
// l'API de prix, devise et volume à droite.
export function StatusBar({ theme, onToggleTheme, items, right }: Props) {
  const pricing = usePricingStatus()
  const ok = pricing.status?.status === 'ready'
  return (
    <footer className="status">
      <button type="button" className="themebtn" onClick={onToggleTheme} aria-label={theme === 'dark' ? 'Passer au thème clair' : 'Passer au thème sombre'}>
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
          <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
        </svg>
        {theme === 'dark' ? 'Thème clair' : 'Thème sombre'}
      </button>
      {items.map((t) => (
        <span key={t}>{t}</span>
      ))}
      <span title={pricing.error ?? (pricing.status ? `jeu ${pricing.status.dataset_version ?? '?'} · tick ${pricing.status.tick ?? '?'}` : undefined)}>
        <span className={`dot ${pricing.status === null && pricing.error === null ? 'na' : ok ? 'ok' : 'over'}`} aria-hidden="true" />{' '}
        {pricing.error ? 'API de prix injoignable' : pricing.status === null ? 'API de prix…' : ok ? 'API de prix joignable' : `API de prix : ${pricing.status.status}`}
      </span>
      <span className="sp" />
      {right && <span>{right}</span>}
    </footer>
  )
}
