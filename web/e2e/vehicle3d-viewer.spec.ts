import { expect, test, type Page } from '@playwright/test'
import { writeFile } from 'node:fs/promises'
import multipla from '../src/features/vehicle3d/demo/multipla-nodes.json' with { type: 'json' }

// Le viewer est déjà exposé uniquement en développement par V2. Les assertions lisent la scène
// réellement rendue ; les actions passent par les contrôles de la page de démonstration.
type Probe = {
  built: { items: { node: { position: { toArray(): number[] } } }[] } | null
  renders: number
  screenPositionOf(key: string): { x: number; y: number } | null
  diagnostics: { renders: number; drawCalls: number; triangles: number; geometries: number; textures: number; items: number }
}

async function ready(page: Page, fixture = 'multipla', mode = 'simple', select: number | null = null) {
  await page.goto(`/vehicle3d-demo.html?fixture=${fixture}&mode=${mode}&color=system${select === null ? '' : `&select=${select}`}`)
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'ready', { timeout: 30_000 })
  await expect(page.locator('.v3d-canvas')).toBeVisible()
}

async function positions(page: Page) {
  return page.locator('.v3d-root').evaluate(el => {
    const viewer = (el as HTMLElement & { __viewer: Probe }).__viewer
    return viewer.built!.items.map(item => item.node.position.toArray())
  })
}

for (const fixture of ['multipla', 'valhalla']) {
  test(`${fixture} : chargement, éclaté réversible, sélection BOM et rendu au repos`, async ({ page }, testInfo) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await ready(page, fixture)
    await page.getByRole('button', { name: 'Matériaux', exact: true }).click()
    await expect(page.getByText('Matériaux du modèle · aucune indication de budget')).toBeVisible()
    await expect(page.getByText('Formes', { exact: false }).first()).toBeVisible()
    const assembled = await positions(page)
    expect(assembled.length).toBeGreaterThan(30)
    await page.screenshot({ path: testInfo.outputPath(`${fixture}-assemble.png`) })
    const metrics = await page.locator('.v3d-root').evaluate(el => (el as HTMLElement & { __viewer: Probe }).__viewer.diagnostics)
    expect(metrics.drawCalls).toBeGreaterThan(0)
    expect(metrics.triangles).toBeGreaterThan(0)
    await writeFile(testInfo.outputPath(`${fixture}-metrics.json`), JSON.stringify(metrics, null, 2))
    const slider = page.locator('.demo-bar input[type="range"]')
    await slider.focus()
    await slider.press('End')
    await expect.poll(() => positions(page)).not.toEqual(assembled)
    await expect(page.locator('.v3d-host')).toHaveAttribute('data-animation', 'idle')
    await page.screenshot({ path: testInfo.outputPath(`${fixture}-eclate.png`) })
    await slider.press('Home')
    await expect.poll(() => positions(page)).toEqual(assembled)
    // Inversion pendant le déplacement : l'arrivée doit rester strictement identique à l'assemblage.
    await slider.press('End')
    await slider.press('Home')
    await expect.poll(() => positions(page)).toEqual(assembled)
    // Clic réel dans le canvas sur une porte, puis sélection d'une roue depuis la BOM de démo.
    const point = await page.locator('.v3d-root').evaluate(el => (el as HTMLElement & { __viewer: Probe }).__viewer.screenPositionOf('door_fl'))
    const canvas = await page.locator('.v3d-canvas').boundingBox()
    expect(point && canvas).toBeTruthy()
    await page.mouse.click(canvas!.x + point!.x, canvas!.y + point!.y)
    await expect(page.locator('.demo-sel')).toContainText(/Porte .*gauche/)
    await page.getByRole('button', { name: 'Roue avant gauche', exact: true }).click()
    await expect(page.locator('.demo-sel')).toContainText('Roue avant gauche')
    await page.getByRole('button', { name: 'Focus sélection', exact: true }).click()
    await expect(page.locator('.v3d-root')).toHaveAttribute('data-focus', 'wheel_fl')
    await page.getByLabel('Carrosserie transparente').check()
    await page.getByRole('button', { name: 'Réinitialiser', exact: true }).click()
    await expect(page.locator('.v3d-root')).toHaveAttribute('data-focus', '')
    await expect(page.getByLabel('Carrosserie transparente')).not.toBeChecked()
    await expect(slider).toHaveValue('0')
    await expect.poll(() => positions(page)).toEqual(assembled)
    await expect.poll(async () => page.locator('.v3d-host').getAttribute('data-renders')).toBeTruthy()
    // Le moteur doit cesser de dessiner une fois les transitions finies.
    await page.waitForTimeout(800)
    const before = await page.locator('.v3d-host').getAttribute('data-renders')
    await page.waitForTimeout(150)
    expect(await page.locator('.v3d-host').getAttribute('data-renders')).toBe(before)
    expect(errors).toEqual([])
  })
}

