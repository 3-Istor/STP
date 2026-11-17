// Moteur de la vue 3D (ADR 0035) : scène, caméra, contrôles, sélection, éclaté, focus, couleurs.
// Rendu à la demande : aucune boucle au repos, une image seulement quand quelque chose change.
import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { RoomEnvironment } from 'three/addons/environments/RoomEnvironment.js'
import { budgetColor, systemColor, type StatusPalette } from './colors.ts'
import { axialOffsets, explodeVector, type V3 } from './explode.ts'
import { explodeOptionsFor, focusGroup, type SlotTable, type VehicleIndex, type VehicleObject } from './mapping.ts'
import { disposeTree, itemBox, styleMesh, type Built, type Item } from './scene.ts'
import { buildSimple } from './simpleModel.ts'
import type { ColorBy, RenderMode } from './types.ts'

export type LoadResult = { mode: RenderMode; fallback: string | null; missing: VehicleObject[]; attribution: string | null }

export type ViewerCallbacks = {
  onPick: (obj: VehicleObject) => void
  onHover: (obj: VehicleObject | null, x: number, y: number) => void
  onFocusChange: (obj: VehicleObject | null) => void
}

/** Chargeur du rendu réaliste, injecté pour garder GLTFLoader dans son propre chunk. */
export type RealisticLoader = (index: VehicleIndex) => Promise<Built & { attribution: string }>

const BACKGROUND = '#eef0f3'
const SELECTION_COLOR = new THREE.Color('#1f5fbf')
const DEFAULT_VIEW = new THREE.Vector3(-0.62, 0.36, 0.7).normalize() // avant-gauche, en surplomb

export class Viewer {
  private readonly renderer: THREE.WebGLRenderer
  private readonly scene = new THREE.Scene()
  private readonly camera = new THREE.PerspectiveCamera(32, 1, 0.05, 200)
  private readonly controls: OrbitControls
  private readonly envTexture: THREE.Texture
  private readonly resize: ResizeObserver
  private readonly raycaster = new THREE.Raycaster()
  private built: Built | null = null
  private index: VehicleIndex | null = null
  private table: SlotTable
  private itemsByDepth: Item[] = []
  private objectCenters = new Map<string, V3>()
  private vehicleCenter: V3 = [0, 0.8, 0]
  private vehicleLength = 4.2
  private explode = 0
  private focusKey: string | null = null
  private focusKeys = new Set<string>()
  private selection = new Set<string>()
  private selectionFrames: THREE.Box3Helper[] = []
  private colorBy: ColorBy = 'system'
  private palette: StatusPalette
  private frame = 0
  private tween: { from: [THREE.Vector3, THREE.Vector3]; to: [THREE.Vector3, THREE.Vector3]; t0: number; ms: number } | null = null
  private loadToken = 0
  private pointerDown: { x: number; y: number; t: number } | null = null
  private hoverFrame = 0
  private disposed = false
  /** Nombre d'images rendues (diagnostic : vérifie l'absence de boucle au repos). */
  renders = 0

  private readonly container: HTMLElement
  private readonly cb: ViewerCallbacks

