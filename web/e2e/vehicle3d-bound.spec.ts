import { createHash } from 'node:crypto'
import { expect, test, type Page } from '@playwright/test'

// GLB autonome synthétique : vérifie le vrai GLTFLoader et le rendu, sans prétention automobile.
function boxGLB(width = 1) {
  const p = new Float32Array([
    -width / 2, 0, -0.4, width / 2, 0, -0.4, width / 2, 0.8, -0.4, -width / 2, 0.8, -0.4,
    -width / 2, 0, 0.4, width / 2, 0, 0.4, width / 2, 0.8, 0.4, -width / 2, 0.8, 0.4,
  ])
  const ix = new Uint16Array([0, 2, 1, 0, 3, 2, 4, 5, 6, 4, 6, 7, 0, 1, 5, 0, 5, 4, 3, 7, 6, 3, 6, 2, 1, 2, 6, 1, 6, 5, 0, 4, 7, 0, 7, 3])
  const bin = Buffer.concat([Buffer.from(p.buffer), Buffer.from(ix.buffer)])
  const json = Buffer.from(JSON.stringify({ asset: { version: '2.0' }, scene: 0, scenes: [{ nodes: [0] }], nodes: [{ name: 'Component', mesh: 0 }],
    meshes: [{ primitives: [{ attributes: { POSITION: 0 }, indices: 1, material: 0 }] }],
    materials: [{ pbrMetallicRoughness: { baseColorFactor: [0.2, 0.5, 0.8, 1], metallicFactor: 0.5, roughnessFactor: 0.3 } }],
    buffers: [{ byteLength: bin.length }], bufferViews: [{ buffer: 0, byteLength: p.byteLength }, { buffer: 0, byteOffset: p.byteLength, byteLength: ix.byteLength }],
    accessors: [{ bufferView: 0, componentType: 5126, count: 8, type: 'VEC3', min: [-width / 2, 0, -0.4], max: [width / 2, 0.8, 0.4] }, { bufferView: 1, componentType: 5123, count: ix.length, type: 'SCALAR' }],
  }))
  const padded = Buffer.concat([json, Buffer.alloc((4 - json.length % 4) % 4, 0x20)])
  const out = Buffer.alloc(28 + padded.length + bin.length)
  out.write('glTF'); out.writeUInt32LE(2, 4); out.writeUInt32LE(out.length, 8)
  out.writeUInt32LE(padded.length, 12); out.writeUInt32LE(0x4e4f534a, 16); padded.copy(out, 20)
  out.writeUInt32LE(bin.length, 20 + padded.length); out.writeUInt32LE(0x004e4942, 24 + padded.length); bin.copy(out, 28 + padded.length)
  return out
}

function fixture() {
  const node = (id: number, parent_id: number | null, name: string, part_id: number | null = null) => ({ id, parent_id, name, part_id, offer_id: part_id ? 101 : null, quantity: '1', production_days: 0, totals: null })
  const nodes = [node(1, null, 'Essai synthétique'), node(2, 1, 'Propulsion'), node(3, 2, 'Identique', 40), node(4, 2, 'Identique', 40), node(5, 2, 'Thermique', 41), node(6, 1, 'Sans modèle', 42)]
  const buffers = [boxGLB(), boxGLB(2)]
  const assets = buffers.map((buffer, i) => ({ key: `box${i}`, path: `/models/box${i}.glb`, sha256: createHash('sha256').update(buffer).digest('hex'), unit_scale: 1, source_url: 'https://example.test/synthetic', license: 'CC0', license_url: 'https://creativecommons.org/publicdomain/zero/1.0/', author: 'Test synthétique', geometry_status: 'schematic' }))
  const binding = (id: number, part: number, asset: number, system = 'electric') => ({ node_id: id, parent_id: 2, part_id: part, asset_key: `box${asset}`, mesh_path: ['Component'], system, position_status: 'approximation', instances: [
    { key: 'a', position: [id === 3 ? -1.4 : id === 4 ? 1.4 : 0, 0, id === 5 ? -2 : 0], rotation: [0, 0, 0], scale: [1, 1, 1], explode_vector: [id === 3 ? -1 : 1, 0.8, 0] },
    { key: 'b', position: [-1.4, 0, 2], rotation: [0, 0, 0], scale: [1, 1, 1], explode_vector: [0, 1.6, 0] },
  ] })
  const manifest = { schema_version: 1, revision: 'synthetic-v1', scope: { kind: 'draft', id: 1 }, vehicle_label: 'Essai synthétique — aucune voiture réelle', assets,
    bindings: [binding(3, 40, 0), binding(4, 40, 0), binding(5, 41, 0, 'thermal'), binding(3, 43, 1)], assemblies: [] }
  return { nodes, buffers, manifest }
}

