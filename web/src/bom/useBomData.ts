// useBomData : données d'une BOM ouverte, brouillon ou snapshot publié, partagées par les 5 étapes et la
// colonne Synthèse. Reprend le chargement de BomView et SnapshotView (V1) : après chaque action, l'arbre et
// ses totaux sont relus, aucun recalcul côté front.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  api,
  errorMessage,
  type BOM,
  type CostAnomalies,
  type FrozenCheck,
  type Explanation,
  type NodeAnomaly,
  type NodeBudget,
  type Planning,
  type Snapshot,
  type StockLine,
  type TreeNode,
} from '../api.ts'
import type { Catalog } from '../catalog.ts'
import { loadOfferPrices, noOfferPrices, withOfferPrices, type OfferPrices } from '../offerPrices.ts'
import type { Target } from '../router.ts'
import { staleQuotes } from './liveQuotes.ts'

export type ReloadCause = 'user' | 'quotes'

export type BomData = {
  simulation?: { mode: 'thermal' | 'electric' | 'hybrid'; pending: boolean; priceAsOf: string | null }
  target: Target
  readOnly: boolean
  name: string
  bom: BOM | null // brouillon
  snapshot: Snapshot | null // snapshot publié
  frozenChecks: FrozenCheck[] | null // contrôles figés à la publication (snapshot), null pour un brouillon
  currency: string
  volume: number
  annualHours: string | null
  priceSnapshotID: number | null
  nodes: TreeNode[]
  byID: ReadonlyMap<number, TreeNode>
  root: TreeNode | undefined
  explanation: Explanation
  stock: StockLine[]
  planning: Planning | null
  catalog: Catalog // catalogue avec les prix des offres à la source des totaux
  // Budgets cibles par nœud, lus avec l'arbre (même calcul que les coûts affichés). unavailable : pourquoi il
  // n'y a aucun budget à montrer (snapshot publié, API sans budgets) ; une BOM sans budget a une table vide.
  budgets: Budgets
  anomalies: { data: CostAnomalies | null; error: string | null }
  anomalyByNode: ReadonlyMap<number, NodeAnomaly>
  threshold: string
  applyThreshold: (th: string) => void
  newQuotes: boolean // un tick de prix plus récent que les totaux affichés a été validé (brouillon en direct)
  version: number // change à chaque relecture de l'arbre
  busy: boolean
  // run exécute une action de l'utilisateur puis relit l'arbre ; une erreur de l'API est affichée telle quelle.
  run: (action: () => Promise<unknown>, opts?: { bomChanged?: boolean; cause?: ReloadCause }) => Promise<boolean>
  reloadCause: ReloadCause // 'quotes' : relecture due à une nouvelle cotation, pas à une action de l'utilisateur
}

export type Budgets = { byNode: ReadonlyMap<number, NodeBudget>; unavailable: string | null }

const noBudgets = new Map<number, NodeBudget>()

type State =
  | { status: 'loading' }
  | { status: 'error'; error: string }
  | { status: 'ready'; data: BomData }

const emptyAnomalies = { data: null, error: null }