  constructor(container: HTMLElement, table: SlotTable, palette: StatusPalette, cb: ViewerCallbacks) {
    this.container = container
    this.cb = cb
    this.table = table
    this.palette = palette
    this.renderer = new THREE.WebGLRenderer({ antialias: true })
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    this.renderer.shadowMap.enabled = true
    this.renderer.shadowMap.type = THREE.PCFShadowMap
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping
    this.renderer.toneMappingExposure = 1.05
    this.renderer.domElement.className = 'v3d-canvas'
    container.appendChild(this.renderer.domElement)

    this.scene.background = new THREE.Color(BACKGROUND)
    const pmrem = new THREE.PMREMGenerator(this.renderer)
    const room = new RoomEnvironment()
    this.envTexture = pmrem.fromScene(room, 0.04).texture
    room.dispose()
    pmrem.dispose()
    this.scene.environment = this.envTexture
    const sun = new THREE.DirectionalLight('#ffffff', 1.6)
    sun.position.set(-4, 9, 6)
    sun.castShadow = true
    sun.shadow.mapSize.set(2048, 2048)
    Object.assign(sun.shadow.camera, { left: -7, right: 7, top: 7, bottom: -7, near: 1, far: 30 })
    sun.shadow.bias = -0.0004
    this.scene.add(sun, new THREE.HemisphereLight('#ffffff', '#9aa3ad', 0.6))
    const ground = new THREE.Mesh(new THREE.PlaneGeometry(60, 60), new THREE.ShadowMaterial({ opacity: 0.16 }))
    ground.rotation.x = -Math.PI / 2
    ground.receiveShadow = true
    this.scene.add(ground)

    this.controls = new OrbitControls(this.camera, this.renderer.domElement)
    this.controls.enableDamping = false // pas d'inertie : sinon il faudrait une boucle de rendu
    this.controls.maxPolarAngle = Math.PI * 0.495
    this.controls.addEventListener('change', this.requestRender)

    const el = this.renderer.domElement
    el.addEventListener('pointerdown', this.onPointerDown)
    el.addEventListener('pointerup', this.onPointerUp)
    el.addEventListener('pointermove', this.onPointerMove)
    el.addEventListener('pointerleave', this.onPointerLeave)
    el.addEventListener('dblclick', this.onDoubleClick)

    this.resize = new ResizeObserver(() => this.onResize())
    this.resize.observe(container)
    this.onResize()
  }

  // ——— Chargement ———

  async load(mode: RenderMode, index: VehicleIndex, realistic: RealisticLoader): Promise<LoadResult> {
    const token = ++this.loadToken
    let built: Built
    let fallback: string | null = null
    let attribution: string | null = null
    let effective: RenderMode = mode
    if (mode === 'realistic') {
      try {
        const r = await realistic(index)
        built = r
        attribution = r.attribution
      } catch (err) {
        fallback = err instanceof Error ? err.message : String(err)
        effective = 'simple'
        built = buildSimple(index, this.table)
      }
    } else {
      built = buildSimple(index, this.table)
    }
    if (this.disposed || token !== this.loadToken) {
      disposeTree(built.root)
      throw new Error('chargement remplacé')
    }
    this.replace(built, index)
    return { mode: effective, fallback, missing: built.missing, attribution }
  }

  private replace(built: Built, index: VehicleIndex) {
    if (this.built) {
      this.scene.remove(this.built.root)
      disposeTree(this.built.root)
    }
    this.built = built
    this.index = index
    this.focusKey = null
    this.focusKeys.clear()
    this.scene.add(built.root)
    built.root.updateMatrixWorld(true)
    for (const it of built.items) {
      it.baseLocal.copy(it.node.position)
      it.node.getWorldPosition(it.baseWorld)
      let d = 0
      for (let p = it.node.parent; p; p = p.parent) d++
      it.depth = d
    }
    this.itemsByDepth = [...built.items].sort((a, b) => a.depth - b.depth)
    // Centres d'objets et du véhicule, sur la position assemblée : base de l'éclaté.
    const all = new THREE.Box3()
    const per = new Map<string, THREE.Box3>()
    for (const it of built.items) {
      const b = itemBox(it)
      if (b.isEmpty()) continue
      if (!it.obj?.estimated) all.union(b)
      if (it.obj) per.set(it.obj.key, (per.get(it.obj.key) ?? new THREE.Box3()).union(b))
    }
    const c = all.getCenter(new THREE.Vector3())
    this.vehicleCenter = [c.x, c.y, c.z]
    this.vehicleLength = Math.max(all.getSize(new THREE.Vector3()).x, 1)
    this.objectCenters = new Map([...per].map(([k, b]) => {
      const v = b.getCenter(new THREE.Vector3())
      return [k, [v.x, v.y, v.z] as V3]
    }))
    this.applyPositions()
    this.applyStyles()
    this.recenter(false)
  }

  // ——— Réglages pilotés par les props ———

  setColors(by: ColorBy, palette: StatusPalette) {
    this.colorBy = by
    this.palette = palette
    this.applyStyles()
  }

