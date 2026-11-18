import assert from 'node:assert/strict'
import { test } from 'node:test'
import * as THREE from 'three'
import { rememberMaterial, styleMesh } from './scene.ts'

test('GLB multi-matériaux : transparence et restauration préservent chaque matériau', () => {
  const materials = [new THREE.MeshStandardMaterial({ color: '#335577' }), new THREE.MeshPhysicalMaterial({ color: '#7799aa', transparent: true, opacity: 0.5 })]
  const mesh = new THREE.Mesh(new THREE.BoxGeometry(), materials)
  mesh.castShadow = true
  const before = materials.map(m => m.color.getHex())
  rememberMaterial(mesh, true)
  styleMesh(mesh, { tint: null, ghost: true, estimated: false })
  assert.deepEqual(materials.map(m => m.opacity), [0.12, 0.12])
  styleMesh(mesh, { tint: null, ghost: false, estimated: false })
  assert.deepEqual(materials.map(m => m.opacity), [1, 0.5])
  assert.deepEqual(materials.map(m => m.color.getHex()), before)
  assert.equal(mesh.castShadow, true)
})
