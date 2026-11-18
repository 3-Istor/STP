import { Fragment, useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { api, errorMessage, type BOM, type PriceSnapshotRef, type PublicationCheck, type Version, type Comparison } from '../api.ts'
import { CurrencySelect } from '../CurrencySelect.tsx'
import { formatDateTime, formatMoney, formatPercent, formatSignedMoney, decimalSign, formatDecimal } from '../money.ts'
import { bomRoute, href, navigate } from '../router.ts'
import { byName, normalize } from '../text.ts'
import { useAppDialog } from '../useAppDialog.tsx'

type Props = {
  boms: BOM[]
  error: string | null
  onChanged: () => void
}

// Famille de versions (ADR 0033, D7 du lot B) : la BOM d'origine, ses snapshots publiés (V1, V2…) et les
// brouillons créés depuis eux (Brouillon A, B…). GET /boms/{id}/versions renvoie toute la famille : deux BOM
// de la même famille donnent la même liste, gardée une fois.
type Family = { key: string; name: string; versions: Version[]; boms: BOM[] }
type Loaded = { versions: Map<number, Version[] | string> }

function useVersions(boms: BOM[]): Loaded {
  const [versions, setVersions] = useState<Map<number, Version[] | string>>(new Map())
  const key = boms.map((b) => b.id).join(',')
  useEffect(() => {
    let cancelled = false
    for (const id of key === '' ? [] : key.split(',').map(Number)) {
      api
        .versions(id)
        .then((v) => {
          if (!cancelled) setVersions((m) => new Map(m).set(id, v))
        })
        .catch((e: unknown) => {
          if (!cancelled) setVersions((m) => new Map(m).set(id, errorMessage(e)))
        })
    }
    return () => {
      cancelled = true
    }
  }, [key])
  return { versions }
}

// Dans le tableau, l'en-tête dit « UTC » une fois ; l'aperçu garde le suffixe.
const stamp = (iso: string) => formatDateTime(iso).replace(/ UTC$/, '')

const kindOrder = (v: Version) => (v.kind === 'draft' ? 0 : 1)

// AllBoms (maquette I4-0) : une ligne par famille de BOM, ses versions en dessous (brouillons puis versions
// publiées), dates à la seconde, coût, % du budget, prix utilisés, proposé / validé par. Fonctions de la V1 :
// ouvrir, créer, renommer, supprimer un brouillon ; ouvrir un snapshot ou en tirer un nouveau brouillon.
export function AllBoms({ boms, error, onChanged }: Props) {
  const [query, setQuery] = useState('')
  const [creating, setCreating] = useState(false)
  const [selected, setSelected] = useState<string | null>(null) // ref de version (« bom:3 », « snapshot:1 »)
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const [actionError, setActionError] = useState<string | null>(null)
  const [draftingFrom, setDraftingFrom] = useState<number | null>(null) // snapshot dont un brouillon est en création
  const dialogs = useAppDialog()
  const { versions } = useVersions(boms)
  // Instant des snapshots de prix (à la seconde) pour la colonne « Prix utilisés » ; en échec, le numéro seul.
  const [priceSnaps, setPriceSnaps] = useState<Map<number, PriceSnapshotRef>>(new Map())
  useEffect(() => {
    api
      .listPriceSnapshots()
      .then((l) => setPriceSnaps(new Map(l.map((s) => [s.id, s]))))
      .catch(() => setPriceSnaps(new Map()))
  }, [boms])
  const priceLabel = (id: number | null, short = false) => {
    if (id === null) return 'en direct'
    const s = priceSnaps.get(id)
    return s ? `#${id} · ${short ? stamp(s.taken_at) : formatDateTime(s.taken_at)}` : `#${id}`
  }

  const families = useMemo(() => {
    const q = normalize(query)
    const byBOM = new Map(boms.map((b) => [b.id, b]))
    const out: Family[] = []
    const seen = new Set<string>()
    for (const b of [...boms].sort((x, y) => byName.compare(x.name, y.name) || x.id - y.id)) {
      const v = versions.get(b.id)
      // Versions pas encore lues ou en échec : la BOM seule, comme un brouillon sans famille connue.
      const list: Version[] = Array.isArray(v) ? v : []
      const key = list.length > 0 ? list.map((x) => x.ref).sort().join('|') : `bom:${b.id}`
      if (seen.has(key)) continue
      seen.add(key)
      const members = list.length > 0 ? list.flatMap((x) => (x.kind === 'draft' && byBOM.has(x.id) ? [byBOM.get(x.id)!] : [])) : [b]
      const sorted = [...list].sort((x, y) => kindOrder(x) - kindOrder(y) || y.at.localeCompare(x.at) || x.id - y.id)
      const name = b.name
      if (q !== '' && !normalize(name).includes(q) && !sorted.some((x) => normalize(x.label).includes(q))) continue
      out.push({ key, name, versions: sorted, boms: members.length > 0 ? members : [b] })
    }
    return out
  }, [boms, versions, query])
  const versionCount = families.reduce((n, f) => n + Math.max(f.versions.length, f.boms.length), 0)

  const allVersions = families.flatMap((f) => f.versions)
  const preview = allVersions.find((v) => v.ref === selected) ?? null
  const previewBOM = selected?.startsWith('bom:') ? (boms.find((b) => `bom:${b.id}` === selected) ?? null) : null

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
    if (draftingFrom !== null) return
    setActionError(null)
    setDraftingFrom(snapshotID)
    api
      .newDraft(snapshotID)
      .then((b) => {
        onChanged()
        navigate(bomRoute({ kind: 'draft', id: b.id }))
      })
      .catch((e: unknown) => setActionError(errorMessage(e)))
      .finally(() => setDraftingFrom(null))
  }

  const draftActions = (b: BOM) => (
    <>
      <a className="btn pri" href={href(bomRoute({ kind: 'draft', id: b.id }))} aria-label={`Ouvrir ${b.name}`} onClick={(e) => e.stopPropagation()}>
        Ouvrir
      </a>
      <button type="button" className="btn ico" title="Renommer" onClick={(e) => { e.stopPropagation(); rename(b) }} aria-label={`Renommer ${b.name}`}>
        ✎
      </button>
      <button type="button" className="btn danger ico" title="Supprimer" onClick={(e) => { e.stopPropagation(); remove(b) }} aria-label={`Supprimer ${b.name}`}>
        ×
      </button>
    </>
  )

  return (
    <div className="work">
      {dialogs.element}
      <main className="main4" id="main-content">
        <div className="listbar">
          <input type="search" className="search" placeholder="Rechercher une BOM…" aria-label="Rechercher une BOM" value={query} onChange={(e) => setQuery(e.target.value)} />
          <span className="sp" />
          {preview?.kind === 'draft' && (
            <a className="btn" href={href(bomRoute({ kind: 'draft', id: preview.id }, 'arbitrer'))}>
              Comparer deux versions
            </a>
          )}
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
                <th>BOM · version</th>
                <th>Date (UTC)</th>
                <th className="n">Coût</th>
                <th className="n">Budget</th>
                <th className="opt">Prix utilisés (UTC)</th>
                <th className="opt">Proposé · validé</th>
                <th>
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {families.map((f) => {
                const open = !collapsed.has(f.key)
                const loadError = f.boms.map((b) => versions.get(b.id)).find((v) => typeof v === 'string') as string | undefined
                const loading = f.versions.length === 0 && f.boms.every((b) => !versions.has(b.id))
                return (
                  <Fragment key={f.key}>
                    <tr className="family-row">
                      <td colSpan={7}>
                        {f.versions.length > 0 && (
                          <button
                            type="button"
                            className="toggle"
                            aria-expanded={open}
                            aria-label={open ? `Replier ${f.name}` : `Déplier ${f.name}`}
                            onClick={() => setCollapsed((c) => { const n = new Set(c); if (!n.delete(f.key)) n.add(f.key); return n })}
                          >
                            {open ? '▾' : '▸'}
                          </button>
                        )}
                        <b>{f.name}</b>
                        {loading && <span className="muted small"> · versions…</span>}
                        {f.versions[0] && <span className="muted small"> · {formatDecimal(String(f.versions[0].node_count), 0)} nœuds</span>}
                        {loadError && <span className="over small"> · versions indisponibles : {loadError}</span>}
                      </td>
                    </tr>
                    {f.versions.length === 0 &&
                      f.boms.map((b) => (
                        <tr key={`b${b.id}`} className={`bom-row ${selected === `bom:${b.id}` ? 'sel' : ''}`} onClick={() => setSelected(`bom:${b.id}`)}>
                          <td className="indent">
                            <a className="bom-name" href={href(bomRoute({ kind: 'draft', id: b.id }))} onClick={(e) => e.stopPropagation()}>
                              {b.name}
                            </a>
                          </td>
                          <td>{stamp(b.created_at)}</td>
                          <td className="n muted">…</td>
                          <td className="n muted">…</td>
                          <td className="opt mono small">{priceLabel(b.price_snapshot_id, true)}</td>
                          <td className="opt" />
                          <td className="acts-cell">{draftActions(b)}</td>
                        </tr>
                      ))}
                    {open &&
                      f.versions.map((v) => {
                        const b = v.kind === 'draft' ? f.boms.find((x) => x.id === v.id) : undefined
                        return (
                          <tr key={v.ref} className={`bom-row ${v.kind === 'snapshot' ? 'snap-row' : ''} ${selected === v.ref ? 'sel' : ''}`} onClick={() => setSelected(v.ref)}>
                            <td className="indent">
                              {v.kind === 'draft' ? (
                                <a className="bom-name" href={href(bomRoute({ kind: 'draft', id: v.id }))} onClick={(e) => e.stopPropagation()}>
                                  {v.label}
                                </a>
                              ) : (
                                <>
                                  <b>{v.label}</b> <span className="tag">publiée · snapshot #{v.id}</span>
                                </>
                              )}
                            </td>
                            <td>{stamp(v.at)}</td>
                            <td className={`n ${v.cost ? '' : 'over'}`} title={v.cost_error ?? undefined}>
                              {v.cost ? formatMoney(v.cost) : 'incomplet'}
                            </td>
                            <td className="n" title={v.target_budget ? `budget ${formatMoney(v.target_budget)}` : 'aucun budget global'}>
                              {v.budget_percent !== null ? formatPercent(v.budget_percent, 1) : <span className="muted">{v.target_budget ? 'n/d' : 'pas de budget'}</span>}
                            </td>
                            <td className="opt mono small">{priceLabel(v.price_snapshot_id, true)}</td>
                            <td className="opt small">
                              {v.proposed_by ?? '—'} · {v.validated_by ?? '—'}
                            </td>
                            <td className="acts-cell">
                              {v.kind === 'draft' ? (
                                b ? draftActions(b) : (
                                  <a className="btn" href={href(bomRoute({ kind: 'draft', id: v.id }))}>
                                    Ouvrir
                                  </a>
                                )
                              ) : (
                                <>
                                  <a className="btn" href={href(bomRoute({ kind: 'snapshot', id: v.id }))} onClick={(e) => e.stopPropagation()} aria-label={`Ouvrir ${v.label}`}>
                                    Ouvrir
                                  </a>
                                  <button type="button" className="btn" title="Nouveau brouillon depuis cette version" aria-label={`Nouveau brouillon depuis ${v.label}`} disabled={draftingFrom !== null} onClick={(e) => { e.stopPropagation(); newDraft(v.id) }}>
                                    {draftingFrom === v.id ? 'Création…' : '+ Brouillon'}
                                  </button>
                                </>
                              )}
                            </td>
                          </tr>
                        )
                      })}
                  </Fragment>
                )
              })}
              {families.length === 0 && (
                <tr>
                  <td colSpan={7} className="muted">
                    {boms.length === 0 ? 'Aucune BOM : créez la première.' : `Aucune BOM ne correspond à « ${query} ».`}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </section>
        <p className="muted small">
          Une ligne par BOM, ses brouillons et ses versions publiées en dessous ; dates à la seconde (UTC). Coût et % du budget calculés par l’API selon la méthode de chaque version.
          {versionCount > 0 && ` ${formatDecimal(String(versionCount), 0)} version(s).`}
        </p>
      </main>
      <aside className="col" aria-label="Aperçu">
        <div className="syn">
          {preview ? (
            <VersionPreview key={preview.ref} version={preview} family={families.find((f) => f.versions.includes(preview))?.versions ?? []} priceLabel={priceLabel} />
          ) : previewBOM ? (
            <>
              <div className="eyebrow">Aperçu</div>
              <h2>{previewBOM.name} · brouillon</h2>
              <dl className="dl">
                <dt>Créée le</dt>
                <dd>{formatDateTime(previewBOM.created_at)}</dd>
                <dt>Devise</dt>
                <dd>{previewBOM.currency}</dd>
                <dt>Volume</dt>
                <dd>{previewBOM.volume} véhicules</dd>
                <dt>Destination</dt>
                <dd>{previewBOM.destination_country ?? 'non renseignée'}</dd>
                <dt>Prix</dt>
                <dd>{priceLabel(previewBOM.price_snapshot_id)}</dd>
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

// VersionPreview : aperçu d'une version (maquette I4-0) — origine, coût, budget, méthode, prix, noms, note ;
// pour un brouillon issu d'un snapshot, l'écart avec lui (GET /compare) et l'état des contrôles de
// publication, lus à la demande (calculs de l'API, quelques secondes sur 30 000 nœuds).
function VersionPreview({ version: v, family, priceLabel }: { version: Version; family: Version[]; priceLabel: (id: number | null, short?: boolean) => string }) {
  const source = v.source_snapshot_id !== null ? family.find((x) => x.kind === 'snapshot' && x.id === v.source_snapshot_id) : undefined
  const [cmp, setCmp] = useState<Comparison | string | null>(null)
  const [checks, setChecks] = useState<PublicationCheck[] | string | null>(null)
  const asked = useRef(false)
  useEffect(() => {
    if (asked.current || v.kind !== 'draft') return
    asked.current = true
    if (v.source_snapshot_id !== null) {
      api.compare(`snapshot:${v.source_snapshot_id}`, v.ref).then(setCmp, (e: unknown) => setCmp(errorMessage(e)))
    }
    api.publicationChecks(v.id).then(setChecks, (e: unknown) => setChecks(errorMessage(e)))
  }, [v])
  const okCount = Array.isArray(checks) ? checks.filter((c) => c.level === 'ok').length : 0
  const blocked = Array.isArray(checks) && checks.some((c) => c.level === 'block')
  return (
    <>
      <div className="eyebrow">Aperçu</div>
      <h2>
        {v.name} · {v.label}
      </h2>
      <dl className="dl">
        <dt>{v.kind === 'draft' ? 'Créé le' : 'Publiée le'}</dt>
        <dd>{formatDateTime(v.at)}</dd>
        <dt>Créé depuis</dt>
        <dd>{source ? `${source.label} (${formatDateTime(source.at)})` : v.source_snapshot_id !== null ? `snapshot #${v.source_snapshot_id}` : '—'}</dd>
        <dt>Coût</dt>
        <dd className={v.cost ? undefined : 'over'}>{v.cost ? formatMoney(v.cost) : (v.cost_error ?? 'incomplet')}</dd>
        <dt>Budget</dt>
        <dd>{v.target_budget ? `${formatMoney(v.target_budget)} · ${formatPercent(v.budget_percent, 1)}` : 'aucun budget global'}</dd>
        <dt>Prix</dt>
        <dd className="mono small">{priceLabel(v.price_snapshot_id)}</dd>
        <dt>Pièces · nœuds</dt>
        <dd>
          {formatDecimal(String(v.part_count), 0)} · {formatDecimal(String(v.node_count), 0)}
        </dd>
        <dt>Proposé par</dt>
        <dd>{v.proposed_by ?? '—'}</dd>
        <dt>Validé par</dt>
        <dd>{v.validated_by ?? '—'}</dd>
        {v.note && (
          <>
            <dt>Note</dt>
            <dd>{v.note}</dd>
          </>
        )}
        {v.kind === 'draft' && v.source_snapshot_id !== null && (
          <>
            <dt>Écart avec {source?.label ?? `#${v.source_snapshot_id}`}</dt>
            <dd className={typeof cmp === 'object' && cmp ? (decimalSign(cmp.delta.cost.amount) > 0 ? 't-over' : 't-ok') : 'muted'}>
              {cmp === null ? 'calcul…' : typeof cmp === 'string' ? `indisponible : ${cmp}` : formatSignedMoney(cmp.delta.cost)}
            </dd>
            <dt>Pièces modifiées</dt>
            <dd>{cmp && typeof cmp === 'object' ? formatDecimal(String(cmp.changed_parts.length), 0) : '…'}</dd>
          </>
        )}
        {v.kind === 'draft' && (
          <>
            <dt>Contrôles de publication</dt>
            <dd className={blocked ? 't-over' : Array.isArray(checks) && okCount < checks.length ? 't-warn' : undefined}>
              {checks === null ? 'calcul…' : typeof checks === 'string' ? `indisponibles : ${checks}` : `${okCount} sur ${checks.length}${blocked ? ' · bloqué' : ''}`}
            </dd>
          </>
        )}
      </dl>
    </>
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
