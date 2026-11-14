import { useEffect, useRef, useState, type FormEvent } from 'react'
import { CurrencySelect } from '../CurrencySelect.tsx'
import { CurrenciesContext, currencyChoices, loadCurrencies } from '../currencies.ts'
import { decimalInput } from '../money.ts'
import { byName, normalize } from '../text.ts'
import { errorMessage, request } from './api'
import type { Supplier } from './types'

// Même liste que internal/catalog/country.go (ISO 3166-1 alpha-2) : l'API refuse tout autre code.
const COUNTRY_CODES = 'AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW'.split(' ')
const regionNames = new Intl.DisplayNames(['fr'], { type: 'region' })
const COUNTRIES = COUNTRY_CODES.map((code) => ({ code, name: regionNames.of(code) ?? code })).sort((a, b) => byName.compare(a.name, b.name))
const MAX_RESULTS = 50

type CatalogPart = { id: number; name: string; category: string; zone: string; offers: { supplier_id: number }[] }
type Money = { amount: string; currency: string }

// État du formulaire « nouvelle pièce » : tous les attributs que l'API exige pour une pièce (PartInput).
const emptyPart = {
  name: '', category: '', zone: '', mass_kg: '', co2_kg_per_unit: '', recycled_percent: '', recyclability_percent: '',
  manufacturing_cost: '', manufacturing_currency: 'EUR', production_days: '0', labor_hours: '', hourly_rate: '', hourly_currency: 'EUR',
}

