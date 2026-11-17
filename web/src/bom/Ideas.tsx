import { useState, type FormEvent } from 'react'
import { api, errorMessage, type IdeaStatus, type IdeasSummary } from '../api.ts'
import { absMoney, decimalInput, decimalSign, formatMoney } from '../money.ts'
import { useAppDialog } from '../useAppDialog.tsx'
import { useLoad } from './useLoad.ts'
import type { BomData } from './useBomData.ts'

// Pistes de réduction (maquette I4-6, ADR 0033, D8 du lot B) : liste, ajout, statut, suppression. Total des
// pistes ouvertes, écart à la cible (coût − budget global) et reste à trouver (écart − pistes) sont calculés
// par l'API et signés : négatif = couvert. Le front n'additionne rien.

export const ideaStatusLabels: Record<IdeaStatus, string> = {
  idea: 'idée',
  in_progress: 'en cours',
  to_validate: 'à valider',
  done: 'faite',
  dropped: 'abandonnée',
}

export function Ideas({ data }: { data: BomData }) {
  const bomID = data.target.id
  const [version, setVersion] = useState(0)
  const value = useLoad(`ideas-${bomID}-${data.version}-${version}`, () => api.ideas(bomID))
  const [error, setError] = useState<string | null>(null)
  const dialogs = useAppDialog()
  const act = (action: () => Promise<unknown>) => {
    setError(null)
    action().then(
      () => setVersion((v) => v + 1),
      (e: unknown) => setError(errorMessage(e)),
    )
  }
  if (data.readOnly) return null
  return (
    <section className="ideas sec" aria-label="Pistes de réduction">
      {dialogs.element}
      {value.status === 'loading' && <p className="muted small">Lecture des pistes…</p>}
      {value.status === 'error' && <p className="error small">Pistes indisponibles : {value.error}</p>}
      {value.status === 'ready' && <IdeaList s={value.data} data={data} act={act} confirmDelete={(title) => dialogs.confirm({ title: `Supprimer la piste « ${title} » ?`, message: 'La piste est retirée définitivement.', confirmLabel: 'Supprimer', danger: true })} />}
      {error && (
        <p className="error small" role="alert">
          {error}
        </p>
      )}
      <NewIdea currency={data.currency} onAdd={(idea) => act(() => api.createIdea(bomID, idea))} />
    </section>
  )
}

function Gap({ s }: { s: IdeasSummary }) {
  if (!s.gap_to_target) return <span className="muted">{s.gap_error ?? 'pas de budget global'}</span>
  return decimalSign(s.gap_to_target.amount) <= 0 ? (
    <span className="t-ok">budget tenu, marge de {formatMoney(absMoney(s.gap_to_target))}</span>
  ) : (
    <span className="t-over">{formatMoney(s.gap_to_target)} au-dessus de la cible</span>
  )
}

function IdeaList({ s, data, act, confirmDelete }: { s: IdeasSummary; data: BomData; act: (a: () => Promise<unknown>) => void; confirmDelete: (title: string) => Promise<boolean> }) {
  const bomID = data.target.id
  return (
    <>
      <h3 className="ideas-h">
        Pistes de réduction · <Gap s={s} />
      </h3>
      {s.ideas.length === 0 && <p className="muted small">Aucune piste pour ce brouillon.</p>}
      <ul className="idea-list">
        {s.ideas.map((i) => (
          <li key={i.id} className={i.status === 'dropped' ? 'dropped' : undefined}>
            <span className="idea-title" title={`${i.owner} · créée le ${i.created_at.slice(0, 10).split('-').reverse().join('/')}`}>
              {i.title}
              <span className="muted small"> · {i.owner}{i.draft_bom_id !== null ? (i.draft_bom_id === bomID ? ' · dans ce brouillon' : ` · brouillon #${i.draft_bom_id}`) : ''}</span>
            </span>
            <span className="n t-ok">{formatMoney(i.estimated_gain)}</span>
            <select aria-label={`Statut de ${i.title}`} value={i.status} onChange={(e) => act(() => api.patchIdea(bomID, i.id, { status: e.target.value as IdeaStatus }))}>
              {(Object.keys(ideaStatusLabels) as IdeaStatus[]).map((st) => (
                <option key={st} value={st}>
                  {ideaStatusLabels[st]}
                </option>
              ))}
            </select>
            <button
              type="button"
              className="ib"
              aria-label={`Supprimer ${i.title}`}
              title="Supprimer"
              onClick={() =>
                void confirmDelete(i.title).then((ok) => {
                  if (ok) act(() => api.deleteIdea(bomID, i.id))
                })
              }
            >
              ×
            </button>
          </li>
        ))}
      </ul>
      <dl className="dl ideas-totals">
        <dt>
          <b>Total des pistes ouvertes</b>
        </dt>
        <dd>
          <b>{formatMoney(s.open_total)}</b>
        </dd>
        <dt>Reste à trouver</dt>
        <dd>
          {s.remaining === null ? (
            <span className="muted">n/d</span>
          ) : decimalSign(s.remaining.amount) <= 0 ? (
            <span className="t-ok">rien : couvert avec {formatMoney(absMoney(s.remaining))} d’avance</span>
          ) : (
            <span className="t-over">{formatMoney(s.remaining)}</span>
          )}
        </dd>
      </dl>
      <p className="muted small">Ouvertes : idée, en cours, à valider. Écart à la cible = coût actuel − budget global.</p>
    </>
  )
}

function NewIdea({ currency, onAdd }: { currency: string; onAdd: (idea: Parameters<typeof api.createIdea>[1]) => void }) {
  const [title, setTitle] = useState('')
  const [owner, setOwner] = useState('')
  const [gain, setGain] = useState('')
  const amount = decimalInput(gain)
  const valid = title.trim() !== '' && owner.trim() !== '' && /^\d+(\.\d+)?$/.test(amount)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid) return
    onAdd({ title: title.trim(), owner: owner.trim(), estimated_gain: { amount, currency }, status: 'idea', draft_bom_id: null })
    setTitle('')
    setGain('')
  }
  return (
    <details className="new-idea">
      <summary>+ Ajouter une piste</summary>
      <form className="stack" onSubmit={submit}>
        <input aria-label="Titre de la piste" placeholder="Titre (ex. Jante alu → tôle)" value={title} onChange={(e) => setTitle(e.target.value)} />
        <div className="row">
          <input aria-label="Porteur" placeholder="Porteur" value={owner} onChange={(e) => setOwner(e.target.value)} />
          <input aria-label={`Gain estimé (${currency})`} placeholder={`Gain / véh. (${currency})`} inputMode="decimal" size={10} value={gain} onChange={(e) => setGain(e.target.value)} />
          <button type="submit" className="btn pri" disabled={!valid}>
            Ajouter
          </button>
        </div>
      </form>
    </details>
  )
}
