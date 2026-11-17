import { useEffect, useState } from 'react'
import { api, errorMessage, type CostMethod, type PriceSnapshotRef, type TreeNode } from '../api.ts'
import { partName } from '../catalog.ts'
import { formatDate, formatDateTime, formatDecimal, formatMoney } from '../money.ts'
import type { BomData } from '../bom/useBomData.ts'
import { shareWidths } from './share.ts'
import { budgetBarWidth, budgetClass, budgetPercentText, budgetTitle } from '../bom/budget.ts'

// Nombre de snapshots de prix montrés d'emblée ; les autres à la demande (« Tous les snapshots »).
const SHOWN_SNAPSHOTS = 3

// Summary : colonne « Synthèse », identique sur les 5 étapes (maquettes I4, composant Summary de la V1).
// Tout vient de l'API : rien n'est recalculé ni inventé. Budgets (état, %) : lus avec l'arbre (ADR 0018, 0031).
export function Summary({ data, onSelectNode }: { data: BomData; onSelectNode: (id: number) => void }) {
  const t = data.root?.totals ?? null
  const planning = data.planning
  const subsystems = data.root ? data.nodes.filter((n) => n.parent_id === data.root!.id) : []
  const widths = shareWidths(t?.cost ?? null, subsystems.map((n) => n.totals?.cost ?? null))
  const label = (n: TreeNode) => (n.part_id !== null ? partName(data.catalog, n.part_id) : n.name)
  const budgets = data.budgets
  const rootBudget = data.root ? budgets.byNode.get(data.root.id) : undefined
  // Barres « % du budget » dès qu'un sous-système a un budget ; sinon part du coût du véhicule (lot C).
  const byBudget = budgets.unavailable === null && subsystems.some((n) => budgets.byNode.has(n.id))
  const overBudget = subsystems.filter((n) => budgets.byNode.get(n.id)?.state === 'over')

  let state: { text: string; cls?: string; status?: boolean }
  if (data.busy) state = { text: 'Recalcul en cours…', cls: 'recalc', status: true }
  else if (t?.cost_error_code) state = { text: `incomplet : ${t.missing_cost.length} nœud(s) sans prix`, cls: 'over' }
  else state = { text: data.readOnly ? 'figé (snapshot publié)' : 'à jour' }

  return (
    <section className="syn" aria-label="Synthèse">
      <div className="synh">
        <h2>Synthèse</h2>
        <PriceStatePill data={data} />
      </div>
      <p className="calc-state small">
        État du calcul :{' '}
        <span className={state.cls} role={state.status ? 'status' : undefined}>
          {state.text}
        </span>
      </p>
      <div className="totals total" aria-busy={data.busy}>
        <span className="lb">Coût par véhicule</span>
        <strong className={`vl ${t?.cost_error_code ? 'over' : ''}`} title={t?.cost_error ?? undefined}>
          {t?.cost ? formatMoney(t.cost) : 'incomplet'}
        </strong>
        <RootBudget data={data} />
      </div>
      <dl className="dl">
        <dt>Disponibilité</dt>
        <dd>
          {planning ? (
            <>
              {planning.lead_time_days} j
              {planning.allowed_days !== null && (
                <span className={planning.target_met ? 't-ok' : 't-over'}> (cible {planning.allowed_days} j)</span>
              )}
            </>
          ) : t ? (
            `${t.lead_time_days} j`
          ) : (
            'n/d'
          )}
        </dd>
        <dt>CO₂</dt>
        <dd>{t ? `${formatDecimal(t.co2_kg, 1)} kg` : 'n/d'}</dd>
        <dt>Masse</dt>
        <dd>{t ? `${formatDecimal(t.mass_kg, 1)} kg` : 'n/d'}</dd>
        <dt>ETP</dt>
        <dd className={t?.labor_error ? 'over' : undefined} title={t?.labor_error ?? undefined}>
          {t?.fte ? formatDecimal(t.fte, 2) : 'n/d'}
        </dd>
        <dt>Hors budget</dt>
        <dd className={overBudget.length > 0 ? 't-over' : 'muted'} title={budgets.unavailable ?? 'Sous-systèmes dont le coût dépasse le budget cible'}>
          {budgets.unavailable !== null
            ? 'n/d'
            : overBudget.length > 0
              ? overBudget.map((n, i) => (
                  <span key={n.id}>
                    {i > 0 && ', '}
                    <button type="button" className="link over-link" onClick={() => onSelectNode(n.id)}>
                      {label(n)}
                    </button>
                  </span>
                ))
              : rootBudget || byBudget
                ? 'aucun sous-système'
                : 'aucun budget défini'}
        </dd>
      </dl>

      {subsystems.length > 0 && (
        <>
          <div className="h3">{byBudget ? 'Par sous-système · % du budget' : 'Par sous-système · coût par véhicule'}</div>
          <ul className={`sys ${byBudget ? 'by-budget' : ''}`}>
            {subsystems.map((n, i) => {
              const b = byBudget ? budgets.byNode.get(n.id) : undefined
              const width = byBudget ? budgetBarWidth(b?.percent ?? null) : widths[i]
              return (
                <li key={n.id}>
                  <button type="button" className="link sys-name" onClick={() => onSelectNode(n.id)} title={label(n)}>
                    {label(n)}
                  </button>
                  <span className="bar" aria-hidden="true">
                    {width !== null && <i className={b ? `fill-${b.state}` : undefined} style={{ width: `${width}%` }} />}
                  </span>
                  <span className={`n ${n.totals?.cost_error_code ? 'over' : ''}`}>
                    {n.totals?.cost ? formatMoney(n.totals.cost) : 'incomplet'}
                  </span>
                  {byBudget && (
                    <span className={`p ${budgetClass(b)}`} title={b ? budgetTitle(b) : 'Pas de budget cible sur ce sous-système'}>
                      {b ? budgetPercentText(b) : '—'}
                    </span>
                  )}
                </li>
              )
            })}
          </ul>
          <p className="small muted hint">
            {byBudget
              ? 'Barre : coût ÷ budget cible (vert < 90 %, orange ≥ 90 %, rouge au-delà de 100 %).'
              : `Barre : part du coût du véhicule${budgets.unavailable ? ` (${budgets.unavailable.toLowerCase()})` : ''}.`}
          </p>
        </>
      )}

      <PricesUsed data={data} />

      <Basis data={data} />

      {data.explanation.cost_method && (
        <>
          <div className="h3">Méthode</div>
          <div className="small">{methodLabel(data.explanation.cost_method)}</div>
        </>
      )}
    </section>
  )
}

