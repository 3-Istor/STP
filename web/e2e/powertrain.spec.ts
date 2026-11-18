import { expect, test, type Page } from '@playwright/test'
import profile from '../src/bom/data/powertrain-valhalla.json' with { type: 'json' }

// Montants de réponse arbitraires : le navigateur doit afficher l'API, pas recalculer.
// Les quantités, communs, prix/change et l'absence de double comptage sont prouvés en Go.
async function fixture(page: Page, unavailable = false, removedCount = 0) {
  const money = (amount: string) => ({ amount, currency: 'GBP' })
  const totals = (amount: string | null) => ({ real_quantity: '1', cost: amount ? money(amount) : null,
    cost_error_code: amount ? null : 'missing_cost', cost_error: amount ? null : 'offre sans cotation', missing_cost: amount ? [] : [32415],
    co2_kg: '2', mass_kg: '3', lead_time_days: 4, own_cost: null, own_lead_time_days: 0, quote: null, rate: null,
    labor_hours: null, labor_error: null, missing_labor: [], fte: null, stock_covered: null, in_stock: false })
  const extras = [[32412, 29978], [32414, 32413], [34538, 34537], [34606, 34537], [34674, 34537], [34757, 34537]]
  const nodes = [...profile.guards, ...extras.map(([id, parent_id]) => ({ id, parent_id, part_id: null })),
    ...Array.from({ length: removedCount }, (_, i) => ({ id: 100000 + i, parent_id: 34537, part_id: 325 }))].map(n => ({ ...n,
    name: `Occurrence ${n.id}`, offer_id: n.part_id, quantity: '1', production_days: 0, strategy: null, totals: totals('987.65') }))
  const bom = { id: 2, name: 'Aston Martin Valhalla', currency: 'GBP', volume: 1, annual_hours: '1607', destination_country: 'GB', price_snapshot_id: null as number | null, source_snapshot_id: null, created_at: '2026-01-01T00:00:00Z' }
  const explanation = { currency: 'GBP', price_snapshot_id: null as number | null, rates: [], cost_method: { basis: 'purchase', fx: 'latest', program_volume: null }, cost_as_of: '2026-01-01T00:00:00Z', budget_rates: [] }
  const tree = { bom, nodes, explanation, stock: [], budgets: [], planning: null }
  const calls: string[] = [], requests: unknown[] = []
  let thermalAmount = '731.29'
  await page.route('**/api/**', async route => {
    const req = route.request(), path = new URL(req.url()).pathname
    if (req.method() !== 'GET') calls.push(`${req.method()} ${path}`)
    if (path.endsWith('/configuration-simulation')) {
      requests.push(req.postDataJSON())
      const scenarios = profile.scenarios.map(s => {
        const excluded = new Set<number>(s.excluded_roots)
        for (let pass = 0; pass < nodes.length; pass++) for (const n of nodes) if (n.parent_id !== null && excluded.has(n.parent_id)) excluded.add(n.id)
        const active = nodes.filter(n => !excluded.has(n.id))
        const amount = unavailable && s.key !== 'electric' ? null : s.key === 'thermal' ? thermalAmount : s.key === 'electric' ? '612.43' : '987.65'
        return { key: s.key, active_ids: active.map(n => n.id), excluded_ids: [...excluded], delta: amount && !unavailable ? money(s.key === 'hybrid' ? '0' : '-12.34') : null,
          delta_percent: amount && !unavailable ? '-1.25' : null, tree: { ...tree, nodes: active.map(n => ({ ...n, totals: totals(amount) })) } }
      })
      return route.fulfill({ json: { root_id: profile.root_id, persistent: false, price_as_of: '2026-01-01T00:00:00Z', scenarios } })
    }
    const json = path === '/api/v1/boms' ? [bom] : path === '/api/v1/boms/2/tree' ? tree : path.endsWith('/anomalies') ? { nodes: [], rates: [] }
      : path.endsWith('/profile') ? { preset: null, weights: null }
      : path === '/api/v1/parts' ? [...new Set(profile.guards.flatMap(g => g.part_id === null ? [] : [g.part_id]))].map(id => ({ id, name: `Pièce ${id}`, category: 'Test', co2_kg_per_unit: '1', mass_kg: '1', production_days: 0, offers: [] }))
      : path === '/api/v1/fx' ? { rates: [] } : []
    return route.fulfill({ json })
  })
  await page.goto('/#/boms/2/structurer')
  return { calls, requests, changeQuote: () => { thermalAmount = '744.91' }, changeSettings: () => {
    bom.volume = 100; bom.annual_hours = '1750'; bom.price_snapshot_id = 73; explanation.price_snapshot_id = 73
  } }
}

