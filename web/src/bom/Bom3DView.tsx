import { Canvas, useFrame, useThree, type ThreeEvent } from '@react-three/fiber'
import { Component, useCallback, useEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { api, errorMessage, type Assembly, type Generation, type Placement, type TreeNode, type Vec3 } from '../api.ts'
import { partName, type Catalog } from '../catalog.ts'
import { formatDecimal } from '../money.ts'
import { fitScale, useModel, type LoadedModel } from './models.ts'
import { explodedPosition, matchLabels, visiblePlacements, zoneColors } from './scene.ts'

type Props = {
  assembly: Assembly | null
  loading: boolean
  error: string | null
  nodes: TreeNode[]
  catalog: Catalog
  selectedID: number | null
  onSelect: (id: number) => void
  // Retour à l'arbre sur la pièce sélectionnée (synchronisation 3D → BOM).
  onShowInTree: () => void
  // Les modèles persistés ont changé (génération finie) : le parent relit la scène.
  onModelsChanged: () => void
}

// Caméra de départ : trois quarts avant, assez loin pour voir le gabarit entier ; cible au centre du véhicule.
const HOME = { position: new THREE.Vector3(6.5, 3.6, 6.5), target: new THREE.Vector3(0, 0.6, 0) }

// Bom3DView : la même BOM que l'arbre, rendue en 3D (ADR 0031). Chargée à la demande (import différé
// dans BomView). L'état partagé (pièce sélectionnée) vient du parent : les deux vues ne divergent jamais.
export default function Bom3DView(p: Props) {
  const { assembly, nodes, catalog, selectedID, onSelect } = p
  // Niveau d'éclatement demandé (0–1) ; la valeur animée vit dans une ref, hors du cycle React.
  const [target, setTarget] = useState(0)
  const levelRef = useRef(0)
  const [hiddenZones, setHiddenZones] = useState<ReadonlySet<string>>(new Set())
  const [hovered, setHovered] = useState<number | null>(null)
  const [homeKey, setHomeKey] = useState(0)
  // Sans WebGL (navigateur sans accélération, bac à sable), on le dit au lieu de laisser Three.js échouer.
  const webgl = useMemo(() => hasWebGL(), [])
  const colors = useMemo(() => (assembly ? zoneColors(assembly) : new Map<string, string>()), [assembly])
  const placements = useMemo(() => (assembly ? visiblePlacements(assembly, hiddenZones) : []), [assembly, hiddenZones])
  const byNode = useMemo(() => new Map((assembly?.placements ?? []).map((pl) => [pl.node_id, pl])), [assembly])
  const selectedPlacement = selectedID === null ? null : (byNode.get(selectedID) ?? null)
  const selectedNode = selectedID === null ? null : (nodes.find((n) => n.id === selectedID) ?? null)
  const label = (id: number) => {
    const n = nodes.find((x) => x.id === id)
    return n ? (n.part_id !== null ? partName(catalog, n.part_id) : n.name) : `nœud #${id}`
  }
  const generation = useGeneration(assembly, p.onModelsChanged)

  function toggleZone(zone: string) {
    setHiddenZones((prev) => {
      const next = new Set(prev)
      if (!next.delete(zone)) next.add(zone)
      return next
    })
  }

  return (
    <div className="viz3d" aria-label="Vue 3D de la BOM" role="region">
      <div className="viz3d-toolbar">
        <label className="viz3d-slider">
          Éclatement{' '}
          <input
            type="range"
            min={0}
            max={100}
            step={1}
            value={Math.round(target * 100)}
            aria-label="Niveau d'éclatement"
            onChange={(e) => setTarget(Number(e.target.value) / 100)}
          />
        </label>
        <button type="button" className="small" onClick={() => setTarget(target < 0.5 ? 1 : 0)}>
          {target < 0.5 ? 'Éclater' : 'Assembler'}
        </button>
        <button type="button" className="small" onClick={() => setHomeKey((k) => k + 1)}>
          Recentrer
        </button>
        {generation && <GenerationControls {...generation} />}
        {assembly && (
          <span className="viz3d-zones" role="group" aria-label="Zones affichées">
            {assembly.zones.map((z) => (
              <label key={z.zone} className="viz3d-zone" title={z.known ? `${z.count} pièce(s)` : `${z.count} pièce(s), zone inconnue du gabarit : posées à côté du véhicule`}>
                <input type="checkbox" checked={!hiddenZones.has(z.zone)} onChange={() => toggleZone(z.zone)} />
                <span className="viz3d-swatch" style={{ background: colors.get(z.zone) }} aria-hidden="true" />
                {z.zone || 'sans zone'}
                {!z.known && ' ?'}
              </label>
            ))}
          </span>
        )}
      </div>

      {p.error && <p role="alert" className="error viz3d-message">{p.error}</p>}
      {p.loading && !assembly && <p className="muted viz3d-message">Construction de la scène…</p>}

      {assembly && !webgl && (
        <p role="alert" className="error viz3d-message">
          WebGL indisponible dans ce navigateur : la vue 3D ne peut pas s'afficher ici. La vue BOM reste utilisable.
        </p>
      )}
      {assembly && webgl && (
        <SceneBoundary>
        <Canvas
          camera={{ position: HOME.position.toArray(), fov: 40, near: 0.05, far: 100 }}
          dpr={[1, 2]}
          onPointerMissed={() => setHovered(null)}
        >
          <color attach="background" args={['#f4f7fb']} />
          <ambientLight intensity={0.9} />
          <directionalLight position={[5, 8, 4]} intensity={1.4} />
          <directionalLight position={[-6, 3, -4]} intensity={0.5} />
          <gridHelper args={[12, 24, '#c9d3e0', '#e3e9f1']} />
          {/* Gabarit du véhicule, en filaire : donne l'échelle et dit où les pièces sont posées. */}
          <mesh position={[0, assembly.vehicle.height / 2, 0]}>
            <boxGeometry args={[assembly.vehicle.length, assembly.vehicle.height, assembly.vehicle.width]} />
            <meshBasicMaterial color="#9fb3cc" wireframe transparent opacity={0.35} />
          </mesh>
          <Explosion target={target} levelRef={levelRef} />
          <Controls homeKey={homeKey} focus={selectedPlacement} levelRef={levelRef} />
          <Parts
            placements={placements}
            colors={colors}
            selectedID={selectedID}
            hoveredID={hovered}
            levelRef={levelRef}
            onSelect={onSelect}
            onHover={setHovered}
          />
        </Canvas>
        </SceneBoundary>
      )}

      {hovered !== null && hovered !== selectedID && <p className="viz3d-hover muted small">{label(hovered)}</p>}
      {generation?.error && <p role="alert" className="error viz3d-message">{generation.error}</p>}

      {assembly && (
        <aside className="viz3d-legend small">
          {selectedPlacement ? (
            <>
              <strong>{label(selectedPlacement.node_id)}</strong>
              <span className="muted">
                {' '}
                ×{selectedPlacement.quantity} · {selectedPlacement.zone || 'sans zone'} · {selectedPlacement.category}
              </span>
              <br />
              <span className="badge">{matchLabels[selectedPlacement.match_type]}</span>{' '}
              <span className="muted">confiance {formatDecimal(selectedPlacement.confidence, 2)}</span>
              {selectedPlacement.estimated && <span className="muted"> · position estimée</span>}
              {selectedPlacement.model_url !== null && <span className="muted"> · fichier GLB</span>}
              {selectedPlacement.notes.map((n) => (
                <span key={n} className="muted viz3d-note">
                  {n}
                </span>
              ))}
              <br />
              <button type="button" className="link" onClick={p.onShowInTree}>
                Voir dans l'arbre
              </button>
            </>
          ) : selectedNode ? (
            <>
              <strong>{label(selectedNode.id)}</strong>
              <span className="muted"> — {selectedNode.part_id === null ? 'sous-ensemble, sans forme propre' : 'pièce sans modèle 3D'}</span>
              <br />
              <button type="button" className="link" onClick={p.onShowInTree}>
                Voir dans l'arbre
              </button>
            </>
          ) : (
            <span className="muted">Cliquez une pièce pour la sélectionner. Formes générées depuis la BOM : positions estimées.</span>
          )}
          {assembly.missing.length > 0 && (
            <details>
              <summary>
                {assembly.missing.length} pièce{assembly.missing.length > 1 ? 's' : ''} sans modèle 3D
              </summary>
              <ul className="viz3d-missing">
                {assembly.missing.map((m) => (
                  <li key={m.node_id}>
                    <button type="button" className="link" onClick={() => onSelect(m.node_id)}>
                      {label(m.node_id)}
                    </button>{' '}
                    <span className="muted">{m.reason}</span>
                  </li>
                ))}
              </ul>
            </details>
          )}
        </aside>
      )}
    </div>
  )
}

type GenerationState = {
  status: Generation | null
  busy: boolean
  error: string | null
  start: () => void
}

// useGeneration suit la génération des modèles : état initial venu de la scène, puis relecture de
// /3d/status toutes les secondes tant qu'elle est en attente ou en cours ; à la fin, le parent relit la
// scène (les modèles persistés y entrent). null quand le serveur n'a pas de stockage de modèles.
function useGeneration(assembly: Assembly | null, onModelsChanged: () => void): GenerationState | null {
  const initial = assembly?.generation ?? null
  const bomID = assembly?.bom_id ?? null
  // L'état local (relectures, lancement) est rattaché à la scène qui l'a amorcé : une nouvelle scène le remplace.
  const [local, setLocal] = useState<{ base: Generation | null; status: Generation | null }>({ base: initial, status: initial })
  const status = local.base === initial ? local.status : initial
  const setStatus = useCallback((g: Generation) => setLocal({ base: initial, status: g }), [initial])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const pending = status !== null && (status.status === 'queued' || status.status === 'running')
  useEffect(() => {
    if (!pending || bomID === null) return
    let cancelled = false
    const tick = async () => {
      try {
        const next = await api.generationStatus(bomID)
        if (cancelled) return
        setStatus(next)
        if (next.status !== 'queued' && next.status !== 'running') onModelsChanged()
      } catch (e: unknown) {
        if (!cancelled) setError(errorMessage(e))
      }
    }
    const id = setInterval(() => void tick(), 1000)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [pending, bomID, onModelsChanged, setStatus])
  if (initial === null || bomID === null) return null
  return {
    status,
    busy,
    error,
    start: () => {
      setBusy(true)
      setError(null)
      api
        .generateModels(bomID)
        .then((g) => setStatus(g))
        .catch((e: unknown) => setError(errorMessage(e)))
        .finally(() => setBusy(false))
    },
  }
}

// GenerationControls : le bouton « Générer les modèles », la progression et l'état (stale, échec).
function GenerationControls({ status, busy, start }: GenerationState) {
  const st = status?.status ?? 'none'
  const pending = st === 'queued' || st === 'running'
  const labelFor: Record<Generation['status'], string> = {
    none: 'Générer les modèles',
    queued: 'Génération en attente…',
    running: 'Génération en cours…',
    done: 'Régénérer les modèles',
    failed: 'Relancer la génération',
    stale: 'Régénérer les modèles',
  }
  return (
    <span className="viz3d-generation" role="group" aria-label="Génération des modèles">
      <button type="button" className="small" disabled={pending || busy} onClick={start}>
        {labelFor[st]}
      </button>
      {status && pending && (
        <>
          <progress aria-label="Avancement de la génération" value={status.done} max={Math.max(1, status.total)} />
          <span className="muted">
            {status.done} / {status.total}
          </span>
        </>
      )}
      {st === 'done' && status?.result && (
        <span className="muted" title={`${status.result.exact} exact(s), ${status.result.similar} similaire(s), ${status.result.generated} générée(s)`}>
          {status.done} modèle{status.done > 1 ? 's' : ''}
          {status.result.failed.length > 0 && `, ${status.result.failed.length} sans modèle`}
        </span>
      )}
      {st === 'stale' && <span className="badge warn">BOM modifiée depuis la génération</span>}
      {st === 'failed' && <span className="error">Échec : {status?.error}</span>}
    </span>
  )
}

function hasWebGL(): boolean {
  try {
    const c = document.createElement('canvas')
    return c.getContext('webgl2') !== null || c.getContext('webgl') !== null
  } catch {
    return false
  }
}

// SceneBoundary : une erreur de Three.js (contexte perdu, pilote) n'emporte pas toute l'application ;
// la vue BOM et le panneau restent utilisables.
class SceneBoundary extends Component<{ children: ReactNode }, { error: string | null }> {
  state: { error: string | null } = { error: null }
  static getDerivedStateFromError(e: unknown) {
    return { error: e instanceof Error ? e.message : String(e) }
  }
  render() {
    if (this.state.error !== null) {
      return (
        <p role="alert" className="error viz3d-message">
          La vue 3D n'a pas pu démarrer : {this.state.error}. La vue BOM reste utilisable.
        </p>
      )
    }
    return this.props.children
  }
}

// Explosion anime le niveau d'éclatement vers la valeur demandée (interpolation par image, hors React).
function Explosion({ target, levelRef }: { target: number; levelRef: RefObject<number> }) {
  useFrame((_, dt) => {
    const k = Math.min(1, dt * 6)
    levelRef.current += (target - levelRef.current) * k
    if (Math.abs(target - levelRef.current) < 0.001) levelRef.current = target
  })
  return null
}

// Controls : rotation 360°, zoom et déplacement (OrbitControls), recentrage, et cible qui suit la pièce
// sélectionnée (même quand elle s'éclate).
function Controls({ homeKey, focus, levelRef }: { homeKey: number; focus: Placement | null; levelRef: RefObject<number> }) {
  const { camera, gl } = useThree()
  const controls = useRef<OrbitControls | null>(null)
  const goal = useRef(HOME.target.clone())

  useEffect(() => {
    const c = new OrbitControls(camera, gl.domElement)
    c.enableDamping = true
    c.dampingFactor = 0.12
    c.maxDistance = 40
    c.minDistance = 0.3
    c.target.copy(HOME.target)
    controls.current = c
    return () => c.dispose()
  }, [camera, gl])

  useEffect(() => {
    if (homeKey === 0) return
    camera.position.copy(HOME.position)
    goal.current.copy(HOME.target)
    controls.current?.target.copy(HOME.target)
  }, [homeKey, camera])

  useFrame(() => {
    const c = controls.current
    if (!c) return
    if (focus) {
      const [x, y, z] = explodedPosition(focus, focus.instances[0].position, levelRef.current)
      goal.current.set(x, y, z)
    }
    c.target.lerp(goal.current, 0.12)
    c.update()
  })
  return null
}

type PartsProps = {
  placements: Placement[]
  colors: Map<string, string>
  selectedID: number | null
  hoveredID: number | null
  levelRef: RefObject<number>
  onSelect: (id: number) => void
  onHover: (id: number | null) => void
}

type Kind = Placement['shape']['kind']
type Slot = { placement: Placement; base: Vec3; rotation: Vec3 }
const KINDS: Kind[] = ['box', 'cylinder', 'wheel', 'sphere']

function slotsOf(placements: Placement[]): Slot[] {
  const out: Slot[] = []
  for (const pl of placements) for (const inst of pl.instances) out.push({ placement: pl, base: inst.position, rotation: inst.rotation })
  return out
}

// Parts : les pièces avec un fichier GLB sont regroupées par URL (un chargement, des instances) ; les
// autres, par primitive : un InstancedMesh par forme (quatre au plus), quel que soit le nombre de pièces.
function Parts(p: PartsProps) {
  const { primitives, models } = useMemo(() => {
    const byUrl = new Map<string, Placement[]>()
    const rest: Placement[] = []
    for (const pl of p.placements) {
      if (pl.model_url === null) rest.push(pl)
      else byUrl.set(pl.model_url, [...(byUrl.get(pl.model_url) ?? []), pl])
    }
    return { primitives: rest, models: [...byUrl.entries()].map(([url, pls]) => ({ url, slots: slotsOf(pls) })) }
  }, [p.placements])
  return (
    <>
      <PrimitiveParts {...p} placements={primitives} />
      {models.map((m) => (
        <ModelParts key={m.url} url={m.url} slots={m.slots} {...p} />
      ))}
    </>
  )
}

function PrimitiveParts(p: PartsProps) {
  const groups = useMemo(() => {
    const byKind: Record<Kind, Slot[]> = { box: [], cylinder: [], wheel: [], sphere: [] }
    for (const s of slotsOf(p.placements)) byKind[s.placement.shape.kind].push(s)
    return byKind
  }, [p.placements])
  return (
    <>
      {KINDS.filter((k) => groups[k].length > 0).map((k) => (
        <InstancedKind key={`${k}-${groups[k].length}`} kind={k} slots={groups[k]} {...p} />
      ))}
    </>
  )
}

// ModelParts : chargement progressif. Tant que le GLB n'est pas là (ou s'il échoue), les pièces gardent
// leur primitive ; une fois chargé, chaque maillage du modèle devient un InstancedMesh (une instance par
// occurrence), ajusté dans l'encombrement du placement.
function ModelParts({ url, slots, ...p }: PartsProps & { url: string; slots: Slot[] }) {
  const { model } = useModel(url)
  if (!model || model.meshes.length === 0) {
    return <PrimitiveParts {...p} placements={[...new Set(slots.map((s) => s.placement))]} />
  }
  return (
    <>
      {model.meshes.map((mesh, i) => (
        <InstancedModelMesh key={i} model={model} mesh={mesh.geometry} material={mesh.material} local={mesh.matrix} slots={slots} {...p} />
      ))}
    </>
  )
}

const scratchMatrix = new THREE.Matrix4()

function InstancedModelMesh({ model, mesh, material, local, slots, selectedID, levelRef, onSelect, onHover }: PartsProps & {
  model: LoadedModel
  mesh: THREE.BufferGeometry
  material: THREE.Material | THREE.Material[]
  local: THREE.Matrix4
  slots: Slot[]
}) {
  const ref = useRef<THREE.InstancedMesh>(null)
  const drawn = useRef<{ level: number; selected: number | null } | null>(null)
  useFrame(() => {
    const m = ref.current
    if (!m) return
    const level = levelRef.current
    if (drawn.current && drawn.current.level === level && drawn.current.selected === selectedID) return
    drawn.current = { level, selected: selectedID }
    for (let i = 0; i < slots.length; i++) {
      const s = slots[i]
      const [x, y, z] = explodedPosition(s.placement, s.base, level)
      scratch.position.set(x, y, z)
      scratch.rotation.set(s.rotation[0], s.rotation[1], s.rotation[2])
      const k = fitScale(model.extent, s.placement.shape.size) * (s.placement.node_id === selectedID ? 1.15 : 1)
      scratch.scale.set(k, k, k)
      scratch.updateMatrix()
      scratchMatrix.multiplyMatrices(scratch.matrix, local)
      m.setMatrixAt(i, scratchMatrix)
    }
    m.instanceMatrix.needsUpdate = true
  })
  const nodeAt = (e: ThreeEvent<PointerEvent | MouseEvent>) => (e.instanceId === undefined ? null : slots[e.instanceId].placement.node_id)
  return (
    <instancedMesh
      ref={ref}
      args={[mesh, material, slots.length]}
      frustumCulled={false}
      onClick={(e) => {
        e.stopPropagation()
        const id = nodeAt(e)
        if (id !== null) onSelect(id)
      }}
      onPointerMove={(e) => {
        e.stopPropagation()
        onHover(nodeAt(e))
        document.body.style.cursor = 'pointer'
      }}
      onPointerOut={() => {
        onHover(null)
        document.body.style.cursor = ''
      }}
    />
  )
}

function makeGeometry(kind: Kind): THREE.BufferGeometry {
  switch (kind) {
    case 'box':
      return new THREE.BoxGeometry(1, 1, 1)
    case 'cylinder':
      return new THREE.CylinderGeometry(0.5, 0.5, 1, 12)
    case 'wheel':
      return new THREE.CylinderGeometry(0.5, 0.5, 1, 32)
    case 'sphere':
      return new THREE.SphereGeometry(0.5, 8, 6)
  }
}

const SELECTED = '#245dc5'
const HOVER = '#6796ec'
const WHEEL = '#2f3640'
const scratch = new THREE.Object3D()
const scratchColor = new THREE.Color()

function InstancedKind({ kind, slots, colors, selectedID, hoveredID, levelRef, onSelect, onHover }: PartsProps & { kind: Kind; slots: Slot[] }) {
  const ref = useRef<THREE.InstancedMesh>(null)
  const geometry = useMemo(() => makeGeometry(kind), [kind])
  const material = useMemo(() => new THREE.MeshStandardMaterial({ roughness: 0.6, metalness: 0.15 }), [])
  useEffect(
    () => () => {
      geometry.dispose()
      material.dispose()
    },
    [geometry, material],
  )
  // Matrices recalculées quand le niveau d'éclatement bouge ou que la sélection change (pièce grossie) ;
  // rien n'est recalculé sur une scène immobile.
  const drawn = useRef<{ level: number; selected: number | null } | null>(null)
  useFrame(() => {
    const m = ref.current
    if (!m) return
    const level = levelRef.current
    if (drawn.current && drawn.current.level === level && drawn.current.selected === selectedID) return
    drawn.current = { level, selected: selectedID }
    for (let i = 0; i < slots.length; i++) {
      const s = slots[i]
      const [x, y, z] = explodedPosition(s.placement, s.base, level)
      scratch.position.set(x, y, z)
      scratch.rotation.set(s.rotation[0], s.rotation[1], s.rotation[2])
      const sz = s.placement.shape.size
      const k = s.placement.node_id === selectedID ? 1.15 : 1
      scratch.scale.set(sz[0] * k, sz[1] * k, sz[2] * k)
      scratch.updateMatrix()
      m.setMatrixAt(i, scratch.matrix)
    }
    m.instanceMatrix.needsUpdate = true
  })
  useEffect(() => {
    const m = ref.current
    if (!m) return
    for (let i = 0; i < slots.length; i++) {
      const id = slots[i].placement.node_id
      scratchColor.set(id === selectedID ? SELECTED : id === hoveredID ? HOVER : kind === 'wheel' ? WHEEL : (colors.get(slots[i].placement.zone) ?? '#8a8a8a'))
      m.setColorAt(i, scratchColor)
    }
    if (m.instanceColor) m.instanceColor.needsUpdate = true
  }, [slots, selectedID, hoveredID, colors, kind])

  const nodeAt = (e: ThreeEvent<PointerEvent | MouseEvent>) => (e.instanceId === undefined ? null : slots[e.instanceId].placement.node_id)
  return (
    <instancedMesh
      ref={ref}
      args={[geometry, material, slots.length]}
      frustumCulled={false}
      onClick={(e) => {
        e.stopPropagation()
        const id = nodeAt(e)
        if (id !== null) onSelect(id)
      }}
      onPointerMove={(e) => {
        e.stopPropagation()
        onHover(nodeAt(e))
        document.body.style.cursor = 'pointer'
      }}
      onPointerOut={() => {
        onHover(null)
        document.body.style.cursor = ''
      }}
    />
  )
}
