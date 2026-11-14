import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

// Lisibilité de l'arbre BOM : frères triés par ordre alphabétique, en-têtes de colonnes, seuls les
// premiers niveaux dépliés à l'ouverture, recherche (casse et accents ignorés) qui garde le chemin
// jusqu'aux résultats. Devise de référence choisie dans une liste issue des taux publiés.

const bom = {
  id: 1, name: 'Fiat Multipla', currency: 'EUR', volume: 1000, annual_hours: '1607', destination_country: null,
  price_snapshot_id: null, source_snapshot_id: null, created_at: '2026-11-04T12:00:00Z',
}
const node = (id: number, parent_id: number | null, name: string) =>
  ({ id, parent_id, name, part_id: null, offer_id: null, quantity: '1', production_days: 0, totals: null })
// Ids volontairement dans un autre ordre que les noms : l'affichage ne doit pas suivre les ids.
const nodes = [
  node(1, null, 'Fiat Multipla'),
  node(2, 1, 'Habitacle'),
  node(3, 1, 'Châssis'),
  node(4, 1, 'Aérodynamique'),
  node(5, 3, 'Train avant'),
  node(6, 5, 'Écrou M8'),
  node(7, 5, 'Bras de suspension'),
]

async function mockAPI(page: Page) {
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const json = {
      '/api/v1/boms': [bom, { ...bom, id: 2, name: 'Aston Martin', currency: 'GBP' }],
      '/api/v1/boms/1/tree': { bom, nodes, explanation: { currency: 'EUR', price_snapshot_id: null, rates: [] }, stock: [], planning: { schedule: null, lead_time_days: 0, available_on: null, critical: [], allowed_days: null, target_met: null, late: [] } },
      '/api/v1/boms/1/anomalies': { threshold: '3', currency: 'EUR', price_snapshot_id: null, rates: [], nodes: [] },
      '/api/v1/parts': [],
      '/api/v1/suppliers': [],
      '/api/v1/fx': { rates: [{ base_currency: 'USD', quote_currency: 'EUR' }, { base_currency: 'EUR', quote_currency: 'USD' }] },
    }[path]
    if (json === undefined) await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
    else await route.fulfill({ json })
  })
}

const rowNames = (page: Page) => page.locator('.bom-space .tree-row .name')

test('arbre trié, en-têtes de colonnes, premiers niveaux dépliés et recherche', async ({ page }) => {
  await mockAPI(page)
  await page.goto('/#boms')
  // Liste des BOM triée par nom.
  await expect(page.locator('.bom-list li button.link')).toHaveText(['Aston Martin', 'Fiat Multipla'])
  await page.getByRole('button', { name: 'Fiat Multipla', exact: true }).click()

  await expect(page.locator('.bom-space .tree-head')).toContainText('Ensemble / pièce')
  await expect(page.locator('.bom-space .tree-head')).toContainText('Coût / véhicule')
  // Profondeur 0 à 2 visible, « Train avant » (profondeur 2) replié : ses enfants sont cachés.
  await expect(rowNames(page)).toHaveText(['Fiat Multipla', 'Aérodynamique', 'Châssis', 'Train avant', 'Habitacle'])
  await expect(page.getByRole('treeitem').filter({ hasText: 'Train avant' })).toHaveAttribute('aria-expanded', 'false')

  const search = page.getByRole('searchbox', { name: 'Rechercher dans la BOM' })
  await search.fill('ecrou')
  await expect(rowNames(page)).toHaveText(['Fiat Multipla', 'Châssis', 'Train avant', 'Écrou M8'])
  await search.fill('châssis')
  // Un ensemble trouvé garde son sous-arbre (selon l'état replié).
  await expect(rowNames(page)).toHaveText(['Fiat Multipla', 'Châssis', 'Train avant'])
  await search.fill('introuvable')
  await expect(page.getByText('Aucun résultat pour « introuvable ».')).toBeVisible()
  await search.fill('')
  await expect(rowNames(page)).toHaveCount(5)
})

test('devise de référence choisie dans la liste des devises cotées', async ({ page }) => {
  await mockAPI(page)
  await page.goto('/#boms')
  // Taux publiés (EUR, USD) plus devises déjà utilisées par une BOM (GBP), sans doublon, triées.
  await expect(page.getByRole('combobox', { name: 'Devise', exact: true }).locator('option')).toHaveText(['EUR', 'GBP', 'USD'])
  await page.getByRole('button', { name: 'Fiat Multipla', exact: true }).click()
  await expect(page.getByRole('combobox', { name: 'Devise de référence' })).toHaveValue('EUR')
})

test('toute la ligne d’une BOM l’ouvre, pas seulement son nom', async ({ page }, testInfo) => {
  await mockAPI(page)
  await page.goto('/#boms')
  const details = page.locator('.bom-list li').filter({ hasText: 'Fiat Multipla' }).locator('.muted')
  if (testInfo.project.use.hasTouch) await details.tap()
  else await details.click()
  await expect(page.getByRole('heading', { name: /Fiat Multipla/ })).toBeVisible()
  await expect(page.locator('.bom-list li.selected')).toContainText('Fiat Multipla')
})
