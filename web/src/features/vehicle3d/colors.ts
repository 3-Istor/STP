// Couleurs de la vue 3D (AGENTS §5 : vert / orange / rouge = --ok / --warn / --over). Logique pure.
import type { BudgetStatus, ColorBy, Vehicle3DNode } from './types.ts'

export type StatusPalette = { ok: string; warn: string; over: string }
/** Valeurs de repli, identiques à web/src/index.css, si les variables CSS ne sont pas définies. */
export const DEFAULT_STATUS: StatusPalette = { ok: '#15734b', warn: '#a85d0b', over: '#b83240' }
/** Statut absent : gris « indisponible », jamais une couleur de réussite (ADR 0018). */
export const UNAVAILABLE = '#c9ced6'
const NEUTRAL_BODY = '#d3d7dd'
const SYSTEM_PALETTE = ['#8a96a8', '#6f8fbf', '#b08a4f', '#7a9a7e', '#9a7fb0', '#6aa5a6', '#a8877a', '#7f8fa0']

export function systemColor(index: number): string {
  return SYSTEM_PALETTE[((index % SYSTEM_PALETTE.length) + SYSTEM_PALETTE.length) % SYSTEM_PALETTE.length]
}

function parseHex(hex: string): [number, number, number] {
  const h = hex.trim().replace('#', '')
  const full = h.length === 3 ? h.split('').map(c => c + c).join('') : h
  if (!/^[0-9a-fA-F]{6}$/.test(full)) throw new Error(`couleur invalide : ${hex}`)
  return [0, 2, 4].map(i => parseInt(full.slice(i, i + 2), 16)) as [number, number, number]
}

/** Mélange linéaire a → b (t = 0 → a, t = 1 → b), résultat « #rrggbb ». */
export function mixHex(a: string, b: string, t: number): string {
  const ca = parseHex(a), cb = parseHex(b)
  return '#' + ca.map((v, i) => Math.round(v + (cb[i] - v) * t).toString(16).padStart(2, '0')).join('')
}

/**
 * « ok » est éclairci vers le gris carrosserie pour que les dépassements ressortent (comme les maquettes) ;
 * « warn » et « over » gardent la couleur exacte des variables CSS.
 */
export function budgetColor(status: BudgetStatus | null, palette: StatusPalette = DEFAULT_STATUS): string {
  switch (status) {
    case 'ok': return mixHex(palette.ok, NEUTRAL_BODY, 0.7)
    case 'warn': return palette.warn
    case 'over': return palette.over
    default: return UNAVAILABLE
  }
}

/**
 * Coloration effective : « budget » seulement si au moins un nœud porte le champ budget_status (l'API le
 * fournit) ; sinon repli sur les systèmes avec un avis à afficher, jamais un vert inventé.
 */
export function resolveColorMode(colorBy: ColorBy, nodes: Vehicle3DNode[]): { mode: ColorBy; notice: string | null } {
  if (colorBy !== 'budget') return { mode: colorBy, notice: null }
  const provided = nodes.some(n => n.budget_status !== undefined)
  return provided
    ? { mode: 'budget', notice: null }
    : { mode: 'system', notice: 'Statut budget absent des données : couleurs par système.' }
}
