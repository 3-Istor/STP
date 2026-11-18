// Nœuds transmis à la vue 3D (ADR 0035) : la structure de l'arbre et, si l'API fournit les budgets, le statut
// budget de chaque nœud (ADR 0018, calculé par le back). Logique pure, testée par `node --test`.
import type { NodeBudget, TreeNode } from '../api.ts'
import type { BudgetStatus, Vehicle3DNode } from '../features/vehicle3d/types.ts'

/**
 * budget_status : état du budget (ok / warn / over) du nœud s'il en porte un, sinon celui du sous-ensemble
 * budgété le plus proche au-dessus de lui : une pièce de la porte avant gauche prend la couleur du budget de
 * la porte, pas un gris « sans budget » (ADR 0018 : la règle reste au back, ici on lit seulement l'état).
 * La racine ne se propage pas : un dépassement global ne colore pas toute la voiture d'un même rouge.
 * null si aucun budget ne couvre le nœud, ou si le budget le plus proche est indisponible (gris, jamais
 * vert) ; champ absent quand l'API ne fournit pas de budgets (snapshot) : la 3D colore alors par système.
 */
export function vehicle3dNodes(nodes: TreeNode[], budgets: ReadonlyMap<number, NodeBudget> | null, parts?: ReadonlyMap<number, { name: string }>): Vehicle3DNode[] {
  const parentOf = new Map(nodes.map((n) => [n.id, n.parent_id]))
  const governing = (id: number): NodeBudget | undefined => {
    const own = budgets?.get(id)
    if (own) return own
    const seen = new Set<number>([id])
    let cur = parentOf.get(id) ?? null
    while (cur !== null && !seen.has(cur)) {
      seen.add(cur)
      if (parentOf.get(cur) === null) return undefined // racine : ne se propage pas
      const b = budgets?.get(cur)
      if (b) return b
      cur = parentOf.get(cur) ?? null // parent absent de la liste : on s'arrête, sans rien inventer
    }
    return undefined
  }
  return nodes.map((n) => {
    const base: Vehicle3DNode = { id: n.id, parent_id: n.parent_id, name: n.part_id === null ? n.name : (parts?.get(n.part_id)?.name ?? n.name), part_id: n.part_id, quantity: n.quantity }
    if (budgets === null) return base
    const st = governing(n.id)?.state
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

/** Les placements liés dépendent aussi des quantités, contrairement au schéma historique. */
export function boundVehicle3dKey(nodes: Vehicle3DNode[]): string {
  return JSON.stringify(nodes.map(n => [n.id, n.parent_id, n.part_id, n.name, n.quantity, n.budget_status]))
}
