import type { PriceSnapshotRef } from '../api.ts'
import { formatDateTime } from '../money.ts'

// Nom d'un snapshot de prix dans les sélecteurs : son instant figé à la seconde près (deux snapshots
// pris à quelques secondes d'écart restent distinguables), son périmètre et son numéro.
export function priceSnapshotName(s: PriceSnapshotRef): string {
  return `${formatDateTime(s.taken_at)} · ${scopeLabel(s)} · #${s.id}`
}

function scopeLabel(s: PriceSnapshotRef): string {
  switch (s.scope) {
    case 'catalog':
      return 'tout le catalogue'
    case 'bom':
      return `BOM n°${s.scope_id}`
    case 'supplier':
      return `fournisseur n°${s.scope_id}`
    case 'offer':
      return `offre n°${s.scope_id}`
  }
}
