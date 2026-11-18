// staleQuotes : vrai quand le flux de prix a validé un tick plus récent que celui des totaux affichés.
// On compare les ticks, pas les dates : l'horloge simulée boucle sur l'année (ADR 0043), le tick ne fait que croître.
export function staleQuotes(loadedTick: number | null, latestTick: number | null): boolean {
  return loadedTick !== null && latestTick !== null && latestTick > loadedTick
}
