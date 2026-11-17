// Contrat public de la vue 3D (voir ADR 0035). Aucun import de `three` ici : ce fichier est chargé avec le shell.
import type { TreeNode } from '../../api.ts'

export type RenderMode = 'simple' | 'realistic'
export type ColorBy = 'budget' | 'system' | 'material'
/** Statut budget d'un nœud, calculé par le back (ADR 0018) ; la 3D ne le calcule jamais. */
export type BudgetStatus = 'ok' | 'warn' | 'over'

/** Nœud lu par la 3D : un TreeNode, éventuellement enrichi d'un statut budget quand l'API le fournit. */
export type Vehicle3DNode = Pick<TreeNode, 'id' | 'parent_id' | 'name' | 'part_id'> & {
  budget_status?: BudgetStatus | null
}

export type Vehicle3DProps = {
  bomId: number
  nodes: Vehicle3DNode[]
  selectedNodeId: number | null
  onSelectNode: (id: number) => void
  mode: RenderMode
  colorBy: ColorBy
  /** 0 = assemblé, 1 = éclaté maximal. */
  explode: number
}
