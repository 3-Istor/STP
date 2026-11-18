import { useState } from 'react'
import { api, ApiError, errorMessage, type CheckLevel, type PublicationCheck, type Snapshot, type Version } from '../../api.ts'
import { formatDateTime, formatMoney, formatPercent } from '../../money.ts'
import { bomRoute, href } from '../../router.ts'
import { useLoad } from '../useLoad.ts'
import type { BomData } from '../useBomData.ts'

type Props = {
  data: BomData
  onPublished: (s: Snapshot) => void
  onOpenDraft: (bomID: number) => void
  onFix: (nodeID: number) => void // « Corriger » : nœud concerné, étape Structurer
}

// Publier (maquette I4-7, ADR 0033) : « Est-ce prêt ? ». Contrôles avant publication recalculés par l'API
// (ok, avertissement à justifier, bloquant), note de décision, proposé par, validé par, puis publication :
// 409 = bloqué par un contrôle, 422 = avertissement sans justification (surligné). Historique des versions.
// Un snapshot publié montre ses contrôles figés, sa trace, et permet d'en tirer un nouveau brouillon.
export function Publier({ data, onPublished, onOpenDraft, onFix }: Props) {
  const [published, setPublished] = useState(0) // relit contrôles et versions après une publication
  const bomID = data.snapshot ? data.snapshot.bom_id : data.target.id
  return (
    <>
      {data.snapshot ? <SnapshotTrace data={data} onOpenDraft={onOpenDraft} /> : <PublishDraft data={data} reload={published} onFix={onFix} onPublished={(s) => { setPublished((n) => n + 1); onPublished(s) }} />}
      <Versions bomID={bomID} current={`${data.target.kind === 'draft' ? 'bom' : 'snapshot'}:${data.target.id}`} reload={`${data.version}-${published}`} />
    </>
  )
}

const levelIcon: Record<CheckLevel, { icon: string; label: string }> = {
  ok: { icon: '✓', label: 'contrôle passé' },
  warn: { icon: '!', label: 'avertissement : justification requise' },
  block: { icon: '×', label: 'bloquant' },
}

