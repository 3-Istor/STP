import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

// exploded-3d-view, tranche 2 (ADR 0031) : bouton « Générer les modèles », progression lue sur /3d/status,
// scène relue à la fin avec les modèles persistés (niveau exact, fichier GLB), BOM modifiée → « stale ».
// Le rendu WebGL n'est pas vérifié (absent de certains navigateurs headless) : on vérifie l'API appelée,
// les états affichés et la légende.

const bom = {
  id: 1, name: 'Fiat Multipla', currency: 'EUR', volume: 1000, annual_hours: '1607', destination_country: null,
  price_snapshot_id: null, source_snapshot_id: null, created_at: '2026-11-04T12:00:00Z',
}
const nodes = [
  { id: 1, parent_id: null, name: 'Fiat Multipla', part_id: null, offer_id: null, quantity: '1', production_days: 0, strategy: null, totals: null },
  { id: 2, parent_id: 1, name: 'Liaison au sol', part_id: null, offer_id: null, quantity: '1', production_days: 2, strategy: null, totals: null },
  { id: 3, parent_id: 2, part_id: 102, offer_id: 1020, name: '', quantity: '4', production_days: 0, strategy: null, totals: null },
  { id: 4, parent_id: 2, part_id: 103, offer_id: 1030, name: '', quantity: '2', production_days: 0, strategy: null, totals: null },
]
const parts = [
  { id: 102, name: 'Pneu 205/55', category: 'Roues', zone: 'Liaison au sol', mass_kg: '9', co2_kg_per_unit: '20', production_days: 1,
    offers: [{ id: 1020, supplier_id: 1, currency: 'EUR', lead_time_days: 3, origin_country: 'FR' }] },
  { id: 103, name: 'Jante alu', category: 'Roues', zone: 'Liaison au sol', mass_kg: '8', co2_kg_per_unit: '30', production_days: 1,
    offers: [{ id: 1030, supplier_id: 1, currency: 'EUR', lead_time_days: 3, origin_country: 'FR' }] },
]
const corner = (x: number, z: number) => ({ position: [x, 0.33, z], rotation: [1.5708, 0, 0] })
const placement = (nodeID: number, partID: number, extra: Record<string, unknown>) => ({
  node_id: nodeID, parent_id: 2, part_id: partID, zone: 'Liaison au sol', category: 'Roues', quantity: '4', match_type: 'generated',
  confidence: '0.20', estimated: true, notes: [], shape: { kind: 'wheel', size: [0.65, 0.22, 0.65] },
  instances: [corner(1.45, -0.95), corner(1.45, 0.95), corner(-1.45, -0.95), corner(-1.45, 0.95)], explode: [0, 0.2, 1.2],
  model_id: null, model_url: null, ...extra,
})
const generation = (status: string, done: number, extra: Record<string, unknown> = {}) => ({
  bom_id: 1, status, done, total: 2, error: '', result: null, updated_at: null, ...extra,
})
const doneResult = { finder: 'generated-v1', exact: 1, similar: 0, generated: 1, failed: [] }
const empty = { schedule: null, lead_time_days: 0, available_on: null, critical: [], allowed_days: null, target_met: null, late: [] }

type Fixture = { statuses: Record<string, unknown>[]; posts: number; scenes: number }

async function mockAPI(page: Page, f: Fixture) {
  await page.route('**/api/**', async (route) => {
    const req = route.request()
    const path = new URL(req.url()).pathname
    if (path === '/api/v1/boms/1/3d/generate' && req.method() === 'POST') {
      f.posts++
      await route.fulfill({ status: 202, json: generation('queued', 0) })
      return
    }
    if (path === '/api/v1/boms/1/3d/status') {
      // Chaque relecture avance d'un cran ; la dernière valeur reste.
      const next = f.statuses.length > 1 ? f.statuses.shift() : f.statuses[0]
      await route.fulfill({ json: next })
      return
    }
    if (path === '/api/v1/boms/1/3d') {
      f.scenes++
      // Avant la génération : primitives ; après : le pneu a un modèle exact avec son fichier.
      const generated = f.posts > 0 && f.statuses.length === 1
      await route.fulfill({
        json: {
          bom_id: 1, source: 'generated', vehicle: { length: 4.4, height: 1.5, width: 1.8 },
          zones: [{ zone: 'Liaison au sol', count: 2, known: true }],
          placements: [
            placement(3, 102, generated ? { match_type: 'exact', confidence: '1.00', model_id: 7, model_url: '/api/v1/models/7/file' } : {}),
            placement(4, 103, generated ? { model_id: 8 } : {}),
          ],
          missing: [],
          generation: generated ? generation('done', 2, { result: doneResult }) : generation('none', 0),
        },
      })
      return
    }
    if (path === '/api/v1/models/7/file') {
      // Pas un vrai GLB : le chargeur échoue et la pièce garde sa primitive (chargement progressif).
      await route.fulfill({ status: 404, json: { error: 'fixture sans fichier' } })
      return
    }
    const json = {
      '/api/v1/boms': [bom],
      '/api/v1/boms/1/tree': { bom, nodes, explanation: { currency: 'EUR', price_snapshot_id: null, rates: [] }, stock: [], planning: empty },
      '/api/v1/boms/1/anomalies': { threshold: '3', currency: 'EUR', price_snapshot_id: null, rates: [], nodes: [] },
      '/api/v1/parts': parts,
      '/api/v1/suppliers': [{ id: 1, name: 'Atlas' }],
      '/api/v1/fx': { rates: [] },
    }[path]
    if (json === undefined) await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
    else await route.fulfill({ json })
  })
}

