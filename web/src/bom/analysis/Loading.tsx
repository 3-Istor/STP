import type { ReactNode } from 'react'
import type { Loaded } from '../useLoad.ts'

// Show : état d'une lecture d'analyse — calcul en cours, erreur de l'API affichée telle quelle, ou contenu.
export function Show<T>({ value, label, children }: { value: Loaded<T>; label: string; children: (data: T) => ReactNode }) {
  if (value.status === 'loading')
    return (
      <p className="muted small recalc" role="status">
        Calcul en cours (serveur)…
      </p>
    )
  if (value.status === 'error')
    return (
      <p className="error small" role="alert">
        {label} indisponible : {value.error}
      </p>
    )
  return <>{children(value.data)}</>
}
