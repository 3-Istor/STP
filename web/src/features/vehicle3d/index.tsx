// Point d'entrée de la vue 3D (ADR 0035). Ce fichier ne charge pas `three` : le composant réel et `three`
// arrivent dans un chunk séparé, au premier affichage (React.lazy + import dynamique).
import { lazy, Suspense } from 'react'
import type { Vehicle3DProps } from './types.ts'

export type { BudgetStatus, ColorBy, RenderMode, Vehicle3DNode, Vehicle3DProps } from './types.ts'

const Vehicle3DView = lazy(() => import('./Vehicle3DView.tsx'))

export function Vehicle3D(props: Vehicle3DProps) {
  return (
    <Suspense fallback={<div className="v3d-root v3d-loading" role="status">Chargement de la vue 3D…</div>}>
      <Vehicle3DView {...props} />
    </Suspense>
  )
}