// RootBudget : budget global (budget de la racine) et part consommée, sous le total (maquette I4-1).
function RootBudget({ data }: { data: BomData }) {
  const b = data.root ? data.budgets.byNode.get(data.root.id) : undefined
  if (data.budgets.unavailable !== null) return <span className="small muted">budget : {data.budgets.unavailable.toLowerCase()}</span>
  if (!b) return <span className="small muted">budget : aucun budget global défini</span>
  const width = budgetBarWidth(b.percent)
  return (
    <>
      <span className="small" title={budgetTitle(b)}>
        budget {formatMoney(b.target)} · <b className={budgetClass(b)}>{budgetPercentText(b)}</b>
      </span>
      <span className="budget-track" aria-hidden="true">
        {width !== null && <i className={`fill-${b.state}`} style={{ width: `${width}%` }} />}
      </span>
    </>
  )
}

function PriceStatePill({ data }: { data: BomData }) {
  if (data.priceSnapshotID === null) return <span className="state">prix en direct</span>
  return <span className="state">snapshot de prix #{data.priceSnapshotID}</span>
}

// usePriceSnapshots : liste des snapshots de prix (GET /api/v1/price-snapshots), relue quand la source change.
function usePriceSnapshots(current: number | null) {
  const [list, setList] = useState<PriceSnapshotRef[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    let cancelled = false
    api
      .listPriceSnapshots()
      .then((l) => {
        if (!cancelled) {
          setList(l)
          setError(null)
        }
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(errorMessage(e))
      })
    return () => {
      cancelled = true
    }
  }, [current])
  return { list, error }
}

