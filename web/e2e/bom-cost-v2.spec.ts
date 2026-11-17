import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

// V2 lot F : budgets (état et % calculés par l'API, jamais par le front), méthode de calcul (affichée, changée
// par PUT puis relecture), « Pourquoi ce coût ? » (lignes de l'API, composante absente et erreur visibles,
// jamais 0) et vue 3D dans Structurer (sélection partagée avec l'arbre par l'URL, panne isolée).

const asOf = '2026-04-07T00:00:00Z'
const money = (amount: string) => ({ amount, currency: 'EUR' })
const totals = (cost: string | null) => ({
  real_quantity: '1', cost: cost === null ? null : money(cost), cost_error_code: cost === null ? 'missing_cost' : null,
  cost_error: cost === null ? 'coût incomplet' : null, missing_cost: cost === null ? [4] : [],
  co2_kg: '1', mass_kg: '1', lead_time_days: 12, own_cost: null, own_lead_time_days: 0, quote: null, rate: null,
  labor_hours: null, labor_error: null, missing_labor: [], fte: null, stock_covered: null, in_stock: false,
})
const node = (id: number, parent_id: number | null, name: string, cost: string | null, part_id: number | null = null) =>
  ({ id, parent_id, name, part_id, offer_id: part_id ? 101 : null, quantity: '1', production_days: 0, strategy: null, totals: totals(cost) })
const nodes = [
  node(1, null, 'Fiat Multipla', '25673.6312'),
  node(2, 1, 'Carrosserie et ouvrants', '3320.95'),
  node(3, 2, 'Porte avant gauche', '472.82'),
  node(4, 3, '', '168.45', 10),
  node(5, 2, 'Capot', '144.19'),
  node(6, 1, 'Habitacle', '5538.64'),
]
const budget = (node_id: number, target: string, cost: string, percent: string | null, state: string, error: string | null = null) =>
  ({ node_id, target: money(target), cost: money(cost), delta: null, percent, state, error })
const budgets = [
  budget(1, '26500.00', '25673.6312', '96.9', 'warn'),
  budget(2, '3000.00', '3320.95', '110.7', 'over'),
  budget(3, '420.00', '472.82', '112.6', 'over'),
  budget(5, '160.00', '144.19', null, 'unavailable', 'taux de change manquant'),
]
const part = {
  id: 10, name: 'Porte avant nue', category: 'Carrosserie', co2_kg_per_unit: null, mass_kg: null, production_days: null,
  offers: [{ id: 101, supplier_id: 1, currency: 'GBP', lead_time_days: 12, origin_country: 'GB' }],
}
const bom = {
  id: 1, name: 'Fiat Multipla', currency: 'EUR', volume: 2000, annual_hours: '1607', destination_country: 'IT',
  price_snapshot_id: null, source_snapshot_id: 7, created_at: asOf,
}
const planning = { schedule: null, lead_time_days: 12, available_on: null, critical: [], allowed_days: null, target_met: null, late: [] }
const full = { basis: 'full', fx: 'budget', program_volume: 60000 }
const explanation = (method: object) => ({ currency: 'EUR', price_snapshot_id: null, rates: [], cost_method: method, cost_as_of: asOf, budget_rates: [] })
const line = (kind: string, label: string, amount: string, source: string, as_of: string | null = asOf) => ({ kind, label, amount: money(amount), source, as_of })
// Ventilation de la porte : pas de ligne « outillage » alors que la méthode est « complet » → « absente ».
const doorBreakdown = {
  node_id: 4, currency: 'EUR', method: full, cost_as_of: asOf,
  lines: [
    line('price', 'Prix coté 149.25 GBP × 1', '171.6375', 'pricesim.v1/demo-v4'),
    line('fx', 'Écart de change : budget 2026', '-3.1875', 'budget 2026'),
    line('discount', 'Contrat expiré : remise non appliquée', '0', 'contrat 11'),
    line('customs', 'Douane (origine GB) : 0 %', '0', 'douane, tables', null),
    line('freight', 'Transport', '3.80', 'transport, tables', null),
    line('packaging', 'Emballage', '1.33', 'emballage de l’offre, tables', null),
  ],
  total: money('173.58'),
}

