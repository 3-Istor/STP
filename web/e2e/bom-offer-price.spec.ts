import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

// NLPF-72 : le sélecteur d'offres affiche le prix de chaque offre cotée, dans sa devise d'origine, à la
// même source que les totaux de la BOM (direct ou snapshot de prix) ; une offre non cotée reste « sans prix ».

const asOf = '2026-11-04T12:00:00Z'
const part = {
  id: 10, name: 'Injecteur', category: 'moteur', co2_kg_per_unit: null, mass_kg: null, production_days: null,
  offers: [
    { id: 101, supplier_id: 1, currency: 'EUR', lead_time_days: 12, origin_country: 'FR' },
    { id: 102, supplier_id: 2, currency: 'USD', lead_time_days: 5, origin_country: 'US' },
    { id: 103, supplier_id: 1, currency: 'EUR', lead_time_days: 3, origin_country: 'FR' },
  ],
}
const nodes = [
  { id: 1, parent_id: null, name: 'Fiat Multipla', part_id: null, offer_id: null, quantity: '1', production_days: 0, totals: null },
  { id: 2, parent_id: 1, name: '', part_id: 10, offer_id: 101, quantity: '4', production_days: 0, totals: null },
]
const quote = (offerID: number, amount: string, currency: string) => ({
  id: offerID * 10, offer_id: offerID, price: { amount, currency }, source: 'e2e-fixture', quoted_at: asOf,
})

async function mockAPI(page: Page, priceSnapshotID: number | null) {
  const bom = {
    id: 1, name: 'Fiat Multipla', currency: 'EUR', volume: 1000,
    price_snapshot_id: priceSnapshotID, source_snapshot_id: null, created_at: asOf,
  }
  const reads: string[] = []
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    reads.push(path)
    const json = {
      '/api/v1/boms': [bom],
      '/api/v1/boms/1/tree': { bom, nodes, explanation: { currency: 'EUR', price_snapshot_id: priceSnapshotID, rates: [] } },
      '/api/v1/parts': [part],
      '/api/v1/suppliers': [{ id: 1, name: 'Atlas' }, { id: 2, name: 'Boréal' }],
      // En direct : 101 et 102 cotées, 103 en erreur de cotation.
      '/api/v1/quotes': {
        context: { mode: 'live', as_of: asOf, tick: 7 },
        quotes: [
          { offer_id: 101, quote: quote(101, '92.00', 'EUR') },
          { offer_id: 102, quote: quote(102, '100.5', 'USD') },
          { offer_id: 103, quote: null, error: 'no price quote' },
        ],
        coverage_complete: false,
      },
      // Snapshot de prix 5 : prix figé différent du direct, 102 et 103 non couvertes.
      '/api/v1/price-snapshots/5': {
        id: 5, scope: 'bom', scope_id: 1, taken_at: '2026-11-01T08:00:00Z',
        quotes: [quote(101, '80.00', 'EUR')], rates: [],
        context: { mode: 'snapshot', as_of: '2026-11-01T08:00:00Z', snapshot_id: 5 },
      },
    }[path]
    if (json === undefined) await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
    else await route.fulfill({ json })
  })
  return reads
}

async function openOfferSelect(page: Page) {
  await page.goto('/#boms')
  await page.getByRole('button', { name: 'Fiat Multipla', exact: true }).click()
  return page.locator('.tree-row').filter({ hasText: 'Injecteur' }).getByRole('combobox', { name: 'Offre' })
}

test('BOM en direct : prix de la dernière cotation, « sans prix » sans cotation', async ({ page }) => {
  await mockAPI(page, null)
  const options = (await openOfferSelect(page)).locator('option')
  await expect(options).toHaveText([/^Atlas · 92,00\s€ · 12 j$/, /^Boréal · 100,50\s\$US · 5 j$/, 'Atlas · sans prix · 3 j'])
  await expect(options.nth(0)).toHaveAttribute('title', 'en direct au 04/11/2026 12:00 UTC')
  await expect(options.nth(2)).not.toHaveAttribute('title')
})

test('BOM figée : prix du snapshot de prix, pas la cotation en direct', async ({ page }) => {
  const reads = await mockAPI(page, 5)
  const options = (await openOfferSelect(page)).locator('option')
  await expect(options).toHaveText([/^Atlas · 80,00\s€ · 12 j$/, 'Boréal · sans prix · 5 j', 'Atlas · sans prix · 3 j'])
  await expect(options.nth(0)).toHaveAttribute('title', 'snapshot de prix #5 du 01/11/2026 08:00 UTC')
  expect(reads).not.toContain('/api/v1/quotes')
})
