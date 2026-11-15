import { useEffect, useState } from 'react'
import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import type { Vec3 } from '../api.ts'

// Un GLB chargé, prêt à être instancié : ses maillages avec leur transformation locale, après recentrage
// sur l'origine et mise à l'échelle (plus grand côté = 1 m). extent : encombrement normalisé (x, y, z).
export type LoadedModel = {
  meshes: { geometry: THREE.BufferGeometry; material: THREE.Material | THREE.Material[]; matrix: THREE.Matrix4 }[]
  extent: Vec3
}

// Cache par URL : une pièce répétée, ou présente dans plusieurs BOM, n'est téléchargée qu'une fois par session.
const cache = new Map<string, Promise<LoadedModel>>()
const loader = new GLTFLoader()

export function loadModel(url: string): Promise<LoadedModel> {
  let p = cache.get(url)
  if (!p) {
    p = loader.loadAsync(url).then((gltf) => normalize(gltf.scene))
    // Un échec n'est pas gardé : un rechargement de la vue retentera.
    p.catch(() => cache.delete(url))
    cache.set(url, p)
  }
  return p
}

// normalize ramène la scène du GLB dans un cube unité centré : le catalogue n'a pas de dimensions, le
// placement donne l'encombrement (shape.size) dans lequel le modèle est ensuite ajusté.
function normalize(scene: THREE.Object3D): LoadedModel {
  scene.updateMatrixWorld(true)
  const box = new THREE.Box3().setFromObject(scene)
  const size = new THREE.Vector3()
  const center = new THREE.Vector3()
  box.getSize(size)
  box.getCenter(center)
  const max = Math.max(size.x, size.y, size.z) || 1
  const k = 1 / max
  const fit = new THREE.Matrix4().makeScale(k, k, k).multiply(new THREE.Matrix4().makeTranslation(-center.x, -center.y, -center.z))
  const meshes: LoadedModel['meshes'] = []
  scene.traverse((o) => {
    if ((o as THREE.Mesh).isMesh) {
      const m = o as THREE.Mesh
      meshes.push({ geometry: m.geometry, material: m.material, matrix: fit.clone().multiply(m.matrixWorld) })
    }
  })
  return { meshes, extent: [size.x * k, size.y * k, size.z * k] }
}

// fitScale : facteur uniforme pour que le modèle normalisé tienne dans l'encombrement du placement.
export function fitScale(extent: Vec3, size: Vec3): number {
  let k = Infinity
  for (let i = 0; i < 3; i++) if (extent[i] > 0) k = Math.min(k, size[i] / extent[i])
  return Number.isFinite(k) ? k : 1
}

// useModel charge (ou relit du cache) le GLB d'une URL ; null tant qu'il n'est pas là, erreur nommée sinon.
export function useModel(url: string): { model: LoadedModel | null; error: string | null } {
  const [state, setState] = useState<{ url: string; model: LoadedModel | null; error: string | null }>({ url, model: null, error: null })
  useEffect(() => {
    let cancelled = false
    loadModel(url)
      .then((model) => {
        if (!cancelled) setState({ url, model, error: null })
      })
      .catch((e: unknown) => {
        if (!cancelled) setState({ url, model: null, error: e instanceof Error ? e.message : String(e) })
      })
    return () => {
      cancelled = true
    }
  }, [url])
  return state.url === url ? state : { model: null, error: null }
}