type Probe = { built: { root: { visible: boolean }; items: { obj: { nodeId: number }; node: { uuid: string; position: { toArray(): number[] } }; meshes: { geometry: { boundingBox: { min: { x: number }; max: { x: number } } }; material: { opacity: number } }[] }[] }; selection: Set<string>; camera: { position: { toArray(): number[] } }; screenPositionOf(key: string): { x: number; y: number }; diagnostics: { items: number; animating: boolean } }
async function probe(page: Page) {
  return page.locator('.v3d-root').evaluate(el => {
    const v = (el as HTMLElement & { __viewer: Probe }).__viewer
    return { rootVisible: v.built.root.visible, selection: [...v.selection], items: v.built.items.map(i => ({ id: i.obj.nodeId, uuid: i.node.uuid, position: i.node.position.toArray(), width: i.meshes[0].geometry.boundingBox.max.x - i.meshes[0].geometry.boundingBox.min.x, opacity: i.meshes[0].material.opacity })) }
  })
}
async function ready(page: Page) { await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'ready', { timeout: 30_000 }) }

test('manifestes : occurrences, picking, quantités, remplacement physique et offre seule', async ({ page }, testInfo) => {
  test.setTimeout(90_000)
  const f = fixture(), requests = [0, 0], errors: string[] = []
  let releaseReplacement = () => {}
  const replacementGate = new Promise<void>(resolve => { releaseReplacement = resolve })
  page.on('pageerror', e => errors.push(e.message))
  let physicalPart = 40
  await page.route('**/models/manifests/draft-1.json', route => route.fulfill({ json: f.manifest }))
  await page.route('**/models/box*.glb', async route => {
    const i = Number(route.request().url().match(/box(\d)/)![1]); requests[i]++
    if (i === 1) await replacementGate
    return route.fulfill({ body: f.buffers[i], contentType: 'model/gltf-binary' })
  })
  const bom = { id: 1, name: 'Essai synthétique', currency: 'EUR', volume: 1, annual_hours: '1607', price_snapshot_id: null, source_snapshot_id: null, created_at: '2026-11-04T12:00:00Z' }
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (route.request().method() === 'PATCH') {
      const n = f.nodes.find(n => n.id === Number(path.split('/').at(-1)))!
      Object.assign(n, route.request().postDataJSON())
      if (n.id === 3) n.part_id = physicalPart
      return route.fulfill({ json: n })
    }
    const json = {
      '/api/v1/boms': [bom], '/api/v1/boms/1/tree': { bom, nodes: f.nodes, explanation: { currency: 'EUR', price_snapshot_id: null, rates: [] }, stock: [], planning: null },
      '/api/v1/parts': [40, 41, 42, 43].map(id => ({ id, name: `Composant ${id}`, category: 'essai', offers: [101, 102].map(id => ({ id, supplier_id: 1, currency: 'EUR', lead_time_days: 1 })) })),
      '/api/v1/suppliers': [{ id: 1, name: 'Fournisseur test' }], '/api/v1/fx': { rates: [] },
    }[path]
    return json === undefined ? route.fulfill({ status: 404, json: { error: 'Fixture absente' } }) : route.fulfill({ json })
  })
  await page.goto('/#/boms/1/structurer?tab=arbre-3d')
  await ready(page)
  await page.getByRole('radio', { name: 'Composants', exact: true }).click()
  await ready(page)
  await expect(page.getByText('3/4 occurrences physiques représentées', { exact: false })).toBeVisible()
  const initial = await probe(page)
  expect(initial.items.map(i => i.id)).toEqual([3, 4, 5])
  // La même référence catalogue n'entraîne pas la sélection de l'autre occurrence.
  await page.evaluate(() => { location.hash = '#/boms/1/structurer?tab=arbre-3d&node=3' })
  await expect.poll(() => probe(page).then(p => p.selection)).toEqual(['node:3/instance:a'])
  // La vue combinée masque la colonne Offre : déclencher son événement teste la relecture sans démonter le canvas.
  const offers = page.getByRole('treeitem', { selected: true }).getByRole('combobox', { name: 'Offre', includeHidden: true })
  await offers.selectOption('102', { force: true })
  await expect(offers).toHaveValue('102')
  expect((await probe(page)).items.map(i => i.uuid)).toEqual(initial.items.map(i => i.uuid))
  expect(requests).toEqual([1, 0])
  const detail = page.getByRole('region', { name: 'Détail du nœud', exact: true })
  await detail.getByLabel('Quantité', { exact: true }).fill('2')
  await detail.getByLabel('Quantité', { exact: true }).locator('..').locator('..').getByRole('button', { name: 'OK', exact: true }).click()
  await expect.poll(() => probe(page).then(p => p.items.filter(i => i.id === 3).length)).toBe(2)
  expect(requests).toEqual([1, 0])
  // Le serveur renvoie la variante physique sur la relecture suivant une action métier.
  physicalPart = 43
  await page.getByRole('treeitem', { selected: true }).getByRole('combobox', { name: 'Offre', includeHidden: true }).selectOption('101', { force: true })
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'loading')
  expect((await probe(page)).rootVisible).toBe(false)
  releaseReplacement()
  await expect.poll(() => probe(page).then(p => p.items.find(i => i.id === 3)?.width)).toBe(2)
  expect(requests).toEqual([1, 1])
  await page.getByRole('button', { name: 'Recentrer', exact: true }).click()
  await expect.poll(() => page.locator('.v3d-root').evaluate(el => (el as HTMLElement & { __viewer: Probe }).__viewer.diagnostics.animating)).toBe(false)
  const assembledCamera = await page.locator('.v3d-root').evaluate(el => (el as HTMLElement & { __viewer: Probe }).__viewer.camera.position.toArray())
  await page.screenshot({ path: testInfo.outputPath('synthetic-bound-assembled.png') })
  const assembled = (await probe(page)).items.map(i => i.position)
  await page.getByLabel('Éclatement', { exact: true }).focus()
  await page.getByLabel('Éclatement', { exact: true }).press('End')
  await expect.poll(() => probe(page).then(p => p.items.map(i => i.position))).not.toEqual(assembled)
  await expect(page.locator('.v3d-host')).toHaveAttribute('data-animation', 'idle')
  await page.screenshot({ path: testInfo.outputPath('synthetic-bound-exploded.png') })
  await page.getByRole('button', { name: 'Réinitialiser', exact: true }).click()
  await expect.poll(() => probe(page).then(p => p.items.map(i => i.position))).toEqual(assembled)
  await expect.poll(async () => {
    const at = await page.locator('.v3d-root').evaluate(el => (el as HTMLElement & { __viewer: Probe }).__viewer.camera.position.toArray())
    return Math.max(...at.map((v, i) => Math.abs(v - assembledCamera[i])))
  }).toBeLessThan(0.000001)
  await page.getByText('Composants et sources', { exact: true }).click()
  await page.getByLabel('Motorisation', { exact: true }).selectOption('thermal')
  await expect.poll(() => probe(page).then(p => p.items.filter(i => i.id !== 5).every(i => i.opacity === 0.12))).toBe(true)
  await page.getByLabel('Motorisation', { exact: true }).selectOption('electric')
  await expect.poll(() => probe(page).then(p => p.items.find(i => i.id === 5)?.opacity)).toBe(0.12)
  await page.getByLabel('Motorisation', { exact: true }).selectOption('hybrid')
  await expect.poll(() => probe(page).then(p => p.items.every(i => i.opacity === 1))).toBe(true)
  await page.getByText('Composants et sources', { exact: true }).click()
  await page.getByRole('button', { name: 'Recentrer', exact: true }).click()
  await expect(page.locator('.v3d-host')).toHaveAttribute('data-animation', 'idle')
  // Picking réel sur la partie basse visible de l'occurrence #4 : le centre peut être sous
  // les commandes sur mobile. Vérifier la cible DOM avant le clic, après le scroll.
  await page.locator('.v3d-canvas').scrollIntoViewIfNeeded()
  const point = await page.locator('.v3d-root').evaluate(el => (el as HTMLElement & { __viewer: Probe }).__viewer.screenPositionOf('node:4/instance:a'))
  const box = (await page.locator('.v3d-canvas').boundingBox())!
  const target = { x: box.x + point.x, y: box.y + point.y + 20 }
  expect(await page.evaluate(({ x, y }) => document.elementFromPoint(x, y)?.classList.contains('v3d-canvas'), target)).toBe(true)
  await page.mouse.click(target.x, target.y)
  await expect(page).toHaveURL(/node=4/)
  physicalPart = 99 // aucune variante connue : l'ancienne géométrie doit disparaître
  await page.evaluate(() => { location.hash = '#/boms/1/structurer?tab=arbre-3d&node=3' })
  await page.getByRole('treeitem', { selected: true }).getByRole('combobox', { name: 'Offre', includeHidden: true }).selectOption('102', { force: true })
  await expect.poll(() => probe(page).then(p => p.items.some(i => i.id === 3))).toBe(false)
  await expect(page.getByText('Modèle 3D de cette pièce indisponible.', { exact: false })).toBeVisible()
  expect(errors).toEqual([])
})

