import { useState, type ReactNode } from 'react'
import { api, type CostBasis, type CostMethod, type FXPolicy, type NewNode, type NodePatch, type Snapshot } from '../api.ts'
import { partName } from '../catalog.ts'
import { CurrencySelect } from '../CurrencySelect.tsx'
import { bomRoute, navigate, type Route, type Step } from '../router.ts'
import { Summary } from '../shell/Summary.tsx'
import { useAppDialog } from '../useAppDialog.tsx'
import { AddNodeDialog } from './AddNodeDialog.tsx'
import { Ideas } from './Ideas.tsx'
import { basisHint, basisLabels, fxLabels } from './costMethod.ts'
import { ProfileSettings } from './ProfileSettings.tsx'
import { NodePanel } from './NodePanel.tsx'
import { Arbitrer } from './steps/Arbitrer.tsx'
import { Chiffrer } from './steps/Chiffrer.tsx'
import { Publier } from './steps/Publier.tsx'
import { Sourcer } from './steps/Sourcer.tsx'
import { Structurer, type TreeView } from './steps/Structurer.tsx'
import type { BomData } from './useBomData.ts'
import { usePowertrainSimulation } from './usePowertrainSimulation.ts'
import { PowertrainPanel } from './PowertrainPanel.tsx'
import './bom.css'

type BomRoute = Extract<Route, { page: 'bom' }>

type Props = {
  route: BomRoute
  data: BomData
  error: string | null
  onPublished: (s: Snapshot) => void
}

