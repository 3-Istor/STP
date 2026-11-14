import { expect, test } from '@playwright/test'
import type { Locator, Page } from '@playwright/test'
import { defaultPages, mockEventSource, mockSupplierAPI, offerView } from './fixtures'

async function chooseSupplier(page: Page, name: string) {
  await page.getByRole('button', { name: `Fournisseur ${name}`, exact: true }).click()
  await expect(page.getByRole('heading', { name, exact: true })).toBeVisible()
}

// Espaces de groupement/localisation normalisés, jamais les chiffres ou les devises.
function localizedText(value: string) {
  return value.replace(/[\u00a0\u202f]/g, ' ').trim()
}

async function semanticColors(locator: Locator) {
  return locator.evaluate((element) => {
    const probe = document.createElement('span')
    document.body.appendChild(probe)
    probe.style.color = 'var(--over)'
    const over = getComputedStyle(probe).color
    probe.style.color = 'var(--ok)'
    const ok = getComputedStyle(probe).color
    probe.remove()
    return { actual: getComputedStyle(element).color, over, ok }
  })
}

test.beforeEach(async ({ page }) => {
  await mockEventSource(page)
})

test('offres isolées, variations explicites et prix manquant visible', async ({ page }) => {
  await mockSupplierAPI(page)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  await expect(page.getByRole('article', { name: 'Offre 101 : Injecteur Atlas' })).toBeVisible()
  await expect(page.getByText('Moteur Boréal', { exact: true })).toHaveCount(0)

  const up = page.getByTestId('variation-101')
  const down = page.getByTestId('variation-102')
  const stable = page.getByTestId('variation-103')
  await expect(up).toHaveText(/\+10,0\s*%/)
  await expect(down).toHaveText(/[-−]10,0\s*%/)
  await expect(stable).toHaveText(/0,0\s*%/)
  const upColor = await semanticColors(up)
  const downColor = await semanticColors(down)
  const stableColor = await semanticColors(stable)
  expect(upColor.actual).toBe(upColor.over)
  expect(downColor.actual).toBe(downColor.ok)
  expect(stableColor.actual).not.toBe(stableColor.ok)
  expect(stableColor.actual).not.toBe(stableColor.over)
  const missing = page.getByRole('article', { name: 'Offre 104 : Sans cotation Atlas' })
  await expect(missing).toContainText('Aucune cotation disponible')
  await expect(missing).not.toContainText('0,00')

  await chooseSupplier(page, 'Boréal')
  await expect(page.getByRole('article', { name: 'Offre 201 : Moteur Boréal' })).toBeVisible()
  await expect(page.getByText('Injecteur Atlas', { exact: true })).toHaveCount(0)
})

test('montants textuels exacts et arrondi half-even dans le moteur réel', async ({ page }) => {
  const cases = [
    { id: 301, amount: '0.125', currency: 'EUR', expected: '0,12 €' },
    { id: 302, amount: '0.135', currency: 'EUR', expected: '0,14 €' },
    { id: 303, amount: '9007199254740993.125', currency: 'EUR', expected: '9 007 199 254 740 993,12 €' },
    { id: 304, amount: '100.5', currency: 'JPY', expected: '100 JPY' },
    { id: 305, amount: '101.5', currency: 'JPY', expected: '102 JPY' },
    { id: 306, amount: '1.2345', currency: 'KWD', expected: '1,234 KWD' },
    { id: 307, amount: '1.2355', currency: 'KWD', expected: '1,236 KWD' },
  ]
  const pages = defaultPages()
  pages['1'].offers = cases.map(({ id, amount, currency }) => offerView({
    id, name: `Précision ${id}`, amount, reference: amount, currency, variation: '0.0', trend: 'stable',
  }))
  await mockSupplierAPI(page, pages)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  for (const value of cases) {
    const price = page.getByTestId(`price-${value.id}`)
    await expect(price).toBeVisible()
    expect(localizedText(await price.innerText())).toBe(value.expected)
  }
})

