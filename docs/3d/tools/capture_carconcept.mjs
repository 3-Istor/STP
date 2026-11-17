// Validation locale du véritable GLB de démonstration ; aucune fidélité Fiat/Aston n'est attestée.
// Prérequis : make models ; Vite sur 41768 ; navigateur Playwright disponible.
// Usage : node docs/3d/tools/capture_carconcept.mjs [http://127.0.0.1:41768/]
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { mkdir, writeFile } from 'node:fs/promises'

const require = createRequire(new URL('../../../web/package.json', import.meta.url))
const { chromium } = require('playwright')
const output = fileURLToPath(new URL('../captures/', import.meta.url))
const baseURL = process.argv[2] ?? 'http://127.0.0.1:41768/'
const browser = await chromium.launch({ headless: true })
const measurements = []
await mkdir(output, { recursive: true })
try {
  for (const fixture of ['multipla', 'valhalla']) {
    const page = await browser.newPage({ viewport: { width: 1440, height: 960 } })
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    const start = Date.now()
    await page.goto(new URL(`vehicle3d-demo.html?fixture=${fixture}&mode=realistic&color=material`, baseURL).href)
    await page.waitForFunction(() => document.querySelector('.v3d-root')?.dataset.status === 'ready', {}, { timeout: 60_000 })
    if (await page.locator('.v3d-root').getAttribute('data-mode') !== 'realistic') throw new Error('CarConcept GLB non chargé')
    await page.locator('.v3d-attribution').waitFor()
    await page.waitForFunction(() => {
      const diagnostics = document.querySelector('.v3d-root')?.__viewer?.diagnostics
      return diagnostics && diagnostics.drawCalls > 0 && !diagnostics.animating
    })
    const loadMs = Date.now() - start
    const stats = await page.locator('.v3d-root').evaluate(el => el.__viewer.diagnostics)
    await page.screenshot({ path: `${output}/carconcept-${fixture}-assemble.png` })
    const assembled = await page.locator('.v3d-root').evaluate(el => JSON.stringify(el.__viewer.built.items.map(item => item.node.position.toArray())))
    const slider = page.locator('.demo-bar input[type=range]')
    await slider.focus()
    await slider.press('End')
    // Ne pas attendre seulement data-animation=idle : cet attribut peut encore décrire l'image précédente.
    await page.waitForFunction(baseline => {
      const viewer = document.querySelector('.v3d-root')?.__viewer
      return viewer && !viewer.diagnostics.animating && JSON.stringify(viewer.built.items.map(item => item.node.position.toArray())) !== baseline
    }, assembled, { timeout: 30_000 })
    await page.screenshot({ path: `${output}/carconcept-${fixture}-eclate.png` })
    measurements.push({ fixture, asset: 'CarConcept.glb', fidelity: 'unrelated_concept_not_the_requested_vehicle', load_ms_in_this_environment: loadMs, stats, page_errors: errors })
    if (errors.length) throw new Error(errors.join('\n'))
    await page.close()
  }
  await writeFile(`${output}/carconcept-validation.json`, JSON.stringify(measurements, null, 2) + '\n')
} finally {
  await browser.close()
}
