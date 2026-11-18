// Live, local demonstration validation. Uses the actual Go API and the supplied GLBs.
// STP_3D_URL=http://127.0.0.1:41769 node docs/3d/tools/validate_soutenance.mjs
import { chromium } from '../../../web/node_modules/playwright-core/index.mjs'
import { expect } from '../../../web/node_modules/@playwright/test/index.mjs'
import { mkdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { resolve } from 'node:path'

const base = process.env.STP_3D_URL ?? 'http://127.0.0.1:41769'
if (!['127.0.0.1', 'localhost'].includes(new URL(base).hostname)) throw new Error('Local demonstration only')
const output = resolve(process.env.STP_3D_OUTPUT ?? fileURLToPath(new URL('../captures/soutenance/', import.meta.url))) + '/'
await mkdir(output, { recursive: true })
const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
const report = { at: new Date().toISOString(), base, vehicles: [], errors: [], legacyModelRequests: [] }
const page = await browser.newPage({ viewport: { width: 1500, height: 1000 } })
page.setDefaultTimeout(30_000)
page.on('pageerror', error => report.errors.push(error.message))
page.on('request', request => { if (request.url().includes('/models/CarConcept')) report.legacyModelRequests.push(request.url()) })

const probe = () => page.locator('.v3d-root').evaluate(el => {
  const v = el.__viewer
  return { metrics: v.diagnostics, selected: [...v.selection], focus: v.focused,
    items: v.built.items.map(i => ({ id: i.obj?.nodeId, key: i.obj?.key, visible: i.node.visible, role: i.displayRole,
      position: i.node.position.toArray(), geometry: i.meshes.map(m => m.geometry.uuid),
      dimensions: i.meshes.map(m => m.geometry.boundingBox.getSize(v.camera.position.clone()).toArray()),
      opacity: i.meshes.flatMap(m => (Array.isArray(m.material) ? m.material : [m.material]).map(material => material.opacity)) })) }
})
async function idle() { await expect(page.locator('.v3d-host')).toHaveAttribute('data-animation', 'idle', { timeout: 30_000 }) }
async function capture(name) { await idle(); await page.mouse.move(2, 2); await page.screenshot({ path: output + name + '.png' }) }
async function ready() {
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'ready', { timeout: 120_000 })
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-mode', 'bound')
  await expect(page.getByRole('radio', { name: 'Réaliste', exact: true })).toHaveCount(0)
  await idle()
}
async function treePick(label, id) {
  await page.getByRole('radio', { name: 'Arbre + 3D', exact: true }).click()
  await page.getByRole('searchbox', { name: 'Rechercher dans la BOM' }).fill(label)
  await page.getByRole('treeitem').filter({ hasText: label }).first().click()
  await expect(page).toHaveURL(new RegExp('[?&]node=' + id + '(?:&|$)'))
  await expect(page.getByRole('region', { name: 'Détail du nœud', exact: true })).toContainText(label)
  await expect.poll(() => probe().then(p => p.selected.length), { timeout: 30_000 }).toBeGreaterThan(0)
  await idle()
}

// Find a visible surface of the requested occurrence, then issue a real mouse click.
// Source names and DOM dispatches never select the object in place of the raycast.
async function clickOccurrence(id) {
  await page.locator('.v3d-canvas').scrollIntoViewIfNeeded()
  const point = await page.locator('.v3d-root').evaluate((el, nodeId) => {
    const v = el.__viewer, rect = v.renderer.domElement.getBoundingClientRect()
    const item = v.built.items.find(i => i.obj?.nodeId === nodeId && i.node.visible)
    if (!item) return null
    let left = Infinity, top = Infinity, right = -Infinity, bottom = -Infinity
    for (const mesh of item.meshes) {
      const box = mesh.geometry.boundingBox
      for (const x of [box.min.x, box.max.x]) for (const y of [box.min.y, box.max.y]) for (const z of [box.min.z, box.max.z]) {
        const p = v.camera.position.clone().set(x, y, z).applyMatrix4(mesh.matrixWorld).project(v.camera)
        const px = rect.x + (p.x + 1) / 2 * rect.width, py = rect.y + (1 - p.y) / 2 * rect.height
        left = Math.min(left, px); right = Math.max(right, px); top = Math.min(top, py); bottom = Math.max(bottom, py)
      }
    }
    for (const fy of [0.5, 0.25, 0.75, 0.1, 0.9]) for (const fx of [0.5, 0.25, 0.75, 0.1, 0.9]) {
      const x = left + (right - left) * fx, y = top + (bottom - top) * fy
      if (!document.elementFromPoint(x, y)?.classList.contains('v3d-canvas')) continue
      if (v.pick({ clientX: x, clientY: y })?.nodeId === nodeId) return { x, y }
    }
    return null
  }, id)
  expect(point, 'A visible, raycastable surface for occurrence ' + id).not.toBeNull()
  await page.mouse.click(point.x, point.y)
  await expect(page).toHaveURL(new RegExp('[?&]node=' + id + '(?:&|$)'))
  await expect(page.getByRole('treeitem', { selected: true })).toBeVisible()
}

