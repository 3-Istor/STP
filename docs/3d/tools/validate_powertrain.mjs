// Local, read-only demonstration against the real Go service and the supplied assets.
import { chromium } from '../../../web/node_modules/playwright-core/index.mjs'
import { expect } from '../../../web/node_modules/@playwright/test/index.mjs'
import { createHash } from 'node:crypto'
import { mkdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const base = process.env.STP_3D_URL ?? 'http://127.0.0.1:41769'
if (!['127.0.0.1', 'localhost'].includes(new URL(base).hostname)) throw new Error('Local demonstration only')
const output = fileURLToPath(new URL('../captures/motorisations/', import.meta.url))
await mkdir(output, { recursive: true })
const browser = await chromium.launch({ args: ['--no-sandbox'] })
const page = await browser.newPage({ viewport: { width: 1600, height: 1100 } })
page.setDefaultTimeout(30_000)
const report = { at: new Date().toISOString(), errors: [], mutations: [], legacyRequests: [], scenarios: [] }
page.on('pageerror', error => report.errors.push(error.message))
page.on('request', r => {
  if (r.url().includes('/models/CarConcept')) report.legacyRequests.push(r.url())
  if (new URL(r.url()).pathname.startsWith('/api/') && r.method() !== 'GET' && !r.url().endsWith('/configuration-simulation')) report.mutations.push(r.method() + ' ' + r.url())
})
const hash = text => createHash('sha256').update(text).digest('hex')
const source = async () => (await page.request.get(base + '/api/v1/boms/2/tree')).text()
const probe = () => page.locator('.v3d-root').evaluate(el => ({
  items: el.__viewer.built.items.map(i => ({ id: i.obj.nodeId, key: i.obj.key, visible: i.node.visible, role: i.displayRole, position: i.node.position.toArray() })),
  selected: [...el.__viewer.selection],
}))
async function ready() {
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'ready', { timeout: 120_000 })
  await expect.poll(() => page.locator('.v3d-root').evaluate(el => !!el.__viewer?.built), { timeout: 30_000 }).toBe(true)
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-mode', 'bound')
  await expect(page.locator('.v3d-host')).toHaveAttribute('data-animation', 'idle', { timeout: 30_000 })
  for (const name of ['Simple', 'Réaliste']) await expect(page.getByRole('radio', { name, exact: true })).toHaveCount(0)
  await expect(page.getByRole('radiogroup', { name: 'Rendu', exact: true })).toHaveCount(0)
}
async function capture(name) {
  await ready(); await page.mouse.move(1, 1)
  await page.screenshot({ path: output + name + '.png' })
}
try {
  const before = await source()
  report.referenceHashBefore = hash(before)
  // Forward the real response unchanged; Chromium evicts this 29 MB body from its inspector cache.
  let resolveComparison, rejectComparison
  const response = new Promise((resolve, reject) => { resolveComparison = resolve; rejectComparison = reject })
  await page.route('**/api/v1/boms/2/configuration-simulation', async route => {
    try {
      const actual = await route.fetch()
      expect(actual.status()).toBe(200)
      resolveComparison(await actual.json())
      await route.fulfill({ response: actual })
    } catch (error) { rejectComparison(error); await route.abort() }
  })
  await page.goto(base + '/#/boms/2/structurer?tab=3d')
  await ready(); await capture('valhalla-reference')
  const panel = page.getByRole('region', { name: 'Simulation de motorisation' })
  await page.getByRole('radio', { name: 'Arbre', exact: true }).click()
  await panel.getByRole('radio', { name: 'Thermique', exact: true }).click()
  const comparison = await response
  await expect(panel.getByRole('radio', { name: 'Thermique', exact: true })).toHaveAttribute('aria-checked', 'true')
  await page.getByRole('radio', { name: '3D', exact: true }).click()
  await expect(page.getByLabel('Carrosserie transparente', { exact: true })).toBeChecked()
  expect(comparison.persistent).toBe(false)
  const reference = comparison.scenarios.find(s => s.key === 'hybrid')
  const referenceNodes = new Map(reference.tree.nodes.map(n => [n.id, n]))
  report.priceAsOf = comparison.price_as_of
  report.explanation = reference.tree.explanation
  for (const [key, label, present, absent] of [
    ['thermal', 'Thermique', [32415], [34539, 34607, 34675, 34758]],
    ['electric', 'Électrique', [34539, 34607, 34675, 34758], [32415]],
    ['hybrid', 'Hybride', [32415, 34539, 34607, 34675, 34758], []],
  ]) {
    if (key !== 'thermal') await panel.getByRole('radio', { name: label, exact: true }).click()
    const scenario = comparison.scenarios.find(s => s.key === key)
    const totals = scenario.tree.nodes.find(n => n.id === comparison.root_id).totals
    const expectedCost = new Intl.NumberFormat('fr-FR', { style: 'currency', currency: totals.cost.currency }).format(totals.cost.amount)
    await expect(page.getByRole('region', { name: 'Synthèse', exact: true }).locator('.total .vl')).toHaveText(expectedCost, { timeout: 30_000 })
    await ready()
    const p = await probe(), active = new Set(scenario.active_ids)
    for (const item of p.items) expect(active.has(item.id), '3D item belongs to active BOM #' + item.id).toBe(true)
    for (const id of present) expect(p.items.some(i => i.id === id && i.visible), 'Active propulsion visible #' + id).toBe(true)
    for (const id of absent) expect(p.items.some(i => i.id === id), 'Excluded propulsion absent #' + id).toBe(false)
    // Every common occurrence keeps its identity, quantity and original quote/FX.
    for (const node of scenario.tree.nodes) {
      const ref = referenceNodes.get(node.id)
      expect([node.parent_id, node.part_id, node.offer_id, node.quantity]).toEqual([ref.parent_id, ref.part_id, ref.offer_id, ref.quantity])
      expect(node.totals.quote).toEqual(ref.totals.quote)
      expect(node.totals.rate).toEqual(ref.totals.rate)
    }
    await capture('valhalla-' + key)
    const slider = page.getByLabel('Éclatement', { exact: true })
    await slider.fill('100'); await ready()
    const exploded = await probe()
    expect(exploded.items.some((item, i) => JSON.stringify(item.position) !== JSON.stringify(p.items[i].position))).toBe(true)
    await capture('valhalla-' + key + '-eclatee')
    await slider.fill('0'); await ready()
    // A visual filter must leave both the active BOM and the calculated price intact.
    await page.getByText('Composants et sources', { exact: true }).click()
    await page.getByLabel('Filtre visuel 3D', { exact: true }).selectOption('thermal')
    await expect(page.getByRole('region', { name: 'Synthèse', exact: true }).locator('.total .vl')).toHaveText(expectedCost)
    await page.getByLabel('Filtre visuel 3D', { exact: true }).selectOption('all')
    await page.getByText('Composants et sources', { exact: true }).click()
    await page.getByRole('radio', { name: 'Arbre + 3D', exact: true }).click(); await ready()
    await capture('valhalla-' + key + '-arbre-3d')
    await page.getByRole('radio', { name: '3D', exact: true }).click(); await ready()
    report.scenarios.push({ key, cost: totals.cost, delta: scenario.delta, delta_percent: scenario.delta_percent,
      active_nodes: scenario.active_ids.length, active_parts: scenario.tree.nodes.filter(n => n.part_id !== null).length,
      excluded_nodes: scenario.excluded_ids.length, mass_kg: totals.mass_kg, co2_kg: totals.co2_kg, lead_time_days: totals.lead_time_days,
      visible_propulsion: present, absent_propulsion: absent, bound_items: p.items.length, source_quotes_and_common_quantities_identical: true,
      both_views: true, explode: true, visual_filter_preserves_cost: true })
    process.stdout.write(key + ': BOM, Go cost, 3D, both views, explode and visual-only filter PASS\n')
  }
  await panel.getByRole('button', { name: 'Quitter la simulation' }).click(); await ready()
  await expect(page.getByRole('button', { name: '+ Ajouter une pièce', exact: true })).toBeVisible()
  await expect(page.getByLabel('Carrosserie transparente', { exact: true })).not.toBeChecked()
  const after = await source()
  report.referenceHashAfter = hash(after)
  expect(after).toBe(before)
  expect(report.mutations).toEqual([]); expect(report.legacyRequests).toEqual([]); expect(report.errors).toEqual([])
  report.referenceUnchanged = true
  await capture('valhalla-retour-reference')
} catch (error) {
  report.errors.push(error.message)
  await page.screenshot({ path: output + 'failure.png' }).catch(() => {})
  report.failureDOM = await page.locator('.v3d-root').evaluate(el => ({ keys: Object.keys(el), html: el.outerHTML.slice(0, 1500) })).catch(() => null)
  throw error
} finally {
  await writeFile(output + 'validation.json', JSON.stringify(report, null, 2) + '\n')
  await browser.close()
}
