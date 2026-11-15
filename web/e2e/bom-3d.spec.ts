import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

// exploded-3d-view, tranche 1 (ADR 0031) : bascule Vue BOM / Vue 3D sans rechargement, arbre intact,
// sélection conservée d'une vue à l'autre, pièce sans modèle signalée. Le rendu WebGL lui-même n'est pas
// vérifié ici (absent de certains navigateurs headless) : on vérifie la bascule, l'état et la synchronisation.

const bom = {
  id: 1, name: 'Fiat Multipla', currency: 'EUR', volume: 1000, annual_hours: '1607', destination_country: null,
  price_snapshot_id: null, source_snapshot_id: null, created_at: '2026-11-04T12:00:00Z',
}
const nodes = [
  { id: 1, parent_id: null, name: 'Fiat Multipla', part_id: null, offer_id: null, quantity: '1', production_days: 0, strategy: null, totals: null },
  { id: 2, parent_id: 1, name: 'Liaison au sol', part_id: null, offer_id: null, quantity: '1', production_days: 2, strategy: null, totals: null },
  { id: 3, parent_id: 2, part_id: 102, offer_id: 1020, name: '', quantity: '4', production_days: 0, strategy: null, totals: null },
  { id: 4, parent_id: 2, part_id: 105, offer_id: 1050, name: '', quantity: '1', production_days: 0, strategy: null, totals: null },
]
const parts = [
  { id: 102, name: 'Pneu 205/55', category: 'Roues', zone: 'Liaison au sol', mass_kg: '9', co2_kg_per_unit: '20', production_days: 1,
    offers: [{ id: 1020, supplier_id: 1, currency: 'EUR', lead_time_days: 3, origin_country: 'FR' }] },
  { id: 105, name: 'Pièce fantôme', category: 'Divers', zone: 'Liaison au sol', mass_kg: null, co2_kg_per_unit: null, production_days: null,
    offers: [{ id: 1050, supplier_id: 1, currency: 'EUR', lead_time_days: 3, origin_country: 'FR' }] },
]
const corner = (x: number, z: number) => ({ position: [x, 0.33, z], rotation: [1.5708, 0, 0] })
const assembly = {
  bom_id: 1, source: 'generated', vehicle: { length: 4.4, height: 1.5, width: 1.8 },
  zones: [{ zone: 'Liaison au sol', count: 1, known: true }],
  placements: [{
    node_id: 3, parent_id: 2, part_id: 102, zone: 'Liaison au sol', category: 'Roues', quantity: '4', match_type: 'generated',
    confidence: '0.20', estimated: true, notes: [], shape: { kind: 'wheel', size: [0.65, 0.22, 0.65] },
    instances: [corner(1.45, -0.95), corner(1.45, 0.95), corner(-1.45, -0.95), corner(-1.45, 0.95)], explode: [0, 0.2, 1.2],
  }],
  missing: [{ node_id: 4, part_id: 105, reason: 'pièce absente du catalogue' }],
}
const empty = { schedule: null, lead_time_days: 0, available_on: null, critical: [], allowed_days: null, target_met: null, late: [] }

async function mockAPI(page: Page, counts: Record<string, number>) {
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    counts[path] = (counts[path] ?? 0) + 1
    const json = {
      '/api/v1/boms': [bom],
      '/api/v1/boms/1/tree': { bom, nodes, explanation: { currency: 'EUR', price_snapshot_id: null, rates: [] }, stock: [], planning: empty },
      '/api/v1/boms/1/3d': assembly,
      '/api/v1/boms/1/anomalies': { threshold: '3', currency: 'EUR', price_snapshot_id: null, rates: [], nodes: [] },
      '/api/v1/parts': parts,
      '/api/v1/suppliers': [{ id: 1, name: 'Atlas' }],
      '/api/v1/fx': { rates: [] },
    }[path]
    if (json === undefined) await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
    else await route.fulfill({ json })
  })
}

test('bascule Vue BOM / Vue 3D, sélection conservée, pièce sans modèle signalée', async ({ page }) => {
  const counts: Record<string, number> = {}
  await mockAPI(page, counts)
  await page.goto('/')
  await page.getByRole('listitem').filter({ hasText: 'Fiat Multipla' }).click()

  // Vue BOM par défaut, arbre tel qu'avant : on sélectionne le pneu.
  const switchGroup = page.getByRole('group', { name: 'Mode de visualisation' })
  await expect(switchGroup.getByRole('button', { name: 'Vue BOM' })).toHaveAttribute('aria-pressed', 'true')
  const tree = page.getByRole('tree', { name: 'Arbre de la BOM' })
  await expect(tree).toBeVisible()
  // Clic sur le libellé : le centre d'une ligne pièce est son sélecteur d'offre, qui ne sélectionne pas le nœud.
  await tree.getByRole('treeitem').filter({ hasText: 'Pneu 205/55' }).locator('.name').click()
  await expect(page.getByRole('complementary', { name: 'Détail du nœud' })).toContainText('Pneu 205/55')

  // Vue 3D : même URL, pas de rechargement ; la scène est lue une fois ; la sélection est conservée.
  await switchGroup.getByRole('button', { name: 'Vue 3D' }).click()
  await expect(switchGroup.getByRole('button', { name: 'Vue 3D' })).toHaveAttribute('aria-pressed', 'true')
  await expect(tree).toBeHidden()
  const view3d = page.getByRole('region', { name: 'Vue 3D de la BOM' })
  await expect(view3d).toBeVisible()
  await expect(view3d.getByLabel("Niveau d'éclatement")).toBeVisible()
  await expect(view3d).toContainText('Pneu 205/55')
  await expect(view3d).toContainText('forme générée')
  await expect(view3d).toContainText('position estimée')
  await expect(page.getByRole('complementary', { name: 'Détail du nœud' })).toContainText('Pneu 205/55')

  // La pièce sans modèle reste dans la BOM et est signalée ; la choisir la sélectionne aussi.
  await view3d.getByText('1 pièce sans modèle 3D').click()
  await view3d.getByRole('button', { name: 'Pièce fantôme' }).click()
  await expect(page.getByRole('complementary', { name: 'Détail du nœud' })).toContainText('Pièce fantôme')
  await expect(view3d).toContainText('pièce sans modèle 3D')

  // Retour à l'arbre : la ligne sélectionnée est dépliée et visible ; rebasculer ne relit ni la scène
  // ni l'arbre (en développement, StrictMode double le premier chargement : on compare avant/après).
  const before = { scene: counts['/api/v1/boms/1/3d'], tree: counts['/api/v1/boms/1/tree'] }
  await view3d.getByRole('button', { name: "Voir dans l'arbre" }).click()
  await expect(tree).toBeVisible()
  await expect(tree.getByRole('treeitem', { selected: true })).toContainText('Pièce fantôme')
  await switchGroup.getByRole('button', { name: 'Vue 3D' }).click()
  await expect(view3d).toBeVisible()
  await expect(view3d).toContainText('Pièce fantôme')
  expect(counts['/api/v1/boms/1/3d']).toBe(before.scene)
  expect(counts['/api/v1/boms/1/tree']).toBe(before.tree)
})
