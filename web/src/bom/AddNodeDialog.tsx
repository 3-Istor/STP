import { useEffect, useRef, useState, type FormEvent } from 'react'
import type { NewNode } from '../api.ts'
import type { Catalog } from '../catalog.ts'
import { formatMoney } from '../money.ts'
import { byName, normalize } from '../text.ts'

// Au-delà, la liste demande d'affiner la recherche : 800+ pièces ne s'affichent pas d'un coup.
const MAX_RESULTS = 50

type Props = {
  parentName: string
  parentID: number
  catalog: Catalog
  onAdd: (n: NewNode) => Promise<boolean>
  onClose: () => void
}

// AddNodeDialog : fenêtre d'ajout sous un nœud. Pièce : recherche (nom, catégorie), filtre par catégorie,
// choix de l'offre (fournisseur, prix, délai, origine) et de la quantité. Sous-ensemble : nom, jours de
// production, quantité. L'offre est toujours choisie par l'utilisateur, jamais automatiquement (spec).
export function AddNodeDialog({ parentName, parentID, catalog, onAdd, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const [kind, setKind] = useState<'part' | 'group'>(catalog.available ? 'part' : 'group')
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState('')
  const [partID, setPartID] = useState<number | null>(null)
  const [offerID, setOfferID] = useState<number | null>(null)
  const [name, setName] = useState('')
  const [days, setDays] = useState('0')
  const [qty, setQty] = useState('1')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (dialog.current && !dialog.current.open) dialog.current.showModal()
    search.current?.focus()
  }, [])

  const categories = [...new Set(catalog.parts.map((p) => p.category))].sort(byName.compare)
  const q = normalize(query)
  const matches = catalog.parts.filter(
    (p) => (category === '' || p.category === category) && (q === '' || normalize(`${p.name} ${p.category}`).includes(q)),
  )
  const part = partID !== null ? catalog.byPart.get(partID) : undefined

  function choosePart(id: number) {
    setPartID(id)
    // Une seule offre : elle est la seule possible ; sinon l'utilisateur choisit.
    const offers = catalog.byPart.get(id)?.offers ?? []
    setOfferID(offers.length === 1 ? offers[0].id : null)
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    const ok =
      kind === 'part'
        ? await onAdd({ parent_id: parentID, part_id: partID!, offer_id: offerID!, quantity: qty.trim() })
        : await onAdd({ parent_id: parentID, name: name.trim(), quantity: qty.trim(), production_days: Number.parseInt(days, 10) })
    setBusy(false)
    if (ok) onClose()
  }

  const ready = kind === 'group' ? name.trim() !== '' : part !== undefined && offerID !== null

  return (
    <dialog
      ref={dialog}
      className="add-dialog"
      aria-labelledby="add-title"
      onCancel={(e) => {
        e.preventDefault()
        onClose()
      }}
    >
      <form onSubmit={(e) => void submit(e)}>
        <div className="dialog-heading">
          <div>
            <p className="eyebrow">Ajouter sous « {parentName} »</p>
            <h2 id="add-title">{kind === 'part' ? 'Ajouter une pièce' : 'Ajouter un sous-ensemble'}</h2>
          </div>
          <button type="button" className="icon-button" aria-label="Fermer" onClick={onClose}>
            ×
          </button>
        </div>

        <div className="segmented" role="radiogroup" aria-label="Type de nœud">
          <button type="button" role="radio" aria-checked={kind === 'part'} disabled={!catalog.available} onClick={() => setKind('part')}>
            Pièce du catalogue
          </button>
          <button type="button" role="radio" aria-checked={kind === 'group'} onClick={() => setKind('group')}>
            Sous-ensemble
          </button>
        </div>

        {kind === 'part' ? (
          <>
            <div className="add-filters">
              <input
                ref={search}
                type="search"
                placeholder="Rechercher une pièce (nom, catégorie)…"
                aria-label="Rechercher une pièce"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
              <select aria-label="Catégorie" value={category} onChange={(e) => setCategory(e.target.value)}>
                <option value="">Toutes les catégories</option>
                {categories.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
            </div>
            <ul className="pick-list" role="listbox" aria-label="Pièces">
              {matches.slice(0, MAX_RESULTS).map((p) => (
                <li key={p.id} role="option" aria-selected={p.id === partID}>
                  <button type="button" onClick={() => choosePart(p.id)}>
                    <span className="pick-name">{p.name}</span>
                    <span className="pick-meta">
                      {p.category} · {p.offers.length} offre{p.offers.length > 1 ? 's' : ''}
                    </span>
                  </button>
                </li>
              ))}
              {matches.length === 0 && <li className="muted pick-empty">Aucune pièce ne correspond.</li>}
            </ul>
            {matches.length > MAX_RESULTS && (
              <p className="muted pick-more">
                {matches.length - MAX_RESULTS} autres pièces : affinez la recherche.
              </p>
            )}

            {part && (
              <fieldset>
                <legend>Offre pour « {part.name} »</legend>
                {part.offers.length === 0 && <p className="muted">Aucune offre pour cette pièce : elle ne peut pas être ajoutée.</p>}
                <div className="offer-choices">
                  {part.offers.map((o) => (
                    <label key={o.id} className="offer-choice" title={o.price_source ?? undefined}>
                      <input type="radio" name="offer" checked={offerID === o.id} onChange={() => setOfferID(o.id)} />
                      <span className="pick-name">{o.supplier}</span>
                      <span>{o.price ? formatMoney(o.price) : 'sans prix'}</span>
                      <span className="pick-meta">
                        {o.lead_time_days} j · {o.origin_country}
                      </span>
                    </label>
                  ))}
                </div>
              </fieldset>
            )}
          </>
        ) : (
          <div className="add-group">
            <label>
              Nom du sous-ensemble
              <input ref={search} required value={name} onChange={(e) => setName(e.target.value)} />
            </label>
            <label>
              Jours de production
              <input aria-label="Jours de production" type="number" min={0} step={1} value={days} onChange={(e) => setDays(e.target.value)} />
            </label>
          </div>
        )}

        <div className="form-actions">
          <label className="qty-field">
            Quantité
            <input aria-label="Quantité" required inputMode="decimal" size={6} value={qty} onChange={(e) => setQty(e.target.value)} />
          </label>
          <button type="button" className="secondary-button" onClick={onClose}>
            Annuler
          </button>
          <button type="submit" className="primary-button" disabled={!ready || busy}>
            Ajouter
          </button>
        </div>
      </form>
    </dialog>
  )
}