try {
  const catalog = await (await page.request.get(base + '/api/v1/parts')).json()
  const parts = new Map(catalog.map(p => [p.id, p]))
  const scenarios = process.env.STP_3D_ONLY_VARIANT === '1' ? [] : [{ name: 'multipla', bom: 1, fromBom: 12505, from3D: 15865, group: 12502, absent: 12934, located: 'Culasse' },
    { name: 'valhalla', bom: 2, fromBom: 32415, from3D: 34539, group: 32412, absent: 34760, located: 'Batterie haute tension' }]
  for (const scenario of scenarios) {
    const record = { ...scenario, steps: [] }; report.vehicles.push(record)
    try {
      const tree = await (await page.request.get(base + '/api/v1/boms/' + scenario.bom + '/tree')).json()
      const nodes = new Map(tree.nodes.map(n => [n.id, n]))
      const started = Date.now()
      await page.goto(base + '/#/boms/' + scenario.bom + '/structurer?tab=3d')
      await ready()
      record.loadMs = Date.now() - started
      const initial = await probe()
      expect(initial.items.filter(i => i.role === 'interior' && i.visible)).toHaveLength(0)
      record.normalMetrics = initial.metrics
      await capture(scenario.name + '-normal')
      record.steps.push('Real exterior selected by the verified installation profile')
      const canvas = page.locator('.v3d-canvas'), rect = await canvas.boundingBox()
      const camera = () => page.locator('.v3d-root').evaluate(el => el.__viewer.camera.position.toArray())
      const startCamera = await camera()
      await page.mouse.move(rect.x + rect.width * .6, rect.y + rect.height * .5)
      await page.mouse.down(); await page.mouse.move(rect.x + rect.width * .75, rect.y + rect.height * .6, { steps: 12 }); await page.mouse.up()
      await expect.poll(camera, { timeout: 30_000 }).not.toEqual(startCamera)
      const orbited = await camera()
      await page.mouse.wheel(0, -240)
      await expect.poll(camera, { timeout: 30_000 }).not.toEqual(orbited)
      await page.getByRole('button', { name: 'Recentrer', exact: true }).click(); await idle()
      record.steps.push('Real mouse orbit and wheel zoom; camera recenter')
      const assembled = initial.items.map(i => i.position)
      const slider = page.getByLabel('Éclatement', { exact: true })
      await slider.focus(); await slider.press('Home')
      for (let i = 0; i < 10; i++) await slider.press('ArrowRight')
      await expect(slider).toHaveValue('50'); await idle()
      const half = (await probe()).items.map(i => i.position)
      await capture(scenario.name + '-exploded-half')
      await slider.focus(); await slider.press('End')
      await expect.poll(() => probe().then(p => p.items.map(i => i.position)), { timeout: 30_000 }).not.toEqual(assembled)
      await idle(); await capture(scenario.name + '-exploded')
      record.explodedMetrics = (await probe()).metrics
      const full = (await probe()).items.map(i => i.position)
      const linearError = Math.max(...half.flatMap((p, i) => p.map((v, axis) => Math.abs(v - (assembled[i][axis] + full[i][axis]) / 2))))
      expect(linearError).toBeLessThan(1e-6)
      record.halfExplodeMaxError = linearError
      await slider.press('Home')
      await expect.poll(() => probe().then(p => p.items.map(i => i.position)), { timeout: 30_000 }).toEqual(assembled)
      record.steps.push('0 → 50 → 100 → 0: linear intermediate and exact assembled transforms')
      const label = parts.get(nodes.get(scenario.fromBom).part_id).name
      await treePick(label, scenario.fromBom)
      expect((await probe()).selected).toEqual(['node:' + scenario.fromBom + '/instance:illustration'])
      await capture(scenario.name + '-selection-bom')
      await page.getByText('Composants et sources', { exact: true }).click()
      await page.getByRole('button', { name: 'Isoler la sélection', exact: true }).click()
      await expect.poll(() => probe().then(p => p.items.filter(i => i.visible).map(i => i.id))).toEqual([scenario.fromBom])
      await capture(scenario.name + '-isolated')
      await page.getByRole('button', { name: 'Tout afficher', exact: true }).click()
      await page.getByText('Composants et sources', { exact: true }).click()
      record.steps.push('BOM → exact occurrence → Detail → isolation')
      await page.getByRole('searchbox', { name: 'Rechercher dans la BOM' }).fill('')
      await page.getByRole('button', { name: 'Réinitialiser', exact: true }).click()
      await slider.focus(); await slider.press('End'); await idle()
      await clickOccurrence(scenario.from3D)
      await expect(page.getByRole('region', { name: 'Détail du nœud', exact: true })).toContainText(parts.get(nodes.get(scenario.from3D).part_id).name)
      await capture(scenario.name + '-selection-3d')
      record.steps.push('Real raycast → exact occurrence → expanded tree and Detail')
      await treePick(nodes.get(scenario.group).name, scenario.group)
      await page.getByRole('button', { name: 'Focus sélection', exact: true }).click()
      await expect(page.locator('.v3d-root')).not.toHaveAttribute('data-focus', '')
      await expect(page).toHaveURL(new RegExp('node=' + scenario.group))
      await capture(scenario.name + '-group-focus')
      await page.keyboard.press('Escape')
      await page.getByRole('button', { name: 'Réinitialiser', exact: true }).click()
      await expect.poll(() => probe().then(p => p.items.map(i => i.position)), { timeout: 30_000 }).toEqual(assembled)
      record.steps.push('Group focus keeps the selected BOM group; Escape and reset restore assembly')
      if (scenario.name === 'valhalla') {
        await page.getByRole('radio', { name: '3D', exact: true }).click()
        await page.getByRole('button', { name: 'Recentrer', exact: true }).click(); await idle()
        await page.getByText('Composants et sources', { exact: true }).click()
        record.hybrid = []
        for (const mode of ['thermal', 'electric', 'hybrid']) {
          await page.getByLabel('Motorisation', { exact: true }).selectOption(mode)
          const state = await probe()
          expect(state.items.map(i => i.key)).toEqual(initial.items.map(i => i.key))
          const at = id => state.items.find(i => i.id === id)
          expect(at(32415).opacity.every(v => v === (mode === 'electric' ? 0.12 : 1))).toBe(true)
          for (const id of [34539, 34607, 34675, 34758]) expect(at(id).opacity.every(v => v === (mode === 'thermal' ? 0.12 : 1))).toBe(true)
          await capture('valhalla-' + mode)
          record.hybrid.push({ mode, v8: at(32415).id, frontMotors: [at(34539).id, at(34607).id], rearMotor: at(34675).id, battery: at(34758).id })
        }
        record.steps.push('Thermal / electric / hybrid, three motors and HV battery, same occurrence keys')
      }
      if (scenario.name === 'multipla') {
        await page.evaluate(() => { location.hash = '/boms/1/structurer?tab=arbre-3d&node=12932' })
        await expect.poll(() => probe().then(p => p.selected), { timeout: 30_000 }).toEqual(['node:12932/instance:instance-1', 'node:12932/instance:instance-2'])
        expect((await probe()).items.filter(i => i.id === 12932)).toHaveLength(2)
        await capture('multipla-arbres-a-cames-repares')
        record.steps.push('Repaired camshaft occurrence: two supplied meshes at explicit distinct instance positions')
      }
      await page.evaluate(({ bom, node }) => { location.hash = '/boms/' + bom + '/structurer?tab=arbre-3d&node=' + node }, { bom: scenario.bom, node: scenario.absent })
      await expect(page.getByText('Modèle 3D de cette pièce indisponible.', { exact: false })).toBeVisible()
      await expect.poll(() => probe().then(p => p.selected), { timeout: 30_000 }).toEqual([])
      await page.getByRole('button', { name: 'Localiser le sous-ensemble « ' + scenario.located + ' »', exact: true }).click()
      await expect(page.getByText('Sous-ensemble localisé : ' + scenario.located, { exact: false })).toBeVisible()
      await expect(page).toHaveURL(new RegExp('node=' + scenario.absent))
      await expect.poll(() => probe().then(p => p.selected.length), { timeout: 30_000 }).toBeGreaterThan(0)
      expect((await probe()).selected.some(k => k.startsWith('node:' + scenario.absent + '/'))).toBe(false)
      await capture(scenario.name + '-localisation-sous-ensemble')
      record.steps.push('Absent part stays absent and keeps its BOM ID; explicit ancestor locator frames the reliable assembly')
      process.stdout.write(scenario.name + ': ' + record.steps.join('; ') + '\n')
    } catch (error) {
      process.stderr.write(scenario.name + ': ' + error.message + '\n')
      record.failure = error.message; report.errors.push(scenario.name + ': ' + error.message)
      await page.screenshot({ path: output + scenario.name + '-failure.png' })
    }
  }
  if (process.env.STP_3D_MUTATIONS === '1') {
    // Deliberately only the isolated seed draft B. The original values are restored in finally.
    const tree = await (await page.request.get(base + '/api/v1/boms/3/tree')).json()
    const node = tree.nodes.find(n => n.id === 60776)
    expect(node?.part_id).toBe(195)
    expect(node?.parent_id).toBe(60775)
    const saved = { part_id: node.part_id, offer_id: node.offer_id }
    try {
      await page.goto(base + '/#/boms/3/structurer?tab=arbre-3d&node=60776')
      await ready()
      const detail = page.getByRole('region', { name: 'Détail du nœud', exact: true })
      await expect(detail).toContainText('Batterie 12 V 50 Ah')
      const before = (await probe()).items.find(i => i.id === node.id)
      const otherOffer = parts.get(195).offers.find(o => o.id !== node.offer_id)
      const offerPatch = page.waitForResponse(r => r.url().endsWith('/nodes/' + node.id) && r.request().method() === 'PATCH')
      const offerTree = page.waitForResponse(r => r.url().endsWith('/boms/3/tree') && r.request().method() === 'GET')
      await page.getByLabel('Fournisseur pour cette pièce', { exact: true }).selectOption(String(otherOffer.id))
      expect((await offerPatch).ok()).toBe(true); await offerTree
      await expect(page.getByLabel('Fournisseur pour cette pièce', { exact: true })).toHaveValue(String(otherOffer.id))
      await ready()
      const offered = (await probe()).items.find(i => i.id === node.id)
      expect(offered.geometry).toEqual(before.geometry)
      expect(offered.dimensions).toEqual(before.dimensions)
      await page.getByText('Composants et sources', { exact: true }).click()
      await page.getByRole('button', { name: 'Isoler la sélection', exact: true }).click()
      await page.getByText('Composants et sources', { exact: true }).click()
      await capture('battery-50ah-illustrative')
      await page.getByText('Remplacement physique — variante 3D illustrative', { exact: true }).click()
      await page.getByLabel('Référence physique de remplacement', { exact: true }).selectOption('196')
      await page.getByLabel('Offre du remplacement', { exact: true }).selectOption('476')
      const physicalPatch = page.waitForResponse(r => r.url().endsWith('/nodes/' + node.id) && r.request().method() === 'PATCH')
      const physicalTree = page.waitForResponse(r => r.url().endsWith('/boms/3/tree') && r.request().method() === 'GET')
      await page.getByRole('button', { name: 'Confirmer le remplacement physique', exact: true }).click()
      expect((await physicalPatch).ok()).toBe(true); await physicalTree
      await expect(detail).toContainText('Batterie 12 V 60 Ah')
      await ready()
      const after = (await probe()).items.find(i => i.id === node.id)
      expect(after.key).toBe(before.key)
      expect(after.geometry).not.toEqual(before.geometry)
      expect(after.dimensions[0][0]).toBeGreaterThan(before.dimensions[0][0] * 1.15)
      await capture('battery-60ah-illustrative')
      report.variant = { nodeId: node.id, sourcePart: 195, replacementPart: 196,
        offerOnlySameGeometry: true, occurrenceKeyPreserved: true, differentGeometry: true,
        beforeDimensions: before.dimensions, afterDimensions: after.dimensions,
        accuracy: 'Illustrative casing variant, no measured manufacturer dimensions' }
      process.stdout.write('Draft B: supplier-only geometry preserved; confirmed physical variant changes geometry; exact occurrence key preserved\n')
    } catch (error) {
      report.errors.push('Physical variant: ' + error.message)
      await page.screenshot({ path: output + 'variant-failure.png' })
    } finally {
      const restored = await page.request.patch(base + '/api/v1/boms/3/nodes/' + node.id, { data: saved })
      expect(restored.ok(), 'Restore the original draft B part and offer').toBe(true)
      const current = await (await page.request.get(base + '/api/v1/boms/3/tree')).json()
      const original = current.nodes.find(n => n.id === node.id)
      expect({ part_id: original.part_id, offer_id: original.offer_id }).toEqual(saved)
      if (report.variant) report.variant.originalValuesRestored = true
    }
  }
} finally {
  if (report.legacyModelRequests.length) report.errors.push('Legacy model requested')
  await writeFile(output + (process.env.STP_3D_ONLY_VARIANT === '1' ? 'validation-variant.json' : 'validation.json'), JSON.stringify(report, null, 2) + '\n')
  await browser.close()
}
if (report.errors.length) { process.stderr.write(JSON.stringify(report.errors, null, 2) + '\n'); process.exitCode = 1 }
