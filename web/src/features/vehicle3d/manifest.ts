// Contrat frontend explicite, option 1 autorisée par l'utilisateur (voir ADR 0041).
// Aucune dépendance à Three.js, au réseau ou à une forme de persistance.
import type { Vehicle3DNode } from './types.ts'

export type V3 = [number, number, number]
export type SceneScope = { kind: 'draft' | 'snapshot'; id: number; revision?: string }
export type GeometryStatus = 'verified_exact' | 'approximation' | 'schematic'
export type ComponentSystem = 'body' | 'thermal' | 'electric' | 'shared' | 'other'
export type AssetDefinition = {
  key: string; path: string; sha256: string; unit_scale: number
  source_url: string; license: string; license_url: string; author: string
  geometry_status: GeometryStatus; geometry_evidence_url?: string
}
export type InstancePlacement = {
  key: string; position: V3; rotation: V3; scale: V3; explode_vector: V3
}
export type OccurrenceBinding = {
  node_id: number; parent_id: number | null; part_id: number | null
  asset_key: string; mesh_path: string[]; system: ComponentSystem
  position_status: 'verified' | 'approximation'; position_evidence_url?: string
  instances: InstancePlacement[]
}
export type SceneManifest = {
  schema_version: 1; revision: string; scope: { kind: 'draft' | 'snapshot'; id: number }
  vehicle_label: string; technical_source_url?: string
  assets: AssetDefinition[]; bindings: OccurrenceBinding[]
  assemblies: { node_id: number; explode_vector: V3 }[]
}
export type ResolutionReason = 'no_binding' | 'part_changed' | 'parent_changed' | 'invalid_quantity' | 'positions_missing'
export type ResolvedInstance = {
  key: string; node: Vehicle3DNode; asset: AssetDefinition; binding: OccurrenceBinding
  placement: InstancePlacement; assembly_explode: V3
}
export type ManifestResolution = {
  manifest: SceneManifest; instances: ResolvedInstance[]
  unavailable: Map<number, ResolutionReason>; physical_nodes: number
}

export class ManifestError extends Error {
  readonly code: string
  constructor(code: string, message: string) { super(message); this.name = 'ManifestError'; this.code = code }
}

function fail(field: string): never { throw new ManifestError('invalid_manifest', `Manifeste 3D invalide : ${field}.`) }
function record(value: unknown, field: string): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fail(field)
  return value as Record<string, unknown>
}
function list(value: unknown, field: string): unknown[] {
  if (!Array.isArray(value)) return fail(field)
  return value
}
function text(value: unknown, field: string): string {
  if (typeof value !== 'string' || !value.trim()) return fail(field)
  return value.trim()
}
function id(value: unknown, field: string): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value <= 0) return fail(field)
  return value
}
function key(value: unknown, field: string): string {
  const v = text(value, field)
  if (!/^[a-zA-Z0-9_.-]+$/.test(v)) return fail(field)
  return v
}
function url(value: unknown, field: string): string {
  const v = text(value, field)
  try { if (!['http:', 'https:'].includes(new URL(v).protocol)) return fail(field) } catch { return fail(field) }
  return v
}
function vector(value: unknown, field: string, nonzero = false): V3 {
  const values = list(value, field)
  if (values.length !== 3 || values.some(v => typeof v !== 'number' || !Number.isFinite(v) || (nonzero && v === 0))) return fail(field)
  return [...values] as V3
}
function oneOf<T extends string>(value: unknown, options: readonly T[], field: string): T {
  if (!options.includes(value as T)) return fail(field)
  return value as T
}
function unique<T>(values: T[], identify: (v: T) => string | number, field: string) {
  const seen = new Set<string | number>()
  for (const value of values) { const k = identify(value); if (seen.has(k)) fail(field); seen.add(k) }
}

