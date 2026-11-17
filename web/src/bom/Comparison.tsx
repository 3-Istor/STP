import { useState } from 'react'
import { api, type CauseKind, type Comparison, type Version } from '../api.ts'
import { partName } from '../catalog.ts'
import { compareAbsDecimal, decimalSign, formatDecimal, formatMoney, formatSignedMoney } from '../money.ts'
import { bomRoute, href, type Target } from '../router.ts'
import { useLoad } from './useLoad.ts'
import type { BomData } from './useBomData.ts'

// Comparaison de deux versions et « Pourquoi le coût a bougé » (maquette I4-6, ADR 0032). Tout vient de
// GET /compare : totaux, écarts, causes (Σ causes = écart de coût, exactement) et pièces modifiées. Le front
// trie les causes par montant absolu (comparaison exacte des chaînes) et les met en forme.

export const causeLabels: Record<CauseKind, string> = {
  structure: 'Structure (pièces ajoutées, retirées, quantités)',
  offer_change: 'Changements d’offre',
  price: 'Nouvelles cotations',
  fx: 'Change',
  discount: 'Remises de contrat',
  logistics: 'Douane, transport, emballage',
  tooling: 'Outillage amorti',
  method: 'Méthode de calcul',
}

const targetOf = (ref: string): Target | null => {
  const m = /^(bom|snapshot):(\d+)$/.exec(ref)
  return m ? { kind: m[1] === 'bom' ? 'draft' : 'snapshot', id: Number(m[2]) } : null
}

/** Choix par défaut : dernière version publiée → version courante (ou la précédente si on regarde un snapshot). */
export function defaultPair(versions: Version[], current: string): [string, string] | null {
  const published = versions.filter((v) => v.kind === 'snapshot' && v.ref !== current).sort((a, b) => b.at.localeCompare(a.at))
  const from = published[0] ?? versions.find((v) => v.ref !== current)
  if (!from) return null
  return [from.ref, current]
}

export function ComparisonPanel({ data, onSelect }: { data: BomData; onSelect: (id: number) => void }) {
  const bomID = data.snapshot ? data.snapshot.bom_id : data.target.id
  const current = `${data.target.kind === 'draft' ? 'bom' : 'snapshot'}:${data.target.id}`
  const versions = useLoad(`versions-${bomID}-${data.version}`, () => api.versions(bomID))
  const [pick, setPick] = useState<[string, string] | null>(null)
  if (versions.status !== 'ready')
    return (
      <section className="block" aria-label="Comparer deux versions">
        <h3>Comparer deux versions</h3>
        <p className={`in small ${versions.status === 'error' ? 'error' : 'muted'}`} role={versions.status === 'error' ? 'alert' : 'status'}>
          {versions.status === 'error' ? `Versions indisponibles : ${versions.error}` : 'Lecture des versions…'}
        </p>
      </section>
    )
  const list = versions.data
  const pair = pick ?? defaultPair(list, current)
  return (
    <>
      <section className="block" aria-label="Comparer deux versions">
        <h3>
          Comparer
          {pair ? (
            <>
              <VersionSelect label="De" value={pair[0]} versions={list} onChange={(v) => setPick([v, pair[1]])} />
              <span aria-hidden="true">→</span>
              <VersionSelect label="Vers" value={pair[1]} versions={list} onChange={(v) => setPick([pair[0], v])} />
            </>
          ) : (
            <span className="muted small">une seule version : rien à comparer</span>
          )}
        </h3>
        {pair && pair[0] === pair[1] && <p className="in muted">Choisissez deux versions différentes.</p>}
        {pair && pair[0] !== pair[1] && <CompareResult key={`${pair[0]}>${pair[1]}-${data.version}`} from={pair[0]} to={pair[1]} data={data} current={current} onSelect={onSelect} />}
      </section>
    </>
  )
}

function VersionSelect({ label, value, versions, onChange }: { label: string; value: string; versions: Version[]; onChange: (ref: string) => void }) {
  return (
    <label className="vsel">
      <span className="muted small">{label}</span>
      <select aria-label={`Version ${label === 'De' ? 'de départ' : 'd’arrivée'}`} value={value} onChange={(e) => onChange(e.target.value)}>
        {versions.map((v) => (
          <option key={v.ref} value={v.ref}>
            {v.label}
            {v.kind === 'snapshot' ? ' (publiée)' : ''}
          </option>
        ))}
      </select>
    </label>
  )
}

function CompareResult({ from, to, data, current, onSelect }: { from: string; to: string; data: BomData; current: string; onSelect: (id: number) => void }) {
  const value = useLoad(`${from}>${to}`, () => api.compare(from, to))
  if (value.status === 'loading')
    return (
      <p className="in muted small recalc" role="status">
        Comparaison en cours (serveur, quelques secondes sur 30 000 nœuds)…
      </p>
    )
  if (value.status === 'error')
    return (
      <p className="in error" role="alert">
        Comparaison impossible : {value.error}
      </p>
    )
  return <CompareTables c={value.data} data={data} current={current} onSelect={onSelect} />
}

function signClass(amount: string, upIsBad = true) {
  const s = decimalSign(amount)
  if (s === 0) return ''
  return (s > 0) === upIsBad ? 't-over' : 't-ok'
}