test('générer les modèles : progression, scène relue avec le modèle exact', async ({ page }) => {
  const f: Fixture = { statuses: [generation('running', 1), generation('done', 2, { result: doneResult })], posts: 0, scenes: 0 }
  await mockAPI(page, f)
  await page.goto('/')
  await page.getByRole('listitem').filter({ hasText: 'Fiat Multipla' }).click()
  await page.getByRole('group', { name: 'Mode de visualisation' }).getByRole('button', { name: 'Vue 3D' }).click()
  const view3d = page.getByRole('region', { name: 'Vue 3D de la BOM' })
  await expect(view3d).toBeVisible()
  const controls = view3d.getByRole('group', { name: 'Génération des modèles' })

  // Aucune génération : le bouton est proposé ; le clic lance la génération (202) et la progression apparaît.
  await controls.getByRole('button', { name: 'Générer les modèles' }).click()
  await expect.poll(() => f.posts).toBe(1)
  await expect(controls.getByRole('button')).toBeDisabled()
  await expect(controls.getByRole('progressbar', { name: "Avancement de la génération" })).toBeVisible()
  await expect(controls).toContainText('1 / 2')

  // Fin de la génération : la scène est relue, le pneu a un modèle exact, la jante garde sa forme générée.
  await expect(controls).toContainText('2 modèles', { timeout: 10_000 })
  await expect(controls.getByRole('button', { name: 'Régénérer les modèles' })).toBeEnabled()
  const tree = page.getByRole('tree', { name: 'Arbre de la BOM' })
  await page.getByRole('group', { name: 'Mode de visualisation' }).getByRole('button', { name: 'Vue BOM' }).click()
  await tree.getByRole('treeitem').filter({ hasText: 'Pneu 205/55' }).locator('.name').click()
  await page.getByRole('group', { name: 'Mode de visualisation' }).getByRole('button', { name: 'Vue 3D' }).click()
  await expect(view3d).toContainText('modèle exact')
  await expect(view3d).toContainText('fichier GLB')
  await expect(view3d).toContainText('confiance 1')
  await page.getByRole('group', { name: 'Mode de visualisation' }).getByRole('button', { name: 'Vue BOM' }).click()
  await tree.getByRole('treeitem').filter({ hasText: 'Jante alu' }).locator('.name').click()
  await page.getByRole('group', { name: 'Mode de visualisation' }).getByRole('button', { name: 'Vue 3D' }).click()
  await expect(view3d).toContainText('forme générée')
  await expect(view3d).not.toContainText('fichier GLB')
})

test('génération en cours au chargement, puis BOM modifiée : stale signalé', async ({ page }) => {
  const f: Fixture = { statuses: [generation('stale', 2, { result: doneResult })], posts: 0, scenes: 0 }
  await mockAPI(page, f)
  await page.route('**/api/v1/boms/1/3d', async (route) => {
    await route.fulfill({
      json: {
        bom_id: 1, source: 'generated', vehicle: { length: 4.4, height: 1.5, width: 1.8 },
        zones: [{ zone: 'Liaison au sol', count: 2, known: true }],
        placements: [placement(3, 102, { model_id: 7 }), placement(4, 103, { model_id: 8 })], missing: [],
        generation: generation('stale', 2, { result: doneResult }),
      },
    })
  })
  await page.goto('/')
  await page.getByRole('listitem').filter({ hasText: 'Fiat Multipla' }).click()
  await page.getByRole('group', { name: 'Mode de visualisation' }).getByRole('button', { name: 'Vue 3D' }).click()
  const controls = page.getByRole('region', { name: 'Vue 3D de la BOM' }).getByRole('group', { name: 'Génération des modèles' })
  await expect(controls).toContainText('BOM modifiée depuis la génération')
  await expect(controls.getByRole('button', { name: 'Régénérer les modèles' })).toBeEnabled()
  expect(f.posts).toBe(0)
})