type Call = { method: string; path: string; body: unknown }

async function mockAPI(page: Page, opts: { putDelayMs?: number; breakdownStatus?: number } = {}) {
  const calls: Call[] = []
  let method: object = full
  await page.route('**/api/**', async (route) => {
    const req = route.request()
    const path = new URL(req.url()).pathname
    if (req.method() === 'PUT' && path === '/api/v1/boms/1/cost-method') {
      calls.push({ method: 'PUT', path, body: req.postDataJSON() })
      if (opts.putDelayMs) await new Promise((r) => setTimeout(r, opts.putDelayMs))
      method = req.postDataJSON() as object
      await route.fulfill({ json: method })
      return
    }
    if (path.endsWith('/cost-breakdown')) {
      calls.push({ method: 'GET', path, body: null })
      if (opts.breakdownStatus) await route.fulfill({ status: opts.breakdownStatus, json: { error: 'taux budget absent pour GBP→EUR en 2026' } })
      else await route.fulfill({ json: { ...doorBreakdown, method } })
      return
    }
    const json = {
      '/api/v1/boms': [bom],
      '/api/v1/boms/1/tree': { bom, nodes, explanation: explanation(method), stock: [], planning, budgets },
      '/api/v1/snapshots/7': {
        snapshot: { id: 7, bom_id: 1, price_snapshot_id: 3, name: 'Fiat Multipla', currency: 'EUR', volume: 2000, published_at: asOf },
        nodes, explanation: { ...explanation(full), price_snapshot_id: 3 },
      },
      '/api/v1/boms/1/anomalies': { threshold: '3', currency: 'EUR', price_snapshot_id: null, rates: [], nodes: [] },
      '/api/v1/parts': [part],
      '/api/v1/suppliers': [{ id: 1, name: 'Arden Composites' }],
      '/api/v1/fx': { rates: [] },
      '/api/v1/price-snapshots': [],
    }[path]
    if (json === undefined) await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
    else await route.fulfill({ json })
  })
  return calls
}

const row = (page: Page, name: string) => page.getByRole('treeitem').filter({ has: page.locator('.name', { hasText: new RegExp(`^${name}`) }) })

test('budgets : % et couleur de l’API dans l’arbre et la Synthèse, « — » sans budget, « n/d » indisponible', async ({ page }) => {
  await mockAPI(page)
  await page.goto('/#/boms/1/structurer')
  await expect(row(page, 'Fiat Multipla').locator('.budget')).toHaveText('96,9 %')
  await expect(row(page, 'Fiat Multipla').locator('.budget')).toHaveClass(/t-warn/)
  await expect(row(page, 'Carrosserie et ouvrants').locator('.budget')).toHaveClass(/t-over/)
  await expect(row(page, 'Habitacle').locator('.budget')).toHaveText('—')
  await expect(row(page, 'Capot').locator('.budget')).toHaveText('n/d')
  await expect(row(page, 'Capot').locator('.budget')).toHaveAttribute('title', /taux de change manquant/)

  const summary = page.getByRole('region', { name: 'Synthèse' })
  await expect(summary).toContainText('budget 26 500,00 € · 96,9 %')
  // Hors budget : les sous-systèmes dont l'état API est « over », cliquables.
  await summary.getByRole('button', { name: 'Carrosserie et ouvrants' }).first().click()
  await expect(page).toHaveURL(/node=2/)
  await expect(summary.locator('.sys li').filter({ hasText: 'Carrosserie' }).locator('.p')).toHaveText('110,7 %')
  await expect(summary.locator('.sys li').filter({ hasText: 'Habitacle' }).locator('.p')).toHaveText('—')
  // Le détail du nœud montre son budget propre.
  await expect(page.getByRole('region', { name: 'Détail du nœud' })).toContainText('3 000,00 € · 110,7 %')
})

