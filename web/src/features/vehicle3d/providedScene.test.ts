import assert from 'node:assert/strict'
import { test } from 'node:test'
import { hasProvidedScene, providedVariantParts } from './providedScene.ts'

test('mode initial : seulement une portée et une structure d’installation connues', () => {
  const nodes = [
    { id: 1, parent_id: null, part_id: null, name: 'Fiat Multipla 1.6 16V' },
    { id: 12502, parent_id: 1, part_id: null, name: 'Groupe motopropulseur' },
    { id: 26314, parent_id: 1, part_id: null, name: 'Habitacle' },
  ]
  assert.equal(hasProvidedScene({ kind: 'draft', id: 1 }, nodes), true)
  assert.equal(hasProvidedScene({ kind: 'snapshot', id: 1 }, nodes), false)
  assert.equal(hasProvidedScene({ kind: 'draft', id: 77 }, nodes), false)
  assert.equal(hasProvidedScene({ kind: 'draft', id: 1 }, nodes.slice(0, 1)), false)
  assert.equal(hasProvidedScene({ kind: 'draft', id: 1 }, nodes.map(n => n.id === 1 ? { ...n, name: 'Autre voiture' } : n)), false)
  assert.equal(hasProvidedScene({ kind: 'draft', id: 1 }, nodes.map(n => n.id === 12502 ? { ...n, parent_id: 9 } : n)), false)
})

test('variantes : nœud, parent, pièce et portée explicites, backend toujours responsable du remplacement', () => {
  const node = { id: 15896, parent_id: 15895, part_id: 195, name: 'Batterie' }
  assert.deepEqual(providedVariantParts({ kind: 'draft', id: 1 }, node), [195, 196])
  assert.deepEqual(providedVariantParts({ kind: 'draft', id: 1 }, { ...node, part_id: 196 }), [195, 196])
  for (const patch of [{ id: 1 }, { parent_id: 1 }, { part_id: 999 }]) assert.deepEqual(providedVariantParts({ kind: 'draft', id: 1 }, { ...node, ...patch }), [])
  assert.deepEqual(providedVariantParts({ kind: 'snapshot', id: 1 }, node), [])
})
