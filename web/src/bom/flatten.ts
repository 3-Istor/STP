import type { TreeNode } from '../api.ts'

export type Row = { node: TreeNode; depth: number; hasChildren: boolean }

// flatten : parcours en profondeur itératif (pas de récursion : 30 000 nœuds sur une seule branche ne
// font pas déborder la pile), enfants triés par id comme le calcul des totaux. Les nœuds repliés cachent
// leur sous-arbre.
export function flatten(nodes: TreeNode[], collapsed: ReadonlySet<number>): Row[] {
  const children = new Map<number, TreeNode[]>()
  let root: TreeNode | undefined
  for (const n of nodes) {
    if (n.parent_id === null) {
      root = n
      continue
    }
    const list = children.get(n.parent_id)
    if (list) list.push(n)
    else children.set(n.parent_id, [n])
  }
  for (const list of children.values()) list.sort((a, b) => a.id - b.id)
  const rows: Row[] = []
  if (!root) return rows
  const stack: { node: TreeNode; depth: number }[] = [{ node: root, depth: 0 }]
  while (stack.length > 0) {
    const { node, depth } = stack.pop()!
    const kids = children.get(node.id) ?? []
    rows.push({ node, depth, hasChildren: kids.length > 0 })
    if (collapsed.has(node.id)) continue
    for (let i = kids.length - 1; i >= 0; i--) stack.push({ node: kids[i], depth: depth + 1 })
  }
  return rows
}
