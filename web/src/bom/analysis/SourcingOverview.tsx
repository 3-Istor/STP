import { useState } from 'react'
import { api, type Sourcing } from '../../api.ts'
import { decimalSign, formatDecimal, formatMoney, formatPercent, formatSignedMoney } from '../../money.ts'
import { shareWidths } from '../../shell/share.ts'
import { useLoad } from '../useLoad.ts'
import { Show } from './Loading.tsx'

// Sourcer › Vue d'ensemble (maquette I4-5, lot E) : d'où vient le coût, chez qui, à quelles conditions.
// GET /boms/{id}/analysis/sourcing calcule tout (parts, poids des 5 premiers, mono-source, contrats expirés et
// remise perdue, taux budget vs dernier taux et effet sur la voiture) ; ici, mise en forme seulement.

const SHOWN_SUPPLIERS = 6

export function SourcingOverview({ bomID, version, onSupplier }: { bomID: number; version: number; onSupplier: (id: number) => void }) {
  const value = useLoad(`${bomID}-${version}`, () => api.sourcing(bomID))
  return (
    <Show value={value} label="Vue d’ensemble du sourcing">
      {(s) => (
        <>
          <div className="grid2">
            <ByCountry s={s} />
            <Suppliers s={s} onSupplier={onSupplier} />
          </div>
          <ExpiredContracts s={s} onSupplier={onSupplier} />
          <MonoSource s={s} />
          <Rates s={s} />
        </>
      )}
    </Show>
  )
}

function ByCountry({ s }: { s: Sourcing }) {
  // Pays exposé au change : au moins un de ses fournisseurs facture dans une autre devise que la BOM (étiquette
  // tirée des fournisseurs renvoyés par l'API, aucun montant calculé).
  const exposed = new Set(s.suppliers.filter((x) => x.currencies.some((c) => c !== s.currency)).flatMap((x) => x.countries))
  const max = s.by_country[0]?.cost ?? null
  const widths = shareWidths(max, s.by_country.map((c) => c.cost))
  const foreign = s.by_currency.filter((c) => c.key !== s.currency)
  return (
    <section className="block" aria-label="Origine du coût par pays">
      <h3>Origine du coût par pays</h3>
      <div className="in">
        <ul className="bars">
          {s.by_country.map((c, i) => (
            <li key={c.key} title={`${formatMoney(c.cost)} par véhicule · ${c.parts} pièce(s)${exposed.has(c.key) ? ' · facturé hors ' + s.currency : ''}`}>
              <span className="bar-name">
                <b>{c.key}</b>
              </span>
              <span className="bar" aria-hidden="true">
                {widths[i] !== null && <i className={exposed.has(c.key) ? 'exposed' : undefined} style={{ width: `${widths[i]}%` }} />}
              </span>
              <span className="n">{formatPercent(c.share_percent, 1)}</span>
            </li>
          ))}
        </ul>
        <p className="muted small">
          En orange : pays dont un fournisseur facture hors {s.currency}.{' '}
          {foreign.length > 0
            ? `Par devise d’offre : ${foreign.map((c) => `${c.key} ${formatPercent(c.share_percent, 1)}`).join(', ')}.`
            : `Tout est facturé en ${s.currency}.`}
        </p>
      </div>
    </section>
  )
}

const contractLabel = (c: Sourcing['suppliers'][number]['contract']) =>
  ({ active: 'actif', expired: 'expiré', not_started: 'pas encore en vigueur', none: 'sans contrat' })[c.status] ?? c.status