/** Copie et valide une entrée inconnue ; aucun nom de pièce n'est utilisé comme clé métier. */
export function parseManifest(input: unknown): SceneManifest {
  const m = record(input, 'document')
  if (m.schema_version !== 1) fail('schema_version (1 attendu)')
  const scope = record(m.scope, 'scope')
  const assets: AssetDefinition[] = list(m.assets, 'assets').map((raw, i) => {
    const a = record(raw, `assets[${i}]`)
    const path = text(a.path, 'asset.path')
    // GLB autonome local : ni URL distante, ni traversée de répertoire, ni chemin encodé ambigu.
    if (!/^\/models\/[a-zA-Z0-9_./-]+\.glb$/.test(path) || path.includes('..') || path.includes('//')) fail('asset.path')
    const sha256 = text(a.sha256, 'asset.sha256').toLowerCase()
    if (!/^[a-f0-9]{64}$/.test(sha256)) fail('asset.sha256')
    if (typeof a.unit_scale !== 'number' || !Number.isFinite(a.unit_scale) || a.unit_scale <= 0) fail('asset.unit_scale')
    const status = oneOf(a.geometry_status, ['verified_exact', 'approximation', 'schematic'] as const, 'asset.geometry_status')
    const proof = a.geometry_evidence_url === undefined ? undefined : url(a.geometry_evidence_url, 'asset.geometry_evidence_url')
    if (status === 'verified_exact' && !proof) fail('preuve de géométrie exacte manquante')
    return {
      key: key(a.key, 'asset.key'), path, sha256, unit_scale: a.unit_scale,
      source_url: url(a.source_url, 'asset.source_url'), license: text(a.license, 'asset.license'),
      license_url: url(a.license_url, 'asset.license_url'), author: text(a.author, 'asset.author'),
      geometry_status: status, geometry_evidence_url: proof,
    }
  })
  unique(assets, a => a.key, 'clé d’asset dupliquée')
  const knownAssets = new Set(assets.map(a => a.key))
  const bindings: OccurrenceBinding[] = list(m.bindings, 'bindings').map((raw, i) => {
    const b = record(raw, `bindings[${i}]`)
    const instances = list(b.instances, 'binding.instances').map(rawInstance => {
      const p = record(rawInstance, 'instance')
      return { key: key(p.key, 'instance.key'), position: vector(p.position, 'instance.position'),
        rotation: vector(p.rotation, 'instance.rotation'), scale: vector(p.scale, 'instance.scale', true),
        explode_vector: vector(p.explode_vector, 'instance.explode_vector') }
    })
    unique(instances, p => p.key, 'clé d’instance dupliquée')
    const assetKey = key(b.asset_key, 'binding.asset_key')
    if (!knownAssets.has(assetKey)) fail('asset inconnu dans une liaison')
    const status = oneOf(b.position_status, ['verified', 'approximation'] as const, 'binding.position_status')
    const proof = b.position_evidence_url === undefined ? undefined : url(b.position_evidence_url, 'binding.position_evidence_url')
    if (status === 'verified' && !proof) fail('preuve de position manquante')
    return {
      node_id: id(b.node_id, 'binding.node_id'), parent_id: b.parent_id === null ? null : id(b.parent_id, 'binding.parent_id'),
      part_id: b.part_id === null ? null : id(b.part_id, 'binding.part_id'), asset_key: assetKey,
      mesh_path: list(b.mesh_path, 'binding.mesh_path').map(p => text(p, 'mesh_path segment')),
      system: oneOf(b.system, ['body', 'thermal', 'electric', 'shared', 'other'] as const, 'binding.system'),
      position_status: status, position_evidence_url: proof, instances,
    }
  })
  unique(bindings, b => `${b.node_id}/${b.parent_id}/${b.part_id}`, 'liaison occurrence/parent/pièce dupliquée')
  const assemblies = list(m.assemblies ?? [], 'assemblies').map(raw => {
    const a = record(raw, 'assembly')
    return { node_id: id(a.node_id, 'assembly.node_id'), explode_vector: vector(a.explode_vector, 'assembly.explode_vector') }
  })
  unique(assemblies, a => a.node_id, 'sous-ensemble dupliqué')
  return {
    schema_version: 1, revision: text(m.revision, 'revision'), vehicle_label: text(m.vehicle_label, 'vehicle_label'),
    technical_source_url: m.technical_source_url === undefined ? undefined : url(m.technical_source_url, 'technical_source_url'),
    scope: { kind: oneOf(scope.kind, ['draft', 'snapshot'] as const, 'scope.kind'), id: id(scope.id, 'scope.id') },
    assets, bindings, assemblies,
  }
}

type DecimalQuantity = { value: bigint; scale: number }
function quantity(value: string): DecimalQuantity | null {
  if (!/^\d+(?:\.\d+)?$/.test(value)) return null
  const [whole, decimal = ''] = value.split('.')
  return { value: BigInt(whole + decimal), scale: decimal.length }
}
function multiply(a: DecimalQuantity, b: DecimalQuantity): DecimalQuantity {
  let value = a.value * b.value, scale = a.scale + b.scale
  while (scale > 0 && value % 10n === 0n) { value /= 10n; scale-- }
  return { value, scale }
}

