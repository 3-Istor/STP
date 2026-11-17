import { Fragment, useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { api, errorMessage, type BOM, type PriceSnapshotRef, type Snapshot } from '../api.ts'
import { CurrencySelect } from '../CurrencySelect.tsx'
import { formatDateTime } from '../money.ts'
import { bomRoute, href, navigate } from '../router.ts'
import { byName, normalize } from '../text.ts'
import { useAppDialog } from '../useAppDialog.tsx'

type Props = {
  boms: BOM[]
  published: Snapshot[] // snapshots publiés depuis cet onglet
  error: string | null
  onChanged: () => void
}

// En-têtes des snapshots à l'origine d'un brouillon (GET /api/v1/snapshots/{id}) : l'API n'a pas de liste des
// snapshots publiés (lot B : GET /boms/{id}/versions). La réponse contient tout l'arbre figé : on ne garde que
// l'en-tête, et chaque snapshot n'est lu qu'une fois.
function useSourceSnapshots(ids: number[]) {
  const [byID, setByID] = useState<Map<number, Snapshot | string>>(new Map())
  const asked = useRef(new Set<number>())
  const key = ids.join(',')
  useEffect(() => {
    for (const id of key === '' ? [] : key.split(',').map(Number)) {
      if (asked.current.has(id)) continue
      asked.current.add(id)
      api
        .snapshot(id)
        .then((p) => setByID((m) => new Map(m).set(id, p.snapshot)))
        .catch((e: unknown) => setByID((m) => new Map(m).set(id, errorMessage(e))))
    }
  }, [key])
  return byID
}

type Row = { kind: 'snapshot'; id: number; snapshot: Snapshot | null; error: string | null }

// AllBoms (maquette I4-0) : une ligne par BOM, ses snapshots publiés connus en dessous, dates à la seconde.
// Fonctions de la V1 (liste à gauche) : ouvrir, créer, renommer, supprimer ; et depuis un snapshot,
// l'ouvrir ou créer un nouveau brouillon.
export function AllBoms({ boms, published, error, onChanged }: Props) {
  const [query, setQuery] = useState('')
  const [creating, setCreating] = useState(false)
  const [selected, setSelected] = useState<number | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const dialogs = useAppDialog()
  // Instant des snapshots de prix (à la seconde) pour la colonne « Prix utilisés » ; en échec, le numéro seul.
  const [priceSnaps, setPriceSnaps] = useState<Map<number, PriceSnapshotRef>>(new Map())
  useEffect(() => {
    api
      .listPriceSnapshots()
      .then((l) => setPriceSnaps(new Map(l.map((s) => [s.id, s]))))
      .catch(() => setPriceSnaps(new Map()))
  }, [boms])
  const priceLabel = (id: number | null) => {
    if (id === null) return 'en direct'
    const s = priceSnaps.get(id)
    return s ? `#${id} · ${formatDateTime(s.taken_at)}` : `#${id}`
  }
  const sources = useSourceSnapshots([...new Set(boms.flatMap((b) => (b.source_snapshot_id !== null ? [b.source_snapshot_id] : [])))].sort((a, b) => a - b))

  const rows = useMemo(() => {
    const q = normalize(query)
    const sorted = [...boms].sort((a, b) => byName.compare(a.name, b.name) || a.id - b.id)
    const known = new Set(boms.map((b) => b.id))
    // Chaque snapshot connu va sous la BOM qui l'a publié ; si elle n'existe plus (ou que l'en-tête n'est pas
    // encore lu), sous le brouillon qui en est issu.
    const under = new Map<number, Map<number, Row>>()
    const put = (bomID: number, r: Row) => {
      const m = under.get(bomID) ?? new Map<number, Row>()
      m.set(r.id, r)
      under.set(bomID, m)
    }
    for (const b of boms) {
      if (b.source_snapshot_id === null) continue
      const s = sources.get(b.source_snapshot_id)
      const snap = typeof s === 'object' ? s : null
      put(snap && known.has(snap.bom_id) ? snap.bom_id : b.id, { kind: 'snapshot', id: b.source_snapshot_id, snapshot: snap, error: typeof s === 'string' ? s : null })
    }
    for (const s of published) put(s.bom_id, { kind: 'snapshot', id: s.id, snapshot: s, error: null })
    const out: { bom: BOM; snapshots: Row[] }[] = []
    for (const b of sorted) {
      if (q !== '' && !normalize(b.name).includes(q)) continue
      out.push({ bom: b, snapshots: [...(under.get(b.id)?.values() ?? [])].sort((x, y) => x.id - y.id) })
    }
    return out
  }, [boms, published, sources, query])

  const preview = boms.find((b) => b.id === selected) ?? null

  async function act(action: () => Promise<unknown>) {
    setActionError(null)
    try {
      await action()
      onChanged()
    } catch (e) {
      setActionError(errorMessage(e))
    }
  }

  function rename(b: BOM) {
    void dialogs.prompt({ title: 'Renommer la BOM', label: 'Nouveau nom', initial: b.name, confirmLabel: 'Renommer' }).then((name) => {
      if (name && name !== b.name) void act(() => api.renameBOM(b.id, name))
    })
  }

  function remove(b: BOM) {
    void dialogs
      .confirm({
        title: `Supprimer la BOM « ${b.name} » ?`,
        message: 'Le brouillon et toute son arborescence seront supprimés définitivement.',
        confirmLabel: 'Supprimer',
        danger: true,
      })
      .then((ok) => {
        if (ok) void act(() => api.deleteBOM(b.id))
      })
  }

  function newDraft(snapshotID: number) {
    setActionError(null)
    api
      .newDraft(snapshotID)
      .then((b) => {
        onChanged()
        navigate(bomRoute({ kind: 'draft', id: b.id }))
      })
      .catch((e: unknown) => setActionError(errorMessage(e)))
  }

  return (
    <div className="work">
      {dialogs.element}
      <main className="main4" id="main-content">
        <div className="listbar">
          <input type="search" className="search" placeholder="Rechercher une BOM…" aria-label="Rechercher une BOM" value={query} onChange={(e) => setQuery(e.target.value)} />
          <span className="sp" />
          <button type="button" className="btn pri" onClick={() => setCreating(true)}>
            + Nouvelle BOM
          </button>
        </div>
        {(error || actionError) && (
          <p role="alert" className="error">
            {actionError ?? error}
          </p>
        )}
        <section className="block scroll">
          <table className="boms-table" aria-label="Toutes les BOM">
            <thead>
              <tr>
                <th>BOM</th>
                <th>Version</th>
                <th>Date</th>
                <th className="opt">Devise · volume</th>
                <th className="opt">Prix utilisés</th>
                <th>
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map(({ bom: b, snapshots }) => (
                <Fragment key={b.id}>
                  <tr className={`bom-row ${selected === b.id ? 'sel' : ''}`} onClick={() => setSelected(b.id)}>
                    <td>
                      <a className="bom-name" href={href(bomRoute({ kind: 'draft', id: b.id }))}>
                        {b.name}
                      </a>
                    </td>
                    <td>brouillon{b.source_snapshot_id !== null && <span className="muted"> · depuis #{b.source_snapshot_id}</span>}</td>
                    <td>{formatDateTime(b.created_at)}</td>
                    <td className="opt">
                      {b.currency} · {b.volume} u.
                    </td>
                    <td className="opt mono small">{priceLabel(b.price_snapshot_id)}</td>
                    <td className="acts-cell">
                      <a className="btn pri" href={href(bomRoute({ kind: 'draft', id: b.id }))} aria-label={`Ouvrir ${b.name}`}>
                        Ouvrir
                      </a>
                      <button type="button" className="btn" onClick={(e) => { e.stopPropagation(); rename(b) }} aria-label={`Renommer ${b.name}`}>
                        Renommer
                      </button>
                      <button type="button" className="btn danger" onClick={(e) => { e.stopPropagation(); remove(b) }} aria-label={`Supprimer ${b.name}`}>
                        Supprimer
                      </button>
                    </td>
                  </tr>
                  {snapshots.map((s) => s.kind === 'snapshot' && (
                    <tr key={`s${s.id}`} className="snap-row">
                      <td className="indent">
                        <b>snapshot #{s.id}</b> <span className="tag">publié</span>
                      </td>
                      <td>snapshot</td>
                      <td>{s.snapshot ? formatDateTime(s.snapshot.published_at) : s.error ? <span className="over">{s.error}</span> : '…'}</td>
                      <td className="opt">{s.snapshot ? `${s.snapshot.currency} · ${s.snapshot.volume} u.` : ''}</td>
                      <td className="opt mono small">{s.snapshot ? priceLabel(s.snapshot.price_snapshot_id) : ''}</td>
                      <td className="acts-cell">
                        <a className="btn" href={href(bomRoute({ kind: 'snapshot', id: s.id }))}>
                          Ouvrir
                        </a>
                        <button type="button" className="btn" onClick={() => newDraft(s.id)}>
                          Nouveau brouillon
                        </button>
                      </td>
                    </tr>
                  ))}
                </Fragment>
              ))}
              {rows.length === 0 && (
                <tr>
                  <td colSpan={6} className="muted">
                    {boms.length === 0 ? 'Aucune BOM : créez la première.' : `Aucune BOM ne correspond à « ${query} ».`}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </section>
        <p className="muted small">
          Une ligne par BOM, ses versions publiées en dessous ; dates à la seconde (UTC). Sont listés les snapshots à l’origine d’un brouillon et ceux publiés depuis cet onglet : la liste complète des versions, avec coût et budget, arrive avec le lot B.
        </p>
      </main>
      <aside className="col" aria-label="Aperçu">
        <div className="syn">
          {preview ? (
            <>
              <div className="eyebrow">Aperçu</div>
              <h2>{preview.name} · brouillon</h2>
              <dl className="dl">
                <dt>Créée le</dt>
                <dd>{formatDateTime(preview.created_at)}</dd>
                <dt>Créée depuis</dt>
                <dd>{preview.source_snapshot_id !== null ? `snapshot #${preview.source_snapshot_id}` : '—'}</dd>
                <dt>Devise</dt>
                <dd>{preview.currency}</dd>
                <dt>Volume</dt>
                <dd>{preview.volume} véhicules</dd>
                <dt>Destination</dt>
                <dd>{preview.destination_country ?? 'non renseignée'}</dd>
                <dt>Prix</dt>
                <dd>{priceLabel(preview.price_snapshot_id)}</dd>
              </dl>
            </>
          ) : (
            <p className="muted">Cliquez une ligne pour son aperçu ; « Ouvrir » mène à l’étape Structurer.</p>
          )}
        </div>
      </aside>
      {creating && (
        <CreateDialog
          onClose={() => setCreating(false)}
          onCreate={async (b) => {
            const created = await api.createBOM(b)
            onChanged()
            navigate(bomRoute({ kind: 'draft', id: created.id }))
          }}
        />
      )}
    </div>
  )
}

// CreateDialog : création d'une BOM (nom, devise de référence, volume), formulaire de la V1 en fenêtre.
function CreateDialog({ onClose, onCreate }: { onClose: () => void; onCreate: (b: { name: string; currency: string; volume: number }) => Promise<void> }) {
  const ref = useRef<HTMLDialogElement>(null)
  const [name, setName] = useState('')
  const [currency, setCurrency] = useState('EUR')
  const [volume, setVolume] = useState('1000')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (ref.current && !ref.current.open) ref.current.showModal()
  }, [])
  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await onCreate({ name: name.trim(), currency, volume: Number.parseInt(volume, 10) })
      onClose()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <dialog
      ref={ref}
      className="form-dialog"
      aria-labelledby="create-bom-title"
      onCancel={(e) => {
        e.preventDefault()
        onClose()
      }}
    >
      <form onSubmit={(e) => void submit(e)}>
        <div className="dialog-heading">
          <h2 id="create-bom-title">Nouvelle BOM</h2>
          <button type="button" className="icon-button" aria-label="Fermer" onClick={onClose}>
            ×
          </button>
        </div>
        <label>
          Nom de la BOM
          <input required autoFocus value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <div className="form-columns">
          <label>
            Devise
            <CurrencySelect required aria-label="Devise" value={currency} onChange={setCurrency} />
          </label>
          <label>
            Volume (véhicules)
            <input required aria-label="Volume" type="number" min={1} step={1} value={volume} onChange={(e) => setVolume(e.target.value)} />
          </label>
        </div>
        {error && (
          <p role="alert" className="form-error">
            {error}
          </p>
        )}
        <div className="form-actions">
          <button type="button" className="secondary-button" onClick={onClose}>
            Annuler
          </button>
          <button type="submit" className="primary-button" disabled={busy}>
            Créer
          </button>
        </div>
      </form>
    </dialog>
  )
}
