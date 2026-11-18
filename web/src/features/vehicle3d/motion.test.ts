import assert from 'node:assert/strict'
import { test } from 'node:test'
import { motionProgress } from './motion.ts'

test('transition : bornes, interpolation continue et arrivée exacte', () => {
  const cases = [
    { now: 50, start: 100, duration: 400, want: 0 },
    { now: 100, start: 100, duration: 400, want: 0 },
    { now: 200, start: 100, duration: 400, want: 0.125 },
    { now: 300, start: 100, duration: 400, want: 0.5 },
    { now: 400, start: 100, duration: 400, want: 0.875 },
    { now: 500, start: 100, duration: 400, want: 1 },
    { now: 900, start: 100, duration: 400, want: 1 },
    { now: 100, start: 100, duration: 0, want: 1 },
  ]
  for (const c of cases) assert.equal(motionProgress(c.now, c.start, c.duration), c.want)
})