  setExplode(k: number) {
    this.explode = Math.min(Math.max(k, 0), 1)
    if (!this.focusKey) { this.applyPositions(); this.updateSelectionFrames(); this.requestRender() }
  }

  setSelection(keys: string[], frame: boolean) {
    this.selection = new Set(keys)
    this.applyStyles()
    if (frame && keys.length > 0 && !this.focusKey) this.frameItems(this.itemsOf(new Set(keys)), null)
  }

  /** Focus : le sous-ensemble (et ses associés) s'éclatent dans l'axe, le reste passe en transparence. */
  setFocus(key: string | null) {
    if (!this.index || (key !== null && !this.index.byKey.has(key))) key = null
    this.focusKey = key
    this.focusKeys = new Set(key ? focusGroup(key, this.index!, this.table) : [])
    this.applyPositions()
    this.applyStyles()
    this.cb.onFocusChange(key ? this.index!.byKey.get(key)! : null)
    if (key) this.frameFocus()
    else this.recenter(true)
  }

  get focused() { return this.focusKey }

  /** Position écran (px, relative au canvas) du centre actuel d'un objet : sert aux tests de clic réels. */
  screenPositionOf(key: string): { x: number; y: number } | null {
    const box = this.boxOf(this.itemsOf(new Set([key])))
    if (box.isEmpty()) return null
    const v = box.getCenter(new THREE.Vector3()).project(this.camera)
    const r = this.renderer.domElement.getBoundingClientRect()
    return { x: ((v.x + 1) / 2) * r.width, y: ((1 - v.y) / 2) * r.height }
  }

  recenter(animate = true) {
    const items = this.built?.items.filter(i => !i.obj?.estimated) ?? []
    this.frameItems(items, DEFAULT_VIEW, animate)
  }

  // ——— Positions ———

  private applyPositions() {
    if (!this.built) return
    const offsets = new Map<Item, V3>()
    if (this.focusKey) {
      const parts = this.built.items.filter(i => i.obj && this.focusKeys.has(i.obj.key))
      const center = this.objectCenters.get(this.focusKey) ?? this.vehicleCenter
      const ax = axialOffsets(parts.map((p, i) => ({ id: String(i), axial: p.axial })), center, this.vehicleCenter, this.vehicleLength)
      parts.forEach((p, i) => offsets.set(p, ax.get(String(i))!))
    } else if (this.explode > 0) {
      for (const it of this.built.items) {
        if (!it.obj) continue // pièce non reliée : reste en place
        const center = this.objectCenters.get(it.obj.key)
        if (!center) continue
        const opts = it.obj.slot ? explodeOptionsFor(it.obj.slot, this.table) : {}
        const v = explodeVector(center, this.vehicleCenter, opts)
        offsets.set(it, [v[0] * this.explode, v[1] * this.explode, v[2] * this.explode])
      }
    }
    // Remise à la position d'origine, puis application parent → enfant en coordonnées monde :
    // un item garde sa cible même si un ancêtre (autre item) s'est déplacé.
    for (const it of this.built.items) it.node.position.copy(it.baseLocal)
    this.built.root.updateMatrixWorld(true)
    const target = new THREE.Vector3()
    for (const it of this.itemsByDepth) {
      const off = offsets.get(it) ?? [0, 0, 0]
      const parent = it.node.parent
      if (!parent) continue
      parent.updateWorldMatrix(true, false)
      target.copy(it.baseWorld).add(new THREE.Vector3(...off))
      it.node.position.copy(parent.worldToLocal(target))
      it.node.updateMatrixWorld(true)
    }
  }

  // ——— Styles ———

  private colorOf(obj: VehicleObject): string {
    return this.colorBy === 'budget' ? budgetColor(obj.budgetStatus, this.palette) : systemColor(obj.systemIndex)
  }

  private applyStyles() {
    if (!this.built) return
    for (const it of this.built.items) {
      const ghost = this.focusKey !== null && !(it.obj && this.focusKeys.has(it.obj.key))
      const style = { tint: it.obj ? this.colorOf(it.obj) : null, ghost, estimated: !!it.obj?.estimated }
      for (const m of it.meshes) styleMesh(m, style)
      for (const l of it.lines) (l.material as THREE.LineBasicMaterial).opacity = ghost ? 0.1 : 0.9
    }
    this.updateSelectionFrames()
    this.requestRender()
  }

