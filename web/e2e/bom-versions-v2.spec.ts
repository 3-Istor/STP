import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

// V2 lot F, partie 2 : versions (Toutes les BOM), comparaison et causes d'écart (Arbitrer), pistes de réduction,
// profil de véhicule et classement des alternatives, contrôles et publication tracée (Publier), analyses
// (Chiffrer, Sourcer). API simulée : les chiffres viennent de la fixture, jamais du front.

const at = '2026-04-07T14:32:07Z'
const eur = (amount: string) => ({ amount, currency: 'EUR' })
const method = { basis: 'full', fx: 'budget', program_volume: 60000 }
const totals = (cost: string) => ({
  real_quantity: '1', cost: eur(cost), cost_error_code: null, cost_error: null, missing_cost: [],
  co2_kg: '1', mass_kg: '1', lead_time_days: 12, own_cost: null, own_lead_time_days: 0, quote: null, rate: null,
  labor_hours: null, labor_error: null, missing_labor: [], fte: null, stock_covered: null, in_stock: false,
})
const node = (id: number, parent_id: number | null, name: string, part_id: number | null = null) =>
  ({ id, parent_id, name, part_id, offer_id: part_id ? 101 : null, quantity: '1', production_days: 0, strategy: null, totals: totals('100.00') })
const nodes = [node(1, null, 'Fiat Multipla'), node(2, 1, 'Carrosserie et ouvrants'), node(3, 2, 'Porte avant gauche'), node(4, 3, '', 10), node(5, 3, '', 11)]
const bomB = {
  id: 3, name: 'Fiat Multipla', currency: 'EUR', volume: 2000, annual_hours: '1607', destination_country: 'IT',
  price_snapshot_id: null, source_snapshot_id: 1, created_at: at, cost_method: method,
}
const version = (kind: 'draft' | 'snapshot', id: number, label: string, cost: string, extra: object = {}) => ({
  kind, id, bom_id: kind === 'draft' ? id : 1, ref: `${kind === 'draft' ? 'bom' : 'snapshot'}:${id}`, label, name: 'Fiat Multipla',
  at, source_snapshot_id: kind === 'draft' && id === 3 ? 1 : null, method, cost: eur(cost), cost_error: null,
  target_budget: eur('26500.00'), budget_percent: '97.2', price_snapshot_id: kind === 'snapshot' ? 5 : null,
  proposed_by: null, validated_by: null, note: null, part_count: 2, node_count: 5, ...extra,
})
const versions = [
  version('draft', 1, 'Brouillon A', '25823.86'),
  version('snapshot', 1, 'V1', '25764.893287', { proposed_by: 'Arthur P.', validated_by: 'Joe', note: 'Jalon de lancement' }),
  version('draft', 3, 'Brouillon B', '26096.839607'),
]
const side = (ref: string, label: string, cost: string) => ({ ref, label, name: 'Fiat Multipla', method, price_snapshot_id: null, totals: { cost: eur(cost), co2_kg: '4449.69', mass_kg: '1148.5', lead_time_days: 58 } })
// Σ causes = écart (331,95 €) ; ordre de l'API volontairement pas trié par montant.
const comparison = {
  from: side('snapshot:1', 'V1', '25764.8932874925284273025826666665'),
  to: side('bom:3', 'Brouillon B', '26096.8396072962966585434331666665'),
  delta: { cost: eur('331.9463198037682312408505'), co2_kg: '53.1124', mass_kg: '1.6548', lead_time_days: 0 },
  causes: [
    { kind: 'structure', amount: eur('73.3191773475531202'), explanation: '4 pièces ajoutées (Haut-parleur 13 cm ×4).', node_ids: [5], from_node_ids: [], node_count: 4 },
    { kind: 'offer_change', amount: eur('28.299008152732578179'), explanation: 'Porte avant nue : Arden Composites → Fonderia Lombarda Brenta (2 nœuds).', node_ids: [4], from_node_ids: [], node_count: 2 },
    { kind: 'price', amount: eur('60.2931013229593961688505'), explanation: 'Nouvelles cotations sur 250 offres.', node_ids: [], from_node_ids: [], node_count: 13638 },
    { kind: 'discount', amount: eur('170.035032980523136693'), explanation: 'Contrat Arden Composites expiré : la remise (jusqu’à 9,7 %) ne s’applique plus sur 12 offres.', node_ids: [4], from_node_ids: [], node_count: 39 },
  ],
  changed_parts: [
    { change: 'offer', path: 'Carrosserie et ouvrants > Porte avant gauche > Porte avant nue', node_id: 4, from_node_id: 8438, part_id: 10, before: 'Arden Composites · 1 u.', after: 'Fonderia Lombarda Brenta · 1 u.', delta: eur('14.1495') },
  ],
}
const checks = (blocked: boolean) => [
  { code: 'offers_active', level: 'ok', title: 'Toutes les pièces ont une offre active', detail: '5 nœuds', node_ids: [] },
  { code: 'prices_frozen', level: 'warn', title: 'Prix figés dans un snapshot', detail: 'prix en direct : un snapshot de prix sera créé à la publication', node_ids: [] },
  { code: 'budgets', level: 'warn', title: '1 sous-système(s) au-dessus du budget', detail: 'Carrosserie et ouvrants 118,8 % : justification requise', node_ids: [2] },
  { code: 'expired_contracts', level: blocked ? 'block' : 'ok', title: 'Contrat expiré encore utilisé', detail: 'Arden Composites, 31/03', node_ids: blocked ? [4] : [] },
]
const ideas = {
  ideas: [
    { id: 1, title: 'Portes avant nues → Fonderia', owner: 'Brian P.', estimated_gain: eur('91.00'), status: 'in_progress', draft_bom_id: 3, created_at: at },
    { id: 2, title: 'Jante alu → tôle (base)', owner: 'Arthur P.', estimated_gain: eur('286.00'), status: 'to_validate', draft_bom_id: null, created_at: at },
  ],
  open_total: eur('377.00'),
  gap_to_target: eur('-510.5984148019521432432258333335'),
  remaining: eur('-887.5984148019521432432258333335'),
  gap_error: null,
}
const presets = [
  { preset: 'family', label: 'Familiale', hint: 'Équilibre coût, délai et CO₂.', weights: { cost: 40, co2: 25, lead_time: 20, mass: 5, quality: 10 }, custom: false },
  { preset: 'sport', label: 'Sportive', hint: 'La masse passe devant tout.', weights: { cost: 20, co2: 10, lead_time: 10, mass: 45, quality: 15 }, custom: false },
  { preset: 'custom', label: 'Personnalisé', hint: 'Réglez les poids vous-même (somme 100).', weights: { cost: 30, co2: 30, lead_time: 20, mass: 10, quality: 10 }, custom: true },
]
const parts = [
  { id: 10, name: 'Écrou à embase M8', category: 'Visserie', co2_kg_per_unit: null, mass_kg: null, production_days: null, offers: [
    { id: 18, supplier_id: 1, currency: 'EUR', lead_time_days: 5, origin_country: 'FR' },
    { id: 19, supplier_id: 2, currency: 'EUR', lead_time_days: 9, origin_country: 'DE' },
    { id: 20, supplier_id: 3, currency: 'EUR', lead_time_days: 12, origin_country: 'IT' },
  ] },
  { id: 11, name: 'Haut-parleur 13 cm', category: 'Audio', co2_kg_per_unit: null, mass_kg: null, production_days: null, offers: [{ id: 101, supplier_id: 1, currency: 'EUR', lead_time_days: 3, origin_country: 'FR' }] },
]
const candidate = (offer_id: number, score: string, current = false) => ({
  part_id: 10, offer_id, current, cost: eur('1.00'), lead_time_days: 5, co2_kg: '0.1', quote: null, rate: null,
  score, cost_score: '0', lead_time_score: '0', co2_score: '0', mass_score: '0', quality_score: '0', excluded: null,
})