test('historique utilisable au clic, au clavier et au toucher', async ({ page }, testInfo) => {
  await mockSupplierAPI(page)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  const open = page.getByRole('button', { name: 'Historique de Injecteur Atlas', exact: true })
  if (testInfo.project.use.hasTouch) await open.tap()
  else await open.click()
  const dialog = page.getByRole('dialog', { name: 'Historique : Injecteur Atlas' })
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText(/110,00\s*€/)
  const point = dialog.getByRole('button', { name: /^Cotation du / }).first()
  await point.focus()
  await expect(point).toBeFocused()
  const detail = dialog.getByTestId('quote-detail')
  await expect(detail).toContainText('2025')
  await expect(detail).toContainText(/100,00\s*€/)
  if (testInfo.project.use.hasTouch) await point.tap()
  else { await point.hover(); await point.click() }
  await expect(detail).toContainText(/100,00\s*€/)
  await page.getByRole('button', { name: 'Fermer l’historique', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await open.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog', { name: 'Historique : Injecteur Atlas' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('carte d’offre entièrement cliquable, offres triées et recherche', async ({ page }, testInfo) => {
  await mockSupplierAPI(page)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  // Offres triées par nom de pièce.
  await expect(page.locator('.offer-card h3')).toHaveText(['Capteur Atlas', 'Filtre Atlas', 'Injecteur Atlas', 'Sans cotation Atlas'])
  // Un clic sur le nom de la pièce (hors bouton « Voir l’historique ») ouvre l’historique.
  const title = page.getByRole('article', { name: 'Offre 101 : Injecteur Atlas' }).getByRole('heading', { name: 'Injecteur Atlas' })
  if (testInfo.project.use.hasTouch) await title.tap()
  else await title.click()
  await expect(page.getByRole('dialog', { name: 'Historique : Injecteur Atlas' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)

  const search = page.getByRole('searchbox', { name: 'Rechercher une offre' })
  await search.fill('filtre')
  await expect(page.locator('.offer-card h3')).toHaveText(['Filtre Atlas'])
  await search.fill('aucune pièce')
  await expect(page.getByText('Aucune offre ne correspond à « aucune pièce »')).toBeVisible()
})

test('historique lisible : un point par mois, clôture et bornes réelles du mois', async ({ page }, testInfo) => {
  const pages = defaultPages()
  const view = offerView({ id: 101, name: 'Injecteur Atlas' })
  const q = (id: number, amount: string, at: string) => ({ id, offer_id: 101, price: { amount, currency: 'EUR' }, source: 'e2e-fixture', quoted_at: at })
  // Octobre : trois cotations, la dernière (clôture) n'est ni la plus basse ni la plus haute.
  view.statistics.history = [
    q(1, '100.00', '2026-10-01T00:00:00Z'), q(2, '130.00', '2026-10-15T00:00:00Z'), q(3, '95.00', '2026-10-20T00:00:00Z'),
    q(4, '105.00', '2026-10-28T00:00:00Z'), q(5, '110.00', '2026-11-04T12:00:00Z'),
  ]
  pages['1'].offers[0] = view
  await mockSupplierAPI(page, pages)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  await page.getByRole('button', { name: 'Historique de Injecteur Atlas', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Historique : Injecteur Atlas' })
  await expect(dialog.locator('.chart-month')).toHaveCount(2)
  await expect(dialog.locator('.chart-month text')).toHaveText(['oct. 2026', 'nov.'])
  const october = dialog.locator('.chart-month').first().locator('circle')
  if (testInfo.project.use.hasTouch) await october.tap()
  else await october.hover()
  await expect(dialog.getByTestId('quote-detail')).toContainText(/105,00\s*€/)
  await expect(dialog.getByTestId('quote-detail')).toContainText(/octobre 2026 : min\. 95,00\s*€ · max\. 130,00\s*€ · 4 cotations/)
})

test('petit écran sans débordement et commandes toujours accessibles', async ({ page }, testInfo) => {
  await mockSupplierAPI(page)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  await expect(page.getByTestId('price-101')).toBeVisible()
  const layout = await page.evaluate(() => ({
    viewport: document.documentElement.clientWidth,
    content: document.documentElement.scrollWidth,
  }))
  expect(layout.content).toBeLessThanOrEqual(layout.viewport + 1)
  await page.screenshot({ path: testInfo.outputPath('supplier-page.png'), fullPage: true })
  const refresh = page.getByRole('button', { name: 'Actualiser les données', exact: true })
  await expect(refresh).toBeVisible()
  if (testInfo.project.use.hasTouch) await refresh.tap()
  else await refresh.click()
  await expect(page.getByTestId('price-101')).toHaveText(/110,00\s*€/)
})

test('signal de prix relu sans navigation ni fuite de connexion', async ({ page }) => {
  const api = await mockSupplierAPI(page)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  await expect(page.getByTestId('price-101')).toHaveText(/110,00\s*€/)
  await expect.poll(() => page.evaluate(() => window.__specDrumSse.active)).toBe(1)
  // StrictMode monte, nettoie et remonte les effets : la première connexion doit être fermée.
  expect(await page.evaluate(() => window.__specDrumSse.closed)).toBeGreaterThanOrEqual(1)
  const before = api.calls['1']
  let navigations = 0
  page.on('framenavigated', (frame) => { if (frame === page.mainFrame()) navigations++ })
  const next = offerView({ id: 101, name: 'Injecteur Atlas', amount: '120.00', variation: '20.0' })
  next.statistics.current_quote.id++
  next.statistics.current_quote.quoted_at = '2026-11-05T12:00:00Z'
  next.statistics.as_of = '2026-11-05T12:00:00Z'
  api.pages['1'].as_of = '2026-11-05T12:00:00Z'
  api.pages['1'].offers[0] = next
  await page.evaluate(() => window.__specDrumSse.emit('tick', '{"tick":42,"as_of":"2026-11-05T12:00:00Z"}'))
  await expect.poll(() => api.calls['1']).toBeGreaterThan(before)
  await expect(page.getByTestId('price-101')).toHaveText(/120,00\s*€/)
  await expect(page.getByTestId('variation-101')).toHaveText(/\+20,0\s*%/)
  expect(navigations).toBe(0)
  await page.evaluate(() => window.__specDrumSse.disconnect())
  await expect(page.getByText('Mises à jour automatiques indisponibles, actualisez manuellement', { exact: true })).toBeVisible()
  const beforeReconnect = api.calls['1']
  await page.evaluate(() => window.__specDrumSse.emit('open'))
  await expect(page.getByText('Mises à jour automatiques actives', { exact: true })).toBeVisible()
  await expect.poll(() => api.calls['1']).toBeGreaterThan(beforeReconnect)
  await chooseSupplier(page, 'Boréal')
  await chooseSupplier(page, 'Atlas')
  await expect.poll(() => page.evaluate(() => window.__specDrumSse.active)).toBe(1)
})

test('une réponse Atlas retardée ne remplace pas les offres Boréal', async ({ page }) => {
  const api = await mockSupplierAPI(page)
  const held = api.hold('1')
  try {
    await page.goto('/#fournisseurs')
    await chooseSupplier(page, 'Atlas')
    await held.requested
    await chooseSupplier(page, 'Boréal')
    await expect(page.getByRole('article', { name: 'Offre 201 : Moteur Boréal' })).toBeVisible()
    held.release()
    await held.settled
    // Attendre le rendu après la résolution réseau, sans minuterie arbitraire.
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
    await expect(page.getByRole('heading', { name: 'Boréal', exact: true })).toBeVisible()
    await expect(page.getByRole('article', { name: 'Offre 201 : Moteur Boréal' })).toBeVisible()
    await expect(page.getByText('Injecteur Atlas', { exact: true })).toHaveCount(0)
  } finally {
    held.release()
  }
})

test('erreur REST explicitement visible sans prix fabriqué', async ({ page }) => {
  await mockSupplierAPI(page)
  await page.route('**/api/v1/suppliers/1/offers', (route) => route.fulfill({
    status: 503, json: { error: 'Lecture des offres temporairement indisponible' },
  }))
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  await expect(page.getByText('Lecture des offres temporairement indisponible', { exact: false })).toBeVisible()
  await expect(page.getByTestId('price-101')).toHaveCount(0)
})

test('repli snapshot daté explicitement visible', async ({ page }) => {
  const pages = defaultPages()
  pages['1'].context = { mode: 'snapshot_fallback', snapshot_id: 7, as_of: '2026-11-04T12:00:00Z', reason: 'timeout' }
  await mockSupplierAPI(page, pages)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  const notice = page.getByRole('status').filter({ hasText: 'Source de prix indisponible.' })
  await expect(notice).toContainText('snapshot #7')
  await expect(notice).toContainText('2026')
  await expect(page.getByTestId('price-101')).toHaveText(/110,00\s*€/)
})

test('création fournisseur : validation serveur visible et notes transmises en chaînes', async ({ page }) => {
  await mockSupplierAPI(page)
  const submissions: unknown[] = []
  await page.route('**/api/v1/suppliers', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback()
    const body = route.request().postDataJSON()
    submissions.push(body)
    if (body.quality_score === '6') {
      await route.fulfill({ status: 400, json: { error: 'La qualité doit être comprise entre 0 et 5' } })
    } else {
      await route.fulfill({ status: 201, json: { id: 3, ...body } })
    }
  })
  await page.goto('/#fournisseurs')
  await page.getByRole('button', { name: '+ Nouveau fournisseur', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Nouveau fournisseur', exact: true })
  await dialog.getByLabel('Nom du fournisseur').fill('Cèdre')
  await dialog.getByLabel('Qualité (sur 5)').fill('6')
  await dialog.getByLabel('Confiance (sur 5)').fill('4.125')
  await dialog.getByRole('button', { name: 'Créer le fournisseur', exact: true }).click()
  await expect(dialog.getByRole('alert')).toHaveText('La qualité doit être comprise entre 0 et 5')
  await dialog.getByLabel('Qualité (sur 5)').fill('4.25')
  await dialog.getByRole('button', { name: 'Créer le fournisseur', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Cèdre', exact: true })).toBeVisible()
  expect(submissions).toEqual([
    { name: 'Cèdre', quality_score: '6', trust_score: '4.125' },
    { name: 'Cèdre', quality_score: '4.25', trust_score: '4.125' },
  ])
})

test('contrat expiré lu du serveur et édition sans conversion des paliers', async ({ page }) => {
  await mockSupplierAPI(page)
  let contract = {
    id: 10, supplier_id: 1, starts_on: '2025-01-01', ends_on: '2025-12-31',
    conditions: 'Livraison groupée', offer_ids: [101],
    tiers: [{ min_quantity: '500', discount_percent: '5.125' }], status: 'expired',
  }
  let submission: unknown
  await page.route('**/api/v1/suppliers/1/contracts*', (route) => route.fulfill({ json: [contract] }))
  await page.route('**/api/v1/contracts/10', async (route) => {
    expect(route.request().method()).toBe('PATCH')
    submission = route.request().postDataJSON()
    contract = { ...contract, ...route.request().postDataJSON() }
    await route.fulfill({ json: contract })
  })
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  await page.getByRole('tab', { name: /^Contrats/ }).click()
  await expect(page.getByText('Expiré', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Modifier le contrat #10', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Modifier le contrat #10', exact: true })
  await expect(dialog.getByLabel('Quantité minimale (palier 1)')).toHaveValue('500')
  await dialog.getByLabel('Conditions du contrat').fill('Conditions modifiées')
  await dialog.getByRole('button', { name: 'Enregistrer le contrat', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByText('Conditions modifiées', { exact: true })).toBeVisible()
  expect(submission).toEqual({
    starts_on: '2025-01-01', ends_on: '2025-12-31', conditions: 'Conditions modifiées',
    offer_ids: [101], tiers: [{ min_quantity: '500', discount_percent: '5.125' }],
  })
})
