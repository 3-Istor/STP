// Exhaustive, read-only audit against the actual API, production resolver and loaded Three.js scene.
// No BOM mutation. Run from repository root with the existing Playwright browser installation.
import { chromium } from '../../../web/node_modules/playwright-core/index.mjs'
import { readFile, writeFile, mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { createHash } from 'node:crypto'
import { gzipSync } from 'node:zlib'

const base = process.env.STP_3D_URL ?? 'http://127.0.0.1:41769'
if (!['127.0.0.1', 'localhost'].includes(new URL(base).hostname)) throw new Error('Local audit only')
const directory = fileURLToPath(new URL('../', import.meta.url))
const layouts = JSON.parse(await readFile(directory + 'soutenance-assembly-layout.json'))
const identities = JSON.parse(await readFile(directory + 'soutenance-source-parts.json'))
const sourceParts = new Map(identities.map(p => [p.source_part_key, p.part_id]))
const browser = await chromium.launch({ args: ['--no-sandbox'] })
const page = await browser.newPage({ viewport: { width: 1500, height: 1000 } })
const errors = [], legacyRequests = []
page.on('pageerror', e => errors.push(e.message))
page.on('request', r => { if (r.url().includes('/models/CarConcept')) legacyRequests.push(r.url()) })
const reports = []
try {
  const response = await page.request.get(base + '/api/v1/parts')
  if (!response.ok()) throw new Error('Catalogue HTTP ' + response.status())
  const catalog = await response.json(), parts = new Map(catalog.map(p => [p.id, p]))
  for (const p of identities) if (parts.get(p.part_id)?.name !== p.catalog_name) throw new Error('Source identity no longer valid: part ' + p.part_id)
  for (const [name, layout] of Object.entries(layouts)) {
    process.stdout.write(name + ': loading actual application\n')
    const treeResponse = await page.request.get(base + '/api/v1/boms/' + layout.bom_id + '/tree')
    if (!treeResponse.ok()) throw new Error('Tree HTTP ' + treeResponse.status())
    const tree = await treeResponse.json()
    const manifestText = await (await page.request.get(base + '/models/manifests/draft-' + layout.bom_id + '.json')).text()
    const manifest = JSON.parse(manifestText)
    const target = '/boms/' + layout.bom_id + '/structurer?tab=3d'
    if (reports.length) await page.evaluate(hash => { location.hash = hash }, target)
    else await page.goto(base + '/#' + target)
    await page.locator('.v3d-root[data-status=ready][data-mode=bound]').waitFor({ timeout: 120_000 })
    if (await page.getByRole('radio', { name: 'Réaliste', exact: true }).count()) throw new Error('Legacy control still exposed')
    await page.emulateMedia({ reducedMotion: 'reduce' })
    const nodes = tree.nodes.map(n => ({ ...n, name: n.part_id === null ? n.name : (parts.get(n.part_id)?.name ?? n.name) }))
    const sourceGeometry = layout.interior.filter(x => sourceParts.has(x.source_part_key)).map(x => ({ ...x, source_part_id: sourceParts.get(x.source_part_key) }))
    const audit = await page.locator('.v3d-root').evaluate(async (element, data) => {
      const { sceneSelection, locateContainingAssembly } = await import('/src/features/vehicle3d/selection.ts')
      const { parseManifest, resolveManifest } = await import('/src/features/vehicle3d/manifest.ts')
      const v = element.__viewer, index = v.built.index, byId = new Map(data.nodes.map(n => [n.id, n]))
      const manifest = parseManifest(data.manifest)
      const resolution = resolveManifest(manifest, { kind: 'draft', id: data.bom }, data.nodes)
      const items = v.built.items, byNode = new Map()
      for (const item of items) { const group = byNode.get(item.obj.nodeId) ?? []; group.push(item); byNode.set(item.obj.nodeId, group) }
      v.setFocus(null); v.setIsolatedKeys(null); v.setIsolatedSystem(null); v.setPropulsion('all')
      v.setShellTransparency(false); v.setExplode(0); v.setSelection([], false)
      const visible = item => item.node.visible && v.built.root.visible && item.meshes.some(mesh => mesh.visible &&
        (Array.isArray(mesh.material) ? mesh.material : [mesh.material]).some(m => m.visible && m.opacity > 0))
      const normal = new Map(items.map(i => [i.obj.key, visible(i)]))
      v.setExplode(1)
      const exploded = new Map(items.map(i => [i.obj.key, visible(i)]))
      v.setExplode(0)
      const geometry = []
      for (const [id, group] of byNode) {
        const keys = group.map(i => i.obj.key)
        v.setIsolatedKeys(keys); v.setSelection(keys, false)
        v.frameDestination(group, null, false)
        v.camera.updateMatrixWorld(true); v.built.root.updateMatrixWorld(true)
        const rect = v.renderer.domElement.getBoundingClientRect()
        let pickable = false
        for (const item of group) {
          for (const mesh of item.meshes) {
            if (!mesh.visible) continue
            const p = mesh.geometry.attributes.position, ix = mesh.geometry.index
            const count = ix?.count ?? p.count
            for (const fraction of [.5, .1, .9, .25, .75, 0]) {
              const offset = Math.min(Math.floor((count / 3 - 1) * fraction) * 3, count - 3)
              const point = v.camera.position.clone().set(0, 0, 0)
              for (let j = 0; j < 3; j++) {
                const vertex = ix ? ix.getX(offset + j) : offset + j
                point.x += p.getX(vertex) / 3; point.y += p.getY(vertex) / 3; point.z += p.getZ(vertex) / 3
              }
              point.applyMatrix4(mesh.matrixWorld).project(v.camera)
              if (Math.abs(point.x) > 1 || Math.abs(point.y) > 1 || Math.abs(point.z) > 1) continue
              const hit = v.pick({ clientX: rect.x + (point.x + 1) * rect.width / 2, clientY: rect.y + (1 - point.y) * rect.height / 2 })
              if (hit?.nodeId === id) { pickable = true; break }
            }
            if (pickable) break
          }
          if (pickable) break
        }
        geometry.push({ id, keys, meshCount: group.reduce((n, i) => n + i.meshes.length, 0),
          realGeometry: group.some(i => i.meshes.some(m => m.geometry.attributes.position.count >= 3)),
          normal: group.some(i => normal.get(i.obj.key)), selected: group.some(visible),
          exploded: group.some(i => exploded.get(i.obj.key)), pickable })
      }
      v.setIsolatedKeys(null); v.setSelection([], false); v.recenter(false)
      const geometryByNode = new Map(geometry.map(g => [g.id, g]))
      const records = data.nodes.map(node => {
        const own = geometryByNode.get(node.id), selected = sceneSelection(index, node)
        const locator = selected.unavailable ? locateContainingAssembly(index, node, byId) : null
        const binding = manifest.bindings.filter(b => b.node_id === node.id)
        const reason = v.built.manifestInfo.unavailable.get(node.id) ?? resolution.unavailable.get(node.id) ?? null
        const path = [], seen = new Set(); let parent = byId.get(node.parent_id)
        while (parent && !seen.has(parent.id)) { seen.add(parent.id); path.unshift({ id: parent.id, name: parent.name }); parent = byId.get(parent.parent_id) }
        const source = node.part_id === null ? [] : data.sourceGeometry.filter(x => x.source_part_id === node.part_id)
        const otherBinding = node.part_id === null ? [] : manifest.bindings.filter(b => b.part_id === node.part_id)
        let cause = own ? own.normal ? 'visible' : 'present_masked' : node.part_id === null && selected.keys.length ? 'aggregate' :
          reason === 'part_changed' ? 'occurrence_part' : reason === 'parent_changed' ? 'occurrence_parent' :
          reason && !['no_binding', 'invalid_quantity', 'positions_missing'].includes(reason) ? 'loading' :
          reason === 'invalid_quantity' || reason === 'positions_missing' ? 'positions' :
          source.length || otherBinding.length ? 'mapping_unproven' : node.part_id === null ? 'virtual_no_geometry' : 'geometry_absent'
        if (own && !own.selected) cause = 'visibility_failure'
        if (own && !own.pickable) cause = 'picking_failure'
        return { id: node.id, parent_id: node.parent_id, part_id: node.part_id, name: node.name, path,
          binding: binding.map(b => ({ part_id: b.part_id, parent_id: b.parent_id, asset: b.asset_key, mesh_path: b.mesh_path })),
          own: own ?? null, keys: selected.keys, locator, reason, source_meshes: source.map(x => x.source_node), cause,
          coverage: own ? 'individual' : selected.keys.length || locator ? 'assembly_only' : 'none' }
      })
      return { records, geometry, resolution: { instances: resolution.instances.length, physicalNodes: resolution.physical_nodes },
        diagnostics: v.diagnostics, loadedAssets: v.built.manifestInfo.assets.map(a => a.key) }
    }, { nodes, manifest, bom: layout.bom_id, sourceGeometry })
    const counts = records => Object.fromEntries(['individual', 'assembly_only', 'none'].map(k => [k, records.filter(r => r.coverage === k).length]))
    const record = { name, bom: layout.bom_id, at: new Date().toISOString(), total: nodes.length,
      manifestSha256: createHash('sha256').update(manifestText).digest('hex'), counts: counts(audit.records),
      physicalCounts: counts(audit.records.filter(r => r.part_id !== null)),
      newLocators: audit.records.filter(r => r.locator).length, ...audit }
    reports.push(record)
    record.mappingRepairs = layout.quantified_occurrences?.map(c => c.node_id) ?? []
    const captures = directory + 'captures/corrections-finales/'
    await mkdir(captures, { recursive: true })
    await page.getByRole('button', { name: 'Recentrer', exact: true }).click()
    await page.locator('.v3d-host[data-animation=idle]').waitFor()
    await page.mouse.move(1, 1)
    await page.screenshot({ path: captures + name + '-3d.png' })
    await page.getByRole('radio', { name: 'Arbre + 3D', exact: true }).click()
    await page.getByRole('button', { name: 'Recentrer', exact: true }).click()
    await page.locator('.v3d-host[data-animation=idle]').waitFor()
    await page.screenshot({ path: captures + name + '-arbre-3d.png' })
    await page.reload()
    await page.locator('.v3d-root[data-status=ready][data-mode=bound]').waitFor({ timeout: 120_000 })
    if (await page.getByRole('radio', { name: 'Réaliste', exact: true }).count()) throw new Error('Legacy control returned after reload')
    record.navigation = { view3d: true, viewTree3d: true, reloadedBound: true, switchedUsingRouter: reports.length > 1 }
    process.stdout.write(name + ': ' + JSON.stringify({ total: record.total, counts: record.counts, physical: record.physicalCounts,
      locators: record.newLocators, failedPicking: record.geometry.filter(g => !g.pickable).map(g => g.id) }) + '\n')
  }
} finally { await browser.close() }
if (errors.length || legacyRequests.length) throw new Error(JSON.stringify({ errors, legacyRequests }))
const description = {
  visible: 'Visible en vue normale', present_masked: 'Présent mais masqué volontairement en vue normale ; sélection/éclaté le révèle',
  aggregate: 'Sous-ensemble virtuel représenté par ses descendants ; aucune géométrie individuelle revendiquée',
  occurrence_part: 'Identité de pièce différente du manifeste', occurrence_parent: 'Parent différent du placement vérifié',
  loading: 'Erreur de chargement/mesh : ', positions: 'Quantité ou emplacements non représentables',
  mapping_unproven: 'Mapping individuel non prouvé : géométrie du type disponible, occurrence/placement non attestés',
  virtual_no_geometry: 'Sous-ensemble virtuel sans géométrie propre ni descendant représenté',
  geometry_absent: 'Géométrie individuelle absente du registre disponible ; aucune liaison attestée',
  visibility_failure: 'Géométrie chargée mais invisible après sélection', picking_failure: 'Raycast de l’occurrence non obtenu après isolation',
}
const escape = v => String(v ?? '—').replaceAll('|', '/').replaceAll('\n', ' ')
const yes = value => value ? 'oui' : 'non'
let md = '# Composants non visibles — audit exhaustif automatisé\n\n'
md += 'Audit en lecture seule de l’API réelle et du viewer Three.js. Aucun rapprochement runtime par nom. '
md += 'Chaque ligne est une occurrence BOM, y compris les groupes virtuels ; les quantités ne sont pas multipliées en faux IDs.\n\n'
md += 'Les géométries restent celles fournies : extérieur détaillé et mécanique illustrative, sans certification OEM. '
md += 'Le picking est vérifié par le vrai raycaster, après isolation et cadrage de chaque occurrence chargée ; '
md += 'cela ne prétend pas rendre cliquable une pièce occultée dans toute vue globale. La sélection BOM est vérifiée par la fonction de production pour chaque ID ; '
md += 'les gestes UI sont validés séparément dans le parcours navigateur. Les colonnes de visibilité lisent les états du renderer, '
md += 'sans prétendre détecter toutes les occultations dans la voiture complète. Le contrôle effectif par raycast est effectué après isolation/cadrage ; '
md += 'un organe enfermé peut nécessiter « Isoler la sélection » pour voir sa surface.\n\n'
md += 'Un sous-ensemble localisable ne constitue pas un modèle individuel de la pièce. Le bouton explicite conserve son ID BOM. '
md += 'La racine du véhicule n’est jamais proposée comme substitut de localisation.\n\n'
md += '| Voiture | Occurrences totales | Individuelles | Sous-ensemble seulement | Sans représentation/localisation | Localisations ajoutées | Liaisons réparées | Erreurs chargement/identité/visibilité restantes |\n|---|---:|---:|---:|---:|---:|---:|---:|\n'
for (const r of reports) md += `| ${r.name} | ${r.total} | ${r.counts.individual} | ${r.counts.assembly_only} | ${r.counts.none} | ${r.newLocators} | ${r.mappingRepairs.length} | ${r.records.filter(x => ['occurrence_part','occurrence_parent','loading','visibility_failure','picking_failure'].includes(x.cause)).length} |\n`
md += '\n« Localisations ajoutées » compte les occurrences auparavant sans action de localisation, pas des meshes créés. '
md += 'Le retrait du mode Réaliste est une correction globale supplémentaire. Les nœuds sans géométrie propre restent nommés dans les tableaux exhaustifs ci-dessous.\n\n'
md += 'Corrections de mapping : Fiat #8 (référence #60, deux renforts) et #12932 (référence #104, deux arbres à cames). '
md += 'Les meshes fournis sont des copies translatées vérifiées à 1e-6 près ; les deux placements préexistants sont conservés via les instances du manifeste. '
md += 'Aucune géométrie nouvelle créée. Les quatre fichiers d’origine et les trois autres copies préparées sont inchangés.\n\n'
md += 'Cas volontairement non individualisés : soupapes Fiat (quantités/types du kit divergents), cylindres Valhalla (numérotation non attestée), '
md += 'modules HV Valhalla (15 positions dans le kit contre 12 dans la BOM), radiateurs avant/latéraux partageant une clé source et tuyauteries sans parcours vérifié. '
md += 'Ils restent dans la catégorie « mapping individuel non prouvé » ou « géométrie absente », avec localisation du groupe lorsque possible.\n\n'
for (const r of reports) {
  md += `## ${r.name} — BOM ${r.bom}\n\nDate : ${r.at}. SHA-256 manifeste : \`${r.manifestSha256}\`.\n\n`
  md += `Pièces physiques uniquement : ${r.physicalCounts.individual} individuelles, ${r.physicalCounts.assembly_only} localisables par sous-ensemble, ${r.physicalCounts.none} sans localisation.\n\n`
  const causes = new Map()
  for (const row of r.records) causes.set(row.cause, (causes.get(row.cause) ?? 0) + 1)
  md += '| Cause / état | Nombre |\n|---|---:|\n'
  for (const [cause, count] of causes) md += `| ${description[cause]} | ${count} |\n`
  md += '\n### Toutes les occurrences et les problèmes restants\n\n'
  md += 'Normale/sélection/éclaté décrivent la visibilité du mesh de cette occurrence, pas de celui d’un ancêtre. '
  md += '« BOM » donne les clés sélectionnables ou la localisation explicite ; « 3D » concerne le picking de cet ID exact. '
  md += 'Un groupe agrégé peut être sélectionné depuis la BOM sans posséder une surface portant son propre ID.\n\n'
  md += '| ID / parent | Référence pièce | Nom | Sous-ensemble direct | Manifeste / mesh | Géométrie propre | Normale / sélection / éclaté | BOM → 3D | 3D → cet ID | État / cause exacte |\n|---|---|---|---|---|---|---|---|---|---|\n'
  for (const row of r.records) {
    const parent = row.path.at(-1)
    const binding = row.binding.length ? row.binding.map(b => b.asset + ':' + b.mesh_path.join('/')).join('; ') : 'aucune liaison individuelle'
    const visibility = row.own ? [row.own.normal, row.own.selected, row.own.exploded].map(yes).join(' / ') : 'non / non / non'
    const bom = row.own ? 'individuelle' : row.keys.length ? 'descendants (' + row.keys.length + ')' : row.locator ? 'localiser #' + row.locator.nodeId + ' uniquement' : 'sans modèle ni localisation fiable'
    const cause = description[row.cause] + (row.reason ? ' [' + row.reason + ']' : '') + (row.locator ? ' ; sous-ensemble localisable #' + row.locator.nodeId : '')
    md += '| ' + [row.id + ' / ' + (row.parent_id ?? 'racine'), row.part_id ?? 'groupe virtuel', row.name,
      parent ? '#' + parent.id + ' ' + parent.name : 'racine', binding, row.own?.realGeometry ? row.own.meshCount + ' mesh(es)' : 'non',
      visibility, bom, row.own ? yes(row.own.pickable) : 'non', cause].map(escape).join(' | ') + ' |\n'
  }
}
await mkdir(directory + 'audit-final', { recursive: true })
await writeFile(directory + 'COMPOSANTS_NON_VISIBLES.md', md)
await writeFile(directory + 'audit-final/summary.json', JSON.stringify({ at: new Date().toISOString(), base, errors, legacyRequests,
  vehicles: reports.map(({ records, ...r }) => r) }, null, 2) + '\n')
for (const r of reports) await writeFile(directory + 'audit-final/' + r.name + '-occurrences.json.gz', gzipSync(JSON.stringify(r.records) + '\n'))
process.stdout.write('Audit complete: ' + reports.reduce((n, r) => n + r.total, 0) + ' distinct occurrences; exhaustive Markdown generated\n')