test('import : portée refusée, empreinte incorrecte explicite, aucune substitution générique', async ({ page }) => {
  const f = fixture()
  await page.route('**/models/manifests/*.json', route => route.fulfill({ status: 404 }))
  await page.goto('/vehicle3d-demo.html?fixture=multipla&mode=bound')
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-status', 'error', { timeout: 30_000 })
  await page.getByText('Composants et sources', { exact: true }).click()
  const input = page.getByLabel('Manifeste d’assemblage', { exact: true })
  await input.setInputFiles({ name: 'wrong.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify({ ...f.manifest, scope: { kind: 'draft', id: 99 } })) })
  await expect(page.getByRole('alert').filter({ hasText: 'autre BOM' })).toBeVisible()
  const nodes = (await import('../src/features/vehicle3d/demo/multipla-nodes.json', { with: { type: 'json' } })).default
  const physical = nodes.nodes.find(n => n.part_id !== null)!
  const manifest = { ...f.manifest, scope: { kind: 'draft', id: nodes.bom_id }, assets: [f.manifest.assets[0]], bindings: [{ ...f.manifest.bindings[0], node_id: physical.id, parent_id: physical.parent_id, part_id: physical.part_id }] }
  await page.route('**/models/box0.glb', route => route.fulfill({ body: f.buffers[1], contentType: 'model/gltf-binary' }))
  await input.setInputFiles({ name: 'hash.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(manifest)) })
  await ready(page)
  expect((await probe(page)).items).toEqual([])
  await page.getByText('Occurrences indisponibles', { exact: false }).click()
  await expect(page.getByText('Empreinte incorrecte', { exact: false })).toBeVisible()
  await expect(page.locator('.v3d-root')).toHaveAttribute('data-mode', 'bound')
})

test('BOM complète : diagnostics différés et limités à 100 lignes à la demande', async ({ page }) => {
  const nodes = [{ id: 1, parent_id: null, part_id: null, name: 'BOM volumineuse', quantity: '1' },
    ...Array.from({ length: 20_000 }, (_, i) => ({ id: i + 2, parent_id: 1, part_id: 40, name: 'Occurrence ' + (i + 2), quantity: '1' }))]
  await page.route('**/api/v1/boms', route => route.fulfill({ json: [{ id: 77, name: 'BOM volumineuse' }] }))
  await page.route('**/api/v1/boms/77/tree', route => route.fulfill({ json: { nodes } }))
  await page.route('**/models/manifests/draft-77.json', route => route.fulfill({ json: {
    schema_version: 1, revision: 'large-test', scope: { kind: 'draft', id: 77 }, vehicle_label: 'Fixture volumineuse', assets: [], bindings: [], assemblies: [],
  } }))
  await page.goto('/vehicle3d-demo.html?bom=77&mode=bound')
  await ready(page)
  const settings = page.locator('.v3d-settings')
  await expect(settings.locator('li')).toHaveCount(0)
  await page.getByText('Composants et sources', { exact: true }).click()
  await expect(settings.locator('li')).toHaveCount(0)
  await page.getByText('Occurrences indisponibles (20000)', { exact: true }).click()
  await expect(settings.locator('li')).toHaveCount(100)
  await page.getByRole('button', { name: 'Afficher 100 suivantes', exact: true }).click()
  await expect(settings.locator('li')).toHaveCount(200)
})

test('extérieur prioritaire : intérieur révélé et cliquable à travers la coque transparente', async ({ page }) => {
  const f = fixture(), shell = boxGLB(6)
  const manifest = { ...f.manifest, scope: { kind: 'draft', id: 80 }, assets: [
    ...f.manifest.assets.map(a => ({ ...a, display_role: 'interior' })),
    { ...f.manifest.assets[0], key: 'shell', path: '/models/shell.glb', sha256: createHash('sha256').update(shell).digest('hex'), display_role: 'exterior' },
  ], bindings: [{ ...f.manifest.bindings[0], node_id: 1, parent_id: null, part_id: null, asset_key: 'shell', system: 'body', instances: [
    { key: 'shell', position: [0, 0, 0], rotation: [0, 0, 0], scale: [1, 1, 1], explode_vector: [0, 2, 0] },
  ] }, ...f.manifest.bindings] }
  await page.route('**/api/v1/boms', route => route.fulfill({ json: [{ id: 80, name: 'Essai coque' }] }))
  await page.route('**/api/v1/boms/80/tree', route => route.fulfill({ json: { nodes: f.nodes } }))
  await page.route('**/models/manifests/draft-80.json', route => route.fulfill({ json: manifest }))
  await page.route('**/models/box*.glb', route => route.fulfill({ body: f.buffers[Number(route.request().url().match(/box(\d)/)![1])], contentType: 'model/gltf-binary' }))
  await page.route('**/models/shell.glb', route => route.fulfill({ body: shell, contentType: 'model/gltf-binary' }))
  await page.goto('/vehicle3d-demo.html?bom=80&mode=bound')
  await ready(page)
  const visible = () => page.locator('.v3d-root').evaluate(el => {
    const v = (el as HTMLElement & { __viewer: { built: { items: { obj: { nodeId: number }; node: { visible: boolean } }[] } } }).__viewer
    return v.built.items.filter(i => i.node.visible).map(i => i.obj.nodeId)
  })
  await expect.poll(visible).toEqual([1])
  await page.getByLabel('Carrosserie transparente', { exact: true }).check()
  await expect.poll(visible).toEqual([1, 3, 4, 5])
  const point = await page.locator('.v3d-root').evaluate(el => (el as HTMLElement & { __viewer: Probe }).__viewer.screenPositionOf('node:3/instance:a'))
  const box = (await page.locator('.v3d-canvas').boundingBox())!
  await page.mouse.click(box.x + point.x, box.y + point.y)
  await expect(page.locator('.demo-sel')).toContainText('#3')
  await page.getByRole('button', { name: 'Réinitialiser', exact: true }).click()
  await expect.poll(visible).toEqual([1])
  // An absent exterior must expose the available illustrative interior, with an explicit notice.
  await page.route('**/models/shell.glb', route => route.fulfill({ status: 404 }))
  await page.reload()
  await ready(page)
  await expect.poll(visible).toEqual([3, 4, 5])
  await expect(page.getByText('Vue mécanique illustrative — aucun extérieur chargé.', { exact: true })).toBeVisible()
})
