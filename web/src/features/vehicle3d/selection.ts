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
