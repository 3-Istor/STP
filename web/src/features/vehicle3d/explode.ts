// Vecteurs d'éclaté (ADR 0035), portés de docs/v2/prototype-3d/car-realiste.html. Logique pure, sans `three`.
import type { ExplodeOptions } from './mapping.ts'

export type V3 = [number, number, number]

/**
 * Direction d'éclaté d'un objet depuis le centre de sa boîte englobante : du centre du véhicule vers l'objet,
 * jamais vers le sol, latéral renforcé ; à multiplier par le facteur d'éclatement (0..1).
 */
export function explodeVector(center: V3, vehicleCenter: V3, opts: ExplodeOptions = {}): V3 {
  if (opts.fixed) return [0, 0, 0]
  const k = 0.55
  const dx = (center[0] - vehicleCenter[0]) * 0.6 * (opts.xScale ?? 1)
  const dy = Math.max(center[1] - vehicleCenter[1], 0) + 0.15 + (opts.lift ?? 0)
  const dz = (center[2] - vehicleCenter[2]) * 1.6
  return [dx * k, dy * k, dz * k]
}

export type AxialPart = { id: string; axial: number }

/**
 * Focus : éclatement dans l'axe du sous-ensemble (latéral pour un objet sur un côté, sinon longitudinal,
 * sinon vertical). Les pièces s'écartent par rang `axial` croissant (intérieur → extérieur), à pas fixes
 * proportionnels à la longueur du véhicule ; les pièces de même rang partent ensemble.
 */
export function axialOffsets(parts: AxialPart[], focusCenter: V3, vehicleCenter: V3, length: number): Map<string, V3> {
  const dx = focusCenter[0] - vehicleCenter[0]
  const dz = focusCenter[2] - vehicleCenter[2]
  const side = 0.08 * length
  let axis: V3
  if (Math.abs(dz) > side) axis = [0, 0, Math.sign(dz)]
  else if (Math.abs(dx) > side) axis = [Math.sign(dx), 0, 0]
  else axis = [0, 1, 0]
  const ranks = [...new Set(parts.map(p => p.axial))].sort((a, b) => a - b)
  const out = new Map<string, V3>()
  for (const p of parts) {
    const i = ranks.indexOf(p.axial)
    const d = length * (0.1 + 0.12 * i)
    out.set(p.id, [axis[0] * d, axis[1] * d, axis[2] * d])
  }
  return out
}
