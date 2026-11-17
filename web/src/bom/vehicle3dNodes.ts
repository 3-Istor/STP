// Nœuds transmis à la vue 3D (ADR 0035) : la structure de l'arbre et, si l'API fournit les budgets, le statut
// budget de chaque nœud (ADR 0018, calculé par le back). Logique pure, testée par `node --test`.
import type { NodeBudget, TreeNode } from '../api.ts'
import type { BudgetStatus, Vehicle3DNode } from '../features/vehicle3d/types.ts'

/**
 * budget_status : état du budget propre du nœud (ok / warn / over) ; null si le nœud n'a pas de budget ou si
 * la comparaison est indisponible (gris « indisponible » dans la 3D, jamais vert) ; champ absent quand
 * l'API ne fournit pas de budgets (snapshot) : la 3D colore alors par système et le dit.
 * Un sous-ensemble sans budget propre n'hérite pas du statut d'un ancêtre : ce serait affirmer un
 * dépassement (ou un respect) que personne n'a calculé pour lui.
 */
export function vehicle3dNodes(nodes: TreeNode[], budgets: ReadonlyMap<number, NodeBudget> | null): Vehicle3DNode[] {
  return nodes.map((n) => {
    const base: Vehicle3DNode = { id: n.id, parent_id: n.parent_id, name: n.name, part_id: n.part_id }
    if (budgets === null) return base
    const st = budgets.get(n.id)?.state
    const status: BudgetStatus | null = st === 'ok' || st === 'warn' || st === 'over' ? st : null
    return { ...base, budget_status: status }
  })
}

/**
 * Clé de ce que la 3D affiche : structure (ids, parents, noms des sous-ensembles) et statuts. Les totaux
 * changent à chaque relecture de l'arbre sans changer la scène : la clé évite de reconstruire la 3D (et de
 * relire le modèle réaliste) pour rien.
 */
export function vehicle3dKey(nodes: Vehicle3DNode[]): string {
  let key = ''
  for (const n of nodes) {
    key += `${n.id}/${n.parent_id ?? ''}/${n.part_id ?? ''}/${n.part_id === null ? n.name : ''}/${n.budget_status === undefined ? '-' : (n.budget_status ?? '0')};`
  }
  return key
}