test('GLB absent : repli explicite vers les formes générées', async ({ page }) => {
  await page.route('**/models/CarConcept.glb', route => route.fulfill({ status: 404 }))
  await ready(page, 'multipla', 'realistic')
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-mode', 'simple')
  await expect(page.getByRole('alert')).toContainText('Modèle réaliste absent')
})

test('pièce physique sans géométrie : message explicite et commandes de sélection désactivées', async ({ page }) => {
  const part = multipla.nodes.find(n => n.part_id !== null)!
  await ready(page, 'multipla', 'simple', part.id)
  await expect(page.getByText('Modèle 3D de cette pièce indisponible.', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Cadrer la sélection', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Focus sélection', exact: true })).toBeDisabled()
})

test('GLB valide : chargement, avertissement de fidélité et attribution', async ({ page }) => {
  // Triangle synthétique de test : aucun asset externe requis en CI.
  const positions = new Float32Array([-1, 0, 0, 1, 0, 0, 0, 1, 0])
  const json = Buffer.from(JSON.stringify({
    asset: { version: '2.0' }, scene: 0, scenes: [{ nodes: [0] }],
    nodes: [{ name: 'BodyHood', mesh: 0 }],
    meshes: [{ primitives: [{ attributes: { POSITION: 0 }, material: 0 }] }],
    materials: [{ name: 'Paint', pbrMetallicRoughness: { baseColorFactor: [0.2, 0.4, 0.5, 1] } }],
    buffers: [{ byteLength: positions.byteLength }],
    bufferViews: [{ buffer: 0, byteOffset: 0, byteLength: positions.byteLength }],
    accessors: [{ bufferView: 0, componentType: 5126, count: 3, type: 'VEC3', min: [-1, 0, 0], max: [1, 1, 0] }],
  }))
  const padded = Buffer.concat([json, Buffer.alloc((4 - json.length % 4) % 4, 0x20)])
  const buffer = Buffer.alloc(12 + 8 + padded.length + 8 + positions.byteLength)
  buffer.write('glTF'); buffer.writeUInt32LE(2, 4); buffer.writeUInt32LE(buffer.length, 8)
  buffer.writeUInt32LE(padded.length, 12); buffer.writeUInt32LE(0x4e4f534a, 16); padded.copy(buffer, 20)
  const offset = 20 + padded.length
  buffer.writeUInt32LE(positions.byteLength, offset); buffer.writeUInt32LE(0x004e4942, offset + 4)
  Buffer.from(positions.buffer).copy(buffer, offset + 8)
  await page.route('**/models/CarConcept.glb', route => route.fulfill({ contentType: 'model/gltf-binary', body: buffer }))
  await ready(page, 'multipla', 'realistic')
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-mode', 'realistic')
  await expect(page.getByText('SIMILAIRE', { exact: false })).toBeVisible()
  await expect(page.locator('.v3d-attribution')).toContainText('CC-BY 4.0')
  await page.getByRole('button', { name: 'Roue avant gauche', exact: true }).click()
  await expect(page.getByText('Ce sous-ensemble est absent du modèle chargé.', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Focus sélection', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Cadrer la sélection', exact: true })).toBeDisabled()
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-focus', '')
})

test('préférence de mouvement réduit : les pièces atteignent directement leur destination', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await ready(page)
  const assembled = await positions(page)
  const slider = page.locator('.demo-bar input[type="range"]')
  await slider.focus()
  await slider.press('End')
  await expect.poll(() => positions(page)).not.toEqual(assembled)
  await expect(page.locator('.v3d-host')).toHaveAttribute('data-animation', 'idle')
  await slider.press('Home')
  await expect.poll(() => positions(page)).toEqual(assembled)
})

test('GLB invalide : message explicite et repli vers les formes générées', async ({ page }) => {
  await page.route('**/models/CarConcept.glb', route => route.fulfill({
    contentType: 'model/gltf-binary', body: 'not a glb',
  }))
  await ready(page, 'valhalla', 'realistic')
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-mode', 'simple')
  await expect(page.getByRole('alert')).toContainText('illisible')
})

test('WebGL indisponible puis rétabli : erreur locale et nouvelle tentative', async ({ page }) => {
  await page.addInitScript(() => {
    let blocked = true
    const original = HTMLCanvasElement.prototype.getContext
    Object.assign(window, { __allowWebGL: () => { blocked = false } })
    HTMLCanvasElement.prototype.getContext = function (...args: Parameters<typeof original>) {
      if (blocked && String(args[0]).includes('webgl')) return null
      return original.apply(this, args)
    } as typeof original
  })
  await page.goto('/vehicle3d-demo.html?fixture=multipla')
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'error', { timeout: 30_000 })
  await expect(page.getByRole('alert')).toContainText('Vue 3D indisponible')
  await page.evaluate(() => (window as typeof window & { __allowWebGL(): void }).__allowWebGL())
  await page.getByRole('button', { name: 'Réessayer la vue 3D', exact: true }).click()
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'ready', { timeout: 30_000 })
  await expect(page.locator('.v3d-canvas')).toBeVisible()
})
