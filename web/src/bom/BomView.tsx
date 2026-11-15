import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, errorMessage, type Assembly, type CostAnomalies, type NewNode, type NodePatch, type Snapshot, type Tree } from '../api.ts'
import type { Catalog } from '../catalog.ts'
import { CurrencySelect } from '../CurrencySelect.tsx'
import { formatMoney } from '../money.ts'
import { loadOfferPrices, noOfferPrices, withOfferPrices, type OfferPrices } from '../offerPrices.ts'
import { useAppDialog } from '../useAppDialog.tsx'
import { AnomalySettings } from './Anomalies.tsx'
import { BomTree } from './BomTree.tsx'
import { NodePanel } from './NodePanel.tsx'
import { Totals } from './Totals.tsx'
import { PlanningPanel } from './PlanningPanel.tsx'
import { Workforce } from './Workforce.tsx'

// La vue 3D (Three.js) n'est téléchargée qu'à la première bascule : l'arbre ne paie pas son poids.
const Bom3DView = lazy(() => import('./Bom3DView.tsx'))

type Props = {
  bomID: number
  catalog: Catalog
  onBOMChanged: () => void
  onOpenSnapshot: (id: number) => void
}

// BomView : un brouillon — réglages, totaux de la racine, arbre virtualisé et panneau latéral.
// Après chaque action, l'arbre et ses totaux sont relus (pas de recalcul côté front).
export function BomView({ bomID, catalog, onBOMChanged, onOpenSnapshot }: Props) {
  const [tree, setTree] = useState<Tree | null>(null)
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const [moving, setMoving] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [published, setPublished] = useState<Snapshot | null>(null)
  const dialogs = useAppDialog()
  // Prix lus pour un price_snapshot_id donné : ceux d'une autre source ne sont jamais affichés.
  const [loadedPrices, setLoadedPrices] = useState<{ snapshotID: number | null; prices: OfferPrices } | null>(null)

  // Anomalies de coût (ADR 0027), relues avec l'arbre ; en échec, aucune surbrillance et l'erreur est montrée.
  const [threshold, setThreshold] = useState('3')
  const [anomalies, setAnomalies] = useState<{ data: CostAnomalies | null; error: string | null }>({ data: null, error: null })
  // Seule la dernière requête lancée est affichée : une réponse plus ancienne (autre seuil) est ignorée.
  const anomalyRequest = useRef(0)
  const loadAnomalies = useCallback(
    (th: string) => {
      const req = ++anomalyRequest.current
      return api
        .anomalies(bomID, th)
        .then((data) => {
          if (req === anomalyRequest.current) setAnomalies({ data, error: null })
          return true
        })
        .catch((e: unknown) => {
          if (req === anomalyRequest.current) setAnomalies({ data: null, error: errorMessage(e) })
          return false
        })
    },
    [bomID],
  )
  const anomalyByNode = useMemo(() => new Map((anomalies.data?.nodes ?? []).map((a) => [a.node_id, a])), [anomalies.data])

  // version : change à chaque relecture de l'arbre ; les panneaux qui ont des données dérivées (suggestions)
  // se remontent, une stratégie héritée ou une quantité d'ancêtre ayant pu changer.
  const [version, setVersion] = useState(0)

  // Vue BOM / Vue 3D (ADR 0031) : deux rendus de la même donnée. La sélection (selectedID) est partagée ;
  // la scène est lue à la première bascule puis relue seulement si l'arbre a changé (version).
  const [view, setView] = useState<'tree' | '3d'>('tree')
  const [scene, setScene] = useState<{ data: Assembly | null; error: string | null; version: number }>({ data: null, error: null, version: -1 })
  const [reveal, setReveal] = useState(0)
  // sceneRequest : version de l'arbre dont la lecture est en cours ou faite (une seule requête par version).
  const sceneRequest = useRef(-1)
  useEffect(() => {
    if (view !== '3d' || sceneRequest.current === version) return
    sceneRequest.current = version
    let cancelled = false
    let done = false
    api
      .assembly(bomID)
      .then((data) => {
        done = true
        if (!cancelled) setScene({ data, error: null, version })
      })
      .catch((e: unknown) => {
        done = true
        if (!cancelled) setScene((s) => ({ ...s, error: errorMessage(e), version }))
      })
    return () => {
      // Quitter la vue 3D n'oublie qu'une lecture encore en vol : une scène déjà lue n'est pas relue au retour.
      cancelled = true
      if (!done && sceneRequest.current === version) sceneRequest.current = -1
    }
  }, [view, version, bomID])
  const sceneLoading = view === '3d' && scene.version !== version
  const reload = useCallback(async () => {
    const t = await api.tree(bomID)
    setTree(t)
    setVersion((v) => v + 1)
    void loadAnomalies(threshold)
    return t
  }, [bomID, loadAnomalies, threshold])

  useEffect(() => {
    // La vue est remontée (key) quand on change de BOM : pas d'état à remettre à zéro ici.
    let cancelled = false
    api
      .tree(bomID)
      .then((t) => {
        if (!cancelled) setTree(t)
      })
      .then(() => {
        if (!cancelled) void loadAnomalies('3')
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(errorMessage(e))
      })
    return () => {
      cancelled = true
    }
  }, [bomID, loadAnomalies])

  // Prix des sélecteurs d'offres : même source que les totaux (direct ou snapshot de prix), relus quand la
  // BOM est figée ou défigée. Lecture en échec : aucun prix inventé, les offres restent « sans prix ».
  const priceSnapshotID = tree ? tree.bom.price_snapshot_id : undefined
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

  // run exécute une action de l'utilisateur puis relit l'arbre ; une erreur de l'API est affichée telle quelle.
  async function run(action: () => Promise<unknown>, opts: { bomChanged?: boolean } = {}): Promise<boolean> {
    setBusy(true)
    setError(null)
    try {
      await action()
      await reload()
      if (opts.bomChanged) onBOMChanged()
      return true
    } catch (e) {
      setError(errorMessage(e))
      return false
    } finally {
      setBusy(false)
    }
  }

  if (!tree) {
    return <section className="view">{error ? <p role="alert" className="error">{error}</p> : <p>Chargement…</p>}</section>
  }

  const { bom } = tree
  const root = tree.nodes.find((n) => n.parent_id === null)
  const selected = tree.nodes.find((n) => n.id === selectedID) ?? null
  const frozen = bom.price_snapshot_id !== null

  return (
    <section className="view" aria-busy={busy}>
      {dialogs.element}
      <header className="view-head">
        <div>
          <h1>
            {bom.name}{' '}
            <button
              type="button"
              className="link small"
              onClick={() => {
                void dialogs
                  .prompt({ title: 'Renommer la BOM', label: 'Nouveau nom', initial: bom.name, confirmLabel: 'Renommer' })
                  .then((name) => {
                    if (name && name !== bom.name) void run(() => api.renameBOM(bom.id, name), { bomChanged: true })
                  })
              }}
            >
              renommer
            </button>
          </h1>
          <Settings
            key={`${bom.currency}-${bom.volume}`}
            currency={bom.currency}
            volume={bom.volume}
            onApply={(c, v) => void run(() => api.updateSettings(bom.id, c, v), { bomChanged: true })}
          />
          <Destination
            key={`dest-${bom.destination_country ?? ''}`}
            country={bom.destination_country}
            onApply={(c) => void run(() => api.setDestination(bom.id, c), { bomChanged: true })}
          />
          <Workforce
            key={`fte-${bom.annual_hours}`}
            nodes={tree.nodes}
            annualHours={bom.annual_hours}
            catalog={pricedCatalog}
            readOnly={false}
            onApplyAnnualHours={(h) => void run(() => api.setAnnualHours(bom.id, h), { bomChanged: true })}
          />
          <AnomalySettings
            result={anomalies.data}
            error={anomalies.error}
            threshold={threshold}
            onApply={(th) => {
              // Le seuil n'est retenu que s'il est accepté : un seuil refusé ne fait pas échouer les relectures.
              void loadAnomalies(th).then((ok) => {
                if (ok) setThreshold(th)
              })
            }}
          />
          <PlanningPanel
            key={`plan-${tree.planning.schedule?.start_on ?? ''}-${tree.planning.schedule?.target_on ?? ''}`}
            planning={tree.planning}
            nodes={tree.nodes}
            catalog={pricedCatalog}
            readOnly={false}
            onSetSchedule={(sch) => void run(() => api.setSchedule(bom.id, sch))}
            onClearSchedule={() => void run(() => api.clearSchedule(bom.id))}
            onSelect={setSelectedID}
          />
          {bom.source_snapshot_id !== null && (
            <p className="muted small">
              Créé depuis le{' '}
              <button type="button" className="link" onClick={() => onOpenSnapshot(bom.source_snapshot_id!)}>
                snapshot #{bom.source_snapshot_id}
              </button>
            </p>
          )}
        </div>
        {root?.totals && <Totals totals={root.totals} explanation={tree.explanation} />}
        <div className="actions">
          {frozen ? (
            <button type="button" onClick={() => void run(() => api.unfreezePrices(bom.id), { bomChanged: true })}>
              Repasser en prix en direct
            </button>
          ) : (
            <button type="button" onClick={() => void run(() => api.freezePrices(bom.id), { bomChanged: true })}>
              Figer les prix
            </button>
          )}
          <button
            type="button"
            className="primary"
            onClick={() =>
              void run(async () => {
                setPublished(await api.publish(bom.id))
              })
            }
          >
            Publier
          </button>
        </div>
      </header>

      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {published && (
        <p role="status" className="notice">
          Snapshot #{published.id} publié ({formatMoney(root?.totals?.cost)}).{' '}
          <button type="button" className="link" onClick={() => onOpenSnapshot(published.id)}>
            Ouvrir
          </button>
        </p>
      )}
      {moving !== null && (
        <p role="status" className="notice">
          Cliquez sur le nouveau parent du nœud #{moving}.{' '}
          <button type="button" className="link" onClick={() => setMoving(null)}>
            Annuler
          </button>
        </p>
      )}

      <div className="view-switch" role="group" aria-label="Mode de visualisation">
        <button type="button" aria-pressed={view === 'tree'} onClick={() => setView('tree')}>
          Vue BOM
        </button>
        <button type="button" aria-pressed={view === '3d'} onClick={() => setView('3d')}>
          Vue 3D
        </button>
      </div>
      <div className="bom-workspace">
        {view === 'tree' ? (
          <BomTree
            nodes={tree.nodes}
            catalog={pricedCatalog}
            readOnly={false}
            selectedID={selectedID}
            onSelect={setSelectedID}
            anomalies={anomalyByNode}
            onChangeOffer={(nodeID, offerID) => void run(() => api.patchNode(bom.id, nodeID, { offer_id: offerID }))}
            moving={moving}
            onPickParent={(parentID) => {
              const nodeID = moving!
              setMoving(null)
              void run(() => api.patchNode(bom.id, nodeID, { parent_id: parentID }))
            }}
            reveal={reveal}
          />
        ) : (
          <Suspense fallback={<div className="viz3d"><p className="muted viz3d-message">Chargement de la vue 3D…</p></div>}>
            <Bom3DView
              assembly={scene.data}
              loading={sceneLoading}
              error={scene.error}
              nodes={tree.nodes}
              catalog={pricedCatalog}
              selectedID={selectedID}
              onSelect={setSelectedID}
              onShowInTree={() => {
                setView('tree')
                setReveal((r) => r + 1)
              }}
            />
          </Suspense>
        )}
        {selected ? (
          <NodePanel
            key={selected.id}
            node={selected}
            explanation={tree.explanation}
            volume={bom.volume}
            catalog={pricedCatalog}
            readOnly={false}
            isRoot={selected.parent_id === null}
            onAdd={(n: NewNode) => run(() => api.addNode(bom.id, n))}
            onPatch={(p: NodePatch) => run(() => api.patchNode(bom.id, selected.id, p))}
            stock={tree.stock}
            bomID={bom.id}
            anomaly={anomalyByNode.get(selected.id)}
            treeVersion={version}
            destination={bom.destination_country}
            onSetStock={(partID, q) => run(() => api.setPartStock(bom.id, partID, q))}
            onStartMove={() => setMoving(selected.id)}
            onDelete={() => {
              void dialogs
                .confirm({
                  title: 'Supprimer ce nœud ?',
                  message: 'Le nœud et tout son sous-arbre sont retirés du brouillon.',
                  confirmLabel: 'Supprimer',
                  danger: true,
                })
                .then((ok) => {
                  if (!ok) return
                  setSelectedID(selected.parent_id)
                  void run(() => api.deleteNode(bom.id, selected.id))
                })
            }}
          />
        ) : (
          <aside className="panel muted">Sélectionnez un nœud pour voir son détail.</aside>
        )}
      </div>
    </section>
  )
}

