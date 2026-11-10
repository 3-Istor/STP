import { expect, test } from '@playwright/test'
import type { Locator, Page } from '@playwright/test'

// Non-régression NLPF-71 : une règle de mise en page de l'espace Fournisseurs (.workspace global)
// réduisait la colonne de l'arbre BOM à ~2 px (arbre invisible, non cliquable).

const bom = {
  id: 1, name: 'Fiat Multipla', currency: 'EUR', volume: 1000,
  price_snapshot_id: null, source_snapshot_id: 7, created_at: '2026-11-04T12:00:00Z',
}
const nodes = [
  { id: 1, parent_id: null, name: 'Fiat Multipla', part_id: null, offer_id: null, quantity: '1', production_days: 0, totals: null },
  { id: 2, parent_id: 1, name: 'Châssis', part_id: null, offer_id: null, quantity: '1', production_days: 2, totals: null },
  { id: 3, parent_id: 1, name: 'Habitacle', part_id: null, offer_id: null, quantity: '1', production_days: 1, totals: null },
]
const explanation = { currency: 'EUR', price_snapshot_id: null, rates: [] }

async function mockBomAPI(page: Page) {
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const json = {
      '/api/v1/boms': [bom],
      '/api/v1/boms/1/tree': { bom, nodes, explanation },
      '/api/v1/snapshots/7': {
        snapshot: { id: 7, bom_id: 1, price_snapshot_id: 3, name: 'Fiat Multipla', currency: 'EUR', volume: 1000, published_at: '2026-11-04T12:00:00Z' },
        nodes,
        explanation: { ...explanation, price_snapshot_id: 3 },
      },
      '/api/v1/parts': [],
      '/api/v1/suppliers': [],
    }[path]
    if (json === undefined) await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
    else await route.fulfill({ json })
  })
}

// L'arbre doit occuper la largeur que la vue lui laisse : toute la vue en une colonne (mobile),
// la vue moins le panneau de détail quand ils sont côte à côte. On mesure la vue (.view) et non la
// grille elle-même, car c'est la grille qui se rétractait sur son contenu.
async function expectTreeFillsView(page: Page) {
  const tree = page.locator('.bom-space .tree')
  await expect(tree).toBeVisible()
  const [view, treeBox, panel] = await Promise.all([
    page.locator('.bom-space .view').boundingBox(),
    tree.boundingBox(),
    page.locator('.bom-space .panel').boundingBox(),
  ])
  expect(view && treeBox && panel).toBeTruthy()
  const sideBySide = panel!.x > treeBox!.x + treeBox!.width / 2
  const available = sideBySide ? view!.width - panel!.width : view!.width
  expect(treeBox!.width).toBeGreaterThan(available / 2)
}

async function press(locator: Locator, touch: boolean | undefined) {
  if (touch) await locator.tap()
  else await locator.click()
}

test('arbre BOM pleine largeur et cliquable, brouillon puis snapshot', async ({ page }, testInfo) => {
  const touch = testInfo.project.use.hasTouch
  await mockBomAPI(page)
  await page.goto('/#boms')
  await press(page.getByRole('button', { name: 'Fiat Multipla', exact: true }), touch)

  await expectTreeFillsView(page)
  await press(page.locator('.bom-space .tree-row').filter({ hasText: 'Châssis' }), touch)
  await expect(page.getByRole('complementary', { name: 'Détail du nœud' })).toContainText('Châssis')

  await press(page.getByRole('button', { name: 'snapshot #7', exact: true }), touch)
  await expect(page.getByText('snapshot #7', { exact: true })).toBeVisible()
  await expectTreeFillsView(page)
  await press(page.locator('.bom-space .tree-row').filter({ hasText: 'Habitacle' }), touch)
  await expect(page.getByRole('complementary', { name: 'Détail du nœud' })).toContainText('Habitacle')
})
