import profile from './data/powertrain-valhalla.json' with { type: 'json' }
import type { ConfigurationRequest, PowertrainMode, TreeNode } from '../api.ts'

export const powertrainLabels: Record<PowertrainMode, string> = { thermal: 'Thermique', electric: 'Électrique', hybrid: 'Hybride' }
export const powertrainAssumptions: Record<PowertrainMode, string[]> = {
  hybrid: ['Architecture de référence : tous les composants de la BOM sont conservés.'],
  thermal: ['Chaîne électrique retirée. DCT, transmission et refroidissement conservés par hypothèse commune. Compatibilité technique des adaptations non certifiée.'],
  electric: ['Simulation de conception, pas une Valhalla électrique constructeur.', 'Batterie hybride conservée pour chiffrer les composants disponibles ; capacité adaptée, autonomie et remplacement BEV non établis, ajouts nécessaires non chiffrés.', 'DCT, transmission et refroidissement conservés par hypothèse commune ; adaptations non dimensionnées.'],
}

/** Installation explicite de démonstration ; les identités sont revérifiées par le service Go. */
export function powertrainRequest(target: { kind: 'draft' | 'snapshot'; id: number }, nodes: TreeNode[]): ConfigurationRequest | null {
  if (target.kind !== 'draft' || target.id !== profile.bom_id) return null
  const byId = new Map(nodes.map(n => [n.id, n]))
  if (!profile.guards.every(g => { const n = byId.get(g.id); return n?.parent_id === g.parent_id && n.part_id === g.part_id })) return null
  return { root_id: profile.root_id, guards: profile.guards, scenarios: profile.scenarios.map(s => ({ key: s.key as PowertrainMode, excluded_roots: s.excluded_roots })) }
}
