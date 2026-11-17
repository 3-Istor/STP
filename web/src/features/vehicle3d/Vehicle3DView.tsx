// Composant React de la vue 3D (ADR 0035), chargé à la demande avec `three` par index.tsx.
import { useEffect, useMemo, useRef, useState } from 'react'
import { budgetColor, DEFAULT_STATUS, resolveColorMode, systemColor, UNAVAILABLE, type StatusPalette } from './colors.ts'
import { buildVehicleIndex, objectsForNode, type VehicleObject } from './mapping.ts'
import { loadRealistic } from './realistic.ts'
import { slotTable } from './tables.ts'
import type { Vehicle3DProps } from './types.ts'
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

type Status = { state: 'loading' } | { state: 'ready'; result: LoadResult } | { state: 'error'; message: string }

export default function Vehicle3DView({ bomId, nodes, selectedNodeId, onSelectNode, mode, colorBy, explode }: Vehicle3DProps) {
  const rootRef = useRef<HTMLDivElement>(null)
  const hostRef = useRef<HTMLDivElement>(null)
  const pickedRef = useRef<number | null>(null) // sélection venue d'un clic dans la 3D : pas de recadrage
  const viewerRef = useRef<Viewer | null>(null)
  const onSelectRef = useRef(onSelectNode)
  const [status, setStatus] = useState<Status>({ state: 'loading' })
  const [hover, setHover] = useState<{ obj: VehicleObject; x: number; y: number } | null>(null)
  const [focus, setFocus] = useState<VehicleObject | null>(null)
  const [palette, setPalette] = useState<StatusPalette>(DEFAULT_STATUS)
  const index = useMemo(() => buildVehicleIndex(nodes, slotTable), [nodes])
  const color = useMemo(() => resolveColorMode(colorBy, nodes), [colorBy, nodes])

  useEffect(() => { onSelectRef.current = onSelectNode }, [onSelectNode])

  // Création et libération du moteur (canvas, contexte WebGL, géométries, textures, écouteurs).
  useEffect(() => {
    const p = readPalette(rootRef.current!)
    setPalette(p)
    const viewer = new Viewer(hostRef.current!, slotTable, p, {
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
    viewerRef.current = viewer
    // Développement seulement : accès au moteur pour les captures et tests Playwright (absent du build).
    if (import.meta.env.DEV) Object.assign(rootRef.current!, { __viewer: viewer })
    const onKey = (ev: KeyboardEvent) => { if (ev.key === 'Escape' && viewer.focused) viewer.setFocus(null) }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
      viewer.dispose()
      viewerRef.current = null
    }
  }, [])

  // (Re)construction de la scène : changement de BOM, de nœuds ou de rendu.
  useEffect(() => {
    const viewer = viewerRef.current
    if (!viewer) return
    let live = true
    setStatus({ state: 'loading' })
    setFocus(null)
    viewer.load(mode, index, loadRealistic).then(
      result => { if (live) setStatus({ state: 'ready', result }) },
      err => { if (live) setStatus({ state: 'error', message: err instanceof Error ? err.message : String(err) }) },
    )
    return () => { live = false }
  }, [bomId, mode, index])

  const ready = status.state === 'ready'
  useEffect(() => { if (ready) viewerRef.current?.setColors(color.mode, palette) }, [ready, color.mode, palette])
  useEffect(() => { if (ready) viewerRef.current?.setExplode(explode) }, [ready, explode])
  useEffect(() => {
    if (!ready) return
    const fromClick = selectedNodeId !== null && selectedNodeId === pickedRef.current
    pickedRef.current = null
    viewerRef.current?.setSelection(objectsForNode(index, selectedNodeId), !fromClick)
  }, [ready, index, selectedNodeId])

  const result = status.state === 'ready' ? status.result : null
  const estimated = result?.mode === 'simple' ? index.objects.filter(o => o.estimated).length : 0
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
        {status.state === 'loading' ? <span className="v3d-badge" role="status">Chargement…</span> : null}
        {status.state === 'error' ? <span className="v3d-notice" role="alert">Vue 3D indisponible : {status.message}</span> : null}
        {result?.fallback ? <span className="v3d-notice" role="alert">{result.fallback}</span> : null}
        {focus ? <span className="v3d-badge">Éclatement dans l’axe · le reste du véhicule en transparence · Échap pour sortir</span> : null}
      </div>
      <div className="v3d-overlay v3d-top-right">
        <button type="button" className="v3d-button" onClick={() => viewerRef.current?.recenter()}>Recentrer</button>
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
              <span><i style={{ background: UNAVAILABLE }} />indisponible</span>
            </>
          ) : index.systems.map(s => (
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
