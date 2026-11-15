import type { TreeNode } from '../api.ts'
import { byName, normalize } from '../text.ts'

export type Row = { node: TreeNode; depth: number; hasChildren: boolean; expanded: boolean }

// flatten : parcours en profondeur itératif (pas de récursion : 30 000 nœuds sur une seule branche ne
// font pas déborder la pile). Les frères sont triés par libellé (ordre alphabétique, puis id pour un
// ordre stable) : c'est un ordre d'affichage, les totaux sont calculés côté serveur. Les nœuds repliés
// cachent leur sous-arbre.
//
// Avec une recherche (query non vide), seuls restent les nœuds dont le libellé correspond, leurs ancêtres
// (dépliés de force pour montrer le chemin) et leurs descendants (selon l'état replié).
export function flatten(
  nodes: TreeNode[],
  collapsed: ReadonlySet<number>,
  label: (n: TreeNode) => string = (n) => n.name,
  query = '',
): Row[] {
  const children = new Map<number, TreeNode[]>()
  const byID = new Map<number, TreeNode>()
  let root: TreeNode | undefined
  for (const n of nodes) {
    byID.set(n.id, n)
    if (n.parent_id === null) {
      root = n
      continue
    }
    const list = children.get(n.parent_id)
    if (list) list.push(n)
    else children.set(n.parent_id, [n])
  }
  const labels = new Map(nodes.map((n) => [n.id, label(n)]))
  for (const list of children.values()) list.sort((a, b) => byName.compare(labels.get(a.id)!, labels.get(b.id)!) || a.id - b.id)
  const rows: Row[] = []
  if (!root) return rows

  const q = normalize(query)
  const matches = new Set<number>()
  const onPath = new Set<number>() // ancêtres d'un nœud trouvé
  if (q !== '') {
    for (const n of nodes) {
      if (!normalize(labels.get(n.id)!).includes(q)) continue
      matches.add(n.id)
      for (let p = n.parent_id; p !== null && !onPath.has(p); p = byID.get(p)?.parent_id ?? null) onPath.add(p)
    }
    if (matches.size === 0) return rows
  }

  const stack: { node: TreeNode; depth: number; underMatch: boolean }[] = [{ node: root, depth: 0, underMatch: false }]
  while (stack.length > 0) {
    const { node, depth, underMatch } = stack.pop()!
    const kids = children.get(node.id) ?? []
    const matched = q === '' || underMatch || matches.has(node.id)
    if (!matched && !onPath.has(node.id)) continue
    // Un ancêtre d'un résultat reste ouvert ; ailleurs, l'état replié de l'utilisateur s'applique.
    const open = q !== '' && onPath.has(node.id) ? true : !collapsed.has(node.id)
    rows.push({ node, depth, hasChildren: kids.length > 0, expanded: open && kids.length > 0 })
    if (!open) continue
    const childUnderMatch = q !== '' && (underMatch || matches.has(node.id))
    for (let i = kids.length - 1; i >= 0; i--) stack.push({ node: kids[i], depth: depth + 1, underMatch: childUnderMatch })
  }
  return rows
}

// initiallyCollapsed replie les nœuds à partir de la profondeur `depth` : à l'ouverture, seuls les
// premiers niveaux de l'arbre sont dépliés.
export function initiallyCollapsed(nodes: TreeNode[], depth: number): Set<number> {
  const all = flatten(nodes, new Set())
  return new Set(all.filter((r) => r.hasChildren && r.depth >= depth).map((r) => r.node.id))
}