// BomWorkspace : une BOM ouverte, à l'une des 5 étapes. À gauche le contenu de l'étape, à droite la colonne
// « Synthèse » (identique partout) puis le détail du nœud sélectionné ou les réglages de calcul.
// La sélection de nœud vit dans l'URL (?node=) : elle suit d'une étape à l'autre.
export function BomWorkspace({ route, data: sourceData, error, onPublished }: Props) {
  const dialogs = useAppDialog()
  const [moving, setMoving] = useState<number | null>(null)
  const [adding, setAdding] = useState<number | null>(null)
  const [tab, setTab] = useState<'detail' | 'reglages'>('detail')
  const simulation = usePowertrainSimulation(sourceData, route.step === 'structurer')
  const data = simulation.data
  const selectedID = route.node !== null && data.byID.has(route.node) ? route.node : null
  const selected = selectedID !== null ? data.byID.get(selectedID)! : null
  const go = (patch: Partial<Pick<BomRoute, 'step' | 'node' | 'tab' | 'supplier'>>, replace = false) =>
    navigate({ ...route, ...patch }, { replace })
  const select = (id: number) => {
    setTab('detail')
    go({ node: id }, true)
  }
  const label = (id: number) => {
    const n = data.byID.get(id)
    return !n ? `#${id}` : n.part_id !== null ? partName(data.catalog, n.part_id) : n.name
  }

  const remove = (id: number) => {
    const n = data.byID.get(id)
    if (!n) return
    void dialogs
      .confirm({
        title: 'Supprimer ce nœud ?',
        message: `« ${label(id)} » et tout son sous-arbre sont retirés du brouillon.`,
        confirmLabel: 'Supprimer',
        danger: true,
      })
      .then((ok) => {
        if (!ok) return
        if (n.parent_id !== null) select(n.parent_id)
        void data.run(() => api.deleteNode(data.target.id, id))
      })
  }

  let main: ReactNode
  switch (route.step as Step) {
    case 'structurer':
      main = (<>
        <PowertrainPanel state={simulation} source={sourceData} onSelect={mode => { setMoving(null); setAdding(null); simulation.select(mode) }} />
        <Structurer
          data={data}
          selectedID={selectedID}
          onSelect={select}
          view={(['arbre', 'arbre-3d', '3d'] as const).find((v) => v === route.tab) ?? 'arbre'}
          onView={(v: TreeView) => go({ tab: v === 'arbre' ? null : v }, true)}
          moving={moving}
          onCancelMove={() => setMoving(null)}
          onPickParent={(parentID) => {
            const nodeID = moving!
            setMoving(null)
            void data.run(() => api.patchNode(data.target.id, nodeID, { parent_id: parentID }))
          }}
          onAddUnder={setAdding}
          onStartMove={setMoving}
          onDelete={remove}
        />
      </>)
      break
    case 'chiffrer':
      main = <Chiffrer data={data} selectedID={selectedID} onSelect={select} />
      break
    case 'sourcer':
      main = (
        <Sourcer tab={route.tab} onTab={(t) => go({ tab: t })} supplier={route.supplier} onSupplier={(s) => go({ supplier: s }, true)}
          onOpenSupplier={(s) => go({ tab: 'fournisseurs', supplier: s })} bomID={data.target.id} version={data.version} readOnly={data.readOnly} />
      )
      break
    case 'arbitrer':
      main = <Arbitrer data={data} selectedID={selectedID} onSelect={select} />
      break
    case 'publier':
      main = (
        <Publier
          data={data}
          onPublished={onPublished}
          onOpenDraft={(id) => navigate(bomRoute({ kind: 'draft', id }, 'structurer'))}
          onFix={(id) => go({ step: 'structurer', node: id })}
        />
      )
      break
  }

  return (
    <div className="work" aria-busy={data.busy}>
      {dialogs.element}
      <main className={`main4 step-${route.step}`} id="main-content">
        {error && (
          <p role="alert" className="error">
            {error}
          </p>
        )}
        {!data.catalog.available && <p className="muted small">Catalogue indisponible : seuls les sous-ensembles s'ajoutent.</p>}
        {main}
      </main>
      <aside className="col" aria-label="Synthèse et détail">
        <Summary data={data} onSelectNode={select} />
        {route.step === 'arbitrer' && <Ideas data={data} />}
        <div className="tabs2" role="tablist" aria-label="Détail ou réglages">
          <button type="button" role="tab" aria-selected={tab === 'detail'} className={tab === 'detail' ? 'on' : ''} onClick={() => setTab('detail')}>
            Détail
          </button>
          <button type="button" role="tab" aria-selected={tab === 'reglages'} className={tab === 'reglages' ? 'on' : ''} onClick={() => setTab('reglages')}>
            Réglages de calcul
          </button>
        </div>
        <div className="sec">
          {tab === 'detail' ? (
            selected ? (
              <NodePanel
                key={selected.id}
                node={selected}
                parentName={selected.parent_id !== null ? label(selected.parent_id) : undefined}
                explanation={data.explanation}
                volume={data.volume}
                catalog={data.catalog}
                readOnly={data.readOnly}
                simulation={!!data.simulation}
                isRoot={selected.parent_id === null}
                onAdd={(n: NewNode) => data.run(() => api.addNode(data.target.id, n))}
                onPatch={(p: NodePatch) => data.run(() => api.patchNode(data.target.id, selected.id, p))}
                stock={data.stock}
                anomaly={data.anomalyByNode.get(selected.id)}
                onSetStock={(partID, q) => data.run(() => api.setPartStock(data.target.id, partID, q))}
                target={data.target}
                version={data.version}
                budget={data.budgets.byNode.get(selected.id)}
                onStartMove={() => {
                  setMoving(selected.id)
                  if (route.step !== 'structurer') go({ step: 'structurer' })
                }}
                onDelete={() => remove(selected.id)}
              />
            ) : (
              <p className="muted">Sélectionnez un nœud pour voir son détail.</p>
            )
          ) : (
            <Settings data={data} />
          )}
        </div>
      </aside>
      {adding !== null && (
        <AddNodeDialog
          parentName={label(adding)}
          parentID={adding}
          catalog={data.catalog}
          onAdd={(n) => data.run(() => api.addNode(data.target.id, n))}
          onClose={() => setAdding(null)}
        />
      )}
    </div>
  )
}

// Settings : réglages de calcul de la BOM (V1 : en-tête de la vue BOM) — méthode de calcul (ADR 0031),
// profil de véhicule (ADR 0036), devise de référence, volume, pays de destination.
function Settings({ data }: { data: BomData }) {
  const bom = data.bom
  if (!bom) {
    return (
      <div className="stack settings">
        <ProfileSettings data={data} />
        <MethodSettings data={data} />
        <dl className="facts">
          <dt>Devise</dt>
          <dd>{data.currency}</dd>
          <dt>Volume</dt>
          <dd>{data.volume} véhicules</dd>
          <dt>Destination</dt>
          <dd>{data.snapshot?.destination_country ?? 'non renseignée'}</dd>
        </dl>
      </div>
    )
  }
  return (
    <div className="stack settings">
      <ProfileSettings data={data} />
      <MethodSettings key={`method-${data.explanation.cost_method?.program_volume ?? ''}`} data={data} />
      <CurrencyVolume
        key={`${bom.currency}-${bom.volume}`}
        currency={bom.currency}
        volume={bom.volume}
        onApply={(c, v) => void data.run(() => api.updateSettings(bom.id, c, v), { bomChanged: true })}
      />
      <Destination
        key={`dest-${bom.destination_country ?? ''}`}
        country={bom.destination_country}
        onApply={(c) => void data.run(() => api.setDestination(bom.id, c), { bomChanged: true })}
      />
    </div>
  )
}

function CurrencyVolume({ currency, volume, onApply }: { currency: string; volume: number; onApply: (currency: string, volume: number) => void }) {
  const [c, setC] = useState(currency)
  const [v, setV] = useState(String(volume))
  const changed = c !== currency || v !== String(volume)
  return (
    <form
      className="stack"
      onSubmit={(e) => {
        e.preventDefault()
        onApply(c, Number.parseInt(v, 10))
      }}
    >
      <label className="setting">
        <span>Devise de référence</span>
        <CurrencySelect aria-label="Devise de référence" value={c} onChange={setC} />
      </label>
      <label className="setting">
        <span>Volume (véhicules)</span>
        <input aria-label="Volume" type="number" min={1} step={1} value={v} onChange={(e) => setV(e.target.value)} />
      </label>
      {changed && (
        <button type="submit" className="btn pri">
          Appliquer
        </button>
      )}
    </form>
  )
}

// Destination : pays de l'usine d'assemblage (ISO 3166-1 alpha-2), pour la douane et le transport (ADR 0025).
function Destination({ country, onApply }: { country: string | null; onApply: (country: string | null) => void }) {
  const [c, setC] = useState(country ?? '')
  const next = c.trim().toUpperCase()
  return (
    <form
      className="stack"
      onSubmit={(e) => {
        e.preventDefault()
        onApply(next === '' ? null : next)
      }}
    >
      <label className="setting">
        <span>Pays de destination</span>
        <input aria-label="Pays de destination" size={3} pattern="[A-Za-z]{2}" value={c} onChange={(e) => setC(e.target.value)} />
      </label>
      {next !== (country ?? '') && (
        <button type="submit" className="btn pri">
          Appliquer
        </button>
      )}
    </form>
  )
}

const bases: CostBasis[] = ['purchase', 'landed', 'full']
const fxPolicies: FXPolicy[] = ['budget', 'latest']

// MethodSettings : méthode de calcul (maquette I4-1, onglet « Réglages de calcul »). Chaque changement est
// envoyé à l'API (PUT /cost-method) puis l'arbre est relu : totaux, budgets et Synthèse suivent la méthode,
// rien n'est recalculé ici. « Complet » exige le volume programme (outillage amorti) : l'API le refuse sinon.
function MethodSettings({ data }: { data: BomData }) {
  const m = data.explanation.cost_method
  const [volume, setVolume] = useState(m?.program_volume ? String(m.program_volume) : '')
  if (!m) return <p className="muted small">Méthode de calcul non renvoyée par l’API.</p>
  const readOnly = data.readOnly || !data.bom
  const apply = (next: CostMethod) => {
    if (readOnly) return
    void data.run(() => api.setCostMethod(data.target.id, next))
  }
  const volumeValue = /^[1-9]\d{0,8}$/.test(volume.trim()) ? Number.parseInt(volume.trim(), 10) : null
  const volumeChanged = volume.trim() !== (m.program_volume ? String(m.program_volume) : '')
  return (
    <section className="method-settings" aria-label="Méthode de calcul">
      <h3 className="h3">Méthode de calcul</h3>
      <div className="meth">
        <span>Coût</span>
        <div className="seg" role="radiogroup" aria-label="Base du coût">
          {bases.map((b) => (
            <button key={b} type="button" role="radio" aria-checked={m.basis === b} className={m.basis === b ? 'on' : ''} title={basisHint(b)}
              disabled={readOnly || data.busy} onClick={() => m.basis !== b && apply({ ...m, basis: b })}>
              {basisLabels[b]}
            </button>
          ))}
        </div>
      </div>
      <div className="meth">
        <span>Change</span>
        <div className="seg" role="radiogroup" aria-label="Taux de change">
          {fxPolicies.map((f) => (
            <button key={f} type="button" role="radio" aria-checked={m.fx === f} className={m.fx === f ? 'on' : ''}
              disabled={readOnly || data.busy} onClick={() => m.fx !== f && apply({ ...m, fx: f })}>
              {fxLabels[f]}
            </button>
          ))}
        </div>
      </div>
      <form
        className="meth"
        onSubmit={(e) => {
          e.preventDefault()
          if (volumeValue !== null) apply({ ...m, program_volume: volumeValue })
        }}
      >
        <label htmlFor="program-volume">Volume programme</label>
        <span className="row">
          <input id="program-volume" type="number" min={1} step={1} value={volume} disabled={readOnly || data.busy}
            onChange={(e) => setVolume(e.target.value)} placeholder="non défini" />
          {volumeChanged && !readOnly && (
            <button type="submit" className="btn pri" disabled={volumeValue === null || data.busy}>
              Appliquer
            </button>
          )}
        </span>
      </form>
      {data.busy && (
        <p className="small recalc" role="status">
          Recalcul en cours…
        </p>
      )}
      <p className="muted small">
        {readOnly
          ? 'Méthode figée dans ce snapshot publié.'
          : 'Le snapshot de prix se choisit dans la Synthèse. Méthode et snapshot sont enregistrés dans la version publiée.'}
      </p>
    </section>
  )
}