function PublishDraft({ data, reload, onFix, onPublished }: { data: BomData; reload: number; onFix: (id: number) => void; onPublished: (s: Snapshot) => void }) {
  const bomID = data.target.id
  const checks = useLoad(`checks-${bomID}-${data.version}-${reload}`, () => api.publicationChecks(bomID))
  const [justify, setJustify] = useState<Record<string, string>>({})
  const [open, setOpen] = useState<Record<string, boolean>>({})
  const [note, setNote] = useState('')
  const [proposedBy, setProposedBy] = useState('')
  const [validatedBy, setValidatedBy] = useState('')
  const [result, setResult] = useState<{ kind: 'ok'; snapshot: Snapshot } | { kind: 'blocked' | 'missing' | 'error'; message: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const list: PublicationCheck[] = checks.status === 'ready' ? checks.data : []
  const warns = list.filter((c) => c.level === 'warn')
  const blocks = list.filter((c) => c.level === 'block')
  const missing = new Set(result?.kind === 'missing' ? warns.filter((c) => !(justify[c.code] ?? '').trim()).map((c) => c.code) : [])

  async function publish() {
    setBusy(true)
    setResult(null)
    const justifications: Record<string, string> = {}
    for (const c of warns) if ((justify[c.code] ?? '').trim()) justifications[c.code] = justify[c.code].trim()
    try {
      const s = await api.publish(bomID, { note: note.trim(), proposed_by: proposedBy.trim(), validated_by: validatedBy.trim(), justifications })
      setResult({ kind: 'ok', snapshot: s })
      onPublished(s)
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        setResult({ kind: 'blocked', message: `Publication bloquée par : ${blocks.map((c) => `${c.title} (${c.detail})`).join(' ; ') || e.message}` })
      } else if (e instanceof ApiError && e.status === 422) {
        setResult({ kind: 'missing', message: `Justification requise pour : ${warns.filter((c) => !(justify[c.code] ?? '').trim()).map((c) => c.title).join(' ; ') || e.message}` })
        setOpen(Object.fromEntries(warns.map((c) => [c.code, true])))
      } else setResult({ kind: 'error', message: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }

  const okCount = list.filter((c) => c.level === 'ok').length
  return (
    <>
      <section className="block" aria-label="Contrôles avant publication">
        <h3>Contrôles avant publication{checks.status === 'ready' ? ` · ${okCount} sur ${list.length}` : ''}</h3>
        {checks.status === 'loading' && <p className="in muted small recalc" role="status">Contrôles en cours (serveur)…</p>}
        {checks.status === 'error' && <p className="in error" role="alert">Contrôles indisponibles : {checks.error}</p>}
        {checks.status === 'ready' && (
          <ul className="checks">
            {list.map((c) => (
              <li key={c.code} className={`check lvl-${c.level} ${missing.has(c.code) ? 'missing' : ''}`}>
                <span className={`check-icon lvl-${c.level}`} role="img" aria-label={levelIcon[c.level].label}>
                  {levelIcon[c.level].icon}
                </span>
                <span className="check-title">{c.title}</span>
                <span className="check-detail muted" title={c.detail}>
                  {c.detail}
                </span>
                <span className="check-act">
                  {c.level === 'warn' && (
                    <button type="button" className="btn" aria-expanded={!!open[c.code]} onClick={() => setOpen((o) => ({ ...o, [c.code]: !o[c.code] }))}>
                      {(justify[c.code] ?? '').trim() ? 'Justifié ✓' : 'Justifier'}
                    </button>
                  )}
                  {c.level !== 'ok' && c.node_ids.length > 0 && data.byID.has(c.node_ids[0]) && (
                    <button type="button" className="btn" onClick={() => onFix(c.node_ids[0])} title={`${c.node_ids.length} nœud(s) concerné(s)`}>
                      Corriger
                    </button>
                  )}
                </span>
                {c.level === 'warn' && open[c.code] && (
                  <textarea
                    className="check-justify"
                    aria-label={`Justification : ${c.title}`}
                    placeholder="Pourquoi publier malgré cet avertissement ?"
                    rows={2}
                    value={justify[c.code] ?? ''}
                    onChange={(e) => setJustify((j) => ({ ...j, [c.code]: e.target.value }))}
                  />
                )}
              </li>
            ))}
          </ul>
        )}
        <p className="in muted small">Un contrôle bloquant (×) empêche la publication ; un avertissement (!) demande une justification écrite. Les contrôles sont recalculés au moment de publier.</p>
      </section>

      <section className="block" aria-label="Note de décision">
        <h3>Note de décision</h3>
        <form
          className="in stack publish-form"
          onSubmit={(e) => {
            e.preventDefault()
            void publish()
          }}
        >
          <textarea aria-label="Note de décision" rows={3} placeholder="Ce qui est retenu, ce qui est accepté et pourquoi." value={note} onChange={(e) => setNote(e.target.value)} />
          <div className="row">
            <label>
              Proposé par <input aria-label="Proposé par" value={proposedBy} onChange={(e) => setProposedBy(e.target.value)} />
            </label>
            <label>
              Validé par <input aria-label="Validé par" placeholder="nom du valideur" value={validatedBy} onChange={(e) => setValidatedBy(e.target.value)} />
            </label>
            <span className="sp" />
            <button type="submit" className="btn pri" disabled={busy || data.busy}>
              {busy ? 'Publication…' : 'Publier ce brouillon'}
            </button>
          </div>
          <p className="muted small">
            Publier crée un snapshot immuable : structure, quantités, réglages, méthode, profil, prix et taux copiés, avec cette note, les noms et les justifications (texte libre, non vérifié).
            {blocks.length > 0 && <b className="t-over"> {blocks.length} contrôle(s) bloquant(s) : la publication sera refusée.</b>}
          </p>
          {result?.kind === 'ok' && (
            <p role="status" className="notice">
              Snapshot #{result.snapshot.id} publié le {formatDateTime(result.snapshot.published_at)}.{' '}
              <a href={href(bomRoute({ kind: 'snapshot', id: result.snapshot.id }, 'publier'))}>Ouvrir</a>
            </p>
          )}
          {result && result.kind !== 'ok' && (
            <p role="alert" className="form-error">
              {result.message}
            </p>
          )}
        </form>
      </section>
    </>
  )
}

function SnapshotTrace({ data, onOpenDraft }: { data: BomData; onOpenDraft: (bomID: number) => void }) {
  const s = data.snapshot!
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const checks = data.frozenChecks
  return (
    <>
      <section className="block" aria-label="Snapshot publié">
        <h3>
          Snapshot #{s.id} · publié le {formatDateTime(s.published_at)}
        </h3>
        <div className="in stack">
          <dl className="facts trace">
            <dt>Note de décision</dt>
            <dd>{s.note || '—'}</dd>
            <dt>Proposé par</dt>
            <dd>{s.proposed_by || '—'}</dd>
            <dt>Validé par</dt>
            <dd>{s.validated_by || '—'}</dd>
            <dt>Prix</dt>
            <dd>snapshot de prix #{s.price_snapshot_id}</dd>
          </dl>
          <p className="muted small">Lecture seule : structure, quantités, réglages, prix et taux figés à la publication.</p>
          <div className="row">
            <button
              type="button"
              className="btn pri"
              disabled={creating}
              onClick={() => {
                setError(null)
                setCreating(true)
                api
                  .newDraft(s.id)
                  .then((b) => onOpenDraft(b.id))
                  .catch((e: unknown) => setError(errorMessage(e)))
                  .finally(() => setCreating(false))
              }}
            >
              {creating ? 'Création du brouillon…' : 'Nouveau brouillon depuis ce snapshot'}
            </button>
          </div>
          {error && (
            <p role="alert" className="error">
              {error}
            </p>
          )}
        </div>
      </section>
      <section className="block" aria-label="Contrôles figés">
        <h3>Contrôles à la publication</h3>
        {!checks || checks.length === 0 ? (
          <p className="in muted">Aucun contrôle figé (snapshot publié avant les contrôles de publication).</p>
        ) : (
          <ul className="checks">
            {checks.map((c) => (
              <li key={c.code} className={`check lvl-${c.level}`}>
                <span className={`check-icon lvl-${c.level}`} role="img" aria-label={levelIcon[c.level].label}>
                  {levelIcon[c.level].icon}
                </span>
                <span className="check-title">{c.title}</span>
                <span className="check-detail muted">{c.detail}</span>
                <span className="check-act small">{c.justification ? `« ${c.justification} »` : ''}</span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  )
}

function Versions({ bomID, current, reload }: { bomID: number; current: string; reload: string }) {
  const versions = useLoad(`versions-${bomID}-${reload}`, () => api.versions(bomID))
  return (
    <section className="block" aria-label="Versions">
      <h3>Versions</h3>
      {versions.status === 'loading' && <p className="in muted small">Lecture des versions…</p>}
      {versions.status === 'error' && <p className="in error">Versions indisponibles : {versions.error}</p>}
      {versions.status === 'ready' && (
        <div className="scroll">
          <table>
            <thead>
              <tr>
                <th>Version</th>
                <th>Date (UTC)</th>
                <th className="n">Coût</th>
                <th className="n">Budget</th>
                <th>Proposé par</th>
                <th>Validé par</th>
                <th>Note</th>
              </tr>
            </thead>
            <tbody>
              {versions.data.map((v: Version) => (
                <tr key={v.ref} className={v.ref === current ? 'sel' : undefined}>
                  <td>
                    <a href={href(bomRoute({ kind: v.kind === 'draft' ? 'draft' : 'snapshot', id: v.id }, 'publier'))}>
                      <b>{v.label}</b>
                    </a>
                    {v.kind === 'snapshot' && <span className="tag">publiée</span>}
                  </td>
                  <td>{formatDateTime(v.at).replace(/ UTC$/, '')}</td>
                  <td className="n">{v.cost ? formatMoney(v.cost) : <span className="over">{v.cost_error ?? 'incomplet'}</span>}</td>
                  <td className="n">{v.budget_percent !== null ? formatPercent(v.budget_percent, 1) : '—'}</td>
                  <td>{v.proposed_by ?? '—'}</td>
                  <td>{v.validated_by ?? '—'}</td>
                  <td className="wrap muted">{v.note ?? (v.kind === 'draft' ? 'à rédiger' : '—')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
