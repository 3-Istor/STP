import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { decimalInput } from '../money'
import { errorMessage, request } from './api'
import { scoreError, updateSupplier } from './editApi'
import type { Contract, OfferView, Supplier, Tier } from './types'

function useModal() {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => { if (ref.current && !ref.current.open) ref.current.showModal() }, [])
  return ref
}
// SupplierDialog : création, ou modification (PATCH /suppliers/{id}) quand `current` est fourni.
export function SupplierDialog({ current, onClose, onSaved }: { current?: Supplier; onClose: () => void; onSaved: (supplier: Supplier) => void }) {
  const dialog = useModal()
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [scores, setScores] = useState({ quality_score: current?.quality_score ?? '', trust_score: current?.trust_score ?? '' })
  const scoreErrors = { quality_score: scoreError(scores.quality_score), trust_score: scoreError(scores.trust_score) }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (scoreErrors.quality_score || scoreErrors.trust_score) return
    const fields = new FormData(event.currentTarget)
    const body = { name: String(fields.get('name')).trim(), quality_score: decimalInput(scores.quality_score), trust_score: decimalInput(scores.trust_score) }
    setSaving(true); setError('')
    try {
      onSaved(current ? await updateSupplier(current.id, body) : await request<Supplier>('/suppliers', { method: 'POST', body: JSON.stringify(body) }))
    } catch (error) { setError(errorMessage(error)) } finally { setSaving(false) }
  }
  return <dialog ref={dialog} className="form-dialog" aria-labelledby="supplier-form-title" onClose={onClose} onCancel={onClose}>
    <div className="dialog-heading"><h2 id="supplier-form-title">{current ? 'Modifier le fournisseur' : 'Nouveau fournisseur'}</h2><button type="button" className="icon-button" aria-label="Fermer le formulaire fournisseur" onClick={() => dialog.current?.close()}>×</button></div>
    <form onSubmit={submit}>
      <label>Nom du fournisseur<input name="name" required maxLength={300} autoComplete="organization" defaultValue={current?.name ?? ''} /></label>
      <div className="form-columns"><label>Qualité (sur 5)<input type="text" inputMode="decimal" required placeholder="4.5" value={scores.quality_score} aria-invalid={scoreErrors.quality_score !== ''} onChange={(event) => setScores((old) => ({ ...old, quality_score: event.target.value }))} />{scoreErrors.quality_score && <small role="alert" className="form-error">{scoreErrors.quality_score}</small>}</label><label>Confiance (sur 5)<input type="text" inputMode="decimal" required placeholder="4.5" value={scores.trust_score} aria-invalid={scoreErrors.trust_score !== ''} onChange={(event) => setScores((old) => ({ ...old, trust_score: event.target.value }))} />{scoreErrors.trust_score && <small role="alert" className="form-error">{scoreErrors.trust_score}</small>}</label></div>
      <p className="muted">Notes entre 0 et 5. Séparez les décimales par un point.</p>
      {error && <p className="form-error" role="alert">{error}</p>}
      <div className="form-actions"><button type="button" className="secondary-button" onClick={() => dialog.current?.close()}>Annuler</button><button className="primary-button" disabled={saving}>{saving ? 'Enregistrement…' : current ? 'Enregistrer le fournisseur' : 'Créer le fournisseur'}</button></div>
    </form>
  </dialog>
}

export function ContractDialog({ supplierID, offers, contract, onClose, onSaved }: {
  supplierID: number; offers: OfferView[]; contract: Contract | null; onClose: () => void; onSaved: () => void
}) {
  const dialog = useModal()
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [selected, setSelected] = useState<number[]>(contract?.offer_ids ?? [])
  const [tiers, setTiers] = useState<Tier[]>(contract?.tiers ?? [{ min_quantity: '', discount_percent: '' }])
  const options = [...offers.map((view) => ({ id: view.offer.id, label: `${view.part.name} · offre #${view.offer.id}` })),
    ...(contract?.offer_ids.filter((id) => !offers.some((view) => view.offer.id === id)).map((id) => ({ id, label: `Offre #${id} (indisponible au catalogue)` })) ?? [])]
  function changeTier(index: number, field: keyof Tier, value: string) { setTiers((old) => old.map((tier, i) => i === index ? { ...tier, [field]: value } : tier)) }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const fields = new FormData(event.currentTarget)
    setSaving(true); setError('')
    try {
      const path = contract ? `/contracts/${contract.id}` : `/suppliers/${supplierID}/contracts`
      await request<Contract>(path, { method: contract ? 'PATCH' : 'POST', body: JSON.stringify({ starts_on: fields.get('starts_on'), ends_on: fields.get('ends_on'), conditions: fields.get('conditions'), offer_ids: selected, tiers }) })
      onSaved()
    } catch (error) { setError(errorMessage(error)) } finally { setSaving(false) }
  }
  const title = contract ? `Modifier le contrat #${contract.id}` : 'Nouveau contrat'
  return <dialog ref={dialog} className="form-dialog contract-dialog" aria-labelledby="contract-form-title" onClose={onClose} onCancel={onClose}>
    <div className="dialog-heading"><h2 id="contract-form-title">{title}</h2><button type="button" className="icon-button" aria-label="Fermer le formulaire contrat" onClick={() => dialog.current?.close()}>×</button></div>
    <form onSubmit={submit}>
      <div className="form-columns"><label>Date de début<input type="date" name="starts_on" required defaultValue={contract?.starts_on ?? ''} /></label><label>Date de fin<input type="date" name="ends_on" required defaultValue={contract?.ends_on ?? ''} /></label></div>
      <p className="muted">Le premier et le dernier jour sont inclus.</p>
      <label>Conditions du contrat<textarea name="conditions" rows={3} defaultValue={contract?.conditions ?? ''} /></label>
      <fieldset><legend>Offres couvertes</legend><div className="offer-choices">
        {options.map((option) => <label className="checkbox-label" key={option.id}><input type="checkbox" checked={selected.includes(option.id)} onChange={(event) => setSelected((old) => event.target.checked ? [...old, option.id] : old.filter((id) => id !== option.id))} />{option.label}</label>)}
        {!options.length && <p className="muted">Aucune offre disponible pour ce fournisseur.</p>}
      </div></fieldset>
      <fieldset><legend>Paliers de remise</legend>
        <p className="muted">Le palier atteint s’applique à toute la quantité commandée.</p>
        {tiers.map((tier, index) => <div className="tier-row" key={index}>
          <label>Quantité minimale (palier {index + 1})<input type="text" inputMode="decimal" required value={tier.min_quantity} onChange={(event) => changeTier(index, 'min_quantity', event.target.value)} placeholder="500" /></label>
          <label>Remise en % (palier {index + 1})<input type="text" inputMode="decimal" required value={tier.discount_percent} onChange={(event) => changeTier(index, 'discount_percent', event.target.value)} placeholder="5" /></label>
          <button type="button" className="icon-button" aria-label={`Supprimer le palier ${index + 1}`} onClick={() => setTiers((old) => old.filter((_, i) => i !== index))}>×</button>
        </div>)}
        <button type="button" className="text-button" onClick={() => setTiers((old) => [...old, { min_quantity: '', discount_percent: '' }])}>+ Ajouter un palier</button>
      </fieldset>
      {error && <p className="form-error" role="alert">{error}</p>}
      <div className="form-actions"><button type="button" className="secondary-button" onClick={() => dialog.current?.close()}>Annuler</button><button className="primary-button" disabled={saving}>{saving ? 'Enregistrement…' : contract ? 'Enregistrer le contrat' : 'Créer le contrat'}</button></div>
    </form>
  </dialog>
}
