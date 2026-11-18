import type { SceneScope } from './manifest.ts'

/** Adapter remplaçable par une autre persistance, sans dépendance dans le résolveur ou le viewer. */
export interface ManifestSource {
  load(scope: SceneScope, signal: AbortSignal): Promise<unknown | null>
}

export const staticManifestSource: ManifestSource = {
  async load(scope, signal) {
    const suffix = scope.revision ? `-${encodeURIComponent(scope.revision)}` : ''
    const response = await fetch(`/models/manifests/${scope.kind}-${scope.id}${suffix}.json`, { signal })
    if (response.status === 404 || (response.headers.get('content-type') ?? '').includes('text/html')) return null
    if (!response.ok) throw new Error(`Manifeste 3D indisponible : HTTP ${response.status}.`)
    const value = await response.text()
    if (value.length > 2_000_000) throw new Error('Manifeste 3D trop volumineux (2 Mo maximum).')
    return JSON.parse(value) as unknown
  },
}
