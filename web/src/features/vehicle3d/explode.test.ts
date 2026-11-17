import assert from 'node:assert/strict'
import { test } from 'node:test'
import { axialOffsets, explodeVector, type V3 } from './explode.ts'

const close = (got: V3, want: V3, msg: string) =>
  got.forEach((v, i) => assert.ok(Math.abs(v - want[i]) < 1e-9, `${msg} : ${got} ≠ ${want}`))

test('explodeVector', () => {
  const C: V3 = [0, 0.8, 0]
  const tests: { name: string; center: V3; opts?: object; want: V3 }[] = [
    { name: 'porte gauche : surtout latéral', center: [-0.3, 0.8, 0.86], want: [-0.3 * 0.6 * 0.55, 0.15 * 0.55, 0.86 * 1.6 * 0.55] },
    { name: 'objet bas : jamais vers le sol', center: [1, 0.2, 0], want: [0.6 * 0.55, 0.15 * 0.55, 0] },
    { name: 'fixe', center: [1, 2, 3], opts: { fixed: true }, want: [0, 0, 0] },
    { name: 'montée et renfort x', center: [-1.45, 0.8, 0], opts: { lift: 0.7, xScale: 1.4 }, want: [-1.45 * 0.6 * 1.4 * 0.55, 0.85 * 0.55, 0] },
  ]
  for (const tt of tests) close(explodeVector(tt.center, C, tt.opts), tt.want, tt.name)
})

test('axialOffsets : roue avant gauche, frein → jante → pneu vers l’extérieur', () => {
  const parts = [
    { id: 'pneu', axial: 3 }, { id: 'frein-disque', axial: 1 }, { id: 'frein-etrier', axial: 1 }, { id: 'jante', axial: 2 },
  ]
  const got = axialOffsets(parts, [-1.36, 0.36, 0.74], [0, 0.8, 0], 4)
  close(got.get('frein-disque')!, [0, 0, 0.4], 'frein')
  close(got.get('frein-etrier')!, [0, 0, 0.4], 'même rang, même offset')
  close(got.get('jante')!, [0, 0, 0.88], 'jante')
  close(got.get('pneu')!, [0, 0, 1.36], 'pneu')
})

test('axialOffsets : choix de l’axe', () => {
  const tests: { name: string; focus: V3; want: V3 }[] = [
    { name: 'côté droit → -z', focus: [0, 1, -0.8], want: [0, 0, -0.4] },
    { name: 'centre avant → -x', focus: [-1.5, 1, 0], want: [-0.4, 0, 0] },
    { name: 'centre → haut', focus: [0.1, 1, 0.1], want: [0, 0.4, 0] },
  ]
  for (const tt of tests) close(axialOffsets([{ id: 'a', axial: 0 }], tt.focus, [0, 0.8, 0], 4).get('a')!, tt.want, tt.name)
})