/** Résolution sur l'arbre courant. Les éditions n'utilisent jamais une liaison d'une ancienne identité. */
export function resolveManifest(manifest: SceneManifest, scope: SceneScope, nodes: Vehicle3DNode[]): ManifestResolution {
  if (manifest.scope.kind !== scope.kind || manifest.scope.id !== scope.id) {
    throw new ManifestError('scope_mismatch', 'Ce manifeste appartient à une autre BOM ou version.')
  }
  if (scope.kind === 'snapshot' && (!scope.revision || scope.revision !== manifest.revision)) {
    throw new ManifestError('snapshot_revision_required', 'Une révision explicitement figée est requise pour la vue de ce snapshot.')
  }
  const byId = new Map<number, Vehicle3DNode>()
  for (const node of nodes) {
    if (!Number.isSafeInteger(node.id) || node.id <= 0 || byId.has(node.id)) throw new ManifestError('invalid_tree', 'Identifiant BOM absent, dupliqué ou imprécis.')
    byId.set(node.id, node)
  }
  const quantities = new Map<number, DecimalQuantity | null>()
  const assemblyVectors = new Map(manifest.assemblies.map(a => [a.node_id, a.explode_vector]))
  const offsets = new Map<number, V3>()
  // Itératif et mémorisé : pas de récursion JS ni de parcours quadratique sur un arbre profond.
  for (const node of nodes) {
    const chain: Vehicle3DNode[] = [], seen = new Set<number>()
    let current: Vehicle3DNode | undefined = node
    while (current && !quantities.has(current.id)) {
      if (seen.has(current.id)) throw new ManifestError('invalid_tree', 'Cycle dans l’arbre BOM.')
      seen.add(current.id); chain.push(current)
      if (current.parent_id !== null && !byId.has(current.parent_id)) throw new ManifestError('invalid_tree', 'Parent BOM introuvable.')
      current = current.parent_id === null ? undefined : byId.get(current.parent_id)
    }
    for (let i = chain.length - 1; i >= 0; i--) {
      const n = chain[i], own = quantity(n.quantity ?? '1')
      const parentQuantity = n.parent_id === null ? { value: 1n, scale: 0 } : quantities.get(n.parent_id)
      quantities.set(n.id, own && parentQuantity ? multiply(own, parentQuantity) : null)
      const parentOffset = n.parent_id === null ? [0, 0, 0] : offsets.get(n.parent_id)!
      const ownOffset = assemblyVectors.get(n.id) ?? [0, 0, 0]
      offsets.set(n.id, parentOffset.map((v, axis) => v + ownOffset[axis]) as V3)
    }
  }
  const byNode = new Map<number, OccurrenceBinding[]>()
  for (const b of manifest.bindings) { const group = byNode.get(b.node_id) ?? []; group.push(b); byNode.set(b.node_id, group) }
  const assets = new Map(manifest.assets.map(a => [a.key, a]))
  const unavailable = new Map<number, ResolutionReason>(), instances: ResolvedInstance[] = []
  for (const n of nodes) {
    const candidates = byNode.get(n.id) ?? []
    if (n.part_id === null && candidates.length === 0) continue
    const partMatches = candidates.filter(b => b.part_id === n.part_id)
    const binding = partMatches.find(b => b.parent_id === n.parent_id)
    if (!binding) { unavailable.set(n.id, candidates.length === 0 ? 'no_binding' : partMatches.length === 0 ? 'part_changed' : 'parent_changed'); continue }
    const count = quantities.get(n.id)
    if (!count || count.scale !== 0) { unavailable.set(n.id, 'invalid_quantity'); continue }
    if (count.value > BigInt(binding.instances.length)) { unavailable.set(n.id, 'positions_missing'); continue }
    for (const p of binding.instances.slice(0, Number(count.value))) instances.push({
      key: `node:${n.id}/instance:${p.key}`, node: n, asset: assets.get(binding.asset_key)!, binding,
      placement: p, assembly_explode: offsets.get(n.id)!,
    })
  }
  return { manifest, instances, unavailable, physical_nodes: nodes.filter(n => n.part_id !== null).length }
}