test('motorisations : coût API, BOM active, comparaison commune, actualisation et sortie sans écriture', async ({ page }) => {
  const f = await fixture(page)
  const panel = page.getByRole('region', { name: 'Simulation de motorisation' })
  const select = (name: string) => panel.getByRole('radio', { name, exact: true }).click()
  const summaryCost = page.getByRole('region', { name: 'Synthèse', exact: true }).locator('.total .vl')
  await expect(summaryCost).toContainText('987,65')
  await select('Thermique')
  await expect(summaryCost).toContainText('731,29')
  await expect(page.getByRole('searchbox', { name: 'Rechercher dans la BOM' })).toBeVisible()
  await page.getByRole('searchbox', { name: 'Rechercher dans la BOM' }).fill('Occurrence 34537')
  await expect(page.getByRole('treeitem')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '+ Ajouter une pièce', exact: true })).toHaveCount(0)
  await select('Électrique')
  await expect(summaryCost).toContainText('612,43')
  await expect(panel.getByText('Simulation électrique : scénario de démonstration avec hypothèses techniques.', { exact: true })).toBeVisible()
  const assumptions = panel.locator('details').filter({ has: page.locator('summary', { hasText: 'Hypothèses et limites techniques' }) })
  await expect(assumptions).not.toHaveAttribute('open', '')
  await assumptions.locator('summary').click()
  await expect(assumptions).toContainText('batterie BEV adaptée')
  await expect(assumptions).toContainText('coûts additionnels non établis')
  await expect(assumptions).toContainText('transformations non dimensionnées')
  await assumptions.locator('summary').click()
  await page.getByRole('searchbox', { name: 'Rechercher dans la BOM' }).fill('Occurrence 32413')
  await expect(page.getByRole('treeitem')).toHaveCount(0)
  await select('Hybride')
  await expect(summaryCost).toContainText('987,65')
  await expect(page.getByRole('treeitem').filter({ hasText: 'Occurrence 32413' })).toHaveCount(1)
  expect(f.requests).toHaveLength(1)
  expect(f.requests[0]).toEqual({ root_id: profile.root_id, guards: profile.guards, scenarios: profile.scenarios })
  f.changeQuote()
  await panel.getByRole('button', { name: 'Actualiser la comparaison' }).click()
  await expect(panel.locator('[data-scenario="thermal"]')).toContainText('744,91')
  expect(f.requests).toHaveLength(2)
  await panel.getByRole('button', { name: 'Quitter la simulation' }).click()
  await expect(summaryCost).toContainText('987,65')
  await expect(page.getByRole('button', { name: '+ Ajouter une pièce', exact: true })).toBeVisible()
  expect(f.calls.every(c => c === 'POST /api/v1/boms/2/configuration-simulation')).toBe(true)
})

test('comparaison actualisée : volume et source des prix suivent les réglages capturés dans un autre onglet', async ({ page }) => {
  const f = await fixture(page)
  const panel = page.getByRole('region', { name: 'Simulation de motorisation' })
  await panel.getByRole('radio', { name: 'Thermique', exact: true }).click()
  await expect(panel.locator('[data-scenario=thermal]')).toBeVisible()
  await page.evaluate(() => { location.hash = '/boms/2/structurer?node=32415' })
  const detail = page.getByRole('region', { name: 'Détail du nœud' })
  const ordered = detail.locator('dt:has-text("Quantité commandée") + dd')
  await expect(ordered).toHaveText('1 pour 1 véhicules')
  f.changeSettings()
  await panel.getByRole('button', { name: 'Actualiser la comparaison' }).click()
  await expect(ordered).toHaveText('100 pour 100 véhicules')
  await expect(page.getByRole('region', { name: 'Synthèse', exact: true })).toContainText('Snapshot de prix #73.')
  expect(f.requests).toHaveLength(2)
  expect(f.calls.every(c => c === 'POST /api/v1/boms/2/configuration-simulation')).toBe(true)
})

test('prix manquant : coût incomplet et écart indisponible, scénario excluant la pièce toujours calculable', async ({ page }) => {
  await fixture(page, true)
  const panel = page.getByRole('region', { name: 'Simulation de motorisation' })
  await panel.getByRole('radio', { name: 'Thermique', exact: true }).click()
  await expect(panel.locator('[data-scenario="thermal"]')).toContainText('Coût incomplet')
  await expect(panel.locator('[data-scenario="hybrid"]')).toContainText('n/d')
  await expect(panel.locator('[data-scenario="electric"]')).toContainText('612,43')
  await expect(page.getByRole('region', { name: 'Synthèse', exact: true }).locator('.total .vl')).toHaveText('incomplet')
  await expect(panel).toContainText('Des prix ou taux manquent')
})

