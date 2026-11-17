/** Courbe commune aux déplacements visuels ; les extrémités sont atteintes exactement. */
export function motionProgress(now: number, start: number, duration: number): number {
  if (duration <= 0) return 1
  const t = Math.min(Math.max((now - start) / duration, 0), 1)
  return t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2
}
