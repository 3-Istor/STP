import { expect, test, type Page } from '@playwright/test'

// Le viewer est déjà exposé uniquement en développement par V2. Les assertions lisent la scène
// réellement rendue ; les actions passent par les contrôles de la page de démonstration.
type Probe = {
  built: { items: { node: { position: { toArray(): number[] } } }[] } | null
  renders: number
  screenPositionOf(key: string): { x: number; y: number } | null
}

async function ready(page: Page, fixture = 'multipla', mode = 'simple') {
  await page.goto(`/vehicle3d-demo.html?fixture=${fixture}&mode=${mode}&color=system`)
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
    await page.getByRole('button', { name: 'Roue avant gauche', exact: true }).click()
    await expect(page.locator('.demo-sel')).toContainText('Roue avant gauche')
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
