import { objectsForNode, type VehicleIndex } from './mapping.ts'
import type { Vehicle3DNode } from './types.ts'

/** Le modèle V2 ne représente que des sous-ensembles : aucun mesh n'atteste une pièce physique. */
export function sceneSelection(index: VehicleIndex, node: Vehicle3DNode | undefined, missing: ReadonlySet<string> = new Set()): { keys: string[]; unavailable: boolean } {
  if (!node) return { keys: [], unavailable: false }
  if (node.part_id !== null) return { keys: [], unavailable: true }
  const represented = node.id === index.rootId ? index.objects.map(o => o.key) : objectsForNode(index, node.id)
  const keys = represented.filter(key => !missing.has(key))
  return { keys, unavailable: keys.length === 0 }
}
