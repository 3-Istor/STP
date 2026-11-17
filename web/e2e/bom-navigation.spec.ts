import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

// Lisibilité de l'arbre BOM : frères triés par ordre alphabétique, en-têtes de colonnes, seuls les
// premiers niveaux dépliés à l'ouverture, recherche (casse et accents ignorés) qui garde le chemin
// jusqu'aux résultats. Devise de référence choisie dans une liste issue des taux publiés.

const bom = {
  id: 1, name: 'Fiat Multipla', currency: 'EUR', volume: 1000, annual_hours: '1607', destination_country: null,
  price_snapshot_id: null, source_snapshot_id: null, created_at: '2026-11-04T12:00:00Z',
}
const node = (id: number, parent_id: number | null, name: string) =>
  ({ id, parent_id, name, part_id: null, offer_id: null, quantity: '1', production_days: 0, totals: null })
// Ids volontairement dans un autre ordre que les noms : l'affichage ne doit pas suivre les ids.
const nodes = [
  node(1, null, 'Fiat Multipla'),
  node(2, 1, 'Habitacle'),
  node(3, 1, 'Châssis'),
  node(4, 1, 'Aérodynamique'),
  node(5, 3, 'Train avant'),
  node(6, 5, 'Écrou M8'),
  node(7, 5, 'Bras de suspension'),
]

const parts = [
  { id: 10, name: 'Étrier flottant', category: 'Freinage', co2_kg_per_unit: null, mass_kg: null, production_days: null, offers: [
    { id: 101, supplier_id: 1, currency: 'EUR', lead_time_days: 12, origin_country: 'FR' },
    { id: 102, supplier_id: 2, currency: 'USD', lead_time_days: 5, origin_country: 'US' },
  ] },
  { id: 11, name: 'Disque ventilé', category: 'Freinage', co2_kg_per_unit: null, mass_kg: null, production_days: null, offers: [
    { id: 111, supplier_id: 1, currency: 'EUR', lead_time_days: 3, origin_country: 'FR' },
  ] },
  { id: 12, name: 'Siège conducteur', category: 'Habitacle', co2_kg_per_unit: null, mass_kg: null, production_days: null, offers: [] },
]

async function mockAPI(page: Page, added: unknown[] = []) {
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (['DELETE', 'PATCH'].includes(route.request().method()) && path === '/api/v1/boms/1') {
      added.push({ method: route.request().method(), body: route.request().postDataJSON() })
      await route.fulfill(route.request().method() === 'DELETE' ? { status: 204 } : { json: { ...bom, name: 'Multipla 2' } })
      return
    }
    if (route.request().method() === 'POST' && path === '/api/v1/boms/1/nodes') {
      added.push(route.request().postDataJSON())
      await route.fulfill({ status: 201, json: { id: 99 } })
      return
    }
    const json = {
      '/api/v1/boms': [bom, { ...bom, id: 2, name: 'Aston Martin', currency: 'GBP' }],
      '/api/v1/boms/1/tree': { bom, nodes, explanation: { currency: 'EUR', price_snapshot_id: null, rates: [] }, stock: [], planning: { schedule: null, lead_time_days: 0, available_on: null, critical: [], allowed_days: null, target_met: null, late: [] } },
      '/api/v1/boms/1/anomalies': { threshold: '3', currency: 'EUR', price_snapshot_id: null, rates: [], nodes: [] },
      '/api/v1/parts': parts,
      '/api/v1/suppliers': [{ id: 1, name: 'Atlas' }, { id: 2, name: 'Boréal' }],
      '/api/v1/price-snapshots': [],
      '/api/v1/fx': { rates: [{ base_currency: 'USD', quote_currency: 'EUR' }, { base_currency: 'EUR', quote_currency: 'USD' }] },
    }[path]
    if (json === undefined) await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
    else await route.fulfill({ json })
  })
}

const rowNames = (page: Page) => page.locator('.tree-row .name')