  /** Cadre bleu autour de chaque objet sélectionné (hors focus : l'éclatement désigne déjà l'objet). */
  private updateSelectionFrames() {
    for (const h of this.selectionFrames) { this.scene.remove(h); h.dispose() }
    this.selectionFrames = []
    if (this.focusKey) return
    for (const key of this.selection) {
      const box = this.boxOf(this.itemsOf(new Set([key])))
      if (box.isEmpty()) continue
      box.expandByScalar(0.03)
      const h = new THREE.Box3Helper(box, SELECTION_COLOR)
      this.scene.add(h)
      this.selectionFrames.push(h)
    }
  }

  // ——— Caméra ———

  private itemsOf(keys: Set<string>) {
    return this.built?.items.filter(i => i.obj && keys.has(i.obj.key)) ?? []
  }

  private boxOf(items: Item[]) {
    const box = new THREE.Box3()
    const b = new THREE.Box3()
    for (const it of items) box.union(itemBox(it, b))
    return box
  }

  private fitDistance(radius: number) {
    const v = THREE.MathUtils.degToRad(this.camera.fov)
    const h = 2 * Math.atan(Math.tan(v / 2) * this.camera.aspect)
    return radius / Math.sin(Math.min(v, h) / 2)
  }

  /** Cadre des items ; dir = direction caméra → cible (null : direction actuelle). */
  private frameItems(items: Item[], dir: THREE.Vector3 | null, animate = true) {
    const box = this.boxOf(items)
    if (box.isEmpty()) { this.requestRender(); return }
    const sphere = box.getBoundingSphere(new THREE.Sphere())
    const d = dir ?? this.camera.position.clone().sub(this.controls.target).normalize()
    // cadrage serré sur tout le véhicule, avec du contexte autour d'un petit sous-ensemble sélectionné
    const dist = Math.max(this.fitDistance(sphere.radius) * (dir ? 1.05 : 1.6), dir ? 1.2 : 1.5 * this.vehicleLength)
    this.moveCamera(sphere.center.clone().add(d.clone().multiplyScalar(dist)), sphere.center, animate)
  }

  private frameFocus() {
    if (!this.focusKey || !this.built) return
    const items = this.itemsOf(this.focusKeys)
    const box = this.boxOf(items)
    const c = this.objectCenters.get(this.focusKey) ?? this.vehicleCenter
    const dz = c[2] - this.vehicleCenter[2], dx = c[0] - this.vehicleCenter[0]
    const side = 0.08 * this.vehicleLength
    // Vue de trois quarts du côté où les pièces sortent, comme le focus roue de la maquette I4-3.
    const dir = Math.abs(dz) > side ? new THREE.Vector3(0.78, 0.28, Math.sign(dz) * 0.62)
      : Math.abs(dx) > side ? new THREE.Vector3(Math.sign(dx) * 0.84, 0.3, 0.45)
        : new THREE.Vector3(-0.55, 0.65, 0.55)
    dir.normalize()
    const sphere = box.getBoundingSphere(new THREE.Sphere())
    const dist = Math.max(this.fitDistance(sphere.radius) * 1.25, 2.2)
    this.moveCamera(sphere.center.clone().add(dir.multiplyScalar(dist)), sphere.center, true)
  }

  private moveCamera(pos: THREE.Vector3, target: THREE.Vector3, animate: boolean) {
    if (!animate) {
      this.camera.position.copy(pos)
      this.controls.target.copy(target)
      this.controls.update()
      this.tween = null
      this.requestRender()
      return
    }
    this.tween = { from: [this.camera.position.clone(), this.controls.target.clone()], to: [pos, target.clone()], t0: performance.now(), ms: 450 }
    this.requestRender()
  }

  // ——— Rendu à la demande ———

  readonly requestRender = () => {
    if (this.frame || this.disposed) return
    this.frame = requestAnimationFrame(this.renderFrame)
  }

