// useLoad : lecture d'une ressource de l'API, relue quand `key` change ; seule la dernière réponse compte.
// Pas de cache : chaque écran d'analyse relit l'API (les calculs sont faits par le serveur).
import { useEffect, useState } from 'react'
import { errorMessage } from '../api.ts'

export type Loaded<T> = { status: 'loading' } | { status: 'ready'; data: T } | { status: 'error'; error: string }

export function useLoad<T>(key: string | null, load: () => Promise<T>): Loaded<T> {
  const [state, setState] = useState<{ key: string | null; value: Loaded<T> }>({ key: null, value: { status: 'loading' } })
  useEffect(() => {
    if (key === null) return
    let cancelled = false
    load().then(
      (data) => {
        if (!cancelled) setState({ key, value: { status: 'ready', data } })
      },
      (e: unknown) => {
        if (!cancelled) setState({ key, value: { status: 'error', error: errorMessage(e) } })
      },
    )
    return () => {
      cancelled = true
    }
    // load est recréée à chaque rendu : la clé dit quand relire.
  }, [key])
  return state.key === key ? state.value : { status: 'loading' }
}