type Call = { method: string; path: string; body: unknown }

async function mockAPI(page: Page, opts: { blocked?: boolean } = {}) {
  const calls: Call[] = []
  let profile: { preset: string | null; weights: object | null } = { preset: 'family', weights: presets[0].weights }
  let ideasState = structuredClone(ideas)
  await page.route('**/api/**', async (route) => {
    const req = route.request()
    const url = new URL(req.url())
    const path = url.pathname
    const body = req.postData() ? req.postDataJSON() : null
    if (req.method() !== 'GET') calls.push({ method: req.method(), path, body })
    if (req.method() === 'POST' && path === '/api/v1/boms/3/publish') {
      if (opts.blocked) return route.fulfill({ status: 409, json: { error: 'expired_contracts (Arden Composites, 31/03): publication blocked by a check' } })
      const j = (body as { justifications: Record<string, string> }).justifications
      if (!j.prices_frozen || !j.budgets) return route.fulfill({ status: 422, json: { error: 'justification required' } })
      return route.fulfill({ status: 201, json: { id: 9, bom_id: 3, price_snapshot_id: 12, name: 'Fiat Multipla', currency: 'EUR', volume: 2000, published_at: '2026-04-07T15:00:01Z' } })
    }
    if (req.method() === 'PUT' && path === '/api/v1/boms/3/profile') {
      profile = body as typeof profile
      return route.fulfill({ json: profile })
    }
    if (req.method() === 'PATCH' && path === '/api/v1/boms/3/ideas/2') {
      ideasState.ideas[1].status = (body as { status: string }).status
      return route.fulfill({ json: ideasState.ideas[1] })
    }
    if (path === '/api/v1/boms/3/nodes/4/suggestions') {
      // Le classement dépend du profil enregistré (calcul de l'API, ADR 0036).
      const sport = profile.preset === 'sport'
      return route.fulfill({ json: {
        node_id: 4, strategy: null, strategy_from: null, real_quantity: '1', currency: 'EUR', price_snapshot_id: null,
        candidates: sport ? [candidate(18, '0.1', true), candidate(19, '0.25'), candidate(20, '0.26')] : [candidate(20, '0.2'), candidate(18, '0.3', true), candidate(19, '0.44')],
        excluded: [], profile, weights: profile.weights,
      } })
    }
    if (path === '/api/v1/boms/3/analysis/fx-sensitivity') {
      const shock = url.searchParams.get('shock_percent')
      return route.fulfill({ json: [{ currency: 'USD', share_percent: '21.12', impact: eur(shock === '10' ? '557.35' : '278.67'), budget_percent_after: shock === '10' ? '101.69' : '100.64', shock_percent: shock, exposed: eur('5573.49'), cost_before: eur('26391.74'), cost_after: eur('26670.41'), budget: eur('26500.00'), budget_error: null, rates: [], cost_method: method, price_snapshot_id: null }] })
    }
    const json = {
      '/api/v1/boms': [{ ...bomB, id: 1, source_snapshot_id: null }, bomB],
      '/api/v1/boms/1/versions': versions,
      '/api/v1/boms/3/versions': versions,
      '/api/v1/boms/3/tree': { bom: bomB, nodes, explanation: { currency: 'EUR', price_snapshot_id: null, rates: [], cost_method: method, cost_as_of: at }, stock: [], planning: { schedule: null, lead_time_days: 12, available_on: null, critical: [], allowed_days: null, target_met: null, late: [] }, budgets: [] },
      '/api/v1/compare': comparison,
      '/api/v1/boms/3/publication-checks': checks(!!opts.blocked),
      '/api/v1/boms/3/ideas': ideasState,
      '/api/v1/vehicle-profiles': presets,
      '/api/v1/boms/3/profile': profile,
      '/api/v1/boms/3/anomalies': { threshold: '3', currency: 'EUR', price_snapshot_id: null, rates: [], nodes: [] },
      '/api/v1/boms/3/analysis/anomalies': { method: 'per_kg', threshold: '2', currency: 'EUR', price_snapshot_id: null, rates: [], anomaly_count: 0, parts: [] },
      '/api/v1/boms/3/analysis/price-drivers': [],
      '/api/v1/parts': parts,
      '/api/v1/suppliers': [{ id: 1, name: 'Fonderia Lombarda Brenta' }, { id: 2, name: 'Rheinwerk' }, { id: 3, name: 'Officine Valsusa' }],
      '/api/v1/fx': { rates: [] },
      '/api/v1/price-snapshots': [],
    }[path]
    if (json === undefined) await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
    else await route.fulfill({ json })
  })
  return calls
}

