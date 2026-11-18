import assert from 'node:assert/strict'
import { test } from 'node:test'
import { staleQuotes } from './liveQuotes.ts'

test('totaux périmés seulement si un tick plus récent que le calcul est connu', () => {
  const cases: [number | null, number | null, boolean][] = [
    [null, 5, false], // calcul sans tick de référence : on ne sait pas, on ne signale rien
    [5, null, false],
    [5, 5, false],
    [5, 6, true],
    [6, 5, false], // tick plus ancien (événement en retard) : rien à signaler
  ]
  for (const [loaded, latest, want] of cases) assert.equal(staleQuotes(loaded, latest), want, `${loaded} → ${latest}`)
})