// OfferDialog : ajoute une pièce vendue par le fournisseur, c'est-à-dire une offre (spec catalogue :
// une pièce a une ou plusieurs offres, une par fournisseur). La pièce est choisie dans le catalogue ou
// créée avec tous ses attributs. Aucun prix d'achat n'est saisi : seulement la fourchette mini / maxi
// dans laquelle la source de prix cote l'offre.
export function OfferDialog({ supplier, onClose, onSaved }: { supplier: Supplier; onClose: () => void; onSaved: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [parts, setParts] = useState<CatalogPart[] | null>(null)
  const [currencies, setCurrencies] = useState<string[]>([])
  const [loadError, setLoadError] = useState('')
  const [mode, setMode] = useState<'existing' | 'new'>('existing')
  const [query, setQuery] = useState('')
  const [partID, setPartID] = useState<number | null>(null)
  const [part, setPart] = useState(emptyPart)
  // Pièce déjà créée lors d'un essai précédent dont l'offre a échoué : on ne la recrée pas.
  const [created, setCreated] = useState<{ id: number; name: string } | null>(null)
  const [offer, setOffer] = useState({ currency: 'EUR', price_min: '', price_max: '', lead_time_days: '', quality_score: '', origin_country: '' })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (dialog.current && !dialog.current.open) dialog.current.showModal()
    let active = true
    request<CatalogPart[]>('/parts')
      .then((items) => { if (active) setParts([...items].sort((a, b) => byName.compare(a.name, b.name) || a.id - b.id)) })
      .catch((e: unknown) => { if (active) { setParts([]); setLoadError(errorMessage(e)) } })
    // Sans taux lisibles, seul l'euro est proposé : aucune devise n'est inventée.
    loadCurrencies().then((c) => { if (active) setCurrencies(c) }).catch(() => {})
    return () => { active = false }
  }, [])

  const q = normalize(query)
  const matches = (parts ?? []).filter((p) => q === '' || normalize(`${p.name} ${p.category} ${p.zone}`).includes(q))
  const categories = [...new Set((parts ?? []).map((p) => p.category))].sort(byName.compare)
  const zones = [...new Set((parts ?? []).map((p) => p.zone))].sort(byName.compare)
  const chosen = parts?.find((p) => p.id === partID)
  const setP = (key: keyof typeof emptyPart) => (value: string) => setPart((old) => ({ ...old, [key]: value }))
  const setO = (key: keyof typeof offer) => (value: string) => setOffer((old) => ({ ...old, [key]: value }))

  async function submit(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError('')
    try {
      let id = mode === 'existing' ? partID : created?.id ?? null
      if (mode === 'new' && id === null) {
        const money = (amount: string, currency: string): Money => ({ amount: decimalInput(amount), currency })
        const body = {
          name: part.name.trim(), category: part.category.trim(), zone: part.zone.trim(),
          mass_kg: decimalInput(part.mass_kg), co2_kg_per_unit: decimalInput(part.co2_kg_per_unit),
          recycled_percent: decimalInput(part.recycled_percent), recyclability_percent: decimalInput(part.recyclability_percent),
          manufacturing_cost: money(part.manufacturing_cost, part.manufacturing_currency),
          production_days: Number.parseInt(part.production_days, 10),
          labor_hours: decimalInput(part.labor_hours), hourly_rate: money(part.hourly_rate, part.hourly_currency),
        }
        const p = await request<{ id: number; name: string }>('/parts', { method: 'POST', body: JSON.stringify(body) })
        setCreated({ id: p.id, name: p.name })
        id = p.id
      }
      if (id === null) throw new Error('Choisissez la pièce vendue par le fournisseur.')
      await request(`/parts/${id}/offers`, {
        method: 'POST',
        body: JSON.stringify({
          supplier_id: supplier.id, currency: offer.currency,
          price_min: decimalInput(offer.price_min), price_max: decimalInput(offer.price_max),
          lead_time_days: Number.parseInt(offer.lead_time_days, 10), quality_score: decimalInput(offer.quality_score),
          origin_country: offer.origin_country,
        }),
      })
      onSaved()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  return (
    <CurrenciesContext value={currencyChoices(currencies, ['EUR'])}>
      <dialog ref={dialog} className="form-dialog offer-dialog" aria-labelledby="offer-form-title" onCancel={(e) => { e.preventDefault(); onClose() }}>
        <div className="dialog-heading">
          <div><p className="eyebrow">{supplier.name}</p><h2 id="offer-form-title">Nouvelle pièce vendue</h2></div>
          <button type="button" className="icon-button" aria-label="Fermer le formulaire d’offre" onClick={onClose}>×</button>
        </div>
        <form onSubmit={(e) => void submit(e)}>
          <div className="segmented" role="radiogroup" aria-label="Pièce">
            <button type="button" role="radio" aria-checked={mode === 'existing'} disabled={created !== null} onClick={() => setMode('existing')}>Pièce du catalogue</button>
            <button type="button" role="radio" aria-checked={mode === 'new'} onClick={() => setMode('new')}>Nouvelle pièce</button>
          </div>

          {mode === 'existing' ? <>
            <label>Pièce
              <input type="search" autoFocus placeholder="Rechercher (nom, catégorie, zone)…" aria-label="Rechercher une pièce" value={query} onChange={(e) => setQuery(e.target.value)} />
            </label>
            {loadError && <p className="form-error" role="alert">Catalogue illisible : {loadError}</p>}
            {parts === null && <p className="muted" role="status">Chargement du catalogue…</p>}
            <ul className="pick-list" role="listbox" aria-label="Pièces du catalogue">
              {matches.slice(0, MAX_RESULTS).map((p) => {
                const sold = p.offers.some((o) => o.supplier_id === supplier.id)
                return <li key={p.id} role="option" aria-selected={p.id === partID}>
                  <button type="button" onClick={() => setPartID(p.id)}>
                    <span className="pick-name">{p.name}</span>
                    <span className="pick-meta">{p.category} · {p.zone}{sold ? ' · déjà vendue par ce fournisseur' : ''}</span>
                  </button>
                </li>
              })}
              {parts !== null && matches.length === 0 && <li className="muted pick-empty">Aucune pièce ne correspond : créez-la avec « Nouvelle pièce ».</li>}
            </ul>
            {matches.length > MAX_RESULTS && <p className="muted pick-more">{matches.length - MAX_RESULTS} autres pièces : affinez la recherche.</p>}
            {chosen && <p className="muted">Pièce choisie : <strong>{chosen.name}</strong></p>}
          </> : created ? (
            <p className="notice" role="status">Pièce « {created.name} » créée (#{created.id}). Corrigez l’offre ci-dessous puis réessayez : la pièce ne sera pas recréée.</p>
          ) : (
            <fieldset>
              <legend>Pièce</legend>
              <label>Nom<input required autoFocus maxLength={300} value={part.name} onChange={(e) => setP('name')(e.target.value)} /></label>
              <div className="form-columns">
                <label>Catégorie<input required list="offer-categories" value={part.category} onChange={(e) => setP('category')(e.target.value)} /></label>
                <label>Zone de la voiture<input required list="offer-zones" value={part.zone} onChange={(e) => setP('zone')(e.target.value)} /></label>
              </div>
              <datalist id="offer-categories">{categories.map((c) => <option key={c} value={c} />)}</datalist>
              <datalist id="offer-zones">{zones.map((z) => <option key={z} value={z} />)}</datalist>
              <div className="form-columns">
                <label>Masse (kg)<input required inputMode="decimal" value={part.mass_kg} onChange={(e) => setP('mass_kg')(e.target.value)} /></label>
                <label>CO2 (kg CO2e par unité)<input required inputMode="decimal" value={part.co2_kg_per_unit} onChange={(e) => setP('co2_kg_per_unit')(e.target.value)} /></label>
                <label>Matière recyclée (%)<input required inputMode="decimal" value={part.recycled_percent} onChange={(e) => setP('recycled_percent')(e.target.value)} /></label>
                <label>Recyclabilité (%)<input required inputMode="decimal" value={part.recyclability_percent} onChange={(e) => setP('recyclability_percent')(e.target.value)} /></label>
              </div>
              <div className="form-columns">
                <label>Coût de fabrication
                  <span className="money-field"><input required inputMode="decimal" aria-label="Coût de fabrication" value={part.manufacturing_cost} onChange={(e) => setP('manufacturing_cost')(e.target.value)} /><CurrencySelect aria-label="Devise du coût de fabrication" value={part.manufacturing_currency} onChange={setP('manufacturing_currency')} /></span>
                </label>
                <label>Délai de production (jours)<input required type="number" min={0} step={1} value={part.production_days} onChange={(e) => setP('production_days')(e.target.value)} /></label>
                <label>Main-d’œuvre (heures par unité)<input required inputMode="decimal" value={part.labor_hours} onChange={(e) => setP('labor_hours')(e.target.value)} /></label>
                <label>Taux horaire
                  <span className="money-field"><input required inputMode="decimal" aria-label="Taux horaire" value={part.hourly_rate} onChange={(e) => setP('hourly_rate')(e.target.value)} /><CurrencySelect aria-label="Devise du taux horaire" value={part.hourly_currency} onChange={setP('hourly_currency')} /></span>
                </label>
              </div>
            </fieldset>
          )}

          <fieldset>
            <legend>Offre de {supplier.name}</legend>
            <div className="form-columns">
              <label>Devise de l’offre<CurrencySelect aria-label="Devise de l’offre" value={offer.currency} onChange={setO('currency')} /></label>
              <label>Pays d’origine
                <select required aria-label="Pays d’origine" value={offer.origin_country} onChange={(e) => setO('origin_country')(e.target.value)}>
                  <option value="" disabled>Choisir un pays</option>
                  {COUNTRIES.map((c) => <option key={c.code} value={c.code}>{c.name} ({c.code})</option>)}
                </select>
              </label>
              <label>Prix mini<input required inputMode="decimal" value={offer.price_min} onChange={(e) => setO('price_min')(e.target.value)} /></label>
              <label>Prix maxi<input required inputMode="decimal" value={offer.price_max} onChange={(e) => setO('price_max')(e.target.value)} /></label>
              <label>Délai de livraison (jours)<input required type="number" min={0} step={1} value={offer.lead_time_days} onChange={(e) => setO('lead_time_days')(e.target.value)} /></label>
              <label>Qualité (sur 5)<input required inputMode="decimal" value={offer.quality_score} onChange={(e) => setO('quality_score')(e.target.value)} /></label>
            </div>
            <p className="muted">Pas de prix d’achat à saisir : le prix affiché sera la dernière cotation, comprise entre le mini et le maxi.</p>
          </fieldset>

          {error && <p className="form-error" role="alert">{error}</p>}
          <div className="form-actions">
            <button type="button" className="secondary-button" onClick={onClose}>Annuler</button>
            <button className="primary-button" disabled={saving || (mode === 'existing' && partID === null)}>{saving ? 'Ajout…' : 'Ajouter la pièce'}</button>
          </div>
        </form>
      </dialog>
    </CurrenciesContext>
  )
}
