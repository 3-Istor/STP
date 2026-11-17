// Composant React de la vue 3D (ADR 0035), chargé à la demande avec `three` par index.tsx.
import { useEffect, useMemo, useRef, useState } from 'react'
import { budgetColor, DEFAULT_STATUS, resolveColorMode, systemColor, UNAVAILABLE, type StatusPalette } from './colors.ts'
import { buildVehicleIndex, type VehicleObject } from './mapping.ts'
import { sceneSelection } from './selection.ts'
import { loadRealistic } from './realistic.ts'
import { slotTable } from './tables.ts'
import type { PropulsionMode, Vehicle3DProps } from './types.ts'
import { BoundAssetStore, buildBoundScene } from './boundModel.ts'
import { parseManifest, resolveManifest, type SceneManifest } from './manifest.ts'
import { staticManifestSource } from './manifestSource.ts'
import { Viewer, type LoadResult } from './viewer.ts'
import './vehicle3d.css'

function readPalette(el: HTMLElement): StatusPalette {
  const css = getComputedStyle(el)
  const get = (name: string, fallback: string) => {
    const v = css.getPropertyValue(name).trim()
    return /^#[0-9a-f]{3}([0-9a-f]{3})?$/i.test(v) ? v : fallback
  }
  return { ok: get('--ok', DEFAULT_STATUS.ok), warn: get('--warn', DEFAULT_STATUS.warn), over: get('--over', DEFAULT_STATUS.over) }
}

/** Fond de la scène : variable --v3d-bg du thème courant, sinon le gris clair d'origine. */
function readBackground(el: HTMLElement): string {
  const v = getComputedStyle(el).getPropertyValue('--v3d-bg').trim()
  return /^#[0-9a-f]{6}$/i.test(v) ? v : '#eef0f3'
}

type Status = { state: 'loading' } | { state: 'ready'; result: LoadResult } | { state: 'error'; message: string }

