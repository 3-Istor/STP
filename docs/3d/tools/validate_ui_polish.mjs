// Actual laptop UI, supplied assets and Go prices. No reference BOM writes.
import { chromium } from '../../../web/node_modules/playwright-core/index.mjs'
import { expect } from '../../../web/node_modules/@playwright/test/index.mjs'
import { formatMoney } from '../../../web/src/money.ts'
import { mkdir, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import { fileURLToPath } from 'node:url'

const base = 'http://127.0.0.1:41769'
const output = fileURLToPath(new URL('../captures/polish-ui/', import.meta.url))
await mkdir(output, { recursive: true })
const browser = await chromium.launch({ args: ['--no-sandbox'] })
let page = await browser.newPage({ viewport: { width: 1366, height: 768 } })
page.setDefaultTimeout(30_000)
const report = { at: new Date().toISOString(), errors: [], mutations: [], legacy: [], laptops: [] }
function observe(target) {
  target.on('pageerror', e => report.errors.push(e.message))
  target.on('request', r => {
    if (r.url().includes('/models/CarConcept')) report.legacy.push(r.url())
    if (r.url().includes('/api/') && r.method() !== 'GET' && !r.url().endsWith('/configuration-simulation')) report.mutations.push(r.method() + ' ' + r.url())
  })
}
observe(page)
const referenceHashes = async () => {
  const out = {}
  for (const id of [1, 2]) out[id] = createHash('sha256').update(await (await page.request.get(base + '/api/v1/boms/' + id + '/tree')).body()).digest('hex')
  return out
}
async function ready() {
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'ready', { timeout: 120_000 })
  await expect.poll(() => page.locator('.v3d-root').evaluate(el => !!el.__viewer?.built), { timeout: 30_000 }).toBe(true)
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-mode', 'bound')
  await expect(page.locator('.v3d-host')).toHaveAttribute('data-animation', 'idle', { timeout: 30_000 })
  for (const name of ['Simple', 'Réaliste']) await expect(page.getByRole('radio', { name, exact: true })).toHaveCount(0)
}
async function capture(name) {
  await ready(); await page.mouse.move(1, 1); await page.screenshot({ path: output + name + '.png' })
}
async function sources() {
  const settings = page.locator('.v3d-settings')
  await settings.locator(':scope > summary').click()
  await expect(page.getByLabel('Manifeste d’assemblage', { exact: true })).not.toBeVisible()
  const unavailable = settings.locator('details').filter({ has: page.locator('summary').filter({ hasText: /^Occurrences indisponibles/ }) })
  if (await unavailable.getAttribute('open') === null) await unavailable.locator(':scope > summary').click()
  await expect(unavailable.locator('li')).not.toHaveCount(0)
  const count = await unavailable.locator('li').count()
  await settings.hover(); await page.mouse.wheel(0, 20000)
  await expect.poll(() => settings.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2)
  await settings.getByRole('button', { name: 'Afficher 100 suivantes', exact: true }).click()
  await expect(unavailable.locator('li')).toHaveCount(count + 100)
  await expect.poll(() => settings.evaluate(el => {
    const root = el.closest('.v3d-root'), a = root.getBoundingClientRect(), b = el.getBoundingClientRect()
    return root.scrollTop === 0 && b.top >= a.top && b.bottom <= a.bottom
  })).toBe(true)
  await settings.hover(); await page.mouse.wheel(0, 20000)
  const provenance = settings.getByText('Provenance et licences', { exact: true }).locator('..')
  if (await provenance.getAttribute('open') === null) await provenance.locator(':scope > summary').click()
  await settings.hover(); await page.mouse.wheel(0, 20000)
  await expect(settings.getByRole('link').last()).toBeInViewport()
  return settings
}
// Pick a genuinely visible surface with a real mouse click (same raycast as the application).
async function clickOccurrence(id) {
  await page.locator('.v3d-canvas').scrollIntoViewIfNeeded()
  const point = await page.locator('.v3d-root').evaluate((el, nodeId) => {
    const v = el.__viewer, rect = v.renderer.domElement.getBoundingClientRect()
    const item = v.built.items.find(i => i.obj.nodeId === nodeId && i.node.visible)
    if (!item) return null
    let left = Infinity, top = Infinity, right = -Infinity, bottom = -Infinity
    for (const mesh of item.meshes) {
      const b = mesh.geometry.boundingBox
      for (const x of [b.min.x, b.max.x]) for (const y of [b.min.y, b.max.y]) for (const z of [b.min.z, b.max.z]) {
        const p = v.camera.position.clone().set(x, y, z).applyMatrix4(mesh.matrixWorld).project(v.camera)
        const px = rect.x + (p.x + 1) / 2 * rect.width, py = rect.y + (1 - p.y) / 2 * rect.height
        left = Math.min(left, px); right = Math.max(right, px); top = Math.min(top, py); bottom = Math.max(bottom, py)
      }
    }
    for (const fy of [.5, .25, .75, .1, .9]) for (const fx of [.5, .25, .75, .1, .9]) {
      const x = left + (right - left) * fx, y = top + (bottom - top) * fy
      if (document.elementFromPoint(x, y)?.classList.contains('v3d-canvas') && v.pick({ clientX: x, clientY: y })?.nodeId === nodeId) return { x, y }
    }
    return null
  }, id)
  expect(point).not.toBeNull()
  await page.mouse.click(point.x, point.y)
  await expect(page).toHaveURL(new RegExp('node=' + id + '(?:&|$)'))
}
try {
  report.before = await referenceHashes()
  const parts = new Map((await (await page.request.get(base + '/api/v1/parts')).json()).map(p => [p.id, p]))
  let resolveComparison
  const comparisonResponse = new Promise(resolve => { resolveComparison = resolve })
  await page.route('**/api/v1/boms/2/configuration-simulation', async route => {
    const actual = await route.fetch()
    expect(actual.ok()).toBe(true)
    resolveComparison(await actual.json()) // inspector cache cannot retain this 29 MB body
    await route.fulfill({ response: actual })
  })
  await page.goto(base + '/#/boms/2/structurer?tab=3d'); await ready()
  const panel = page.getByRole('region', { name: 'Simulation de motorisation' })
  await panel.getByRole('radio', { name: 'Hybride', exact: true }).click()
  const comparison = await comparisonResponse
  report.costs = Object.fromEntries(comparison.scenarios.map(s => [s.key, s.tree.nodes.find(n => n.id === comparison.root_id).totals.cost]))
  for (const viewport of [{ width: 1366, height: 768 }, { width: 1280, height: 720 }]) {
    await page.setViewportSize(viewport)
    const suffix = viewport.width + 'x' + viewport.height
    const record = { viewport, modes: [] }; report.laptops.push(record)
    for (const [key, label] of [['hybrid', 'Hybride'], ['thermal', 'Thermique'], ['electric', 'Électrique']]) {
      await panel.getByRole('radio', { name: label, exact: true }).click()
      await expect(page.locator('.syn .total .vl')).toHaveText(formatMoney(report.costs[key]))
      await ready()
      if (key === 'electric') {
        const details = panel.locator('details').filter({ has: page.locator('summary', { hasText: 'Hypothèses et limites techniques' }) })
        await expect(details).not.toHaveAttribute('open', '')
        await expect(panel.getByText('Simulation électrique : scénario de démonstration avec hypothèses techniques.', { exact: true })).toBeVisible()
        await expect(panel.locator('.warn')).toHaveCount(0)
        await capture('valhalla-electrique-' + suffix)
        await details.locator('summary').click()
        for (const text of ['batterie BEV adaptée', 'transformations non dimensionnées', 'coûts additionnels non établis']) await expect(details).toContainText(text)
        await capture('hypotheses-electriques-' + suffix)
        await details.locator('summary').click()
      }
      record.modes.push({ key, cost: report.costs[key], calculated: true })
    }
    for (const view of ['3D', 'Arbre + 3D']) {
      await page.getByRole('radio', { name: view, exact: true }).click(); await ready()
      const settings = await sources()
      await capture('sources-' + (view === '3D' ? '3d-' : 'arbre-3d-') + suffix)
      await settings.locator(':scope > summary').click()
    }
    await panel.locator('summary').filter({ hasText: 'Composants retirés' }).click()
    const removed = page.getByRole('region', { name: 'Liste des composants retirés' })
    await removed.hover(); await page.mouse.wheel(0, 20000)
    await expect.poll(() => removed.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2)
    await removed.getByRole('button', { name: 'Afficher 100 suivantes', exact: true }).click()
    await expect(removed.locator('li')).toHaveCount(200)
    await panel.locator('summary').filter({ hasText: 'Composants retirés' }).click()
    await page.getByRole('tab', { name: 'Réglages de calcul', exact: true }).click()
    const sidebar = page.locator('.col')
    await sidebar.hover({ position: { x: 3, y: 100 } }); await page.mouse.wheel(0, 20000)
    await expect(page.getByLabel('Pays de destination', { exact: true })).toBeInViewport()
    await page.getByRole('tab', { name: 'Détail', exact: true }).click()
    record.sources_scroll = record.removed_scroll = record.sidebar_scroll = record.advanced_collapsed = true
    process.stdout.write(suffix + ': three Go costs, neutral electric summary, hypotheses, sources and sidebar scroll PASS\n')
  }
  await panel.getByRole('radio', { name: 'Hybride', exact: true }).click(); await ready()
  const engineName = parts.get(297).name
  await page.getByRole('searchbox', { name: 'Rechercher dans la BOM' }).fill(engineName)
  await page.getByRole('treeitem').filter({ hasText: engineName }).first().click()
  await expect.poll(() => page.locator('.v3d-root').evaluate(el => [...el.__viewer.selection].some(k => k.startsWith('node:32415/')))).toBe(true)
  await page.getByRole('searchbox', { name: 'Rechercher dans la BOM' }).fill('')
  await page.getByLabel('Éclatement', { exact: true }).fill('100'); await ready()
  await clickOccurrence(34539)
  await expect(page.getByRole('treeitem', { selected: true })).toBeInViewport()
  await capture('valhalla-selection-eclatee-laptop')
  report.selection_and_explode = true
  await panel.getByRole('button', { name: 'Quitter la simulation' }).click()
  // Release the repeated scenario scenes before verifying a second large supplied model.
  await page.close()
  page = await browser.newPage({ viewport: { width: 1280, height: 720 } })
  page.setDefaultTimeout(30_000)
  observe(page)
  await page.goto(base + '/#/boms/1/structurer?tab=3d')
  await expect(page.locator('.v3d-badge').filter({ hasText: 'Fiat Multipla 1998' })).toBeVisible({ timeout: 120_000 })
  await ready()
  for (const view of ['3D', 'Arbre + 3D']) {
    await page.getByRole('radio', { name: view, exact: true }).click(); await ready()
    await page.getByLabel('Éclatement', { exact: true }).fill('50'); await ready()
    const settings = await sources()
    await capture('fiat-' + (view === '3D' ? '3d' : 'arbre-3d') + '-laptop')
    await settings.locator(':scope > summary').click()
  }
  report.fiat_views_and_scroll = true
  report.after = await referenceHashes()
  expect(report.after).toEqual(report.before)
  expect(report.errors).toEqual([]); expect(report.mutations).toEqual([]); expect(report.legacy).toEqual([])
} catch (error) {
  report.errors.push(error.message)
  await page.screenshot({ path: output + 'failure.png' }).catch(() => {})
  throw error
} finally {
  await writeFile(output + 'validation.json', JSON.stringify(report, null, 2) + '\n')
  await browser.close()
}