  private readonly renderFrame = (now: number) => {
    this.frame = 0
    if (this.tween) {
      const t = Math.min((now - this.tween.t0) / this.tween.ms, 1)
      const e = t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2
      this.camera.position.lerpVectors(this.tween.from[0], this.tween.to[0], e)
      this.controls.target.lerpVectors(this.tween.from[1], this.tween.to[1], e)
      this.controls.update()
      if (t < 1) this.requestRender() // seule boucle : pendant une animation de caméra
      else this.tween = null
    }
    this.renderer.render(this.scene, this.camera)
    this.renders++
    this.container.dataset.renders = String(this.renders)
  }

  private onResize() {
    const w = Math.max(this.container.clientWidth, 1)
    const h = Math.max(this.container.clientHeight, 1)
    this.renderer.setSize(w, h)
    this.camera.aspect = w / h
    this.camera.updateProjectionMatrix()
    this.requestRender()
  }

  // ——— Interaction ———

  private pick(ev: PointerEvent | MouseEvent): VehicleObject | null {
    if (!this.built) return null
    const r = this.renderer.domElement.getBoundingClientRect()
    const ndc = new THREE.Vector2(((ev.clientX - r.left) / r.width) * 2 - 1, -((ev.clientY - r.top) / r.height) * 2 + 1)
    this.raycaster.setFromCamera(ndc, this.camera)
    const meshes: THREE.Object3D[] = []
    for (const it of this.built.items) {
      // en focus, seuls les sous-ensembles éclatés sont cliquables (le reste est un fantôme)
      if (this.focusKey && !(it.obj && this.focusKeys.has(it.obj.key))) continue
      for (const m of it.meshes) if (m.visible) { m.userData.item = it; meshes.push(m) }
    }
    const hit = this.raycaster.intersectObjects(meshes, false)[0]
    return (hit?.object.userData.item as Item | undefined)?.obj ?? null
  }

  private readonly onPointerDown = (ev: PointerEvent) => {
    this.pointerDown = { x: ev.clientX, y: ev.clientY, t: performance.now() }
  }

  private readonly onPointerUp = (ev: PointerEvent) => {
    const d = this.pointerDown
    this.pointerDown = null
    if (!d || ev.button !== 0) return
    // un clic, pas une rotation : peu de mouvement et bref
    if (Math.hypot(ev.clientX - d.x, ev.clientY - d.y) > 5 || performance.now() - d.t > 600) return
    const obj = this.pick(ev)
    if (obj) this.cb.onPick(obj)
  }

  private readonly onDoubleClick = (ev: MouseEvent) => {
    const obj = this.pick(ev)
    if (!obj) return
    this.setFocus(obj.key)
  }

  private readonly onPointerMove = (ev: PointerEvent) => {
    if (this.pointerDown || this.hoverFrame) return
    const { clientX, clientY } = ev
    this.hoverFrame = requestAnimationFrame(() => {
      this.hoverFrame = 0
      if (this.disposed) return
      const r = this.renderer.domElement.getBoundingClientRect()
      this.cb.onHover(this.pick(ev), clientX - r.left, clientY - r.top)
    })
  }

  private readonly onPointerLeave = () => this.cb.onHover(null, 0, 0)

  dispose() {
    this.disposed = true
    this.loadToken++
    cancelAnimationFrame(this.frame)
    cancelAnimationFrame(this.hoverFrame)
    this.resize.disconnect()
    const el = this.renderer.domElement
    el.removeEventListener('pointerdown', this.onPointerDown)
    el.removeEventListener('pointerup', this.onPointerUp)
    el.removeEventListener('pointermove', this.onPointerMove)
    el.removeEventListener('pointerleave', this.onPointerLeave)
    el.removeEventListener('dblclick', this.onDoubleClick)
    this.controls.removeEventListener('change', this.requestRender)
    this.controls.dispose()
    disposeTree(this.scene)
    this.envTexture.dispose()
    this.renderer.dispose()
    this.renderer.forceContextLoss()
    el.remove()
  }
}