// PricesUsed : source des prix (PR #23, déplacée de l'en-tête V1). En direct, ou un snapshot de prix existant
// nommé par son instant à la seconde près, ou « Figer maintenant ». L'API refuse un snapshot qui ne couvre
// pas la BOM (offres manquantes listées) : l'erreur s'affiche, la BOM ne bouge pas.
function PricesUsed({ data }: { data: BomData }) {
  const { list, error } = usePriceSnapshots(data.priceSnapshotID)
  const [all, setAll] = useState(false)
  const current = data.priceSnapshotID
  const sorted = list ?? []
  let shown = all ? sorted : sorted.slice(0, SHOWN_SNAPSHOTS)
  // Le snapshot attaché reste toujours visible, même ancien ou absent de la liste (liste en échec).
  const attached = current !== null ? sorted.find((s) => s.id === current) : undefined
  if (attached && !shown.includes(attached)) shown = [...shown, attached]
  const choose = (value: number | null) => {
    if (data.readOnly || value === current) return
    const bomID = data.target.id
    void data.run(() => (value === null ? api.unfreezePrices(bomID) : api.usePriceSnapshot(bomID, value)), { bomChanged: true })
  }
  return (
    <>
      <div className="h3" id="prices-used">
        Prix utilisés
      </div>
      <div className="snaps" role="radiogroup" aria-labelledby="prices-used">
        {!data.readOnly && (
          <label className={`snap ${current === null ? 'on' : ''}`}>
            <input type="radio" name="price-source" checked={current === null} disabled={data.busy} onChange={() => choose(null)} />
            <span>En direct</span>
            <span className="muted">dernière cotation</span>
          </label>
        )}
        {current !== null && !attached && (
          <label className="snap on">
            <input type="radio" name="price-source" checked disabled={data.readOnly} onChange={() => {}} />
            <span>#{current}</span>
            <span className="muted">{list === null && !error ? 'chargement…' : 'date indisponible'}</span>
          </label>
        )}
        {(data.readOnly ? shown.filter((s) => s.id === current) : shown).map((s) => (
          <label key={s.id} className={`snap ${s.id === current ? 'on' : ''}`}>
            <input type="radio" name="price-source" checked={s.id === current} disabled={data.busy || data.readOnly} onChange={() => choose(s.id)} />
            <span>
              #{s.id} · {formatDateTime(s.taken_at)}
            </span>
            <span className="muted">{scopeLabel(s)}</span>
          </label>
        ))}
      </div>
      {!data.readOnly && sorted.length > SHOWN_SNAPSHOTS && (
        <button type="button" className="link small" onClick={() => setAll((v) => !v)}>
          {all ? 'Moins de snapshots' : `Tous les snapshots (${sorted.length})…`}
        </button>
      )}
      {!data.readOnly && (
        <div className="row small snap-actions">
          <button type="button" className="btn" disabled={data.busy} onClick={() => void data.run(() => api.freezePrices(data.target.id), { bomChanged: true })}>
            Figer maintenant
          </button>
        </div>
      )}
      {error && <p className="error small">Snapshots de prix indisponibles : {error}</p>}
    </>
  )
}

function scopeLabel(s: PriceSnapshotRef): string {
  switch (s.scope) {
    case 'catalog':
      return 'catalogue'
    case 'bom':
      return `BOM n°${s.scope_id}`
    case 'supplier':
      return `fournisseur n°${s.scope_id}`
    case 'offer':
      return `offre n°${s.scope_id}`
  }
}

// Basis : sur quoi repose le total (devise, source des prix, taux et leur date), à la demande.
function Basis({ data }: { data: BomData }) {
  const e = data.explanation
  return (
    <details className="basis small">
      <summary>Sur quoi repose ce total</summary>
      <ul className="explain">
        <li>Devise : {e.currency}</li>
        <li>{e.price_snapshot_id === null ? 'Prix en direct (dernière cotation)' : `Prix figés : snapshot de prix #${e.price_snapshot_id}`}</li>
        {e.rates.map((r) => (
          <li key={`${r.from}-${r.to}`}>
            1 {r.from} = {r.rate} {r.to} ({r.source}, {formatDate(r.quoted_at)})
          </li>
        ))}
        {e.rates.length === 0 && <li>Aucune conversion de devise</li>}
      </ul>
    </details>
  )
}

function methodLabel(m: CostMethod): string {
  const basis = { purchase: 'Coût d’achat', landed: 'Coût rendu', full: 'Coût complet' }[m.basis] ?? m.basis
  const fx = { budget: 'taux budget', latest: 'dernier taux' }[m.fx] ?? m.fx
  const vol = m.program_volume ? ` · volume programme ${formatDecimal(String(m.program_volume), 0)}` : ''
  return `${basis} · ${fx}${vol}`
}