function Settings({
  currency,
  volume,
  onApply,
}: {
  currency: string
  volume: number
  onApply: (currency: string, volume: number) => void
}) {
  const [c, setC] = useState(currency)
  const [v, setV] = useState(String(volume))
  const changed = c !== currency || v !== String(volume)
  return (
    <form
      className="row small"
      onSubmit={(e) => {
        e.preventDefault()
        onApply(c, Number.parseInt(v, 10))
      }}
    >
      <label>
        Devise{' '}
        <CurrencySelect aria-label="Devise de référence" value={c} onChange={setC} />
      </label>
      <label>
        Volume <input aria-label="Volume" type="number" min={1} step={1} value={v} onChange={(e) => setV(e.target.value)} />
      </label>
      {changed && <button type="submit">Appliquer</button>}
    </form>
  )
}

// Destination : pays de l'usine d'assemblage (ISO 3166-1 alpha-2), pour la douane et le transport (ADR 0025).
function Destination({ country, onApply }: { country: string | null; onApply: (country: string | null) => void }) {
  const [c, setC] = useState(country ?? '')
  const next = c.trim().toUpperCase()
  return (
    <form
      className="row small"
      onSubmit={(e) => {
        e.preventDefault()
        onApply(next === '' ? null : next)
      }}
    >
      <label>
        Destination <input aria-label="Pays de destination" size={3} pattern="[A-Za-z]{2}" value={c} onChange={(e) => setC(e.target.value)} />
      </label>
      {next !== (country ?? '') && <button type="submit">Appliquer</button>}
    </form>
  )
}