test('Toutes les BOM : une famille, ses versions avec coût, % du budget et noms ; aperçu avec l’écart', async ({ page }) => {
  await mockAPI(page)
  await page.goto('/#/boms')
  const table = page.getByRole('table', { name: 'Toutes les BOM' })
  // Deux BOM de la même famille : la liste des versions n'apparaît qu'une fois.
  await expect(table.locator('tr.family-row')).toHaveCount(1)
  const v1 = table.getByRole('row').filter({ hasText: 'V1' })
  await expect(v1).toContainText('25 764,89 €')
  await expect(v1).toContainText('97,2 %')
  await expect(v1).toContainText('Arthur P. · Joe')
  await table.getByRole('row').filter({ hasText: 'Brouillon B' }).getByRole('cell').nth(1).click()
  const preview = page.getByRole('complementary', { name: 'Aperçu' })
  await expect(preview).toContainText('Fiat Multipla · Brouillon B')
  await expect(preview).toContainText('V1 (07/04/2026 14:32:07 UTC)')
  await expect(preview).toContainText('+ 331,95 €')
  await expect(preview).toContainText('2 sur 4')
})

test('Arbitrer : comparaison V1 → Brouillon B, causes triées par montant, lien vers les nœuds ; pistes signées', async ({ page }) => {
  const calls = await mockAPI(page)
  await page.goto('/#/boms/3/arbitrer')
  await expect(page.getByRole('combobox', { name: 'Version de départ' })).toHaveValue('snapshot:1')
  await expect(page.getByRole('combobox', { name: 'Version d’arrivée' })).toHaveValue('bom:3')
  const causes = page.getByRole('region', { name: 'Pourquoi le coût a bougé' })
  await expect(causes).toContainText('Pourquoi le coût a bougé entre V1 et Brouillon B · + 331,95 €')
  // Triées par montant absolu : remise (170,04) > structure (73,32) > cotations (60,29) > offre (28,30).
  await expect(causes.locator('li .n')).toHaveText(['+ 170,04 €', '+ 73,32 €', '+ 60,29 €', '+ 28,30 €'])
  await expect(causes.locator('li').first()).toContainText('Contrat Arden Composites expiré')
  await causes.locator('li').first().getByRole('button', { name: 'Écrou à embase M8' }).click()
  await expect(page).toHaveURL(/node=4/)
  await expect(page.getByRole('table', { name: 'Pièces modifiées' })).toContainText('Fonderia Lombarda Brenta · 1 u.')

  // Pistes : écart à la cible et reste signés par l'API (négatif = couvert).
  const pistes = page.getByRole('region', { name: 'Pistes de réduction' })
  await expect(pistes).toContainText('budget tenu, marge de 510,60 €')
  await expect(pistes).toContainText('377,00 €')
  await expect(pistes).toContainText('couvert avec 887,60 € d’avance')
  await pistes.getByRole('combobox', { name: 'Statut de Jante alu → tôle (base)' }).selectOption('done')
  await expect.poll(() => calls.filter((c) => c.method === 'PATCH')).toEqual([{ method: 'PATCH', path: '/api/v1/boms/3/ideas/2', body: { status: 'done' } }])
})

