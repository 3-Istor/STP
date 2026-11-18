import { useEffect, useState } from 'react'
import { errorMessage, request } from './api'
import { ContractDialog, SupplierDialog } from './Forms'
import { HistoryDialog } from './HistoryDialog'
import { OfferDialog } from './OfferDialog'
import { OfferEditDialog } from './OfferEditDialog'
import { PartDialog } from './PartDialog'
import { archiveOffer, attempt, updateOffer, updatePart } from './editApi'
import { useAppDialog } from '../useAppDialog'
import { formatDay, formatDecimal, formatMoney, formatUTC, formatVariation } from './format'
import { DISPLAY_CURRENCY, displayStats } from './display'
import { bucketLabel, bucketSeries, bucketTitle, inPeriod, monthlyChart, PERIODS, type PeriodID } from './graph'
import { byName, normalize } from '../text'
import type { Contract, OffersPage, OfferView, Supplier } from './types'

type StreamState = 'connecting' | 'connected' | 'unavailable'
type PageState = { supplierID: number; data: OffersPage }
function sortByName(items: Supplier[]) { return [...items].sort((a, b) => byName.compare(a.name, b.name) || a.id - b.id) }

type ContractsState = { supplierID: number; asOf: string | null; data: Contract[] }

type Props = {
  // Fournisseur choisi, porté par l'URL (?supplier=) ; null : le premier par ordre alphabétique.
  selectedID: number | null
  onSelect: (id: number) => void
  // onSaved : un contrat ou une offre vient d'être enregistré ; dans une BOM (Sourcer), relit l'arbre pour que
  // totaux, remises et contrôles (badge « contrat expiré ») suivent.
  onSaved?: () => void
}

const score = (v: string | null) => (v === null ? '–' : formatDecimal(v))

const contractStatus = (c: Contract) =>
  c.status === 'active' ? 'Actif' : c.status === 'expired' ? 'Expiré' : c.status === 'not_started' ? 'À venir' : 'Validité non évaluée'

