// Correspondance BOM ↔ objets 3D (ADR 0035). Logique pure, sans `three` : testée par `node --test`.
import type { BudgetStatus, Vehicle3DNode } from './types.ts'
import type { ComponentSystem } from './manifest.ts'

export type ExplodeOptions = { fixed?: boolean; lift?: number; xScale?: number }
export type SlotDef = { slot: string; match: string; focusWith?: string[]; explode?: ExplodeOptions }
export type ZoneDef = { match: string; at: [number, number, number] }
export type SlotTable = { slots: SlotDef[]; systemZones: ZoneDef[]; defaultZone: [number, number, number] }

/** Un objet 3D = un sous-ensemble de la BOM (niveau 2, ou niveau 1 sans sous-ensemble enfant). */
export type VehicleObject = {
  key: string // slot, ou « est:<id> » pour un objet en position estimée
  nodeId: number
  name: string
  slot: string | null
  estimated: boolean
  systemId: number | null
  systemName: string
  systemIndex: number // rang du système (niveau 1) dans l'ordre des ids : couleur par système
  budgetStatus: BudgetStatus | null
  componentSystem?: ComponentSystem
  positionStatus?: 'verified' | 'approximation'
}

export type VehicleIndex = {
  representation?: 'manifest'
  rootId: number | null
  objects: VehicleObject[]
  byKey: Map<string, VehicleObject>
  byNodeId: Map<number, VehicleObject>
  systems: { id: number; name: string; index: number }[]
  parentOf: Map<number, number | null>
}

/** Minuscules, sans accents, espaces simples : « Côté de caisse  Gauche » → « cote de caisse gauche ». */
export function normalizeName(name: string): string {
  return name.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/\s+/g, ' ').trim()
}

export function slotFor(name: string, table: SlotTable): SlotDef | null {
  const n = normalizeName(name)
  return table.slots.find(s => new RegExp(s.match).test(n)) ?? null
}

/** Construit les objets 3D à partir des nœuds de l'arbre. Déterministe : ordre des ids. */
export function buildVehicleIndex(nodes: Vehicle3DNode[], table: SlotTable): VehicleIndex {
  const sorted = [...nodes].sort((a, b) => a.id - b.id)
  const byId = new Map(sorted.map(n => [n.id, n]))
  const parentOf = new Map(sorted.map(n => [n.id, n.parent_id]))
  const root = sorted.find(n => n.parent_id === null) ?? null
  const isAssembly = (n: Vehicle3DNode) => n.part_id === null
  const children = new Map<number, Vehicle3DNode[]>()
  for (const n of sorted) if (n.parent_id !== null) {
    const list = children.get(n.parent_id) ?? []
    list.push(n)
    children.set(n.parent_id, list)
  }
  const systemNodes = root ? (children.get(root.id) ?? []).filter(isAssembly) : []
  const systems = systemNodes.map((n, index) => ({ id: n.id, name: n.name, index }))

  const objects: VehicleObject[] = []
  const used = new Set<string>()
  const add = (n: Vehicle3DNode, system: { id: number; name: string; index: number }) => {
    const def = slotFor(n.name, table)
    const free = def !== null && !used.has(def.slot)
    if (free) used.add(def.slot)
    objects.push({
      key: free ? def.slot : `est:${n.id}`,
      nodeId: n.id,
      name: n.name,
      slot: free ? def.slot : null,
      estimated: !free,
      systemId: system.id,
      systemName: system.name,
      systemIndex: system.index,
      budgetStatus: n.budget_status ?? null,
    })
  }
  for (const s of systems) {
    const subs = (children.get(s.id) ?? []).filter(isAssembly)
    if (subs.length === 0) add(byId.get(s.id)!, s)
    for (const sub of subs) add(sub, s)
  }
  return {
    rootId: root?.id ?? null,
    objects,
    byKey: new Map(objects.map(o => [o.key, o])),
    byNodeId: new Map(objects.map(o => [o.nodeId, o])),
    systems,
    parentOf,
  }
}

/**
 * Objets à mettre en évidence pour un nœud sélectionné : on remonte jusqu'au sous-ensemble représenté ;
 * un système (niveau 1) sélectionne tous ses objets ; la racine ou un nœud inconnu ne sélectionne rien.
 */
export function objectsForNode(index: VehicleIndex, nodeId: number | null): string[] {
  let id: number | null = nodeId
  const seen = new Set<number>()
  while (id !== null && !seen.has(id)) {
    seen.add(id)
    const obj = index.byNodeId.get(id)
    if (obj) return [obj.key]
    const sys = index.systems.find(s => s.id === id)
    if (sys) return index.objects.filter(o => o.systemId === sys.id).map(o => o.key)
    id = index.parentOf.get(id) ?? null
  }
  return []
}

/** Slots éclatés ensemble lors d'un focus : le slot lui-même puis ses associés présents. */
export function focusGroup(key: string, index: VehicleIndex, table: SlotTable): string[] {
  const def = table.slots.find(s => s.slot === key)
  const extra = (def?.focusWith ?? []).filter(k => index.byKey.has(k))
  return [key, ...extra]
}

export function explodeOptionsFor(key: string, table: SlotTable): ExplodeOptions {
  return table.slots.find(s => s.slot === key)?.explode ?? {}
}

/** Zone d'un objet en position estimée : celle de son système, sinon la zone par défaut (au-dessus). */
export function zoneFor(systemName: string, table: SlotTable): [number, number, number] {
  const n = normalizeName(systemName)
  return table.systemZones.find(z => new RegExp(z.match).test(n))?.at ?? table.defaultZone
}

/** k-ième objet estimé d'une zone : grille 4 × 3 par étage, déterministe. */
export function estimatedPosition(zone: [number, number, number], k: number, step = 0.22): [number, number, number] {
  return [
    zone[0] + ((k % 4) - 1.5) * step,
    zone[1] + Math.floor(k / 12) * step,
    zone[2] + ((Math.floor(k / 4) % 3) - 1) * step,
  ]
}