test('profil de véhicule changé → classement des alternatives changé', async ({ page }) => {
  const calls = await mockAPI(page)
  await page.goto('/#/boms/3/arbitrer?node=4')
  const alts = page.getByRole('region', { name: 'Stratégie et alternatives' })
  await expect(alts).toContainText('Classées selon le profil « Familiale » (coût 40 · co₂ 25 · délai 20 · masse 5 · qualité 10)')
  await expect(alts.locator('tbody tr td:first-child')).toHaveText([/Officine Valsusa/, /Fonderia/, /Rheinwerk/])

  await page.getByRole('tab', { name: 'Réglages de calcul' }).click()
  const profile = page.getByRole('region', { name: 'Profil de véhicule' })
  await expect(profile.getByRole('radio', { name: 'Familiale' })).toHaveAttribute('aria-checked', 'true')
  await profile.getByRole('radio', { name: 'Sportive' }).click()
  await expect(profile.getByRole('radio', { name: 'Sportive' })).toHaveAttribute('aria-checked', 'true')
  expect(calls.filter((c) => c.method === 'PUT')).toEqual([
    { method: 'PUT', path: '/api/v1/boms/3/profile', body: { preset: 'sport', weights: presets[1].weights } },
  ])
  await expect(alts).toContainText('Classées selon le profil « Sportive »')
  await expect(alts.locator('tbody tr td:first-child')).toHaveText([/Fonderia/, /Rheinwerk/, /Officine Valsusa/])
})