test('méthode : affichée avec sa date, changée par l’API avec « Recalcul en cours… »', async ({ page }) => {
  const calls = await mockAPI(page, { putDelayMs: 600 })
  await page.goto('/#/boms/1/structurer')
  const summary = page.getByRole('region', { name: 'Synthèse' })
  await expect(summary).toContainText('Coût complet par véhicule')
  await expect(summary).toContainText('Coût complet · taux budget · volume programme 60 000')
  await expect(summary).toContainText('Calculé au 07/04/2026 00:00 UTC')

  await page.getByRole('tab', { name: 'Réglages de calcul' }).click()
  const method = page.getByRole('region', { name: 'Méthode de calcul' })
  await expect(method.getByRole('radio', { name: 'Complet' })).toHaveAttribute('aria-checked', 'true')
  await method.getByRole('radio', { name: 'Achat' }).click()
  await expect(summary.getByRole('status')).toHaveText('Recalcul en cours…')
  await expect(summary).toContainText('Coût d’achat par véhicule')
  await expect(method.getByRole('radio', { name: 'Achat' })).toHaveAttribute('aria-checked', 'true')
  await method.getByRole('radio', { name: 'Dernier taux' }).click()
  await expect(summary).toContainText('Coût d’achat · dernier taux')
  expect(calls.filter((c) => c.method === 'PUT').map((c) => c.body)).toEqual([
    { basis: 'purchase', fx: 'budget', program_volume: 60000 },
    { basis: 'purchase', fx: 'latest', program_volume: 60000 },
  ])
})

test('« Pourquoi ce coût ? » : lignes de l’API, composante absente, total ; brouillon puis snapshot', async ({ page }) => {
  const calls = await mockAPI(page)
  await page.goto('/#/boms/1/structurer?node=4')
  const cascade = page.getByRole('region', { name: 'Pourquoi ce coût ?' })
  await expect(cascade).toContainText('Pourquoi ce coût ? · Porte avant nue')
  const rows = cascade.getByRole('row')
  // Libellé de l'API arrondi à l'affichage (149,25 GBP), exact en infobulle ; montant de l'API arrondi au centime.
  const price = rows.filter({ hasText: 'Prix coté 149,25' })
  await expect(price).toContainText('171,64 €')
  await expect(price).toContainText('pricesim.v1/demo-v4 · 07/04/2026 00:00 UTC')
  await expect(price.locator('.casc-label')).toHaveAttribute('title', 'Prix coté 149.25 GBP × 1')
  await expect(rows.filter({ hasText: 'Écart de change' })).toContainText('-3,19 €')
  // La méthode « complet » attend une ligne d'outillage : sans ligne, « absente », pas 0.
  await expect(rows.filter({ hasText: 'Outillage amorti' })).toContainText('absente')
  await expect(rows.filter({ hasText: 'Outillage amorti' })).not.toContainText('0,00')
  await expect(rows.filter({ hasText: 'Total' })).toContainText('173,58 €')
  expect(calls.some((c) => c.path === '/api/v1/boms/1/nodes/4/cost-breakdown')).toBe(true)

  await page.goto('/#/snapshots/7/structurer?node=4')
  await expect(page.getByRole('region', { name: 'Pourquoi ce coût ?' }).getByRole('row').filter({ hasText: 'Total' })).toContainText('173,58 €')
  expect(calls.some((c) => c.path === '/api/v1/snapshots/7/nodes/4/cost-breakdown')).toBe(true)
})

test('« Pourquoi ce coût ? » : une erreur de l’API est affichée, jamais un coût à 0', async ({ page }) => {
  await mockAPI(page, { breakdownStatus: 409 })
  await page.goto('/#/boms/1/structurer?node=4')
  const cascade = page.getByRole('region', { name: 'Pourquoi ce coût ?' })
  await expect(cascade.getByRole('alert')).toHaveText('Ventilation indisponible : taux budget absent pour GBP→EUR en 2026')
  await expect(cascade).not.toContainText('€')
})

// Sélection d'un objet de la vue 3D, lue sur le moteur exposé en développement (Vehicle3DView, `__viewer`).
const selection3d = (page: Page) =>
  page.evaluate(() => [...((document.querySelector('.v3d-root') as unknown as { __viewer: { selection: Set<string> } }).__viewer.selection)])

