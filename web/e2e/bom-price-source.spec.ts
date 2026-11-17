import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

// Source des prix d'un brouillon : choisir parmi les snapshots de prix existants, nommés par leur instant à
// la seconde près (deux snapshots pris à 3 s d'écart se distinguent) ; et un changement de stratégie dit
// « Recalcul en cours… » tant que l'arbre n'est pas relu.

const asOf = '2026-11-04T12:00:00Z'
const bom = {
  id: 1, name: 'Fiat Multipla', currency: 'EUR', volume: 1000, annual_hours: '1607', destination_country: null,
  price_snapshot_id: null as number | null, source_snapshot_id: null, created_at: asOf,
}
const part = {
  id: 10, name: 'Injecteur', category: 'moteur', co2_kg_per_unit: null, mass_kg: null, production_days: null,
  offers: [{ id: 101, supplier_id: 1, currency: 'EUR', lead_time_days: 12, origin_country: 'FR' }],
}
const totals = {
  real_quantity: '1', cost: { amount: '92.00', currency: 'EUR' }, cost_error_code: null, cost_error: null, missing_cost: [],
  co2_kg: '1', mass_kg: '1', lead_time_days: 12, own_cost: null, own_lead_time_days: 0, quote: null, rate: null,
  labor_hours: null, labor_error: null, missing_labor: [], fte: null, stock_covered: null, in_stock: false,
}
const nodes = [
  { id: 1, parent_id: null, name: 'Fiat Multipla', part_id: null, offer_id: null, quantity: '1', production_days: 0, totals, strategy: null },
  { id: 2, parent_id: 1, name: '', part_id: 10, offer_id: 101, quantity: '4', production_days: 0, totals, strategy: null },
]
const snapshots = [
  { id: 7, scope: 'bom', scope_id: 1, taken_at: '2026-11-04T09:15:03Z' },
  { id: 6, scope: 'bom', scope_id: 1, taken_at: '2026-11-04T09:15:00Z' },
  { id: 2, scope: 'catalog', taken_at: '2026-11-01T08:00:00Z' },
]

type Call = { method: string; path: string; body: unknown }

async function mockAPI(page: Page, opts: { patchDelayMs?: number } = {}) {
  const calls: Call[] = []
  await page.route('**/api/**', async (route) => {
    const req = route.request()
    const path = new URL(req.url()).pathname
    if (req.method() === 'PUT' && path === '/api/v1/boms/1/freeze-prices') {
      calls.push({ method: 'PUT', path, body: req.postDataJSON() })
      await route.fulfill({ json: { ...bom, price_snapshot_id: (req.postDataJSON() as { price_snapshot_id: number }).price_snapshot_id } })
      return
    }
    if (req.method() === 'PATCH' && path === '/api/v1/boms/1/nodes/2') {
      calls.push({ method: 'PATCH', path, body: req.postDataJSON() })
      if (opts.patchDelayMs) await new Promise((r) => setTimeout(r, opts.patchDelayMs))
      await route.fulfill({ json: { ...nodes[1], strategy: 'co2' } })
      return
    }
    const json = {
      '/api/v1/boms': [bom],
      '/api/v1/boms/1/tree': { bom, nodes, explanation: { currency: 'EUR', price_snapshot_id: null, rates: [] }, stock: [], planning: { schedule: null, lead_time_days: 0, available_on: null, critical: [], allowed_days: null, target_met: null, late: [] } },
      '/api/v1/boms/1/anomalies': { threshold: '3', currency: 'EUR', price_snapshot_id: null, rates: [], nodes: [] },
      '/api/v1/parts': [part],
      '/api/v1/suppliers': [{ id: 1, name: 'Atlas' }],
      '/api/v1/fx': { rates: [] },
      '/api/v1/quotes': { context: { mode: 'live', as_of: asOf, tick: 7 }, quotes: [], coverage_complete: true },
      '/api/v1/price-snapshots': snapshots,
    }[path]
    if (json === undefined) await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
    else await route.fulfill({ json })
  })
  return calls
}

async function openBOM(page: Page) {
  await page.goto('/#/boms/1/structurer')
}

test('les snapshots de prix sont nommés à la seconde près et un choix attache le snapshot', async ({ page }) => {
  const calls = await mockAPI(page)
  await openBOM(page)
  // Synthèse, « Prix utilisés » : en direct puis les snapshots du plus récent au plus ancien.
  const source = page.getByRole('radiogroup', { name: 'Prix utilisés' })
  await expect(source.locator('label')).toHaveText([
    /^En direct/,
    /^#7 · 04\/11\/2026 09:15:03 UTC\s*BOM n°1$/,
    /^#6 · 04\/11\/2026 09:15:00 UTC\s*BOM n°1$/,
    /^#2 · 01\/11\/2026 08:00:00 UTC\s*catalogue$/,
  ])
  await expect(source.getByRole('radio', { name: /^En direct/ })).toBeChecked()
  // Le choix part à l'API ; le bouton coché suit la BOM relue (ici le mock relit toujours « en direct »).
  await source.getByRole('radio', { name: /^#6 · / }).click()
  await expect.poll(() => calls).toEqual([{ method: 'PUT', path: '/api/v1/boms/1/freeze-prices', body: { price_snapshot_id: 6 } }])
})

test('un changement de stratégie reste affiché et dit « Recalcul en cours… » jusqu’à la relecture', async ({ page }) => {
  const calls = await mockAPI(page, { patchDelayMs: 1500 })
  await openBOM(page)
  await page.getByRole('treeitem', { name: /Injecteur/ }).getByText('Injecteur', { exact: true }).click()
  await expect(page).toHaveURL(/#\/boms\/1\/structurer\?node=2$/)
  // La stratégie est dans l'étape Arbitrer ; la sélection suit d'une étape à l'autre par l'URL.
  await page.getByRole('navigation', { name: 'Étapes' }).getByRole('link', { name: /Arbitrer/ }).click()
  await expect(page).toHaveURL(/#\/boms\/1\/arbitrer\?node=2$/)
  const strategy = page.getByRole('combobox', { name: 'Stratégie du nœud' })
  await strategy.selectOption('co2')
  // Pendant la requête : le choix reste visible, le sélecteur est verrouillé, le statut le dit, les totaux sont grisés.
  await expect(strategy).toHaveValue('co2')
  await expect(strategy).toBeDisabled()
  await expect(page.getByRole('status').filter({ hasText: 'Recalcul en cours…' }).first()).toBeVisible()
  await expect(page.locator('.totals')).toHaveAttribute('aria-busy', 'true')
  expect(calls).toEqual([{ method: 'PATCH', path: '/api/v1/boms/1/nodes/2', body: { strategy: 'co2' } }])
  // Une fois l'arbre relu : plus de statut, sélecteur de nouveau actif.
  await expect(page.getByRole('status').filter({ hasText: 'Recalcul en cours…' })).toHaveCount(0, { timeout: 5000 })
  await expect(strategy).toBeEnabled()
  await expect(page.locator('.totals')).toHaveAttribute('aria-busy', 'false')
})
