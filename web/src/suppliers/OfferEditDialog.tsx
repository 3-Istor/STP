import { useEffect, useRef, useState, type FormEvent } from 'react'
import { compareAbsDecimal, decimalInput, sameDecimal } from '../money.ts'
import { COUNTRIES } from './countries.ts'
import { errorMessage } from './api'
import { daysError, decimalError, getOffer, scoreError, type OfferPatch } from './editApi.ts'
import type { Offer } from './types'

// OfferEditDialog : bornes de cotation, délai, qualité et origine d'une offre (PATCH /offers/{id}). Devise et
// fournisseur restent ceux de l'offre : l'API refuse de les changer une fois l'offre cotée. `save` renvoie le message
// d'erreur, null si enregistré.
export function OfferEditDialog({ offerID, onClose, save }: { offerID: number; onClose: () => void; save: (patch: OfferPatch) => Promise<string | null> }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [offer, setOffer] = useState<Offer | null>(null)
  const [form, setForm] = useState({ price_min: '', price_max: '', lead_time_days: '', quality_score: '', origin_country: '' })
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (dialog.current && !dialog.current.open) dialog.current.showModal()
    let active = true
    getOffer(offerID).then((o) => {
      if (!active) return
      setOffer(o)
      setForm({ price_min: o.price_min.amount, price_max: o.price_max.amount, lead_time_days: String(o.lead_time_days), quality_score: o.quality_score ?? '', origin_country: o.origin_country })
    }, (e: unknown) => { if (active) setError(errorMessage(e)) })
    return () => { active = false }
  }, [offerID])

  const set = (key: keyof typeof form) => (value: string) => setForm((old) => ({ ...old, [key]: value }))
  const min = decimalInput(form.price_min), max = decimalInput(form.price_max)
  const errs = {
    price_min: decimalError(form.price_min),
    price_max: decimalError(form.price_max) || (!decimalError(form.price_min) && compareAbsDecimal(min, max) > 0 ? 'Le prix maxi doit être au moins égal au prix mini.' : ''),
    lead_time_days: daysError(form.lead_time_days),
    quality_score: scoreError(form.quality_score),
  }
  const invalid = Object.values(errs).some(Boolean)

  const patch: OfferPatch = {}
  if (offer && !invalid) {
    if (!sameDecimal(min, offer.price_min.amount)) patch.price_min = min
    if (!sameDecimal(max, offer.price_max.amount)) patch.price_max = max
    if (Number(form.lead_time_days) !== offer.lead_time_days) patch.lead_time_days = Number(form.lead_time_days)
    if (!sameDecimal(decimalInput(form.quality_score), offer.quality_score ?? '-1')) patch.quality_score = decimalInput(form.quality_score)
    if (form.origin_country !== offer.origin_country) patch.origin_country = form.origin_country
  }
  const changed = Object.keys(patch).length > 0

  async function submit(e: FormEvent) {
    e.preventDefault()
    setSaving(true); setError('')
    const message = await save(patch)
    setSaving(false)
    if (message) setError(message)
    else onClose()
  }

  const input = (key: keyof typeof errs, label: string) => (
    <label>{label}
      <input inputMode="decimal" aria-invalid={errs[key] !== ''} value={form[key]} onChange={(e) => set(key)(e.target.value)} />
      {errs[key] && <small role="alert" className="form-error">{errs[key]}</small>}
    </label>
  )

  return (
    <dialog ref={dialog} className="form-dialog" aria-labelledby="offer-edit-title" onCancel={(e) => { e.preventDefault(); onClose() }}>
      <div className="dialog-heading">
        <div><p className="eyebrow">Offre #{offerID}</p><h2 id="offer-edit-title">Modifier l’offre</h2></div>
        <button type="button" className="icon-button" aria-label="Fermer le formulaire de l’offre" onClick={onClose}>×</button>
      </div>
      <p className="notice small">Le prix affiché reste la dernière cotation du simulateur ; le mini et le maxi bornent les prochaines cotations.</p>
      {!offer && !error && <p className="muted" role="status">Chargement de l’offre…</p>}
      {offer && (
        <form onSubmit={(e) => void submit(e)}>
          <div className="form-columns">
            {input('price_min', `Prix mini (${offer.currency})`)}
            {input('price_max', `Prix maxi (${offer.currency})`)}
            <label>Délai de livraison (jours)
              <input type="number" min={0} step={1} aria-invalid={errs.lead_time_days !== ''} value={form.lead_time_days} onChange={(e) => set('lead_time_days')(e.target.value)} />
              {errs.lead_time_days && <small role="alert" className="form-error">{errs.lead_time_days}</small>}
            </label>
            {input('quality_score', 'Qualité (sur 5)')}
            <label>Pays d’origine
              <select aria-label="Pays d’origine" value={form.origin_country} onChange={(e) => set('origin_country')(e.target.value)}>
                {COUNTRIES.map((c) => <option key={c.code} value={c.code}>{c.name} ({c.code})</option>)}
              </select>
            </label>
          </div>
          {error && <p className="form-error" role="alert">{error}</p>}
          <div className="form-actions">
            <button type="button" className="secondary-button" onClick={onClose}>Annuler</button>
            <button className="primary-button" disabled={saving || !changed || invalid}>{saving ? 'Enregistrement…' : 'Enregistrer l’offre'}</button>
          </div>
        </form>
      )}
      {!offer && error && <p className="form-error" role="alert">{error}</p>}
    </dialog>
  )
}
