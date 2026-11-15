import type { Assembly, Placement, Vec3 } from '../api.ts'

// Couleurs des zones : une teinte par zone, dans l'ordre (trié) des zones de la scène, stable d'un
// rendu à l'autre. Teintes sourdes pour laisser la sélection (bleu primaire) ressortir.
export const zonePalette = ['#6b8bb5', '#b08d57', '#7a9a7e', '#9b7bb0', '#b57b7b', '#6fa3a8', '#8a8a8a']

export function zoneColors(a: Assembly): Map<string, string> {
  return new Map(a.zones.map((z, i) => [z.zone, zonePalette[i % zonePalette.length]]))
}

// explodedPosition : position d'une instance à un niveau d'éclatement donné (0 = assemblé, 1 = complet).
export function explodedPosition(p: Placement, base: Vec3, level: number): Vec3 {
  return [base[0] + p.explode[0] * level, base[1] + p.explode[1] * level, base[2] + p.explode[2] * level]
}

// Libellé du niveau de correspondance : un modèle généré n'est jamais présenté comme exact.
export const matchLabels = { exact: 'modèle exact', similar: 'modèle similaire', generated: 'forme générée' } as const

// visiblePlacements : les pièces des zones non masquées, dans l'ordre de la scène.
export function visiblePlacements(a: Assembly, hiddenZones: ReadonlySet<string>): Placement[] {
  return a.placements.filter((p) => !hiddenZones.has(p.zone))
}