// SuppliersPage (maquette I4-5b) : fournisseurs, offres et contrats en tables denses. Reprend toutes les
// fonctions de la page Fournisseurs V1 : création de fournisseur, d'offre (pièce vendue), de contrat et
// modification, prix en direct (flux SSE), historique des prix (période au choix, détail en fenêtre).
export default function SuppliersPage({ selectedID: wanted, onSelect, onSaved }: Props) {
  const [suppliers, setSuppliers] = useState<Supplier[]>([])
  const [supplierLoading, setSupplierLoading] = useState(true)
  const [supplierError, setSupplierError] = useState('')
  const [page, setPage] = useState<PageState | null>(null)
  const [contracts, setContracts] = useState<ContractsState | null>(null)
  const [finishedRequest, setFinishedRequest] = useState<{ supplierID: number; version: number } | null>(null)
  const [pageError, setPageError] = useState('')
  const [contractsError, setContractsError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const [stream, setStream] = useState<StreamState>('connecting')
  const [createSupplier, setCreateSupplier] = useState(false)
  const [contractForm, setContractForm] = useState<{ supplierID: number; contract: Contract | null } | null>(null)
  const [historyID, setHistoryID] = useState<number | null>(null)
  const [chartOfferID, setChartOfferID] = useState<number | null>(null)
  const [period, setPeriod] = useState<PeriodID>('all')
  const [offerQuery, setOfferQuery] = useState('')
  const [offerForm, setOfferForm] = useState(false)
  const [supplierQuery, setSupplierQuery] = useState('')
  const [editSupplier, setEditSupplier] = useState(false)
  const [editOfferID, setEditOfferID] = useState<number | null>(null)
  const [editPartID, setEditPartID] = useState<number | null>(null)
  const [notice, setNotice] = useState('')
  const dialogs = useAppDialog()

  const selectedID = wanted ?? suppliers[0]?.id ?? null

  useEffect(() => {
    const controller = new AbortController()
    let active = true
    request<Supplier[]>('/suppliers', { signal: controller.signal })
      .then((items) => { if (active) setSuppliers(sortByName(items)) })
      .catch((error) => { if (active) setSupplierError(errorMessage(error)) })
      .finally(() => { if (active) setSupplierLoading(false) })
    return () => { active = false; controller.abort() }
  }, [])

  useEffect(() => {
    if (selectedID === null) return
    const id = selectedID
    const controller = new AbortController()
    let active = true
    async function load() {
      let asOf: string | null = null
      try {
        const data = await request<OffersPage>(`/suppliers/${id}/offers?currency=${DISPLAY_CURRENCY}`, { signal: controller.signal })
        if (!active) return
        asOf = data.as_of
        setPage({ supplierID: id, data })
      } catch (error) { if (!active) return; setPageError(errorMessage(error)) }
      try {
        const query = asOf ? `?as_of=${encodeURIComponent(asOf)}` : ''
        const data = await request<Contract[]>(`/suppliers/${id}/contracts${query}`, { signal: controller.signal })
        if (active) setContracts({ supplierID: id, asOf, data })
      } catch (error) { if (active) setContractsError(errorMessage(error)) }
      finally { if (active) setFinishedRequest({ supplierID: id, version: refresh }) }
    }
    void load()
    return () => { active = false; controller.abort() }
  }, [selectedID, refresh])

  useEffect(() => {
    const source = new EventSource('/api/v1/quotes/stream')
    const refreshPrices = () => { setPageError(''); setContractsError(''); setRefresh((version) => version + 1) }
    source.onopen = () => { setStream('connected'); refreshPrices() }
    source.onerror = () => setStream('unavailable')
    source.onmessage = refreshPrices
    source.addEventListener('tick', refreshPrices)
    source.addEventListener('status', refreshPrices)
    return () => { source.removeEventListener('tick', refreshPrices); source.removeEventListener('status', refreshPrices); source.onopen = null; source.onerror = null; source.onmessage = null; source.close() }
  }, [])

  const loading = selectedID !== null && (finishedRequest?.supplierID !== selectedID || finishedRequest.version !== refresh)
  const supplier = suppliers.find((item) => item.id === selectedID)
  const currentPage = page?.supplierID === selectedID ? page.data : null
  const currentContracts = contracts?.supplierID === selectedID ? contracts.data : null
  const contractAsOf = contracts?.supplierID === selectedID ? contracts.asOf : null
  // Filtre de la liste (nom ou numéro) ; le fournisseur affiché reste sélectionné même s'il est masqué.
  const supplierFilter = normalize(supplierQuery)
  const shownSuppliers = supplierFilter === '' ? suppliers : suppliers.filter((item) => normalize(`${item.name} #${item.id}`).includes(supplierFilter))
  const historyView = currentPage?.offers.find((view) => view.offer.id === historyID)
  // Offres triées par nom de pièce, filtrées par la recherche (nom, catégorie, zone ou numéro d'offre).
  const query = normalize(offerQuery)
  const shownOffers = currentPage ? [...currentPage.offers]
    .sort((a, b) => byName.compare(a.part.name, b.part.name) || a.offer.id - b.offer.id)
    .filter((view) => query === '' || normalize([view.part.name, view.part.category, view.part.zone, `#${view.offer.id}`].filter(Boolean).join(' ')).includes(query)) : []
  // Offre de la carte d'historique : celle choisie, sinon la première affichée qui a un historique.
  const chartView = currentPage?.offers.find((view) => view.offer.id === chartOfferID) ?? shownOffers.find((view) => view.statistics?.history.length)
  const offerName = new Map((currentPage?.offers ?? []).map((view) => [view.offer.id, view.part.name]))
  function refreshData() { setPageError(''); setContractsError(''); setRefresh((version) => version + 1) }
  // Après une modification du catalogue : relit les offres et, dans une BOM, l'arbre (totaux, impact).
  const edited = () => { refreshData(); onSaved?.() }
  const saveWith = async (call: () => Promise<unknown>) => { const message = await attempt(call); if (!message) edited(); return message }
  function archive(view: OfferView) {
    void dialogs.confirm({
      title: 'Archiver cette offre ?',
      message: `L’offre #${view.offer.id} de « ${view.part.name} » n’est plus proposée. Si une BOM ou un contrat l’utilise, elle est archivée et reste lisible ; sinon elle est supprimée.`,
      confirmLabel: 'Archiver', danger: true,
    }).then(async (ok) => {
      if (!ok) return
      setNotice(''); setPageError('')
      try { const r = await archiveOffer(view.offer.id); setNotice(r.archived ? `Offre #${view.offer.id} archivée.` : `Offre #${view.offer.id} supprimée (aucune BOM ni contrat ne l’utilisait).`); edited() } catch (error) { setPageError(errorMessage(error)) }
    })
  }
  function chooseSupplier(id: number) {
    // Une sélection inchangée ne relance pas l'effet : conserver aussi ses erreurs visibles.
    if (id === selectedID) return
    setPageError(''); setContractsError(''); onSelect(id); setHistoryID(null); setChartOfferID(null); setContractForm(null); setOfferQuery(''); setOfferForm(false)
  }

  return <div className="suppliers">
    <section className="block supplier-list-block" aria-label="Fournisseurs">
      <div className="tbar">
        <input type="search" className="supplier-search" placeholder="Rechercher…" aria-label="Filtrer les fournisseurs" value={supplierQuery} onChange={(event) => setSupplierQuery(event.target.value)} />
        <span className="count supplier-count">{supplierFilter === '' ? suppliers.length : `${shownSuppliers.length} / ${suppliers.length}`}</span>
        <span className="sp" />
        <button type="button" className="btn" disabled={supplierLoading} onClick={() => setCreateSupplier(true)}>+ Fournisseur</button>
      </div>
      {supplierLoading && <p className="muted in" role="status">Chargement des fournisseurs…</p>}
      {supplierError && <p className="form-error" role="alert">{supplierError}</p>}
      {!supplierLoading && !supplierError && !suppliers.length && <p className="muted in">Ajoutez votre premier fournisseur pour retrouver ses offres et ses contrats.</p>}
      {supplierFilter !== '' && !shownSuppliers.length && <p className="muted in">Aucun fournisseur ne correspond à « {supplierQuery} ».</p>}
      <div className="scroll">
        <table aria-label="Choisir un fournisseur">
          <thead><tr><th>Fournisseur</th><th className="n" title="Note qualité · note de confiance, sur 5">Notes /5</th></tr></thead>
          <tbody>{shownSuppliers.map((item) => <tr key={item.id} className={`clickable ${item.id === selectedID ? 'sel' : ''}`} onClick={() => chooseSupplier(item.id)}>
            <td><button type="button" className="link row-link" aria-label={`Fournisseur ${item.name}`} aria-current={item.id === selectedID ? 'true' : undefined} onClick={(event) => { event.stopPropagation(); chooseSupplier(item.id) }}>{item.name}</button></td>
            <td className="n" title={`Qualité ${formatDecimal(item.quality_score)} · confiance ${formatDecimal(item.trust_score)}`}>{score(item.quality_score)} · {score(item.trust_score)}</td>
          </tr>)}</tbody>
        </table>
      </div>
    </section>

    <div className="supplier-detail" aria-label="Détail du fournisseur">
      {supplier ? <>
        <section className="block supplier-head">
          <div className="in head-row">
            <div><p className="eyebrow">Fournisseur #{supplier.id}</p><h2>{supplier.name}</h2>
              <div className="muted small">Qualité <b>{formatDecimal(supplier.quality_score)}{supplier.quality_score !== null ? ' / 5' : ''}</b> · Confiance <b>{formatDecimal(supplier.trust_score)}{supplier.trust_score !== null ? ' / 5' : ''}</b></div></div>
            <div className="feed-status small" aria-live="polite"><span className={`dot ${stream === 'connected' ? 'ok' : stream === 'unavailable' ? 'over' : 'na'}`} aria-hidden="true" /> {stream === 'connected' ? 'Mises à jour automatiques actives' : stream === 'connecting' ? 'Connexion aux mises à jour…' : 'Mises à jour automatiques indisponibles, actualisez manuellement'}{loading && <span className="muted"> · actualisation…</span>}</div>
            <button type="button" className="btn" onClick={() => setEditSupplier(true)}>Modifier</button>
            <button type="button" className="btn" onClick={refreshData} aria-label="Actualiser les données">↻ Actualiser</button>
          </div>
        </section>

        <section className="block" aria-label={`Offres de ${supplier.name}`}>
          <h3>Offres de {supplier.name} {currentPage && <span className="count">{currentPage.offers.length}</span>}<span className="sp" />
            {!!currentPage?.offers.length && <input type="search" className="offer-search" placeholder="Rechercher une pièce…" aria-label="Rechercher une offre" value={offerQuery} onChange={(event) => setOfferQuery(event.target.value)} />}
            <button type="button" className="btn" onClick={() => setOfferForm(true)}>+ Offre</button></h3>
          {currentPage?.context?.mode === 'snapshot_fallback' && <p className="notice" role="status">Source de prix indisponible. Prix du snapshot #{currentPage.context.snapshot_id}, figé au {formatUTC(currentPage.context.as_of)}.</p>}
          {notice && <p className="notice" role="status">{notice}</p>}
          {pageError && <p className="form-error" role="alert">Impossible d’actualiser les offres : {pageError}{currentPage ? ' Les dernières données reçues restent affichées.' : ''}</p>}
          {!currentPage && loading && <p className="loading-state" role="status">Chargement des offres…</p>}
          {currentPage?.offers.length === 0 && <div className="empty-state"><strong>Aucune offre pour ce fournisseur</strong><p>Les offres rattachées à {supplier.name} apparaîtront ici.</p></div>}
          {!!currentPage?.offers.length && !shownOffers.length && <div className="empty-state"><strong>Aucune offre ne correspond à « {offerQuery} »</strong></div>}
          {!!shownOffers.length && <div className="scroll offers-scroll"><table className="offers-table">
            <thead><tr><th>Pièce</th><th className="n">Dernière cotation</th><th className="n">Fourchette cotée</th><th className="n">Variation</th><th className="n">Délai</th><th>Origine</th><th><span className="sr-only">Historique</span></th><th><span className="sr-only">Actions</span></th></tr></thead>
            <tbody>{shownOffers.map((view) => <OfferRow key={view.offer.id} view={view} selected={chartView?.offer.id === view.offer.id} onChart={() => setChartOfferID(view.offer.id)} onHistory={() => setHistoryID(view.offer.id)} onEdit={() => setEditOfferID(view.offer.id)} onEditPart={() => setEditPartID(view.part.id)} onArchive={() => archive(view)} />)}</tbody>
          </table></div>}
          <p className="muted small in">{currentPage?.as_of ? `Prix observés au ${formatUTC(currentPage.as_of)}, dans la devise de l’offre.` : 'Les prix et leur historique apparaîtront à la réception de cotations.'} Cliquez une ligne pour sa courbe de prix.</p>
        </section>

        {chartView && <PriceHistory view={chartView} period={period} onPeriod={setPeriod} onDetail={() => setHistoryID(chartView.offer.id)} />}

        <section className="block" aria-label="Contrats et remises">
          <h3>Contrats et remises {currentContracts && <span className="count">{currentContracts.length}</span>}<span className="sp" /><button type="button" className="btn" onClick={() => setContractForm({ supplierID: supplier.id, contract: null })}>+ Contrat</button></h3>
          <p className="muted small in">{contractAsOf ? `Validité évaluée au ${formatUTC(contractAsOf)}` : 'Validité non évaluée : aucune date de prix connue.'}</p>
          {contractsError && <p className="form-error" role="alert">Lecture des contrats impossible : {contractsError}</p>}
          {!currentContracts && loading && <p role="status" className="loading-state">Chargement des contrats…</p>}
          {currentContracts?.length === 0 && <div className="empty-state"><strong>Aucun contrat enregistré</strong><p>Définissez les offres couvertes et les paliers de remise de ce fournisseur.</p></div>}
          {!!currentContracts?.length && <div className="scroll"><table className="contracts-table">
            <thead><tr><th>Contrat</th><th>Offres couvertes</th><th>Paliers</th><th>Validité</th><th>Statut</th><th><span className="sr-only">Modifier</span></th></tr></thead>
            <tbody>{currentContracts.map((contract) => <tr key={contract.id} aria-label={`Contrat ${contract.id}`} className={contract.status === 'expired' ? 'expired' : undefined}>
              <td>#{contract.id}{contract.conditions && <div className="muted small conditions">{contract.conditions}</div>}</td>
              <td title={contract.offer_ids.map((id) => offerName.get(id) ?? `offre #${id}`).join(', ')}>{contract.offer_ids.length} offre{contract.offer_ids.length > 1 ? 's' : ''}{contract.offer_ids.length > 0 && <span className="muted"> ({contract.offer_ids.slice(0, 2).map((id) => offerName.get(id) ?? `#${id}`).join(', ')}{contract.offer_ids.length > 2 ? '…' : ''})</span>}</td>
              <td>{contract.tiers.length ? contract.tiers.map((tier) => `−${formatDecimal(tier.discount_percent)} % dès ${formatDecimal(tier.min_quantity)} u.`).join(' · ') : <span className="muted">aucun palier</span>}</td>
              <td className={contract.status === 'expired' ? 'over' : undefined}>{formatDay(contract.starts_on)} → {formatDay(contract.ends_on)}</td>
              <td><span className={`contract-status ${contract.status ?? 'unknown'}`}>{contractStatus(contract)}</span></td>
              <td className="n"><button type="button" className="link" aria-label={`Modifier le contrat #${contract.id}`} onClick={() => setContractForm({ supplierID: supplier.id, contract })}>Modifier</button></td>
            </tr>)}</tbody>
          </table></div>}
        </section>
      </> : !supplierLoading && <section className="block"><div className="in"><h2>Un regard clair sur vos fournisseurs</h2><p className="muted">Retrouvez les offres, les prix et les contrats au même endroit.</p><button className="btn pri" onClick={() => setCreateSupplier(true)}>Créer le premier fournisseur</button></div></section>}
    </div>
    {createSupplier && <SupplierDialog onClose={() => setCreateSupplier(false)} onSaved={(item) => { setSuppliers((old) => sortByName([...old.filter((s) => s.id !== item.id), item])); chooseSupplier(item.id); setCreateSupplier(false) }} />}
    {contractForm && supplier?.id === contractForm.supplierID && <ContractDialog key={contractForm.contract?.id ?? 'new'} supplierID={supplier.id} offers={currentPage?.offers ?? []} contract={contractForm.contract} onClose={() => setContractForm(null)} onSaved={() => { setContractForm(null); setRefresh((version) => version + 1); onSaved?.() }} />}
    {offerForm && supplier && <OfferDialog supplier={supplier} onClose={() => setOfferForm(false)} onSaved={() => { setOfferForm(false); refreshData(); onSaved?.() }} />}
    {editSupplier && supplier && <SupplierDialog current={supplier} onClose={() => setEditSupplier(false)} onSaved={(item) => { setSuppliers((old) => sortByName([...old.filter((s) => s.id !== item.id), item])); setEditSupplier(false) }} />}
    {editOfferID !== null && <OfferEditDialog key={editOfferID} offerID={editOfferID} onClose={() => setEditOfferID(null)} save={(patch) => saveWith(() => updateOffer(editOfferID, patch))} />}
    {editPartID !== null && <PartDialog key={editPartID} partID={editPartID} onClose={() => setEditPartID(null)} save={(patch) => saveWith(() => updatePart(editPartID, patch))} />}
    {dialogs.element}
    {historyView && <HistoryDialog key={historyView.offer.id} view={historyView} onClose={() => setHistoryID(null)} />}
  </div>
}

function OfferRow({ view, selected, onChart, onHistory, onEdit, onEditPart, onArchive }: { view: OfferView; selected: boolean; onChart: () => void; onHistory: () => void; onEdit: () => void; onEditPart: () => void; onArchive: () => void }) {
  const act = (fn: () => void) => (event: { stopPropagation: () => void }) => { event.stopPropagation(); fn() }
  const stats = view.statistics
  return (
    <tr className={`clickable offer-row ${selected ? 'sel' : ''}`} aria-label={`Offre ${view.offer.id} : ${view.part.name}`} onClick={onChart}>
      <td title={[`offre #${view.offer.id}`, view.part.category, view.part.zone].filter(Boolean).join(' · ')}><span className="part-name">{view.part.name}</span>{view.error && <div className="offer-warning">{view.error}</div>}</td>
      <td className="n"><span data-testid={`price-${view.offer.id}`}>{formatMoney(stats?.current_quote?.price)}</span></td>
      <td className="n muted">{formatMoney(view.offer.price_min)} – {formatMoney(view.offer.price_max)}</td>
      <td className="n"><span className={`trend trend-${stats?.trend ?? 'unknown'}`} data-testid={`variation-${view.offer.id}`} data-trend={stats?.trend ?? 'unknown'}>{formatVariation(stats?.variation_percent, stats?.trend)}</span></td>
      <td className="n">{view.offer.lead_time_days} j</td>
      <td>{view.offer.origin_country}</td>
      <td className="n"><button type="button" className="link" aria-label={`Historique de ${view.part.name}`} title="Toutes les cotations" onClick={(event) => { event.stopPropagation(); onHistory() }}>↗</button></td>
      <td className="n offer-actions">
        <button type="button" className="link" aria-label={`Modifier l’offre #${view.offer.id}`} onClick={act(onEdit)}>Offre</button>{' · '}
        <button type="button" className="link" aria-label={`Modifier la pièce ${view.part.name}`} onClick={act(onEditPart)}>Pièce</button>{' · '}
        <button type="button" className="link" aria-label={`Archiver l’offre #${view.offer.id}`} onClick={act(onArchive)}>Archiver</button>
      </td>
    </tr>
  )
}

const CHART = { width: 760, height: 200, padding: { top: 14, right: 20, bottom: 30, left: 90 } }

// PriceHistory : courbe de prix de l'offre choisie, période au choix (comptée depuis l'instant des statistiques
// de l'API). Chaque point est une cotation réelle (clôture de la période) ; min. et max. au survol.
function PriceHistory({ view, period, onPeriod, onDetail }: { view: OfferView; period: PeriodID; onPeriod: (p: PeriodID) => void; onDetail: () => void }) {
  const [hover, setHover] = useState<string | null>(null)
  const p = PERIODS.find((x) => x.id === period)!
  const shown = displayStats(view)
  const history = inPeriod(shown?.history ?? [], view.statistics?.as_of, p.months)
  const buckets = bucketSeries(history, p.unit)
  const chart = monthlyChart(buckets, CHART.width, CHART.height, CHART.padding)
  const every = Math.max(1, Math.ceil(buckets.length / 8)) // au plus ~8 étiquettes d'axe
  const active = buckets.find((b) => b.key === hover) ?? buckets.at(-1)
  return <section className="block" aria-label={`Historique du prix · ${view.part.name}`}>
    <h3>Historique du prix · {view.part.name}<span className="sp" />
      <span className="seg" role="radiogroup" aria-label="Période">{PERIODS.map((x) => <button key={x.id} type="button" role="radio" aria-checked={x.id === period} className={x.id === period ? 'on' : ''} onClick={() => onPeriod(x.id)}>{x.label}</button>)}</span>
    </h3>
    <div className="in">
      {chart ? <svg viewBox={`0 0 ${CHART.width} ${CHART.height}`} className="detail-chart" role="img" aria-label={`Prix de ${view.part.name}, ${buckets.length} point${buckets.length > 1 ? 's' : ''}, de ${formatMoney(chart.low.price)} à ${formatMoney(chart.high.price)}`}>
        <line x1={CHART.padding.left} y1={chart.top} x2={CHART.width - CHART.padding.right} y2={chart.top} className="chart-grid" />
        <line x1={CHART.padding.left} y1={chart.bottom} x2={CHART.width - CHART.padding.right} y2={chart.bottom} className="chart-grid" />
        <text x={CHART.padding.left - 8} y={chart.top + 4} className="chart-axis" textAnchor="end">{formatMoney(chart.high.price)}</text>
        <text x={CHART.padding.left - 8} y={chart.bottom + 4} className="chart-axis" textAnchor="end">{formatMoney(chart.low.price)}</text>
        <polyline points={chart.points.map((pt) => `${pt.x},${pt.close}`).join(' ')} className="chart-line" />
        {chart.points.map((pt, index) => {
          const on = active?.key === pt.month.key
          const half = chart.points.length > 1 ? (chart.points[1].x - chart.points[0].x) / 2 : CHART.width / 2
          return <g key={pt.month.key} className={on ? 'chart-month active' : 'chart-month'} onPointerEnter={() => setHover(pt.month.key)} onPointerDown={() => setHover(pt.month.key)}>
            <rect x={pt.x - half} y={0} width={half * 2} height={CHART.height} className="chart-hit" />
            {on && <line x1={pt.x} y1={chart.top} x2={pt.x} y2={chart.bottom} className="chart-cursor" />}
            <circle cx={pt.x} cy={pt.close} r={on ? 4.5 : 2.5} className={on ? 'chart-dot selected' : 'chart-dot'} />
            {(index % every === 0 || index === chart.points.length - 1) && <text x={pt.x} y={CHART.height - 8} className="chart-axis" textAnchor="middle">{bucketLabel(pt.month, index, p.unit)}</text>}
          </g>
        })}
      </svg> : <p className="empty-state">Aucune cotation sur cette période.</p>}
      {active && <p className="small" role="status">{bucketTitle(active, p.unit)} : clôture <b>{formatMoney(active.close.price)}</b> · min. {formatMoney(active.low.price)} · max. {formatMoney(active.high.price)} · {active.count} cotation{active.count > 1 ? 's' : ''}</p>}
      <p className="muted small">{shown?.converted ? `En ${DISPLAY_CURRENCY}, au taux coté le jour de chaque cotation (devise d’origine : ${view.offer.currency}).` : 'Dans la devise d’origine : conversion indisponible.'} Fourchette contractuelle de cotation : {formatMoney(view.offer.price_min)} – {formatMoney(view.offer.price_max)}. <button type="button" className="link" onClick={onDetail}>Toutes les cotations…</button></p>
    </div>
  </section>
}
