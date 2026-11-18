import { expect, test } from '@playwright/test'
import { formatRoute, parseRoute, type Route } from '../src/router.ts'

// Routeur maison (ADR 0034) : fonctions pures, testées sans navigateur.

test('aller-retour : chaque route se relit à l’identique', () => {
  const routes: Route[] = [
    { page: 'boms' },
    { page: 'suppliers', supplier: null },
    { page: 'suppliers', supplier: 3 },
    { page: 'bom', target: { kind: 'draft', id: 1 }, step: 'structurer', node: 8437, tab: null, supplier: null },
    { page: 'bom', target: { kind: 'draft', id: 1 }, step: 'structurer', node: null, tab: 'arbre-3d', supplier: null },
    { page: 'bom', target: { kind: 'snapshot', id: 7 }, step: 'publier', node: null, tab: null, supplier: null },
    { page: 'bom', target: { kind: 'draft', id: 12 }, step: 'sourcer', node: 5, tab: 'fournisseurs', supplier: 9 },
  ]
  for (const r of routes) expect(parseRoute(formatRoute(r))).toEqual(r)
})

test('URL profonde, ancres de la V1 et adresses invalides', () => {
  const cases: [string, Route][] = [
    ['#/boms/1/structurer?node=8437', { page: 'bom', target: { kind: 'draft', id: 1 }, step: 'structurer', node: 8437, tab: null, supplier: null }],
    ['#/boms/1', { page: 'bom', target: { kind: 'draft', id: 1 }, step: 'structurer', node: null, tab: null, supplier: null }],
    ['#/snapshots/7/chiffrer', { page: 'bom', target: { kind: 'snapshot', id: 7 }, step: 'chiffrer', node: null, tab: null, supplier: null }],
    ['', { page: 'boms' }],
    ['#', { page: 'boms' }],
    ['#boms', { page: 'boms' }],
    ['#fournisseurs', { page: 'suppliers', supplier: null }],
    // Un nœud illisible est ignoré, la page reste ouverte.
    ['#/boms/1/chiffrer?node=abc', { page: 'bom', target: { kind: 'draft', id: 1 }, step: 'chiffrer', node: null, tab: null, supplier: null }],
    ['#/boms/1/chiffrer?node=-3', { page: 'bom', target: { kind: 'draft', id: 1 }, step: 'chiffrer', node: null, tab: null, supplier: null }],
    ['#/boms/0/structurer', { page: 'not_found', hash: '#/boms/0/structurer' }],
    ['#/boms/1e3/structurer', { page: 'not_found', hash: '#/boms/1e3/structurer' }],
    ['#/boms/1/exporter', { page: 'not_found', hash: '#/boms/1/exporter' }],
    ['#/inconnu', { page: 'not_found', hash: '#/inconnu' }],
  ]
  for (const [hash, want] of cases) expect(parseRoute(hash), hash).toEqual(want)
})

test('une ancienne ancre est réécrite sans entrée d’historique', async ({ page }) => {
  await page.route('**/api/**', (route) => route.fulfill({ json: [] }))
  await page.goto('/#boms')
  await expect(page).toHaveURL(/#\/boms$/)
  // about:blank puis la page : la réécriture n'a pas ajouté d'entrée (« précédent » quitte l'app).
  expect(await page.evaluate(() => history.length)).toBe(2)
  await page.goBack()
  await expect(page).toHaveURL('about:blank')
})
