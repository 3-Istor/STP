import profiles from './data/provided-scenes.json' with { type: 'json' }
import type { SceneScope } from './manifest.ts'
import type { Vehicle3DNode } from './types.ts'

/** Choix du mode initial pour les installations vérifiées ; aucune liaison métier par nom ici. */
export function hasProvidedScene(scope: SceneScope, nodes: Vehicle3DNode[]): boolean {
  const profile = profiles.find(p => p.kind === scope.kind && p.id === scope.id)
  if (!profile) return false
  const byId = new Map(nodes.map(n => [n.id, n]))
  const root = byId.get(profile.root_id)
  return root?.parent_id === null && root.name === profile.root_name && profile.anchor_ids.every(id => {
    const n = byId.get(id)
    return n?.parent_id === root.id && n.part_id === null
  })
}

/** Variantes locales explicitement préparées ; le service Go reste garant de la compatibilité. */
export function providedVariantParts(scope: SceneScope, node: Vehicle3DNode): number[] {
  const profile = profiles.find(p => p.kind === scope.kind && p.id === scope.id)
  const variant = profile?.variants?.find(v => v.node_id === node.id && v.parent_id === node.parent_id && node.part_id !== null && v.part_ids.includes(node.part_id))
  return variant?.part_ids ?? []
}