test('Arbre + 3D : clic dans l’arbre → objet sélectionné dans la 3D ; clic dans la 3D → ligne de l’arbre', async ({ page, browserName }) => {
  const webgl = await page.evaluate(() => !!document.createElement('canvas').getContext('webgl2'))
  test.skip(!webgl, `WebGL2 indisponible dans ce navigateur (${browserName}) : vue 3D non testable ici`)
  // WebGL logiciel (SwiftShader) en CI : chaque rendu est lent, deux workers en parallèle le ralentissent encore.
  test.setTimeout(90_000)
  await mockAPI(page)
  await page.goto('/#/boms/1/structurer?tab=arbre-3d')
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'ready', { timeout: 30_000 })
  await expect.poll(() => selection3d(page)).toEqual([])

  await row(page, 'Porte avant gauche').click()
  await expect(page).toHaveURL(/node=3/)
  await expect.poll(() => selection3d(page)).toEqual(['door_fl'])
  // Les groupes sélectionnent leurs descendants. Une pièce sans modèle a un état explicite,
  // conformément à la refonte 3D : son parent ne doit plus être sélectionné à sa place.
  await row(page, 'Fiat Multipla').click()
  await expect.poll(() => selection3d(page)).toEqual(['door_fl', 'hood', 'est:6'])
  await page.evaluate(() => { window.location.hash = '#/boms/1/structurer?tab=arbre-3d&node=4' })
  await expect(page.getByRole('treeitem', { selected: true })).toContainText('Porte avant nue')
  await expect.poll(() => selection3d(page)).toEqual([])
  await expect(page.getByText('Modèle 3D de cette pièce indisponible.', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Focus sélection', exact: true })).toBeDisabled()
  await row(page, 'Carrosserie et ouvrants').click()
  await expect.poll(() => selection3d(page).then((s) => s.sort())).toEqual(['door_fl', 'hood'])

  // Clic au centre projeté du capot (après le recadrage de la caméra) : la sélection revient dans l'URL et l'arbre.
  await page.getByRole('tab', { name: 'Réglages de calcul', exact: true }).click()
  const canvas = page.locator('.v3d-host canvas')
  await canvas.scrollIntoViewIfNeeded()
  await page.waitForTimeout(1200)
  const at = await page.evaluate(() => {
    type V = { project(c: unknown): { x: number; y: number } }
    const v = (document.querySelector('.v3d-root') as unknown as {
      __viewer: { objectCenters: Map<string, [number, number, number]>; camera: { position: { constructor: new (x: number, y: number, z: number) => V } }; renderer: { domElement: HTMLCanvasElement } }
    }).__viewer
    const [x, y, z] = v.objectCenters.get('hood')!
    const p = new v.camera.position.constructor(x, y, z).project(v.camera)
    const r = v.renderer.domElement.getBoundingClientRect()
    return { x: ((p.x + 1) / 2) * r.width, y: ((1 - p.y) / 2) * r.height }
  })
  // Clic souris brut (pas d'attente d'actionnabilité : les badges de la 3D couvrent une partie du canvas).
  const box = (await canvas.boundingBox())!
  await page.mouse.click(box.x + at.x, box.y + at.y)
  await expect(page).toHaveURL(/node=5/)
  await expect(page.getByRole('treeitem', { selected: true })).toContainText('Capot')
  await expect(page.getByRole('tab', { name: 'Détail', exact: true })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('region', { name: 'Détail du nœud', exact: true })).toContainText('Capot')
})

test('3D sans WebGL : la panne reste dans le panneau 3D, l’arbre et la Synthèse restent utilisables', async ({ page }) => {
  await page.addInitScript(() => {
    const original = HTMLCanvasElement.prototype.getContext
    HTMLCanvasElement.prototype.getContext = function (this: HTMLCanvasElement, type: string, ...args: unknown[]) {
      if (type.startsWith('webgl')) return null
      return (original as (...a: unknown[]) => unknown).call(this, type, ...args)
    } as typeof original
  })
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  await mockAPI(page)
  await page.goto('/#/boms/1/structurer?tab=arbre-3d')
  await expect(page.getByRole('region', { name: 'Vue 3D' }).getByRole('alert')).toContainText('Vue 3D indisponible')
  await row(page, 'Capot').click()
  await expect(page.getByRole('region', { name: 'Détail du nœud' })).toContainText('Capot')
  await expect(page.getByRole('region', { name: 'Synthèse' })).toContainText('budget 26 500,00 €')
})
