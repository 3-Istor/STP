import { useEffect, useMemo, useState } from 'react'
import { api, errorMessage, type ConfigurationComparison, type PowertrainMode } from '../api.ts'
import { powertrainRequest } from './powertrain.ts'
import type { BomData } from './useBomData.ts'

/** Les trois résultats partagent une seule entrée Go. Aucune écriture de BOM, aucun calcul de coût TS. */
export function usePowertrainSimulation(source: BomData, visible: boolean) {
  const request = useMemo(() => powertrainRequest({ kind: source.target.kind, id: source.target.id }, source.nodes), [source.target.kind, source.target.id, source.nodes])
  const eligible = visible && request !== null
  const boundary = `${source.target.kind}/${source.target.id}/${eligible}`
  const [choice, setChoice] = useState<{ boundary: string; mode: PowertrainMode | null }>({ boundary, mode: null })
  if (choice.boundary !== boundary) setChoice({ boundary, mode: null })
  const requestedMode = choice.boundary === boundary ? choice.mode : null
  const setMode = (mode: PowertrainMode | null) => setChoice({ boundary, mode })
  const [reload, setReload] = useState(0)
  const [result, setResult] = useState<{ context: string; value: ConfigurationComparison } | null>(null)
  const [failure, setFailure] = useState<{ context: string; message: string } | null>(null)
  const enabled = eligible && requestedMode !== null
  const context = `${source.target.kind}/${source.target.id}/${source.version}/${source.currency}/${source.priceSnapshotID}/${reload}`
  const comparison = enabled && result?.context === context ? result.value : null
  const error = enabled && failure?.context === context ? failure.message : null
  const pending = enabled && !comparison && !error
  useEffect(() => {
    if (!enabled || !request) return
    let live = true
    api.simulateConfigurations(source.target.id, request).then(value => {
      if (!live) return
      if (value.scenarios.some(s => s.tree.bom.currency !== source.currency)) throw new Error('La devise de référence a changé ; quittez puis relancez la simulation.')
      setResult({ context, value })
    }).catch(e => { if (live) setFailure({ context, message: errorMessage(e) }) })
    return () => { live = false }
  }, [enabled, request, context, source.target.id, source.currency])
  const selected = comparison?.scenarios.find(s => s.key === requestedMode)
  const activeMode = selected?.key ?? 'hybrid'
  const data = useMemo<BomData>(() => {
    if (!enabled || (!selected && !pending)) return source
    const simulation = { mode: activeMode, pending, priceAsOf: comparison?.price_as_of ?? null }
    const guard = { readOnly: true, busy: source.busy || pending, simulation, run: async () => false }
    if (!selected) return { ...source, ...guard }
    const tree = selected.tree, byID = new Map(tree.nodes.map(n => [n.id, n]))
    return { ...source, ...guard, bom: tree.bom, nodes: tree.nodes, byID, root: byID.get(comparison!.root_id),
      stock: tree.stock, planning: tree.planning, explanation: tree.explanation,
      budgets: { byNode: new Map((tree.budgets ?? []).map(b => [b.node_id, b])), unavailable: null },
      anomalies: { data: null, error: null }, anomalyByNode: new Map() }
  }, [source, enabled, activeMode, pending, comparison, selected])
  return { eligible, requestedMode, activeMode, comparison, pending, error, data,
    select: setMode, refresh: () => setReload(n => n + 1), exit: () => { setMode(null); setReload(n => n + 1) } }
}