test('panne API : référence conservée, reprise explicite et aucune simulation faussement annoncée', async ({ page }) => {
  const f = await fixture(page)
  await page.route('**/api/v1/boms/2/configuration-simulation', async route => {
    await route.fulfill({ status: 503, json: { error: 'source de prix indisponible' } })
    await page.unroute('**/api/v1/boms/2/configuration-simulation')
  })
  const panel = page.getByRole('region', { name: 'Simulation de motorisation' })
  await panel.getByRole('radio', { name: 'Thermique', exact: true }).click()
  await expect(panel.getByRole('alert')).toContainText('source de prix indisponible')
  const summary = page.getByRole('region', { name: 'Synthèse', exact: true })
  await expect(summary.locator('.total .vl')).toContainText('987,65')
  await expect(summary).not.toContainText('scénario calculé')
  await panel.getByRole('button', { name: 'Actualiser la comparaison' }).click()
  await expect(summary.locator('.total .vl')).toContainText('731,29')
  expect(f.requests).toHaveLength(1)
})

test('scénario choisi dans Arbre : mécanique révélée lors de chaque ouverture 3D ou Arbre + 3D', async ({ page }) => {
  await fixture(page)
  // Le test protège l'état des contrôles au montage ; aucune géométrie fictive n'est nécessaire.
  await page.route('**/models/**', route => route.fulfill({ status: 404 }))
  const panel = page.getByRole('region', { name: 'Simulation de motorisation' })
  for (const mode of ['Thermique', 'Électrique', 'Hybride']) {
    await panel.getByRole('radio', { name: mode, exact: true }).click()
    for (const view of ['3D', 'Arbre + 3D']) {
      await page.getByRole('radio', { name: view, exact: true }).click()
      await page.getByRole('button', { name: 'Réinitialiser', exact: true }).waitFor()
      await expect(page.getByLabel('Carrosserie transparente', { exact: true })).toBeChecked()
      await page.getByRole('radio', { name: 'Arbre', exact: true }).click()
    }
  }
})

for (const viewport of [{ width: 1366, height: 768 }, { width: 1280, height: 720 }]) {
  test(`laptop ${viewport.width}×${viewport.height} : panneaux et listes accessibles jusqu’en bas`, async ({ page }) => {
    await page.setViewportSize(viewport)
    await fixture(page, false, 300)
    await page.route('**/models/manifests/draft-2.json', route => route.fulfill({ json: {
      schema_version: 1, revision: 'scroll-fixture', scope: { kind: 'draft', id: 2 }, vehicle_label: 'BOM de test sans géométrie', assets: [], bindings: [], assemblies: [],
    } }))
    const panel = page.getByRole('region', { name: 'Simulation de motorisation' })
    await panel.getByRole('radio', { name: 'Hybride', exact: true }).click()
    await expect(panel.locator('[data-scenario=hybrid]')).toBeVisible()
    for (const view of ['3D', 'Arbre + 3D']) {
      await page.getByRole('radio', { name: view, exact: true }).click()
      await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'ready', { timeout: 30_000 })
      await page.getByText('Composants et sources', { exact: true }).click()
      await page.getByText('Occurrences indisponibles (', { exact: false }).click()
      const settings = page.locator('.v3d-settings')
      await expect.poll(() => settings.evaluate(el => {
        const root = el.closest('.v3d-root')!, r = root.getBoundingClientRect(), p = el.getBoundingClientRect()
        return root.scrollTop === 0 && p.top >= r.top && p.bottom <= r.bottom
      })).toBe(true)
      await expect(page.getByLabel('Manifeste d’assemblage', { exact: true })).not.toBeVisible()
      await settings.hover()
      await page.mouse.wheel(0, 20000)
      await expect.poll(() => settings.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2)
      await page.getByText('Provenance et licences', { exact: true }).click()
      await page.getByText('Composants et sources', { exact: true }).click()
    }
    await panel.getByRole('radio', { name: 'Thermique', exact: true }).click()
    await panel.locator('summary').filter({ hasText: 'Composants retirés' }).click()
    const removed = page.getByRole('region', { name: 'Liste des composants retirés' })
    await expect(removed.locator('li')).toHaveCount(100)
    await expect.poll(() => removed.evaluate(el => el.scrollHeight > el.clientHeight)).toBe(true)
    await removed.hover(); await page.mouse.wheel(0, 20000)
    await removed.getByRole('button', { name: 'Afficher 100 suivantes', exact: true }).click()
    await expect(removed.locator('li')).toHaveCount(200)
    const main = page.locator('.main4')
    await page.getByRole('radio', { name: '3D', exact: true }).click()
    await main.hover({ position: { x: 3, y: 100 } }); await page.mouse.wheel(0, 20000)
    await expect.poll(() => main.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2)
    await expect(page.locator('.tfoot')).toBeInViewport()
    await page.getByRole('tab', { name: 'Réglages de calcul', exact: true }).click()
    const sidebar = page.locator('.col')
    await sidebar.hover({ position: { x: 3, y: 100 } }); await page.mouse.wheel(0, 20000)
    await expect.poll(() => sidebar.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2)
    await expect(page.getByLabel('Pays de destination', { exact: true })).toBeInViewport()
  })
}