function Suppliers({ s, onSupplier }: { s: Sourcing; onSupplier: (id: number) => void }) {
  const [all, setAll] = useState(false)
  const rows = all ? s.suppliers : s.suppliers.slice(0, SHOWN_SUPPLIERS)
  return (
    <section className="block" aria-label="Fournisseurs">
      <h3>
        Fournisseurs · {formatDecimal(String(s.supplier_count), 0)} au total · 5 premiers = {formatPercent(s.top5_share_percent, 1)} ·{' '}
        {formatDecimal(String(s.mono_source_count), 0)} pièce(s) mono-source
      </h3>
      <div className="scroll">
        <table>
          <thead>
            <tr>
              <th>Fournisseur</th>
              <th>Pays</th>
              <th className="n">Part du coût</th>
              <th>Contrat</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((x) => (
              <tr key={x.supplier_id} className={`clickable ${x.contract.status === 'expired' ? 'expired' : ''}`} onClick={() => onSupplier(x.supplier_id)} title={`${formatMoney(x.cost)} par véhicule · ${x.parts} pièce(s) · ${x.currencies.join(', ')}`}>
                <td>{x.name}</td>
                <td>{x.countries.join(', ')}</td>
                <td className="n">{formatPercent(x.share_percent, 1)}</td>
                <td className={x.contract.status === 'expired' ? 't-over' : 'muted'}>
                  {contractLabel(x.contract)}
                  {x.contract.ends_on && x.contract.status === 'expired' ? ` le ${x.contract.ends_on.split('-').reverse().join('/')}` : ''}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {s.suppliers.length > SHOWN_SUPPLIERS && (
        <p className="in">
          <button type="button" className="link small" onClick={() => setAll((v) => !v)}>
            {all ? 'Moins de fournisseurs' : `Tous les fournisseurs (${s.suppliers.length})…`}
          </button>
        </p>
      )}
    </section>
  )
}

function ExpiredContracts({ s, onSupplier }: { s: Sourcing; onSupplier: (id: number) => void }) {
  return (
    <section className="block" aria-label="Contrats expirés encore utilisés">
      <h3>
        Contrats expirés encore utilisés
        {s.expired_contracts.length > 0 && <span className="bdg over">{s.expired_contracts.length}</span>}
      </h3>
      {s.expired_contracts.length === 0 ? (
        <p className="in muted">Aucun contrat expiré à la date de calcul ({s.as_of.slice(0, 10).split('-').reverse().join('/')}).</p>
      ) : (
        <div className="scroll">
          <table>
            <thead>
              <tr>
                <th>Fournisseur</th>
                <th>Fin</th>
                <th>Pièces</th>
                <th className="n">Remise perdue / véhicule</th>
              </tr>
            </thead>
            <tbody>
              {s.expired_contracts.map((c) => (
                <tr key={c.contract_id} className="clickable" onClick={() => onSupplier(c.supplier_id)}>
                  <td>{c.supplier_name}</td>
                  <td>{c.ends_on.split('-').reverse().join('/')}</td>
                  <td className="wrap small">
                    {c.parts
                      .map((p) => `${p.name} (−${formatDecimal(p.discount_percent, 2)} %${p.tier_min ? ` dès ${formatDecimal(p.tier_min, 0)} u.` : ''}${p.error ? ` : ${p.error}` : ''})`)
                      .join(', ')}
                  </td>
                  <td className="n t-over">{c.lost_amount ? formatMoney(c.lost_amount) : 'n/d'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}

function MonoSource({ s }: { s: Sourcing }) {
  if (s.mono_source.length === 0) return null
  return (
    <section className="block" aria-label="Pièces mono-source">
      <h3>Pièces mono-source · une seule offre active</h3>
      <ul className="in explain">
        {s.mono_source.map((p) => (
          <li key={p.part_id}>
            {p.name} · {formatPercent(p.share_percent, 1)} du coût · {formatMoney(p.cost)}
          </li>
        ))}
      </ul>
    </section>
  )
}

function Rates({ s }: { s: Sourcing }) {
  return (
    <section className="block" aria-label="Change, taux budget et dernier taux">
      <h3>Change · taux budget et dernier taux</h3>
      {s.rates.length === 0 ? (
        <p className="in muted">Aucune conversion de devise.</p>
      ) : (
        <div className="scroll">
          <table>
            <thead>
              <tr>
                <th>Paire</th>
                <th className="n">Taux budget</th>
                <th className="n">Dernier taux</th>
                <th className="n">Écart</th>
                <th className="n">Effet sur la voiture</th>
              </tr>
            </thead>
            <tbody>
              {s.rates.map((r) => (
                <tr key={r.currency} title={[r.budget_rate?.source, r.latest_rate ? `${r.latest_rate.source} du ${r.latest_rate.quoted_at.slice(0, 10)}` : null].filter(Boolean).join(' · ')}>
                  <td>
                    {r.currency} → {s.currency}
                  </td>
                  <td className="n">{r.budget_rate ? formatDecimal(r.budget_rate.rate, 5) : 'absent'}</td>
                  <td className="n">{r.latest_rate ? formatDecimal(r.latest_rate.rate, 5) : 'absent'}</td>
                  <td className="n">{r.gap_percent !== null ? formatPercent(r.gap_percent, 1) : 'n/d'}</td>
                  <td className={`n ${r.effect ? (decimalSign(r.effect.amount) > 0 ? 't-over' : decimalSign(r.effect.amount) < 0 ? 't-ok' : '') : ''}`} title={r.error ?? undefined}>
                    {r.effect ? formatSignedMoney(r.effect) : `n/d${r.error ? ` (${r.error})` : ''}`}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="in muted small">Effet = coût de la voiture si cette devise passait du taux budget au dernier taux (positif : plus cher).</p>
        </div>
      )}
    </section>
  )
}
