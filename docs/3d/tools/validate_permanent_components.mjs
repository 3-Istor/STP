// Real application smoke test, without BOM mutations or scenario calculations.
import { chromium } from '../../../web/node_modules/playwright-core/index.mjs'
import { expect } from '../../../web/node_modules/@playwright/test/index.mjs'
import { mkdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
const base = 'http://127.0.0.1:41769'
const output = fileURLToPath(new URL('../captures/rendu-permanent/', import.meta.url))
await mkdir(output, { recursive: true })
const browser = await chromium.launch({ args: ['--no-sandbox'] })
const page = await browser.newPage({ viewport: { width: 1500, height: 1000 } })
page.setDefaultTimeout(30_000)
const report = { at: new Date().toISOString(), vehicles: [], errors: [], legacyRequests: [] }
page.on('pageerror', e => report.errors.push(e.message))
page.on('request', r => { if (r.url().includes('/models/CarConcept')) report.legacyRequests.push(r.url()) })
async function ready() {
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'ready', { timeout: 120_000 })
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-mode', 'bound')
  for (const name of ['Simple', 'Réaliste']) await expect(page.getByRole('radio', { name, exact: true })).toHaveCount(0)
  await expect(page.getByRole('radiogroup', { name: 'Rendu', exact: true })).toHaveCount(0)
  await expect(page.locator('.v3d-host')).toHaveAttribute('data-animation', 'idle', { timeout: 30_000 })
}
try {
  for (const [id, name, asset] of [[1, 'multipla', 'MultiplaExteriorPrepared'], [2, 'valhalla', 'ValhallaExteriorPrepared']]) {
    const hash = '/boms/' + id + '/structurer?tab=3d'
    if (id === 1) await page.goto(base + '/#' + hash)
    else await page.evaluate(value => { location.hash = value }, hash)
    await ready()
    const assets = await page.locator('.v3d-root').evaluate(el => el.__viewer.built.manifestInfo.assets.map(a => a.key))
    expect(assets).toContain(asset)
    await page.mouse.move(1, 1); await page.screenshot({ path: output + name + '-3d.png' })
    await page.getByRole('radio', { name: 'Arbre + 3D', exact: true }).click()
    await page.getByRole('button', { name: 'Recentrer', exact: true }).click(); await ready()
    await page.screenshot({ path: output + name + '-arbre-3d.png' })
    await page.reload(); await ready()
    await page.getByRole('button', { name: 'Réinitialiser', exact: true }).click(); await ready()
    report.vehicles.push({ id, name, assets, view3d: true, tree3d: true, reload: true, reset: true })
    process.stdout.write(name + ': permanent supplied models, 3D + tree3D, reload and reset PASS\n')
  }
  expect(report.errors).toEqual([]); expect(report.legacyRequests).toEqual([])
} catch(error) {
  report.errors.push(error.message);throw error
} finally {
  await writeFile(output + 'validation.json', JSON.stringify(report, null, 2) + '\n')
  await browser.close()
}
