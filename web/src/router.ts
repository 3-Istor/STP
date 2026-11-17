// Routeur maison sans dépendance (ADR 0034) : l'état de navigation vit dans l'ancre de l'URL, pour qu'un
// lien, un rechargement ou le bouton « précédent » ramène au même écran, à la même étape et au même nœud.
//
//   #/boms                                  toutes les BOM
//   #/boms/1/structurer?node=8437           brouillon 1, étape Structurer, nœud 8437 sélectionné
//   #/snapshots/7/chiffrer                  snapshot publié 7 (lecture seule), étape Chiffrer
//   #/boms/1/sourcer?tab=fournisseurs&supplier=3
//   #/fournisseurs                          fournisseurs hors de toute BOM
//
// Les anciennes ancres de la V1 (#boms, #fournisseurs) restent valides. parseRoute et formatRoute sont
// pures (testées dans e2e/router.spec.ts) ; useRoute les branche sur `hashchange`.
import { useEffect, useState } from 'react'

export const STEPS = ['structurer', 'chiffrer', 'sourcer', 'arbitrer', 'publier'] as const
export type Step = (typeof STEPS)[number]

export const STEP_LABELS: Record<Step, string> = {
  structurer: 'Structurer',
  chiffrer: 'Chiffrer',
  sourcer: 'Sourcer',
  arbitrer: 'Arbitrer',
  publier: 'Publier',
}

export type Target = { kind: 'draft' | 'snapshot'; id: number }

export type Route =
  | { page: 'boms' }
  | { page: 'suppliers'; supplier: number | null }
  | { page: 'bom'; target: Target; step: Step; node: number | null; tab: string | null; supplier: number | null }
  | { page: 'not_found'; hash: string }

const isStep = (s: string): s is Step => (STEPS as readonly string[]).includes(s)

// positiveID : identifiant entier strictement positif, sinon null (« 12abc », « -1 », « 1e3 » refusés).
function positiveID(s: string | null | undefined): number | null {
  if (!s || !/^[1-9]\d{0,15}$/.test(s)) return null
  return Number(s)
}

export function parseRoute(hash: string): Route {
  const raw = hash.startsWith('#') ? hash.slice(1) : hash
  if (raw === '' || raw === '/' || raw === 'boms' || raw === '/boms') return { page: 'boms' }
  if (raw === 'fournisseurs') return { page: 'suppliers', supplier: null }
  const [path, query = ''] = raw.split('?', 2)
  const params = new URLSearchParams(query)
  const parts = path.split('/').filter((p) => p !== '')
  if (parts.length === 1 && parts[0] === 'fournisseurs') return { page: 'suppliers', supplier: positiveID(params.get('supplier')) }
  if ((parts[0] === 'boms' || parts[0] === 'snapshots') && (parts.length === 2 || parts.length === 3)) {
    const id = positiveID(parts[1])
    const step = parts[2] ?? 'structurer'
    if (id !== null && isStep(step)) {
      return {
        page: 'bom',
        target: { kind: parts[0] === 'boms' ? 'draft' : 'snapshot', id },
        step,
        node: positiveID(params.get('node')),
        tab: params.get('tab'),
        supplier: positiveID(params.get('supplier')),
      }
    }
  }
  return { page: 'not_found', hash }
}

export function formatRoute(r: Route): string {
  switch (r.page) {
    case 'boms':
      return '#/boms'
    case 'suppliers':
      return r.supplier === null ? '#/fournisseurs' : `#/fournisseurs?supplier=${r.supplier}`
    case 'not_found':
      return r.hash
    case 'bom': {
      const params = new URLSearchParams()
      if (r.node !== null) params.set('node', String(r.node))
      if (r.tab) params.set('tab', r.tab)
      if (r.supplier !== null) params.set('supplier', String(r.supplier))
      const q = params.toString()
      return `#/${r.target.kind === 'draft' ? 'boms' : 'snapshots'}/${r.target.id}/${r.step}${q ? `?${q}` : ''}`
    }
  }
}

// bomRoute : route d'une étape d'une BOM, sélection et onglet vides par défaut.
export function bomRoute(target: Target, step: Step = 'structurer', extra: Partial<{ node: number | null; tab: string | null; supplier: number | null }> = {}): Route {
  return { page: 'bom', target, step, node: extra.node ?? null, tab: extra.tab ?? null, supplier: extra.supplier ?? null }
}

export function href(r: Route): string {
  return formatRoute(r)
}

// navigate : `replace` pour un changement qui ne mérite pas une entrée d'historique (sélection d'un nœud).
export function navigate(r: Route, opts: { replace?: boolean } = {}) {
  const next = formatRoute(r)
  if (next === window.location.hash) return
  if (opts.replace) {
    window.history.replaceState(window.history.state, '', next)
    window.dispatchEvent(new HashChangeEvent('hashchange'))
  } else {
    window.location.hash = next
  }
}

export function useRoute(): [Route, typeof navigate] {
  const [route, setRoute] = useState<Route>(() => parseRoute(window.location.hash))
  useEffect(() => {
    const onHash = () => setRoute(parseRoute(window.location.hash))
    window.addEventListener('hashchange', onHash)
    // Anciennes ancres (#boms, #fournisseurs) : réécrites dans la forme canonique, sans entrée d'historique.
    const r = parseRoute(window.location.hash)
    if (r.page !== 'not_found' && formatRoute(r) !== window.location.hash) navigate(r, { replace: true })
    return () => window.removeEventListener('hashchange', onHash)
  }, [])
  return [route, navigate]
}