test('Publier sur le brouillon B : refusée (409) par le contrat expiré, message clair, « Corriger » mène au nœud', async ({ page }) => {
  const calls = await mockAPI(page, { blocked: true })
  await page.goto('/#/boms/3/publier')
  const controls = page.getByRole('region', { name: 'Contrôles avant publication' })
  await expect(controls).toContainText('Contrôles avant publication · 1 sur 4')
  await page.getByRole('button', { name: 'Publier ce brouillon' }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Publication bloquée' })).toHaveText('Publication bloquée par : Contrat expiré encore utilisé (Arden Composites, 31/03)')
  expect(calls.filter((c) => c.path.endsWith('/publish'))).toHaveLength(1)
  await controls.getByRole('listitem').filter({ hasText: 'Contrat expiré' }).getByRole('button', { name: 'Corriger' }).click()
  await expect(page).toHaveURL(/#\/boms\/3\/structurer\?node=4$/)
})

test('Publier : avertissements non justifiés surlignés (422), puis publication avec note, noms et justifications', async ({ page }) => {
  const calls = await mockAPI(page)
  await page.goto('/#/boms/3/publier')
  await page.getByRole('textbox', { name: 'Note de décision' }).fill('Portes Fonderia retenues.')
  await page.getByRole('textbox', { name: 'Proposé par' }).fill('Brian P.')
  await page.getByRole('textbox', { name: 'Validé par' }).fill('Joe')
  await page.getByRole('button', { name: 'Publier ce brouillon' }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Justification requise' })).toContainText('Prix figés dans un snapshot ; 1 sous-système(s) au-dessus du budget')
  await expect(page.locator('.check.missing')).toHaveCount(2)
  await page.getByRole('textbox', { name: 'Justification : Prix figés dans un snapshot' }).fill('Prix du jour assumés.')
  await page.getByRole('textbox', { name: 'Justification : 1 sous-système(s) au-dessus du budget' }).fill('Revue en V3.')
  await page.getByRole('button', { name: 'Publier ce brouillon' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Snapshot #9 publié' })).toBeVisible()
  expect(calls.filter((c) => c.path.endsWith('/publish')).map((c) => c.body)).toEqual([
    { note: 'Portes Fonderia retenues.', proposed_by: 'Brian P.', validated_by: 'Joe', justifications: {} },
    { note: 'Portes Fonderia retenues.', proposed_by: 'Brian P.', validated_by: 'Joe', justifications: { prices_frozen: 'Prix du jour assumés.', budgets: 'Revue en V3.' } },
  ])
})

test('Chiffrer : choc de change 5 % puis 10 %, chiffres de l’API', async ({ page }) => {
  await mockAPI(page)
  await page.goto('/#/boms/3/chiffrer')
  const fx = page.getByRole('region', { name: 'Sensibilité au change' })
  await expect(fx.getByRole('row').filter({ hasText: 'USD +5 %' })).toContainText('+ 278,67 €')
  await fx.getByRole('radio', { name: '+10 %' }).click()
  await expect(fx.getByRole('row').filter({ hasText: 'USD +10 %' })).toContainText('+ 557,35 €')
  await expect(fx.getByRole('row').filter({ hasText: 'USD +10 %' })).toContainText('101,7 %')
})
