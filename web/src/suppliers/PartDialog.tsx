import { useEffect, useRef, useState, type FormEvent } from 'react'
import { CurrencySelect } from '../CurrencySelect.tsx'
import { decimalInput, sameDecimal, type Money } from '../money.ts'
import { errorMessage } from './api'
import { daysError, decimalError, getPart, type PartAttrs, type PartPatch } from './editApi.ts'

const DECIMALS = [
  ['mass_kg', 'Masse (kg)', undefined],
  ['co2_kg_per_unit', 'CO₂ (kg CO₂e par unité)', undefined],
  ['recycled_percent', 'Matière recyclée (%)', '100'],
  ['recyclability_percent', 'Recyclabilité (%)', '100'],
  ['labor_hours', 'Main-d’œuvre (heures par unité)', undefined],
] as const
type DecimalKey = (typeof DECIMALS)[number][0]
type MoneyKey = 'manufacturing_cost' | 'hourly_rate'
const MONEYS: [MoneyKey, string][] = [['manufacturing_cost', 'Coût de fabrication'], ['hourly_rate', 'Taux horaire']]

const text = (v: string | number | null) => (v === null ? '' : String(v))

// PartDialog : attributs catalogue d'une pièce (PATCH /parts/{id}). `save` renvoie le message d'erreur, null si
// enregistré. Seuls les champs modifiés partent ; un champ non renseigné et laissé vide reste non renseigné.
export function PartDialog({ partID, onClose, save }: { partID: number; onClose: () => void; save: (patch: PartPatch) => Promise<string | null> }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [part, setPart] = useState<PartAttrs | null>(null)
  const [form, setForm] = useState<Record<string, string>>({})
  const [currencies, setCurrencies] = useState<Record<MoneyKey, string>>({ manufacturing_cost: 'EUR', hourly_rate: 'EUR' })
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (dialog.current && !dialog.current.open) dialog.current.showModal()
    let active = true
    getPart(partID).then((p) => {
      if (!active) return
      setPart(p)
      setForm({
        ...Object.fromEntries(DECIMALS.map(([k]) => [k, text(p[k])])),
        manufacturing_cost: text(p.manufacturing_cost?.amount ?? null), hourly_rate: text(p.hourly_rate?.amount ?? null),
        production_days: text(p.production_days),
      })
      setCurrencies({ manufacturing_cost: p.manufacturing_cost?.currency ?? 'EUR', hourly_rate: p.hourly_rate?.currency ?? 'EUR' })
    }, (e: unknown) => { if (active) setError(errorMessage(e)) })
    return () => { active = false }
  }, [partID])

  const set = (key: string) => (value: string) => setForm((old) => ({ ...old, [key]: value }))
  const initial = (key: string) => (part ? text(key in part ? (part as unknown as Record<string, string | number | null>)[key] : null) : '')
  const unset = (key: string) => initial(key) === '' && (form[key] ?? '') === ''
  const errorOf = (key: string, max?: string) => (unset(key) ? '' : key === 'production_days' ? daysError(form[key] ?? '') : decimalError(form[key] ?? '', max))

  const patch: PartPatch = {}
  const errors: string[] = []
  if (part) {
    for (const [key, , max] of DECIMALS) {
      const e = errorOf(key, max)
      if (e) errors.push(e)
      else if (!unset(key) && !sameDecimal(decimalInput(form[key]), decimalInput(initial(key) || '0'))) patch[key as DecimalKey] = decimalInput(form[key])
    }
    for (const [key] of MONEYS) {
      const e = errorOf(key)
      if (e) errors.push(e)
      else if (!unset(key)) {
        const was = part[key]
        if (!was || !sameDecimal(decimalInput(form[key]), was.amount) || currencies[key] !== was.currency) {
          patch[key] = { amount: decimalInput(form[key]), currency: currencies[key] } satisfies Money
        }
      }
    }
    const days = errorOf('production_days')
    if (days) errors.push(days)
    else if (!unset('production_days') && Number(form.production_days) !== part.production_days) patch.production_days = Number(form.production_days)
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

  const field = (key: string, label: string, max?: string) => (
    <label key={key}>{label}
      <input inputMode="decimal" aria-invalid={errorOf(key, max) !== ''} value={form[key] ?? ''} onChange={(e) => set(key)(e.target.value)} />
      {errorOf(key, max) && <small role="alert" className="form-error">{errorOf(key, max)}</small>}
    </label>
  )

  return (
    <dialog ref={dialog} className="form-dialog offer-dialog" aria-labelledby="part-edit-title" onCancel={(e) => { e.preventDefault(); onClose() }}>
      <div className="dialog-heading">
        <div><p className="eyebrow">Pièce #{partID}</p><h2 id="part-edit-title">Modifier la pièce{part ? ` « ${part.name} »` : ''}</h2></div>
        <button type="button" className="icon-button" aria-label="Fermer le formulaire de la pièce" onClick={onClose}>×</button>
      </div>
      <p className="notice small">Modifie le catalogue : tous les brouillons qui utilisent cette pièce. Les versions publiées ne changent pas.</p>
      {!part && !error && <p className="muted" role="status">Chargement de la pièce…</p>}
      {part && (
        <form onSubmit={(e) => void submit(e)}>
          <div className="form-columns">
            {DECIMALS.slice(0, 4).map(([key, label, max]) => field(key, label, max))}
            {MONEYS.map(([key, label]) => (
              <label key={key}>{label}
                <span className="money-field">
                  <input inputMode="decimal" aria-label={label} aria-invalid={errorOf(key) !== ''} value={form[key] ?? ''} onChange={(e) => set(key)(e.target.value)} />
                  <CurrencySelect aria-label={`Devise : ${label.toLowerCase()}`} value={currencies[key]} onChange={(c) => setCurrencies((old) => ({ ...old, [key]: c }))} />
                </span>
                {errorOf(key) && <small role="alert" className="form-error">{errorOf(key)}</small>}
              </label>
            ))}
            <label>Délai de production (jours)
              <input type="number" min={0} step={1} aria-invalid={errorOf('production_days') !== ''} value={form.production_days ?? ''} onChange={(e) => set('production_days')(e.target.value)} />
              {errorOf('production_days') && <small role="alert" className="form-error">{errorOf('production_days')}</small>}
            </label>
            {field('labor_hours', DECIMALS[4][1])}
          </div>
          {error && <p className="form-error" role="alert">{error}</p>}
          <div className="form-actions">
            <button type="button" className="secondary-button" onClick={onClose}>Annuler</button>
            <button className="primary-button" disabled={saving || !changed || errors.length > 0}>{saving ? 'Enregistrement…' : 'Enregistrer la pièce'}</button>
          </div>
        </form>
      )}
      {!part && error && <p className="form-error" role="alert">{error}</p>}
    </dialog>
  )
}
