import { expect, test } from '@playwright/test'
import type { Locator, Page } from '@playwright/test'

// Non-régression NLPF-71 : une règle de mise en page de l'espace Fournisseurs (.workspace global)
// réduisait la colonne de l'arbre BOM à ~2 px (arbre invisible, non cliquable).

const bom = {
  id: 1, name: 'Fiat Multipla', currency: 'EUR', volume: 1000, annual_hours: '1607', destination_country: null,
  price_snapshot_id: null, source_snapshot_id: 7, created_at: '2026-11-04T12:00:00Z',
}
const nodes = [
  { id: 1, parent_id: null, name: 'Fiat Multipla', part_id: null, offer_id: null, quantity: '1', production_days: 0, totals: null },
  { id: 2, parent_id: 1, name: 'Châssis', part_id: null, offer_id: null, quantity: '1', production_days: 2, totals: null },
  { id: 3, parent_id: 1, name: 'Habitacle', part_id: null, offer_id: null, quantity: '1', production_days: 1, totals: null },
]
const explanation = { currency: 'EUR', price_snapshot_id: null, rates: [] }
// Versions de la famille (GET /boms/1/versions, lot B) : le brouillon et le snapshot dont il est issu.
const method = { basis: 'purchase', fx: 'latest', program_volume: null }
const version = (kind: 'draft' | 'snapshot', id: number, label: string, at: string, price_snapshot_id: number | null) => ({
  kind, id, bom_id: 1, ref: `${kind === 'draft' ? 'bom' : 'snapshot'}:${id}`, label, name: 'Fiat Multipla', at,
  source_snapshot_id: kind === 'draft' ? 7 : null, method, cost: null, cost_error: 'coût incomplet', target_budget: null,
  budget_percent: null, price_snapshot_id, proposed_by: null, validated_by: null, note: null, part_count: 0, node_count: 3,
})
const versions = [version('draft', 1, 'Brouillon A', '2026-11-04T12:30:00Z', null), version('snapshot', 7, 'V1', '2026-11-04T12:00:00Z', 3)]

async function mockBomAPI(page: Page) {
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const json = {
      '/api/v1/boms': [bom],
      '/api/v1/boms/1/versions': versions,
      '/api/v1/boms/1/tree': { bom, nodes, explanation, stock: [], planning: { schedule: null, lead_time_days: 0, available_on: null, critical: [], allowed_days: null, target_met: null, late: [] } },
      '/api/v1/snapshots/7': {
        snapshot: { id: 7, bom_id: 1, price_snapshot_id: 3, name: 'Fiat Multipla', currency: 'EUR', volume: 1000, published_at: '2026-11-04T12:00:00Z' },
        nodes,
        explanation: { ...explanation, price_snapshot_id: 3 },
      },
      '/api/v1/parts': [],
      '/api/v1/suppliers': [],
      '/api/v1/fx': { rates: [] },
    }[path]
    if (json === undefined) await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
    else await route.fulfill({ json })
  })
}

// L'arbre doit occuper la largeur de l'explorateur (étape Structurer) : on mesure l'explorateur et non
// l'arbre seul, car c'est l'arbre qui se rétractait sur son contenu (NLPF-71).
async function expectTreeFillsView(page: Page) {
  const tree = page.locator('.tree')
  await expect(tree).toBeVisible()
  const [explorer, treeBox] = await Promise.all([page.locator('.explorer').boundingBox(), tree.boundingBox()])
  expect(explorer && treeBox).toBeTruthy()
  expect(treeBox!.width).toBeGreaterThan(explorer!.width * 0.9)
}

async function press(locator: Locator, touch: boolean | undefined) {
  if (touch) await locator.tap()
  else await locator.click()
}

test('arbre BOM pleine largeur et cliquable, brouillon puis snapshot', async ({ page }, testInfo) => {
  const touch = testInfo.project.use.hasTouch
  await mockBomAPI(page)
  await page.goto('/#/boms')
  // Avec ses versions, le brouillon est listé sous son libellé de version (« Brouillon A »).
  await press(page.getByRole('link', { name: 'Brouillon A', exact: true }), touch)
  await expect(page).toHaveURL(/#\/boms\/1\/structurer$/)

  await expectTreeFillsView(page)
  await press(page.locator('.tree-row').filter({ hasText: 'Châssis' }), touch)
  await expect(page.getByRole('region', { name: 'Détail du nœud' })).toContainText('Châssis')

  // Le snapshot à l'origine du brouillon : listé sous la BOM dans « Toutes les BOM » (versions de la famille,
  // date à la seconde, colonne en UTC), ouvert en lecture seule.
  await press(page.getByRole('link', { name: 'Toutes les BOM' }), touch)
  const snapshotRow = page.getByRole('row').filter({ hasText: 'snapshot #7' })
  await expect(snapshotRow).toContainText('04/11/2026 12:00:00')
  await expect(page.getByRole('columnheader', { name: 'Date (UTC)' })).toBeVisible()
  await press(snapshotRow.getByRole('link', { name: 'Ouvrir' }), touch)
  await expect(page).toHaveURL(/#\/snapshots\/7\/structurer$/)
  await expect(page.getByRole('navigation', { name: 'Fil d’Ariane' })).toContainText('snapshot #7')
  await expectTreeFillsView(page)
  await press(page.locator('.tree-row').filter({ hasText: 'Habitacle' }), touch)
  await expect(page.getByRole('region', { name: 'Détail du nœud' })).toContainText('Habitacle')
})