export function useBomData(target: Target, catalog: Catalog, onBOMChanged: () => void): {
  state: State
  error: string | null
  clearError: () => void
} {
  const [bom, setBOM] = useState<{ bom: BOM; nodes: TreeNode[]; explanation: Explanation; stock: StockLine[]; planning: Planning; budgets?: NodeBudget[] } | null>(null)
  const [snap, setSnap] = useState<{ snapshot: Snapshot; nodes: TreeNode[]; explanation: Explanation; stock: StockLine[]; planning: Planning | null; checks?: FrozenCheck[] } | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [version, setVersion] = useState(0)
  const [reloadCause, setReloadCause] = useState<ReloadCause>('user')
  const [threshold, setThreshold] = useState('3')
  const [anomalies, setAnomalies] = useState<{ data: CostAnomalies | null; error: string | null }>(emptyAnomalies)
  const anomalyRequest = useRef(0)
  const [loadedTick, setLoadedTick] = useState<number | null>(null)
  const [latestTick, setLatestTick] = useState<number | null>(null)
  const isDraft = target.kind === 'draft'
  const id = target.id

  // Anomalies de coût (ADR 0027), relues avec l'arbre ; seule la dernière requête lancée est affichée.
  const loadAnomalies = useCallback(
    (th: string) => {
      const req = ++anomalyRequest.current
      return api
        .anomalies(id, th)
        .then((data) => {
          if (req === anomalyRequest.current) setAnomalies({ data, error: null })
          return true
        })
        .catch((e: unknown) => {
          if (req === anomalyRequest.current) setAnomalies({ data: null, error: errorMessage(e) })
          return false
        })
    },
    [id],
  )

  // Tick lu avant l'arbre : au pire le calcul est plus récent que le tick retenu (jamais l'inverse).
  const loadTree = useCallback(async () => {
    const tick = await api.pricingStatus().then((s) => s.tick ?? null, () => null)
    const t = await api.tree(id)
    return { t, tick }
  }, [id])

  useEffect(() => {
    // Le composant est remonté (clé) à chaque changement de BOM : aucun état à remettre à zéro ici.
    let cancelled = false
    if (isDraft) {
      loadTree()
        .then(({ t, tick }) => {
          if (cancelled) return
          setBOM(t)
          setLoadedTick(tick)
          void loadAnomalies('3')
        })
        .catch((e: unknown) => {
          if (!cancelled) setLoadError(errorMessage(e))
        })
    } else {
      api
        .snapshot(id)
        .then((d) => {
          if (!cancelled) setSnap({ ...d, stock: d.stock ?? [], planning: d.planning ?? null })
        })
        .catch((e: unknown) => {
          if (!cancelled) setLoadError(errorMessage(e))
        })
    }
    return () => {
      cancelled = true
    }
  }, [id, isDraft, loadAnomalies, loadTree])

  const reload = useCallback(async (cause: ReloadCause = 'user') => {
    const { t, tick } = await loadTree()
    setBOM(t)
    setLoadedTick(tick)
    setReloadCause(cause)
    setVersion((v) => v + 1)
    void loadAnomalies(threshold)
  }, [loadTree, loadAnomalies, threshold])

  // Flux de prix : seulement pour un brouillon en direct ; on retient le dernier tick validé, sans recharger l'arbre.
  const liveDraft = bom !== null && isDraft && bom.bom.price_snapshot_id === null
  useEffect(() => {
    if (!liveDraft) return
    const source = new EventSource('/api/v1/quotes/stream')
    const onTick = (e: MessageEvent<string>) => {
      try {
        const tick = (JSON.parse(e.data) as { tick?: unknown }).tick
        if (typeof tick === 'number') setLatestTick((t) => (t === null || tick > t ? tick : t))
      } catch { /* événement illisible : ignoré */ }
    }
    source.addEventListener('tick', onTick)
    return () => source.close()
  }, [liveDraft])

  const run = useCallback(
    async (action: () => Promise<unknown>, opts: { bomChanged?: boolean; cause?: ReloadCause } = {}) => {
      setBusy(true)
      setError(null)
      try {
        await action()
        await reload(opts.cause)
        if (opts.bomChanged) onBOMChanged()
        return true
      } catch (e) {
        setError(errorMessage(e))
        return false
      } finally {
        setBusy(false)
      }
    },
    [reload, onBOMChanged],
  )

  // A live draft follows the quotes (spec): one reload per new tick, never retried in a loop on failure.
  const autoTick = useRef<number | null>(null)
  const stale = liveDraft && staleQuotes(loadedTick, latestTick)
  useEffect(() => {
    if (!stale || busy || latestTick === null || autoTick.current === latestTick) return
    autoTick.current = latestTick
    void run(async () => {}, { cause: 'quotes' })
  }, [stale, busy, latestTick, run])

  // Prix des sélecteurs d'offres : même source que les totaux (direct ou snapshot de prix). Lecture en
  // échec : aucun prix inventé, les offres restent « sans prix ».
  const priceSnapshotID = bom ? bom.bom.price_snapshot_id : snap ? snap.snapshot.price_snapshot_id : undefined
  const [loadedPrices, setLoadedPrices] = useState<{ snapshotID: number | null; prices: OfferPrices } | null>(null)
  useEffect(() => {
    if (priceSnapshotID === undefined) return
    let cancelled = false
    loadOfferPrices(priceSnapshotID)
      .then((p) => {
        if (!cancelled) setLoadedPrices({ snapshotID: priceSnapshotID, prices: p })
      })
      .catch(() => {
        if (!cancelled) setLoadedPrices({ snapshotID: priceSnapshotID, prices: noOfferPrices })
      })
    return () => {
      cancelled = true
    }
  }, [priceSnapshotID])
  const prices = loadedPrices && loadedPrices.snapshotID === priceSnapshotID ? loadedPrices.prices : noOfferPrices
  const pricedCatalog = useMemo(() => withOfferPrices(catalog, prices), [catalog, prices])

  const nodes = bom?.nodes ?? snap?.nodes
  const byID = useMemo(() => new Map((nodes ?? []).map((n) => [n.id, n])), [nodes])
  const anomalyByNode = useMemo(() => new Map((anomalies.data?.nodes ?? []).map((a) => [a.node_id, a])), [anomalies.data])
  const treeBudgets = bom?.budgets
  const budgets = useMemo<Budgets>(() => {
    if (!isDraft) return { byNode: noBudgets, unavailable: 'Budgets non figés dans un snapshot publié.' }
    if (!treeBudgets) return { byNode: noBudgets, unavailable: 'Budgets absents de la réponse de l’API.' }
    return { byNode: new Map(treeBudgets.map((b) => [b.node_id, b])), unavailable: null }
  }, [isDraft, treeBudgets])

  const applyThreshold = useCallback(
    (th: string) => {
      // Le seuil n'est retenu que s'il est accepté : un seuil refusé ne fait pas échouer les relectures.
      void loadAnomalies(th).then((ok) => {
        if (ok) setThreshold(th)
      })
    },
    [loadAnomalies],
  )

  let state: State
  if (loadError) state = { status: 'error', error: loadError }
  else if (bom && isDraft) {
    state = {
      status: 'ready',
      data: {
        target, readOnly: false, name: bom.bom.name, bom: bom.bom, snapshot: null, frozenChecks: null,
        currency: bom.bom.currency, volume: bom.bom.volume, annualHours: bom.bom.annual_hours,
        priceSnapshotID: bom.bom.price_snapshot_id, nodes: bom.nodes, byID, root: bom.nodes.find((n) => n.parent_id === null),
        explanation: bom.explanation, stock: bom.stock, planning: bom.planning, catalog: pricedCatalog, budgets,
        anomalies, anomalyByNode, threshold, applyThreshold, version, busy, run, reloadCause,
        newQuotes: liveDraft && staleQuotes(loadedTick, latestTick),
      },
    }
  } else if (snap && !isDraft) {
    state = {
      status: 'ready',
      data: {
        target, readOnly: true, name: snap.snapshot.name, bom: null, snapshot: snap.snapshot, frozenChecks: snap.checks ?? null,
        currency: snap.snapshot.currency, volume: snap.snapshot.volume, annualHours: snap.snapshot.annual_hours,
        priceSnapshotID: snap.snapshot.price_snapshot_id, nodes: snap.nodes, byID, root: snap.nodes.find((n) => n.parent_id === null),
        explanation: snap.explanation, stock: snap.stock, planning: snap.planning, catalog: pricedCatalog, budgets,
        anomalies: emptyAnomalies, anomalyByNode, threshold, applyThreshold, version, busy: false, newQuotes: false, reloadCause: 'user',
        run: async () => false,
      },
    }
  } else state = { status: 'loading' }

  return { state, error, clearError: () => setError(null) }
}
