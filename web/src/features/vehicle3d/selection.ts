import { objectsForNode, type VehicleIndex } from './mapping.ts'
import type { Vehicle3DNode } from './types.ts'

/** Le modèle V2 ne représente que des sous-ensembles : aucun mesh n'atteste une pièce physique. */
export function sceneSelection(index: VehicleIndex, node: Vehicle3DNode | undefined, missing: ReadonlySet<string> = new Set()): { keys: string[]; unavailable: boolean } {
  if (!node) return { keys: [], unavailable: false }
  if (index.representation === 'manifest') {
    const matches = new Map<number, boolean>()
    const belongs = (id: number): boolean => {
      const path: number[] = []
      let current: number | null = id
      const seen = new Set<number>()
      while (current !== null && current !== node.id && !matches.has(current) && !seen.has(current)) {
        seen.add(current); path.push(current); current = index.parentOf.get(current) ?? null
      }
      const value = current === node.id || (current !== null && matches.get(current) === true)
      path.forEach(p => matches.set(p, value))
      return value
    }
    const keys = index.objects.filter(o => (o.nodeId === node.id || (node.part_id === null && belongs(o.nodeId))) && !missing.has(o.key)).map(o => o.key)
    return { keys, unavailable: keys.length === 0 }
  }
  if (node.part_id !== null) return { keys: [], unavailable: true }
  const represented = node.id === index.rootId ? index.objects.map(o => o.key) : objectsForNode(index, node.id)
  const keys = represented.filter(key => !missing.has(key))
  return { keys, unavailable: keys.length === 0 }
}

/** Localisation distincte de la pièce : uniquement par ascendance BOM, jamais par nom/référence. */
export function locateContainingAssembly(index: VehicleIndex, node: Vehicle3DNode | undefined, nodes: Vehicle3DNode[] | ReadonlyMap<number, Vehicle3DNode>, missing: ReadonlySet<string> = new Set()): { nodeId: number; name: string; keys: string[] } | null {
  if (!node || index.representation !== 'manifest' || !sceneSelection(index, node, missing).unavailable) return null
  const byId = Array.isArray(nodes) ? new Map(nodes.map(n => [n.id, n])) : nodes
  const seen = new Set<number>([node.id])
  let id = node.parent_id
  while (id !== null && id !== index.rootId && !seen.has(id)) {
    seen.add(id)
    const parent = byId.get(id)
    if (!parent) return null
    const { keys } = sceneSelection(index, parent, missing)
    if (keys.length) return { nodeId: id, name: parent.name || `Sous-ensemble #${id}`, keys }
    id = parent.parent_id
  }
  return null
}
