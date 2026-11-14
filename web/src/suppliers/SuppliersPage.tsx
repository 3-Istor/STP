import { useEffect, useState } from 'react'
import { errorMessage, request } from './api'
import { ContractDialog, SupplierDialog } from './Forms'
import { HistoryDialog } from './HistoryDialog'
import { formatDay, formatDecimal, formatMoney, formatUTC, formatVariation } from './format'
import { DISPLAY_CURRENCY, displayStats } from './display'
import { monthlyChart, monthlySeries } from './graph'
import { byName, normalize } from '../text'
import type { Contract, OffersPage, OfferView, Supplier } from './types'

type StreamState = 'connecting' | 'connected' | 'unavailable'
type PageState = { supplierID: number; data: OffersPage }
function sortByName(items: Supplier[]) { return [...items].sort((a, b) => byName.compare(a.name, b.name) || a.id - b.id) }

type ContractsState = { supplierID: number; asOf: string | null; data: Contract[] }

export default function SuppliersPage() {
  const [suppliers, setSuppliers] = useState<Supplier[]>([])
  const [supplierLoading, setSupplierLoading] = useState(true)
  const [supplierError, setSupplierError] = useState('')
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const [page, setPage] = useState<PageState | null>(null)
  const [contracts, setContracts] = useState<ContractsState | null>(null)
  const [finishedRequest, setFinishedRequest] = useState<{ supplierID: number; version: number } | null>(null)
  const [pageError, setPageError] = useState('')
  const [contractsError, setContractsError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const [stream, setStream] = useState<StreamState>('connecting')
  const [tab, setTab] = useState<'offers' | 'contracts'>('offers')
  const [createSupplier, setCreateSupplier] = useState(false)
  const [contractForm, setContractForm] = useState<{ supplierID: number; contract: Contract | null } | null>(null)
  const [historyID, setHistoryID] = useState<number | null>(null)
  const [offerQuery, setOfferQuery] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    let active = true
    request<Supplier[]>('/suppliers', { signal: controller.signal })
      .then((items) => { if (active) { const sorted = sortByName(items); setSuppliers(sorted); setSelectedID((old) => old ?? sorted[0]?.id ?? null) } })
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
  const historyView = currentPage?.offers.find((view) => view.offer.id === historyID)
  // Offres triées par nom de pièce, filtrées par la recherche (nom, catégorie, zone ou numéro d'offre).
  const query = normalize(offerQuery)
  const shownOffers = currentPage ? [...currentPage.offers]
    .sort((a, b) => byName.compare(a.part.name, b.part.name) || a.offer.id - b.offer.id)
    .filter((view) => query === '' || normalize([view.part.name, view.part.category, view.part.zone, `#${view.offer.id}`].filter(Boolean).join(' ')).includes(query)) : []
  function refreshData() { setPageError(''); setContractsError(''); setRefresh((version) => version + 1) }
  function chooseSupplier(id: number) {
    // Une sélection inchangée ne relance pas l'effet : conserver aussi ses erreurs visibles.
    if (id === selectedID) return
    setPageError(''); setContractsError(''); setSelectedID(id); setHistoryID(null); setContractForm(null); setOfferQuery('')
  }

  return <>
    <main id="main-content" className="supplier-workspace">
      <aside className="supplier-sidebar" aria-label="Fournisseurs">
        <div className="sidebar-heading"><div><p className="eyebrow">Catalogue</p><h2>Fournisseurs <span className="count">{suppliers.length}</span></h2></div></div>
        <button className="primary-button add-supplier" disabled={supplierLoading} onClick={() => setCreateSupplier(true)}>+ Nouveau fournisseur</button>
        {supplierLoading && <p className="muted" role="status">Chargement des fournisseurs…</p>}
        {supplierError && <p className="form-error" role="alert">{supplierError}</p>}
        {!supplierLoading && !supplierError && !suppliers.length && <p className="muted">Ajoutez votre premier fournisseur pour retrouver ses offres et ses contrats.</p>}
        <nav className="supplier-list" aria-label="Choisir un fournisseur">{suppliers.map((item) => <button type="button" key={item.id} aria-label={`Fournisseur ${item.name}`} aria-current={item.id === selectedID ? 'page' : undefined}
          onClick={() => chooseSupplier(item.id)}><span className="supplier-avatar" aria-hidden="true">{item.name.slice(0, 2).toLocaleUpperCase('fr')}</span><span className="supplier-list-name">{item.name}<small>Fournisseur #{item.id}</small></span><span aria-hidden="true" className="selection-chevron">›</span></button>)}</nav>
      </aside>
      <section className="supplier-content" aria-label="Détail du fournisseur">
        {supplier ? <>
          <div className="page-heading"><div><p className="eyebrow">Votre réseau d’approvisionnement</p><h1>{supplier.name}</h1><div className="supplier-ratings"><span>Qualité <strong>{formatDecimal(supplier.quality_score)}{supplier.quality_score !== null ? ' / 5' : ''}</strong></span><span>Confiance <strong>{formatDecimal(supplier.trust_score)}{supplier.trust_score !== null ? ' / 5' : ''}</strong></span></div></div><button type="button" className="secondary-button refresh-button" onClick={refreshData} aria-label="Actualiser les données">↻ <span>Actualiser</span></button></div>
          <div className="feed-status" aria-live="polite"><span className={`status-dot ${stream === 'connected' ? 'connected' : ''}`} aria-hidden="true" /><span>{stream === 'connected' ? 'Mises à jour automatiques actives' : stream === 'connecting' ? 'Connexion aux mises à jour…' : 'Mises à jour automatiques indisponibles, actualisez manuellement'}</span>{loading && <span className="updating">Actualisation…</span>}</div>
          <div className="tabs" role="tablist" aria-label="Vue du fournisseur" onKeyDown={(event) => {
            if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
              event.preventDefault()
              const next = event.key === 'Home' ? 'offers' : event.key === 'End' ? 'contracts' : tab === 'offers' ? 'contracts' : 'offers'
              setTab(next); document.getElementById(next === 'offers' ? 'offers-tab' : 'contracts-tab')?.focus()
            }
          }}>
            <button type="button" role="tab" id="offers-tab" aria-controls="offers-panel" aria-selected={tab === 'offers'} tabIndex={tab === 'offers' ? 0 : -1} onClick={() => setTab('offers')}>Offres {currentPage && <span className="count">{currentPage.offers.length}</span>}</button>
            <button type="button" role="tab" id="contracts-tab" aria-controls="contracts-panel" aria-selected={tab === 'contracts'} tabIndex={tab === 'contracts' ? 0 : -1} onClick={() => setTab('contracts')}>Contrats {currentContracts && <span className="count">{currentContracts.length}</span>}</button>
          </div>
          {tab === 'offers' ? <section role="tabpanel" id="offers-panel" aria-labelledby="offers-tab">
            {currentPage?.context?.mode === 'snapshot_fallback' && <p className="notice" role="status">Source de prix indisponible. Prix du snapshot #{currentPage.context.snapshot_id}, figé au {formatUTC(currentPage.context.as_of)}.</p>}
            <div className="section-heading"><div><h2>Offres du fournisseur</h2><p className="muted">{currentPage?.as_of ? `Prix observés au ${formatUTC(currentPage.as_of)}` : 'Les prix et leur historique apparaîtront à la réception de cotations.'}</p></div><span className="subtle-label">Devise d’origine</span></div>
            {pageError && <p className="form-error" role="alert">Impossible d’actualiser les offres : {pageError}{currentPage ? ' Les dernières données reçues restent affichées.' : ''}</p>}
            {!currentPage && loading && <div className="loading-state" role="status">Chargement des offres…</div>}
            {currentPage?.offers.length === 0 && <div className="empty-state"><strong>Aucune offre pour ce fournisseur</strong><p>Les offres rattachées à {supplier.name} apparaîtront ici.</p></div>}
            {!!currentPage?.offers.length && <input type="search" className="offer-search" placeholder="Rechercher une pièce…" aria-label="Rechercher une offre" value={offerQuery} onChange={(event) => setOfferQuery(event.target.value)} />}
            {!!currentPage?.offers.length && !shownOffers.length && <div className="empty-state"><strong>Aucune offre ne correspond à « {offerQuery} »</strong></div>}
            <div className="offer-list">{shownOffers.map((view) => <OfferCard key={view.offer.id} view={view} onHistory={() => setHistoryID(view.offer.id)} />)}</div>
          </section> : <section role="tabpanel" id="contracts-panel" aria-labelledby="contracts-tab">
            <div className="section-heading"><div><h2>Contrats et remises</h2><p className="muted">{contractAsOf ? `Validité évaluée au ${formatUTC(contractAsOf)}` : 'Validité non évaluée : aucune date de prix connue.'}</p></div><button type="button" className="primary-button" onClick={() => setContractForm({ supplierID: supplier.id, contract: null })}>+ Nouveau contrat</button></div>
            {contractsError && <p className="form-error" role="alert">Lecture des contrats impossible : {contractsError}</p>}
            {!currentContracts && loading && <p role="status" className="loading-state">Chargement des contrats…</p>}
            {currentContracts?.length === 0 && <div className="empty-state"><strong>Aucun contrat enregistré</strong><p>Définissez les offres couvertes et les paliers de remise de ce fournisseur.</p></div>}
            <div className="contract-list">{currentContracts?.map((contract) => <article className="contract-card" key={contract.id} aria-label={`Contrat ${contract.id}`}>
              <div className="contract-top"><h3>Contrat #{contract.id}</h3><span className={`contract-status ${contract.status ?? 'unknown'}`}>{contract.status === 'active' ? 'Actif' : contract.status === 'expired' ? 'Expiré' : contract.status === 'not_started' ? 'À venir' : 'Validité non évaluée'}</span></div>
              <p className="muted">Du {formatDay(contract.starts_on)} au {formatDay(contract.ends_on)} inclus</p>
              {contract.conditions && <p>{contract.conditions}</p>}
              <div className="contract-tiers">{contract.tiers.map((tier) => <span key={tier.min_quantity}>Dès {formatDecimal(tier.min_quantity)} unités <strong>−{formatDecimal(tier.discount_percent)} %</strong></span>)}{!contract.tiers.length && <span className="muted">Aucun palier de remise.</span>}</div>
              <div className="contract-bottom"><span className="muted">{contract.offer_ids.length} offre{contract.offer_ids.length > 1 ? 's' : ''} couverte{contract.offer_ids.length > 1 ? 's' : ''}</span><button type="button" className="text-button" onClick={() => setContractForm({ supplierID: supplier.id, contract })}>Modifier le contrat #{contract.id}</button></div>
            </article>)}</div>
          </section>}
        </> : <div className="welcome-panel"><span className="welcome-symbol" aria-hidden="true">◇</span><h1>Un regard clair sur vos fournisseurs</h1><p>Retrouvez les offres, les prix et les contrats au même endroit.</p>{!supplierLoading && <button className="primary-button" onClick={() => setCreateSupplier(true)}>Créer le premier fournisseur</button>}</div>}
      </section>
    </main>
    {createSupplier && <SupplierDialog onClose={() => setCreateSupplier(false)} onSaved={(item) => { setSuppliers((old) => sortByName([...old.filter((supplier) => supplier.id !== item.id), item])); chooseSupplier(item.id); setCreateSupplier(false) }} />}
    {contractForm && supplier?.id === contractForm.supplierID && <ContractDialog key={contractForm.contract?.id ?? 'new'} supplierID={supplier.id} offers={currentPage?.offers ?? []} contract={contractForm.contract} onClose={() => setContractForm(null)} onSaved={() => { setContractForm(null); setRefresh((version) => version + 1) }} />}
    {historyView && <HistoryDialog key={historyView.offer.id} view={historyView} onClose={() => setHistoryID(null)} />}
  </>
}

function OfferCard({ view, onHistory }: { view: OfferView; onHistory: () => void }) {
  const stats = view.statistics
  // Aperçu : clôture mensuelle de l'année en devise d'affichage (ADR 0030), le détail est dans l'historique.
  const points = monthlyChart(monthlySeries(displayStats(view)?.history ?? []), 160, 46, { top: 5, right: 5, bottom: 5, left: 5 })?.points.map((p) => ({ x: p.x, y: p.close })) ?? []
  // Toute la carte ouvre l'historique ; le bouton reste la cible clavier et lecteur d'écran.
  return <article className="offer-card" aria-label={`Offre ${view.offer.id} : ${view.part.name}`} onClick={onHistory}>
    <div className="offer-part"><div className="part-icon" aria-hidden="true">◇</div><div><h3>{view.part.name}</h3><p>{[view.part.category, view.part.zone].filter(Boolean).join(' · ') || `Pièce #${view.part.id}`}</p><small>Offre #{view.offer.id} · {view.offer.origin_country} · délai {view.offer.lead_time_days} j</small></div></div>
    <div className="offer-price"><span className="field-caption">Dernière cotation</span><strong data-testid={`price-${view.offer.id}`}>{formatMoney(stats?.current_quote?.price)}</strong><small>{stats?.current_quote ? formatUTC(stats.current_quote.quoted_at) : 'Aucun prix reçu'}</small></div>
    <div className="offer-evolution"><span className={`trend trend-${stats?.trend ?? 'unknown'}`} data-testid={`variation-${view.offer.id}`} data-trend={stats?.trend ?? 'unknown'}>{formatVariation(stats?.variation_percent, stats?.trend)}</span><p>Min. {stats?.minimum ? formatMoney(stats.minimum) : 'indisponible'}<br />Max. {stats?.maximum ? formatMoney(stats.maximum) : 'indisponible'}</p></div>
    <button type="button" className="sparkline-button" onClick={(event) => { event.stopPropagation(); onHistory() }} aria-label={`Historique de ${view.part.name}`}>
      {points.length ? <svg viewBox="0 0 160 46" aria-hidden="true"><polyline points={points.map((point) => `${point.x},${point.y}`).join(' ')} className="chart-line" />{points.length === 1 && <circle cx={points[0].x} cy={points[0].y} r="3" className="chart-dot" />}</svg> : <span className="no-history">Pas de courbe</span>}
      <span>Voir l’historique <span aria-hidden="true">↗</span></span>
    </button>
    {view.error && <p className="offer-warning">{view.error}</p>}
  </article>
}
