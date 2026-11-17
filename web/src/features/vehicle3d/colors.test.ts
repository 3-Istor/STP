import assert from 'node:assert/strict'
import { test } from 'node:test'
import { budgetColor, DEFAULT_STATUS, mixHex, resolveColorMode, systemColor, UNAVAILABLE } from './colors.ts'

test('mixHex', () => {
  const tests = [
    { a: '#000000', b: '#ffffff', t: 0, want: '#000000' },
    { a: '#000000', b: '#ffffff', t: 1, want: '#ffffff' },
    { a: '#000', b: '#fff', t: 0.5, want: '#808080' },
  ]
  for (const tt of tests) assert.equal(mixHex(tt.a, tt.b, tt.t), tt.want)
  assert.throws(() => mixHex('rouge', '#fff', 0.5), /couleur invalide/)
})

test('budgetColor : warn/over exacts, ok adouci, absent jamais vert', () => {
  const p = { ok: '#00ff00', warn: '#ff8800', over: '#ff0000' }
  assert.equal(budgetColor('warn', p), '#ff8800')
  assert.equal(budgetColor('over', p), '#ff0000')
  assert.equal(budgetColor('ok', p), mixHex('#00ff00', '#d3d7dd', 0.55))
  assert.equal(budgetColor(null, p), UNAVAILABLE)
  assert.notEqual(budgetColor(null, DEFAULT_STATUS), budgetColor('ok', DEFAULT_STATUS))
})

test('resolveColorMode', () => {
  const without = [{ id: 1, parent_id: null, name: 'A', part_id: null }]
  const withStatus = [{ id: 1, parent_id: null, name: 'A', part_id: null, budget_status: null }]
  const tests = [
    { by: 'system' as const, nodes: without, mode: 'system', notice: false },
    { by: 'budget' as const, nodes: without, mode: 'system', notice: true },
    { by: 'budget' as const, nodes: withStatus, mode: 'budget', notice: false },
  ]
  for (const tt of tests) {
    const got = resolveColorMode(tt.by, tt.nodes)
    assert.equal(got.mode, tt.mode)
    assert.equal(got.notice !== null, tt.notice)
  }
})

test('systemColor : cyclique et déterministe', () => {
  assert.equal(systemColor(0), systemColor(8))
  assert.notEqual(systemColor(0), systemColor(1))
})