function CompareTables({ c, data, current, onSelect }: { c: Comparison; data: BomData; current: string; onSelect: (id: number) => void }) {
  const causes = [...c.causes].sort((a, b) => compareAbsDecimal(b.amount.amount, a.amount.amount))
  const days = c.delta.lead_time_days
  // Les nœuds d'une cause sont ceux du côté « vers » (ou « de » pour une pièce retirée) : sélectionnables ici
  // quand ce côté est la version ouverte, sinon lien vers cette version.
  const nodeLink = (ref: string, id: number, text: string) => {
    if (ref === current && data.byID.has(id))
      return (
        <button type="button" className="link" onClick={() => onSelect(id)}>
          {text}
        </button>
      )
    const t = targetOf(ref)
    return t ? <a href={href(bomRoute(t, 'structurer', { node: id }))}>{text}</a> : <span>{text}</span>
  }
  const nodeName = (ref: string, id: number) => {
    const n = ref === current ? data.byID.get(id) : undefined
    if (!n) return `nœud #${id}`
    return n.part_id !== null ? partName(data.catalog, n.part_id) : n.name
  }
  return (
    <>
      <div className="scroll">
        <table aria-label="Totaux comparés">
          <thead>
            <tr>
              <th>Indicateur</th>
              <th className="n">{c.from.label}</th>
              <th className="n">{c.to.label}</th>
              <th className="n">Écart</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>
                <b>Coût / véhicule</b>
              </td>
              <td className="n">{formatMoney(c.from.totals.cost)}</td>
              <td className="n">
                <b>{formatMoney(c.to.totals.cost)}</b>
              </td>
              <td className={`n ${signClass(c.delta.cost.amount)}`}>
                <b>{formatSignedMoney(c.delta.cost)}</b>
              </td>
            </tr>
            <tr>
              <td>Disponibilité</td>
              <td className="n">{c.from.totals.lead_time_days} j</td>
              <td className="n">{c.to.totals.lead_time_days} j</td>
              <td className={`n ${days > 0 ? 't-over' : days < 0 ? 't-ok' : ''}`}>{days === 0 ? '0 j' : `${days > 0 ? '+' : '−'} ${Math.abs(days)} j`}</td>
            </tr>
            <tr>
              <td>CO₂ / véhicule</td>
              <td className="n">{formatDecimal(c.from.totals.co2_kg, 1)} kg</td>
              <td className="n">{formatDecimal(c.to.totals.co2_kg, 1)} kg</td>
              <td className={`n ${signClass(c.delta.co2_kg)}`}>{signed(c.delta.co2_kg, 1)} kg</td>
            </tr>
            <tr>
              <td>Masse</td>
              <td className="n">{formatDecimal(c.from.totals.mass_kg, 1)} kg</td>
              <td className="n">{formatDecimal(c.to.totals.mass_kg, 1)} kg</td>
              <td className={`n ${signClass(c.delta.mass_kg)}`}>{signed(c.delta.mass_kg, 1)} kg</td>
            </tr>
            <tr>
              <td>Pièces modifiées</td>
              <td />
              <td className="n">{c.changed_parts.length}</td>
              <td />
            </tr>
          </tbody>
        </table>
      </div>
      <p className="in muted small">
        Méthode : {c.from.label} et {c.to.label} chiffrées chacune avec sa méthode et ses prix ({c.from.price_snapshot_id !== null ? `snapshot de prix #${c.from.price_snapshot_id}` : 'en direct'} →{' '}
        {c.to.price_snapshot_id !== null ? `snapshot de prix #${c.to.price_snapshot_id}` : 'en direct'}).
      </p>

      <section className="causes" aria-label="Pourquoi le coût a bougé">
        <h4>
          Pourquoi le coût a bougé entre {c.from.label} et {c.to.label} · <span className={signClass(c.delta.cost.amount)}>{formatSignedMoney(c.delta.cost)}</span>
        </h4>
        {causes.length === 0 && <p className="muted">Aucun écart de coût.</p>}
        <ul>
          {causes.map((k, i) => {
            const ids = k.node_ids.length > 0 ? k.node_ids : k.from_node_ids
            const ref = k.node_ids.length > 0 ? c.to.ref : c.from.ref
            return (
              <li key={`${k.kind}-${i}`}>
                <div>
                  <b>{causeLabels[k.kind] ?? k.kind}</b>
                  <div className="muted small">{k.explanation}</div>
                  {ids.length > 0 && (
                    <div className="small cause-nodes">
                      {ids.slice(0, 3).map((id, j) => (
                        <span key={id}>
                          {j > 0 && ', '}
                          {nodeLink(ref, id, nodeName(ref, id))}
                        </span>
                      ))}
                      {k.node_count > 3 && <span className="muted"> et {formatDecimal(String(k.node_count - 3), 0)} autre(s)</span>}
                    </div>
                  )}
                </div>
                <div className={`n ${signClass(k.amount.amount)}`}>{formatSignedMoney(k.amount)}</div>
              </li>
            )
          })}
        </ul>
      </section>

      {c.changed_parts.length > 0 && (
        <div className="scroll">
          <table aria-label="Pièces modifiées">
            <thead>
              <tr>
                <th>Pièce modifiée</th>
                <th>Avant</th>
                <th>Après</th>
                <th className="n">Écart / véhicule</th>
              </tr>
            </thead>
            <tbody>
              {c.changed_parts.map((p, i) => (
                <tr key={`${p.path}-${i}`}>
                  <td className="wrap">
                    {p.node_id !== null ? nodeLink(c.to.ref, p.node_id, p.path) : p.from_node_id !== null ? nodeLink(c.from.ref, p.from_node_id, p.path) : p.path}
                  </td>
                  <td>{p.before}</td>
                  <td>{p.after}</td>
                  <td className={`n ${signClass(p.delta.amount)}`}>{formatSignedMoney(p.delta)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}

function signed(value: string, digits: number): string {
  const s = decimalSign(value)
  const text = formatDecimal(value.replace(/^-/, ''), digits)
  return s === 0 ? text : `${s > 0 ? '+' : '−'} ${text}`
}
