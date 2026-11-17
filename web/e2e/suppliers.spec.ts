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
  await expect(page.getByRole('row', { name: 'Offre 101 : Injecteur Atlas' })).toBeVisible()
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
  const missing = page.getByRole('row', { name: 'Offre 104 : Sans cotation Atlas' })
  await expect(missing).toContainText('Aucune cotation disponible')
  // Ni prix ni variation fabriqués : la ligne montre l'absence (la fourchette cotée, elle, est une donnée réelle).
  await expect(page.getByTestId('price-104')).toHaveText('Prix indisponible')
  await expect(page.getByTestId('variation-104')).toHaveText('Variation indisponible')

  await chooseSupplier(page, 'Boréal')
  await expect(page.getByRole('row', { name: 'Offre 201 : Moteur Boréal' })).toBeVisible()
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

test('ligne d’offre cliquable : courbe de prix par période, offres triées et recherche', async ({ page }, testInfo) => {
  await mockSupplierAPI(page)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  // Offres triées par nom de pièce.
  await expect(page.locator('.offer-row .part-name')).toHaveText(['Capteur Atlas', 'Filtre Atlas', 'Injecteur Atlas', 'Sans cotation Atlas'])
  // Un clic sur la ligne (hors bouton d’historique) affiche sa courbe ; le bouton ouvre le détail des cotations.
  const name = page.getByRole('row', { name: 'Offre 101 : Injecteur Atlas' }).getByText('Injecteur Atlas', { exact: true })
  if (testInfo.project.use.hasTouch) await name.tap()
  else await name.click()
  await expect(page.getByRole('region', { name: 'Historique du prix · Injecteur Atlas' })).toBeVisible()
  await expect(page.getByRole('row', { name: 'Offre 101 : Injecteur Atlas' })).toHaveClass(/sel/)
  await page.getByRole('region', { name: 'Historique du prix · Injecteur Atlas' }).getByRole('button', { name: 'Toutes les cotations…' }).click()
  await expect(page.getByRole('dialog', { name: 'Historique : Injecteur Atlas' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)

  const search = page.getByRole('searchbox', { name: 'Rechercher une offre' })
  await search.fill('filtre')
  await expect(page.locator('.offer-row .part-name')).toHaveText(['Filtre Atlas'])
  await search.fill('aucune pièce')
  await expect(page.getByText('Aucune offre ne correspond à « aucune pièce »')).toBeVisible()
})

test('courbe de prix : période comptée depuis l’instant des statistiques, cotations réelles seulement', async ({ page }) => {
  const pages = defaultPages()
  const view = offerView({ id: 101, name: 'Injecteur Atlas' })
  const q = (id: number, amount: string, at: string) => ({ id, offer_id: 101, price: { amount, currency: 'EUR' }, source: 'e2e-fixture', quoted_at: at })
  view.statistics.history = [
    q(1, '100.00', '2026-07-01T00:00:00Z'), q(2, '130.00', '2026-10-15T00:00:00Z'), q(3, '95.00', '2026-10-20T00:00:00Z'),
    q(4, '105.00', '2026-10-28T00:00:00Z'), q(5, '110.00', '2026-11-04T12:00:00Z'),
  ]
  pages['1'].offers[0] = view
  await mockSupplierAPI(page, pages)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  await page.getByRole('row', { name: 'Offre 101 : Injecteur Atlas' }).click()
  const card = page.getByRole('region', { name: 'Historique du prix · Injecteur Atlas' })
  // Tout : un point par mois (juillet, octobre, novembre), clôture réelle du mois.
  await expect(card.locator('.chart-month')).toHaveCount(3)
  // 1 mois avant le 04/11/2026 12:00 : la cotation de juillet sort, un point par jour.
  await card.getByRole('radio', { name: '1 mois' }).click()
  await expect(card.getByRole('radio', { name: '1 mois' })).toHaveAttribute('aria-checked', 'true')
  await expect(card.locator('.chart-month')).toHaveCount(4)
  await expect(card.getByRole('img')).toHaveAttribute('aria-label', /de 95,00\s€ à 130,00\s€/)
  await expect(card.getByRole('status')).toContainText(/clôture 110,00\s€/)
  // Une fenêtre sans cotation le dit, sans courbe à zéro.
  pages['1'].offers[0] = { ...view, statistics: { ...view.statistics, history: [q(1, '100.00', '2026-07-01T00:00:00Z')] } }
  await page.getByRole('button', { name: 'Actualiser les données' }).click()
  await expect(card.getByText('Aucune cotation sur cette période.')).toBeVisible()
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

test('historique affiché en euros au taux du jour, prix d’origine et taux visibles', async ({ page }) => {
  const pages = defaultPages()
  const view = offerView({ id: 101, name: 'Injecteur Atlas', currency: 'GBP', amount: '110.00', reference: '100.00' })
  const [first, last] = view.statistics.history
  const rate = (id: number, value: string, at: string) => ({ id, base_currency: 'GBP', quote_currency: 'EUR', rate: value, source: 'e2e-fx', quoted_at: at })
  const converted = (quoteID: number, at: string, amount: string | null, fx?: ReturnType<typeof rate>) =>
    ({ quote_id: quoteID, quoted_at: at, price: amount ? { amount, currency: 'EUR' } : null, direction: fx ? 'direct' : undefined, fx_quote: fx, error: amount ? undefined : 'aucun taux GBP→EUR' })
  const current = converted(last.id, last.quoted_at, '132.00', rate(2, '1.2', last.quoted_at))
  pages['1'].offers[0] = { ...view, converted: {
    currency: 'EUR', current_quote: current, reference_quote: converted(view.statistics.reference_quote.id, view.statistics.reference_quote.quoted_at, '115.00', rate(1, '1.15', '2025-11-03T00:00:00Z')),
    minimum: { amount: '132.00', currency: 'EUR' }, maximum: { amount: '132.00', currency: 'EUR' }, variation_percent: '14.8', trend: 'up',
    history: [converted(first.id, first.quoted_at, null), current], missing: 1,
  } } as typeof view
  await mockSupplierAPI(page, pages)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  await page.getByRole('button', { name: 'Historique de Injecteur Atlas', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Historique : Injecteur Atlas' })
  await expect(dialog.locator('.history-summary')).toContainText(/132,00\s*€/)
  await expect(dialog.locator('.history-summary')).toContainText('+14,8 %')
  await expect(dialog).toContainText('Prix convertis en EUR au taux coté le jour de chaque cotation (devise d’origine : GBP)')
  // Cotation sans taux du jour : signalée, jamais tracée à zéro.
  await expect(dialog.getByText('1 cotation sans taux de change du jour, non tracée.')).toBeVisible()
  await expect(dialog.locator('.chart-month')).toHaveCount(1)
  const detail = dialog.getByTestId('quote-detail')
  await expect(detail).toContainText(/132,00\s*€/)
  await expect(detail).toContainText(/Coté 110,00\s*£GB · taux GBP→EUR 1,2 du/)
})

// Fenêtre « Nouvelle pièce vendue » : une offre du fournisseur sur une pièce du catalogue ou créée.
async function mockCatalogWrites(page: Page, offerFailures = 0) {
  const posts: { path: string; body: unknown }[] = []
  let failures = offerFailures
  await page.route('**/api/v1/fx', (route) => route.fulfill({ json: { rates: [{ base_currency: 'GBP', quote_currency: 'EUR' }] } }))
  await page.route('**/api/v1/parts**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (route.request().method() === 'GET') {
      await route.fulfill({ json: [
        { id: 10, name: 'Injecteur Atlas', category: 'moteur', zone: 'moteur', offers: [{ supplier_id: 1 }] },
        { id: 11, name: 'Bougie d’allumage', category: 'moteur', zone: 'moteur', offers: [] },
      ] })
      return
    }
    posts.push({ path, body: route.request().postDataJSON() })
    if (path === '/api/v1/parts') await route.fulfill({ status: 201, json: { id: 50, name: 'Turbo' } })
    else if (failures-- > 0) await route.fulfill({ status: 400, json: { error: 'price_min must not exceed price_max' } })
    else await route.fulfill({ status: 201, json: { id: 900 } })
  })
  return posts
}

test('pièce du catalogue ajoutée au fournisseur : recherche, offre complète', async ({ page }) => {
  await mockSupplierAPI(page)
  const posts = await mockCatalogWrites(page)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  await page.getByRole('button', { name: '+ Offre' }).click()
  const dialog = page.getByRole('dialog', { name: 'Nouvelle pièce vendue' })
  const parts = dialog.getByRole('listbox', { name: 'Pièces du catalogue' }).getByRole('option')
  await expect(parts).toHaveText([/^Bougie d’allumage/, /^Injecteur Atlas.*déjà vendue par ce fournisseur/])
  await dialog.getByRole('searchbox', { name: 'Rechercher une pièce' }).fill('bougie')
  await expect(parts).toHaveCount(1)
  const add = dialog.getByRole('button', { name: 'Ajouter la pièce' })
  await expect(add).toBeDisabled()
  await parts.getByRole('button').click()
  // Devise et pays : listes, jamais de saisie libre.
  await expect(dialog.getByRole('combobox', { name: 'Devise de l’offre' }).locator('option')).toHaveText(['EUR', 'GBP'])
  await dialog.getByRole('combobox', { name: 'Devise de l’offre' }).selectOption('GBP')
  await dialog.getByRole('combobox', { name: 'Pays d’origine' }).selectOption('GB')
  await dialog.getByLabel('Prix mini').fill('8,50')
  await dialog.getByLabel('Prix maxi').fill('12')
  await dialog.getByLabel('Délai de livraison (jours)').fill('6')
  await dialog.getByLabel('Qualité (sur 5)').fill('4.5')
  await add.click()
  await expect(dialog).toHaveCount(0)
  expect(posts).toEqual([{ path: '/api/v1/parts/11/offers', body: {
    supplier_id: 1, currency: 'GBP', price_min: '8.50', price_max: '12', lead_time_days: 6, quality_score: '4.5', origin_country: 'GB',
  } }])
})

test('nouvelle pièce : tous ses attributs, et pas de doublon si l’offre est refusée', async ({ page }) => {
  await mockSupplierAPI(page)
  const posts = await mockCatalogWrites(page, 1)
  await page.goto('/#fournisseurs')
  await chooseSupplier(page, 'Atlas')
  await page.getByRole('button', { name: '+ Offre' }).click()
  const dialog = page.getByRole('dialog', { name: 'Nouvelle pièce vendue' })
  await dialog.getByRole('radio', { name: 'Nouvelle pièce' }).click()
  const fill = async (label: string, value: string) => dialog.getByLabel(label, { exact: true }).fill(value)
  await fill('Nom', 'Turbo')
  await fill('Catégorie', 'moteur')
  await fill('Zone de la voiture', 'moteur')
  await fill('Masse (kg)', '7,2')
  await fill('CO2 (kg CO2e par unité)', '35')
  await fill('Matière recyclée (%)', '20')
  await fill('Recyclabilité (%)', '80')
  await fill('Coût de fabrication', '400')
  await fill('Délai de production (jours)', '3')
  await fill('Main-d’œuvre (heures par unité)', '1.5')
  await fill('Taux horaire', '45')
  await dialog.getByRole('combobox', { name: 'Pays d’origine' }).selectOption('DE')
  await fill('Prix mini', '900')
  await fill('Prix maxi', '800')
  await fill('Délai de livraison (jours)', '10')
  await fill('Qualité (sur 5)', '4')
  await dialog.getByRole('button', { name: 'Ajouter la pièce' }).click()
  // L'offre est refusée : l'erreur de l'API est montrée, la pièce créée n'est pas perdue.
  await expect(dialog.getByRole('alert')).toHaveText('price_min must not exceed price_max')
  await expect(dialog).toContainText('Pièce « Turbo » créée (#50)')
  await fill('Prix mini', '700')
  await dialog.getByRole('button', { name: 'Ajouter la pièce' }).click()
  await expect(dialog).toHaveCount(0)
  expect(posts.map((p) => p.path)).toEqual(['/api/v1/parts', '/api/v1/parts/50/offers', '/api/v1/parts/50/offers'])
  expect(posts[0].body).toEqual({
    name: 'Turbo', category: 'moteur', zone: 'moteur', mass_kg: '7.2', co2_kg_per_unit: '35', recycled_percent: '20',
    recyclability_percent: '80', manufacturing_cost: { amount: '400', currency: 'EUR' }, production_days: 3,
    labor_hours: '1.5', hourly_rate: { amount: '45', currency: 'EUR' },
  })
  expect(posts[2].body).toMatchObject({ supplier_id: 1, price_min: '700', price_max: '800', origin_country: 'DE' })
})

test('liste des fournisseurs filtrable, défilement propre sur grand écran', async ({ page }, testInfo) => {
  await mockSupplierAPI(page)
  await page.goto('/#fournisseurs')
  const list = page.getByRole('table', { name: 'Choisir un fournisseur' })
  await expect(list.getByRole('button')).toHaveCount(2)
  const filter = page.getByRole('searchbox', { name: 'Filtrer les fournisseurs' })
  await filter.fill('boreal')
  await expect(list.getByRole('button')).toHaveText([/Boréal/])
  await expect(page.locator('.supplier-count')).toHaveText('1 / 2')
  await filter.fill('zzz')
  await expect(page.getByText('Aucun fournisseur ne correspond à « zzz ».')).toBeVisible()
  await filter.fill('')
  await expect(list.getByRole('button')).toHaveCount(2)
  // Le fournisseur choisi est dans l'URL : un lien ou un rechargement y revient.
  await list.getByRole('button', { name: 'Fournisseur Boréal' }).click()
  await expect(page).toHaveURL(/#\/fournisseurs\?supplier=2$/)
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Boréal', exact: true })).toBeVisible()
  if (!testInfo.project.use.isMobile && !testInfo.project.use.hasTouch) {
    // La liste défile dans son bloc, bornée à la hauteur de la fenêtre.
    await expect(page.locator('.supplier-list-block .scroll')).toHaveCSS('overflow-y', 'auto')
  }
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
    await expect(page.getByRole('row', { name: 'Offre 201 : Moteur Boréal' })).toBeVisible()
    held.release()
    await held.settled
    // Attendre le rendu après la résolution réseau, sans minuterie arbitraire.
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
    await expect(page.getByRole('heading', { name: 'Boréal', exact: true })).toBeVisible()
    await expect(page.getByRole('row', { name: 'Offre 201 : Moteur Boréal' })).toBeVisible()
    await expect(page.getByText('Injecteur Atlas', { exact: true })).toHaveCount(0)
  } finally {
    held.release()
  }
})

test('erreur REST explicitement visible sans prix fabriqué', async ({ page }) => {
  await mockSupplierAPI(page)
  await page.route('**/api/v1/suppliers/1/offers?*', (route) => route.fulfill({
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
  await page.getByRole('button', { name: '+ Fournisseur', exact: true }).click()
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