export default function Vehicle3DView({ bomId, nodes, selectedNodeId, onSelectNode, mode, colorBy, explode, onResetView, sceneScope }: Vehicle3DProps) {
  const rootRef = useRef<HTMLDivElement>(null)
  const hostRef = useRef<HTMLDivElement>(null)
  const pickedRef = useRef<number | null>(null) // sélection venue d'un clic dans la 3D : pas de recadrage
  const viewerRef = useRef<Viewer | null>(null)
  const storeRef = useRef<BoundAssetStore | null>(null)
  const onSelectRef = useRef(onSelectNode)
  const [status, setStatus] = useState<Status>({ state: 'loading' })
  const [hover, setHover] = useState<{ obj: VehicleObject; x: number; y: number } | null>(null)
  const [focus, setFocus] = useState<VehicleObject | null>(null)
  const [palette, setPalette] = useState<StatusPalette>(DEFAULT_STATUS)
  const [transparent, setTransparent] = useState(false)
  const [revision, setRevision] = useState(0)
  const [imported, setImported] = useState<SceneManifest | null>(null)
  const [importError, setImportError] = useState<string | null>(null)
  const [propulsion, setPropulsion] = useState<PropulsionMode>('all')
  const [isolation, setIsolation] = useState<number | null>(null)
  const scopeKind = sceneScope?.kind ?? 'draft', scopeId = sceneScope?.id ?? bomId, scopeRevision = sceneScope?.revision
  const index = useMemo(() => buildVehicleIndex(nodes, slotTable), [nodes])
  const color = useMemo(() => resolveColorMode(colorBy, nodes), [colorBy, nodes])
  const result = status.state === 'ready' ? status.result : null
  const activeIndex = result?.index ?? index
  const info = result?.manifestInfo
  const missingKeys = useMemo(() => new Set(result?.missing.map(o => o.key) ?? []), [result])
  const selectedNode = nodes.find(n => n.id === selectedNodeId)
  const selection = useMemo(() => {
    const value = sceneSelection(activeIndex, selectedNode, missingKeys)
    if (mode !== 'bound' || isolation === null) return value
    return { ...value, keys: value.keys.filter(k => activeIndex.byKey.get(k)?.systemId === isolation) }
  }, [activeIndex, selectedNode, missingKeys, mode, isolation])

  useEffect(() => { onSelectRef.current = onSelectNode }, [onSelectNode])

  // Création et libération du moteur (canvas, contexte WebGL, géométries, textures, écouteurs).
  useEffect(() => {
    const p = readPalette(rootRef.current!)
    setPalette(p)
    let viewer: Viewer
    try {
      viewer = new Viewer(hostRef.current!, slotTable, p, {
        onPick: obj => {
          pickedRef.current = obj.nodeId
          onSelectRef.current(obj.nodeId)
        },
        onHover: (obj, x, y) => setHover(obj ? { obj, x, y } : null),
        onFocusChange: obj => {
          setFocus(obj)
          if (obj) {
            pickedRef.current = obj.nodeId
            onSelectRef.current(obj.nodeId)
          }
        },
      })
    } catch (error) {
      setStatus({ state: 'error', message: error instanceof Error ? error.message : String(error) })
      return
    }
    viewerRef.current = viewer
    const store = new BoundAssetStore()
    storeRef.current = store
    viewer.setBackground(readBackground(rootRef.current!))
    // Bascule de thème (data-theme sur <html>) : couleurs de statut et fond relus, sans reconstruire la scène.
    const themeObserver = new MutationObserver(() => {
      const root = rootRef.current
      if (!root) return
      setPalette(readPalette(root))
      viewer.setBackground(readBackground(root))
    })
    themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
    // Développement seulement : accès au moteur pour les captures et tests Playwright (absent du build).
    if (import.meta.env.DEV) Object.assign(rootRef.current!, { __viewer: viewer })
    const onKey = (ev: KeyboardEvent) => { if (ev.key === 'Escape' && viewer.focused) viewer.setFocus(null) }
    window.addEventListener('keydown', onKey)
    return () => {
      themeObserver.disconnect()
      window.removeEventListener('keydown', onKey)
      viewer.dispose()
      store.dispose()
      viewerRef.current = null
      storeRef.current = null
    }
  }, [revision])

  // (Re)construction de la scène : changement de BOM, de nœuds ou de rendu.
  useEffect(() => {
    const viewer = viewerRef.current
    if (!viewer) return
    let live = true
    const controller = new AbortController()
    const store = storeRef.current!
    setStatus({ state: 'loading' })
    setFocus(null)
    setHover(null)
    const loadBound = async () => {
      const scope = { kind: scopeKind, id: scopeId, revision: scopeRevision }
      const raw = imported ?? await staticManifestSource.load(scope, controller.signal)
      if (!raw) throw new Error('Aucun manifeste pour cette BOM. Importez un manifeste d’assemblage et ses modèles locaux.')
      return buildBoundScene(resolveManifest(parseManifest(raw), scope, nodes), nodes, store, controller.signal)
    }
    viewer.load(mode, index, loadRealistic, loadBound).then(
      result => { if (live) setStatus({ state: 'ready', result }) },
      err => { if (live) setStatus({ state: 'error', message: err instanceof Error ? err.message : String(err) }) },
    )
    return () => { live = false; controller.abort() }
  }, [bomId, mode, index, nodes, revision, imported, scopeKind, scopeId, scopeRevision])

  const ready = status.state === 'ready'
  useEffect(() => { if (ready) viewerRef.current?.setColors(color.mode, palette) }, [ready, color.mode, palette])
  useEffect(() => { if (ready) viewerRef.current?.setExplode(explode) }, [ready, explode])
  useEffect(() => { if (ready) viewerRef.current?.setShellTransparency(transparent) }, [ready, transparent])
  useEffect(() => { if (ready) viewerRef.current?.setPropulsion(propulsion) }, [ready, propulsion])
  useEffect(() => { if (ready) viewerRef.current?.setIsolatedSystem(isolation) }, [ready, isolation])
  useEffect(() => {
    if (!ready) return
    const fromClick = selectedNodeId !== null && selectedNodeId === pickedRef.current
    pickedRef.current = null
    const viewer = viewerRef.current
    if (viewer?.focused && !selection.keys.includes(viewer.focused)) viewer.setFocus(null)
    viewer?.setSelection(selection.keys, !fromClick)
  }, [ready, selection, selectedNodeId])

  const estimated = result?.mode === 'simple' ? activeIndex.objects.filter(o => o.estimated).length : 0
  const missing = result?.missing.length ?? 0

  return (
    <div ref={rootRef} className="v3d-root" data-status={status.state} data-mode={result?.mode ?? ''} data-focus={focus?.key ?? ''}
      aria-label="Vue 3D du véhicule">
      <div ref={hostRef} className="v3d-host" />
      <div className="v3d-overlay v3d-top-left">
        {focus ? (
          <div className="v3d-focusbar">
            <span className="v3d-crumbs">Véhicule › {focus.systemName} › <strong>{focus.name}</strong></span>
            <button type="button" className="v3d-button" onClick={() => viewerRef.current?.setFocus(null)}>Sortir du focus</button>
          </div>
        ) : null}
        {result?.mode === 'simple' ? (
          <span className="v3d-badge">Formes <strong>GÉNÉRÉES</strong> — pas la géométrie du véhicule</span>
        ) : null}
        {result?.mode === 'realistic' ? (
          <span className="v3d-badge"><strong>SIMILAIRE</strong> — pas le modèle exact du véhicule</span>
        ) : null}
        {info ? <span className="v3d-badge"><strong>{info.vehicleLabel}</strong> · {info.representedNodes}/{info.physicalNodes} occurrences physiques représentées</span> : null}
        {info ? <span className="v3d-badge">Géométrie : {info.assets.length === 0 ? 'aucun modèle chargé' : info.assets.some(a => a.geometry_status === 'schematic') ? 'schématique' : info.assets.some(a => a.geometry_status === 'approximation') ? 'approximative' : 'exactitude déclarée par l’auteur, à contrôler'} · {info.approximatePlacements} placement(s) approximatif(s)</span> : null}
        {status.state === 'loading' ? <span className="v3d-badge" role="status">Chargement…</span> : null}
        {status.state === 'error' ? <span className="v3d-notice" role="alert">Vue 3D indisponible : {status.message}</span> : null}
        {status.state === 'error' ? <button type="button" className="v3d-button" onClick={() => setRevision(r => r + 1)}>Réessayer la vue 3D</button> : null}
        {ready && selection.unavailable ? <span className="v3d-notice" role="status">
          {selectedNode?.part_id !== null ? 'Modèle 3D de cette pièce indisponible.' : 'Ce sous-ensemble est absent du modèle chargé.'}
          {' '}Aucun autre composant n’est sélectionné à sa place.
        </span> : null}
        {result?.fallback ? <span className="v3d-notice" role="alert">{result.fallback}</span> : null}
        {focus ? <span className="v3d-badge">Éclatement dans l’axe · le reste du véhicule en transparence · Échap pour sortir</span> : null}
      </div>
      <div className="v3d-overlay v3d-top-right">
        <button type="button" className="v3d-button" disabled={!ready} onClick={() => viewerRef.current?.recenter()}>Recentrer</button>
        <button type="button" className="v3d-button" disabled={!ready || selection.keys.length === 0} onClick={() => viewerRef.current?.frameSelection()}>Cadrer la sélection</button>
        <button type="button" className="v3d-button" disabled={!ready || selection.keys.length !== 1} onClick={() => viewerRef.current?.focusSelection()}>Focus sélection</button>
        {onResetView ? <button type="button" className="v3d-button" disabled={!ready} onClick={() => {
          setTransparent(false)
          setPropulsion('all')
          setIsolation(null)
          viewerRef.current?.setExplode(0)
          viewerRef.current?.setFocus(null)
          onResetView()
        }}>Réinitialiser</button> : null}
        <label className="v3d-transparency"><input type="checkbox" checked={transparent} disabled={!ready} onChange={e => setTransparent(e.target.checked)} />Carrosserie transparente</label>
        {mode === 'bound' ? <details className="v3d-settings">
          <summary>Composants et sources</summary>
          <label>Manifeste d’assemblage<input type="file" accept=".json,application/json" onChange={async e => {
            const file = e.target.files?.[0]
            if (!file) return
            try {
              if (file.size > 2_000_000) throw new Error('Manifeste supérieur à 2 Mo.')
              const manifest = parseManifest(JSON.parse(await file.text()))
              resolveManifest(manifest, { kind: scopeKind, id: scopeId, revision: scopeRevision }, nodes)
              setImported(manifest); setImportError(null)
            } catch (error) { setImportError(error instanceof Error ? error.message : String(error)) }
            e.target.value = ''
          }} /></label>
          {importError ? <p role="alert">{importError}</p> : null}
          {imported ? <button type="button" className="v3d-button" onClick={() => setImported(null)}>Recharger la source du projet</button> : null}
          <label>Motorisation<select aria-label="Motorisation" value={propulsion} disabled={!ready} onChange={e => {
            setPropulsion(e.target.value as PropulsionMode)
            if (e.target.value !== 'all') setTransparent(true)
          }}>
            <option value="all">Véhicule complet</option><option value="thermal">Thermique</option><option value="electric">Électrique</option><option value="hybrid">Hybride</option>
          </select></label>
          <p>Lecture des composants déclarés ; aucune simulation de fonctionnement ou de flux d’énergie.</p>
          <label>Isoler un système<select aria-label="Isoler un système" value={isolation ?? ''} disabled={!ready} onChange={e => setIsolation(e.target.value ? Number(e.target.value) : null)}>
            <option value="">Tous les systèmes</option>{activeIndex.systems.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select></label>
          {info ? <>
            <p>Révision : {info.revision}. Correspondances contrôlées sur l’arbre courant.</p>
            <details><summary>Occurrences indisponibles ({info.unavailable.size})</summary><ul>{[...info.unavailable].map(([id, reason]) => <li key={id}><button type="button" onClick={() => onSelectNode(id)}>#{id} {nodes.find(n => n.id === id)?.name}</button> : {({ no_binding: 'liaison absente', part_changed: 'variante physique sans modèle', parent_changed: 'placement du nouveau parent absent', invalid_quantity: 'quantité non représentable', positions_missing: 'positions d’instances insuffisantes' } as Record<string, string>)[reason] ?? reason}</li>)}</ul></details>
            <details><summary>Provenance et licences</summary><ul>{info.assets.map(a => <li key={a.key}><a href={a.source_url} target="_blank" rel="noreferrer">{a.key}</a> — {a.author}, <a href={a.license_url} target="_blank" rel="noreferrer">{a.license}</a> ({a.geometry_status}){a.geometry_evidence_url ? <> · <a href={a.geometry_evidence_url} target="_blank" rel="noreferrer">preuve déclarée</a></> : null}</li>)}</ul></details>
          </> : null}
        </details> : null}
      </div>
      <div className="v3d-overlay v3d-bottom-left">
        {color.notice ? <span className="v3d-notice">{color.notice}</span> : null}
        {estimated > 0 ? <span className="v3d-notice">{estimated} sous-ensemble(s) en position estimée (nom sans emplacement connu).</span> : null}
        {missing > 0 ? <span className="v3d-notice">{missing} sous-ensemble(s) sans équivalent dans ce modèle : non affichés.</span> : null}
        <div className="v3d-legend" aria-label="Légende des couleurs">
          {color.mode === 'budget' ? (
            <>
              <span><i style={{ background: budgetColor('ok', palette) }} />dans le budget</span>
              <span><i style={{ background: budgetColor('warn', palette) }} />proche</span>
              <span><i style={{ background: budgetColor('over', palette) }} />dépassé</span>
              <span><i style={{ background: UNAVAILABLE }} />sans budget ou indisponible</span>
            </>
          ) : color.mode === 'material' ? <span>Matériaux du modèle · aucune indication de budget</span> : activeIndex.systems.map(s => (
            <span key={s.id}><i style={{ background: systemColor(s.index) }} />{s.name}</span>
          ))}
        </div>
      </div>
      {result?.attribution ? (
        <div className="v3d-overlay v3d-bottom-right">
          <span className="v3d-attribution">Modèle : {result.attribution} (<a href="https://creativecommons.org/licenses/by/4.0/" target="_blank" rel="noreferrer">licence</a>)</span>
        </div>
      ) : null}
      {hover ? (
        <div className="v3d-tooltip" style={{ left: hover.x, top: hover.y }}>
          {hover.obj.name}{hover.obj.estimated ? <em> · position estimée</em> : null}
        </div>
      ) : null}
    </div>
  )
}
