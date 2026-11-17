import type { TreeNode } from '../api.ts'

// API attendue de la vue 3D (lot D, ADR 0035) : le lot D remplace ce composant sans toucher à l'étape
// Structurer. La sélection est partagée avec l'arbre (et l'URL) par selectedNodeId / onSelectNode.
export type Vehicle3DProps = {
  bomId: number
  nodes: TreeNode[]
  selectedNodeId: number | null
  onSelectNode: (id: number) => void
}

export function Vehicle3DSlot({ selectedNodeId }: Vehicle3DProps) {
  return (
    <div className="view3d" role="region" aria-label="Vue 3D">
      <p>
        <strong>Vue 3D à venir</strong>
      </p>
      <p className="muted small">
        Livrée par le lot D. Elle suivra la même sélection que l’arbre{selectedNodeId !== null ? ` (nœud #${selectedNodeId})` : ''}.
      </p>
    </div>
  )
}