async function openFiat(page: Page) {
  await page.goto('/#/boms')
  await page.getByRole('link', { name: 'Fiat Multipla', exact: true }).click()
  await expect(page).toHaveURL(/#\/boms\/1\/structurer$/)
}

test('arbre trié, en-têtes de colonnes, premiers niveaux dépliés et recherche', async ({ page }) => {
  await mockAPI(page)
  await page.goto('/#/boms')
  // Liste des BOM triée par nom.
  await expect(page.locator('.boms-table .bom-name')).toHaveText(['Aston Martin', 'Fiat Multipla'])
  await page.getByRole('link', { name: 'Fiat Multipla', exact: true }).click()

  await expect(page.locator('.tree-head')).toContainText('Ensemble / pièce')
  await expect(page.locator('.tree-head')).toContainText('Coût / véh.')
  // Profondeur 0 à 2 visible, « Train avant » (profondeur 2) replié : ses enfants sont cachés.
  await expect(rowNames(page)).toHaveText(['Fiat Multipla', 'Aérodynamique', 'Châssis', 'Train avant', 'Habitacle'])
  await expect(page.getByRole('treeitem').filter({ hasText: 'Train avant' })).toHaveAttribute('aria-expanded', 'false')

  const search = page.getByRole('searchbox', { name: 'Rechercher dans la BOM' })
  await search.fill('ecrou')
  await expect(rowNames(page)).toHaveText(['Fiat Multipla', 'Châssis', 'Train avant', 'Écrou M8'])
  await search.fill('châssis')
  // Un ensemble trouvé garde son sous-arbre (selon l'état replié).
  await expect(rowNames(page)).toHaveText(['Fiat Multipla', 'Châssis', 'Train avant'])
  await search.fill('introuvable')
  await expect(page.getByText('Aucun résultat pour « introuvable ».')).toBeVisible()
  await search.fill('')
  await expect(rowNames(page)).toHaveCount(5)
})

test('lien profond : le nœud de l’URL est déplié, sélectionné et suit d’une étape à l’autre', async ({ page }) => {
  await mockAPI(page)
  // « Écrou M8 » est sous « Train avant », replié à l'ouverture : le lien profond le révèle.
  await page.goto('/#/boms/1/chiffrer?node=6')
  await expect(page.getByRole('region', { name: 'Détail du nœud' })).toContainText('Écrou M8')
  await page.getByRole('navigation', { name: 'Étapes' }).getByRole('link', { name: /Structurer/ }).click()
  await expect(page).toHaveURL(/#\/boms\/1\/structurer\?node=6$/)
  await expect(page.getByRole('treeitem', { selected: true })).toContainText('Écrou M8')
  await expect(page.getByRole('navigation', { name: 'Étapes' }).getByRole('link', { name: /Structurer/ })).toHaveAttribute('aria-current', 'page')
  // Le bouton « précédent » revient à l'étape Chiffrer avec la même sélection.
  await page.goBack()
  await expect(page).toHaveURL(/#\/boms\/1\/chiffrer\?node=6$/)
  await expect(page.getByRole('region', { name: 'Détail du nœud' })).toContainText('Écrou M8')
})

test('devise de référence choisie dans la liste des devises cotées', async ({ page }) => {
  await mockAPI(page)
  await page.goto('/#/boms')
  await page.getByRole('button', { name: '+ Nouvelle BOM' }).click()
  // Taux publiés (EUR, USD) plus devises déjà utilisées par une BOM (GBP), sans doublon, triées.
  const create = page.getByRole('dialog', { name: 'Nouvelle BOM' })
  await expect(create.getByRole('combobox', { name: 'Devise', exact: true }).locator('option')).toHaveText(['EUR', 'GBP', 'USD'])
  await create.getByRole('button', { name: 'Annuler' }).click()
  await page.getByRole('link', { name: 'Fiat Multipla', exact: true }).click()
  await page.getByRole('tab', { name: 'Réglages de calcul' }).click()
  await expect(page.getByRole('combobox', { name: 'Devise de référence' })).toHaveValue('EUR')
})

test('une ligne de BOM montre son aperçu, « Ouvrir » mène à l’étape Structurer', async ({ page }, testInfo) => {
  await mockAPI(page)
  await page.goto('/#/boms')
  // Sans GET /versions (fixture absente), la BOM reste listée seule, sous la ligne de sa famille.
  const row = page.locator('tr.bom-row').filter({ hasText: 'Fiat Multipla' })
  const date = row.getByRole('cell').nth(1)
  if (testInfo.project.use.hasTouch) await date.tap()
  else await date.click()
  await expect(page.getByRole('complementary', { name: 'Aperçu' })).toContainText('Fiat Multipla · brouillon')
  await expect(row).toHaveClass(/sel/)
  await row.getByRole('link', { name: 'Ouvrir Fiat Multipla' }).click()
  await expect(page).toHaveURL(/#\/boms\/1\/structurer$/)
  await expect(page.getByRole('navigation', { name: 'Fil d’Ariane' })).toContainText('Fiat Multipla')
})

test('ajout d’une pièce dans une fenêtre : recherche, catégorie, offre et quantité', async ({ page }) => {
  const added: unknown[] = []
  await mockAPI(page, added)
  await openFiat(page)
  await page.getByRole('treeitem').filter({ hasText: 'Châssis' }).click()
  // Ajout en un geste depuis la ligne sélectionnée (le panneau de détail garde aussi « + Ajouter dessous »).
  await page.getByRole('treeitem').filter({ hasText: 'Châssis' }).getByRole('button', { name: 'Ajouter dessous' }).click()

  const dialog = page.getByRole('dialog', { name: 'Ajouter une pièce' })
  await expect(dialog).toContainText('Ajouter sous « Châssis »')
  await expect(dialog.getByRole('searchbox', { name: 'Rechercher une pièce' })).toBeFocused()
  const results = dialog.getByRole('listbox', { name: 'Pièces' }).getByRole('option')
  // Pièces triées ; la catégorie filtre, la recherche ignore accents et casse.
  await expect(results).toHaveText([/^Disque ventilé/, /^Étrier flottant/, /^Siège conducteur/])
  await dialog.getByRole('combobox', { name: 'Catégorie' }).selectOption('Freinage')
  await expect(results).toHaveCount(2)
  await dialog.getByRole('searchbox', { name: 'Rechercher une pièce' }).fill('etrier')
  await expect(results).toHaveText([/^Étrier flottant/])
  await results.getByRole('button').click()

  // Deux offres : aucune n'est choisie à la place de l'utilisateur.
  const add = dialog.getByRole('button', { name: 'Ajouter', exact: true })
  await expect(add).toBeDisabled()
  await dialog.getByRole('radio', { name: /Boréal/ }).check()
  await dialog.getByRole('textbox', { name: 'Quantité' }).fill('4')
  await add.click()
  await expect(dialog).toHaveCount(0)
  expect(added).toEqual([{ parent_id: 3, part_id: 10, offer_id: 102, quantity: '4' }])

  // Le panneau de détail ouvre la même fenêtre.
  await page.getByRole('button', { name: '+ Ajouter dessous' }).click()
  await expect(page.getByRole('dialog', { name: 'Ajouter une pièce' })).toContainText('Ajouter sous « Châssis »')
})

test('aucune devise saisie au clavier : toujours une liste', async ({ page }) => {
  await mockAPI(page)
  await openFiat(page)
  await page.getByRole('tab', { name: 'Réglages de calcul' }).click()
  await expect(page.getByRole('combobox', { name: 'Devise de référence' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: /devise/i })).toHaveCount(0)
})

test('suppression et renommage d’une BOM dans les fenêtres de l’app, jamais celles du navigateur', async ({ page }) => {
  const calls: unknown[] = []
  page.on('dialog', (d) => { throw new Error(`fenêtre native ouverte : ${d.type()} ${d.message()}`) })
  await mockAPI(page, calls)
  await page.goto('/#/boms')
  await page.getByRole('button', { name: 'Supprimer Fiat Multipla' }).click()
  const confirm = page.getByRole('dialog', { name: 'Supprimer la BOM « Fiat Multipla » ?' })
  await expect(confirm).toBeVisible()
  // Le bouton sûr a le focus : Entrée ne supprime pas par mégarde.
  await expect(confirm.getByRole('button', { name: 'Annuler' })).toBeFocused()
  await confirm.getByRole('button', { name: 'Annuler' }).click()
  await expect(confirm).toHaveCount(0)
  expect(calls).toEqual([])

  await page.getByRole('button', { name: 'Renommer Fiat Multipla' }).click()
  const rename = page.getByRole('dialog', { name: 'Renommer la BOM' })
  await expect(rename.getByRole('textbox', { name: 'Nouveau nom' })).toHaveValue('Fiat Multipla')
  await rename.getByRole('textbox', { name: 'Nouveau nom' }).fill('  Multipla 2 ')
  await rename.getByRole('button', { name: 'Renommer' }).click()
  await expect(rename).toHaveCount(0)

  await page.getByRole('button', { name: 'Supprimer Fiat Multipla' }).click()
  await page.getByRole('dialog', { name: 'Supprimer la BOM « Fiat Multipla » ?' }).getByRole('button', { name: 'Supprimer' }).click()
  await expect.poll(() => calls).toEqual([{ method: 'PATCH', body: { name: 'Multipla 2' } }, { method: 'DELETE', body: null }])
})

test('suppression d’un nœud depuis l’arbre, avec confirmation de l’app', async ({ page }) => {
  const calls: unknown[] = []
  await mockAPI(page, calls)
  await page.route('**/api/v1/boms/1/nodes/7', async (route) => {
    calls.push({ method: route.request().method(), node: 7 })
    await route.fulfill({ status: 204 })
  })
  await page.goto('/#/boms/1/structurer?node=7')
  const row = page.getByRole('treeitem', { selected: true })
  await expect(row).toContainText('Bras de suspension')
  await row.getByRole('button', { name: 'Supprimer' }).click()
  const confirm = page.getByRole('dialog', { name: 'Supprimer ce nœud ?' })
  await expect(confirm).toContainText('Bras de suspension')
  await confirm.getByRole('button', { name: 'Supprimer' }).click()
  await expect.poll(() => calls).toEqual([{ method: 'DELETE', node: 7 }])
  // La sélection passe au parent.
  await expect(page).toHaveURL(/node=5$/)
})
