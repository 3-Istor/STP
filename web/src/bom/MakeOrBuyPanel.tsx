import { useRef, useState } from 'react'
import { api, errorMessage, type CostLine, type MakeOrBuyResult } from '../api.ts'
import { decimalInput, formatMoney, type Money } from '../money.ts'

const labels: Record<string, string> = {
  parts_landed: 'Pièces (coût rendu)',
  labor: "Main-d'œuvre",
  logistics: 'Logistique interne',
  tier1_price: 'Prix Tier 1',
  customs: 'Douane',
  freight: 'Transport',
  packaging: 'Emballage',
}

type Field = 'fixed_costs' | 'logistics_per_unit' | 'tier1_price' | 'tier1_packaging'

const fields: { key: Field; label: string }[] = [
  { key: 'fixed_costs', label: 'Coûts fixes (total)' },
  { key: 'logistics_per_unit', label: 'Logistique / unité' },
  { key: 'tier1_price', label: 'Prix Tier 1 / unité' },
  { key: 'tier1_packaging', label: 'Emballage Tier 1 / unité' },
]

// MakeOrBuyPanel : simulation fabriquer ou acheter d'un sous-ensemble (ADR 0026). Rien n'est stocké ni appliqué :
// la décision reste à l'utilisateur. Tous les calculs sont faits par l'API.
export function MakeOrBuyPanel({ bomID, nodeID, currency }: { bomID: number; nodeID: number; currency: string }) {
  const [values, setValues] = useState<Record<Field, Money>>(() => ({
    fixed_costs: { amount: '', currency },
    logistics_per_unit: { amount: '0', currency },
    tier1_price: { amount: '', currency },
    tier1_packaging: { amount: '0', currency },
  }))
  const [origin, setOrigin] = useState('')
  const [result, setResult] = useState<MakeOrBuyResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const set = (k: Field, part: Partial<Money>) => {
    setResult(null) // un résultat ne s'affiche que pour les saisies qui l'ont produit
    setValues((v) => ({ ...v, [k]: { ...v[k], ...part } }))
  }
  const request = useRef(0)
  const submit = () => {
    const req = ++request.current
    setError(null)
    setResult(null)
    const norm = (m: Money): Money => ({ amount: decimalInput(m.amount), currency: m.currency.trim().toUpperCase() })
    api
      .makeOrBuy(bomID, nodeID, {
        fixed_costs: norm(values.fixed_costs),
        logistics_per_unit: norm(values.logistics_per_unit),
        tier1_price: norm(values.tier1_price),
        tier1_packaging: norm(values.tier1_packaging),
        tier1_origin: origin.trim().toUpperCase(),
      })
      .then((r) => {
        if (req === request.current) setResult(r)
      })
      .catch((e: unknown) => {
        if (req === request.current) setError(errorMessage(e))
      })
  }
  const diff = result?.difference
  return (
    <details>
      <summary>Fabriquer ou acheter</summary>
      <form
        className="stack small"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        {fields.map((f) => (
          <label key={f.key} className="row">
            {f.label}{' '}
            <input aria-label={f.label} inputMode="decimal" size={8} required value={values[f.key].amount} onChange={(e) => set(f.key, { amount: e.target.value })} />
            <input aria-label={`Devise ${f.label}`} size={3} value={values[f.key].currency} onChange={(e) => set(f.key, { currency: e.target.value })} />
          </label>
        ))}
        <label className="row">
          Origine Tier 1 <input aria-label="Origine Tier 1" size={3} required pattern="[A-Za-z]{2}" value={origin} onChange={(e) => setOrigin(e.target.value)} />
        </label>
        <button type="submit">Simuler</button>
      </form>
      {error && <p className="error">{error}</p>}
      {result && (
        <>
          <table className="compact">
            <thead>
              <tr>
                <th>Par voiture</th>
                <th>Fabriquer</th>
                <th>Acheter</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Coûts fixes ÷ {result.volume}</td>
                <td>{formatMoney(result.fixed_per_car)}</td>
                <td />
              </tr>
              {result.make.map((m) => (
                <tr key={`make-${m.label}`}>
                  <td>{labels[m.label] ?? m.label}</td>
                  <td>
                    <Amount line={m} />
                  </td>
                  <td />
                </tr>
              ))}
              {result.buy.map((b) => (
                <tr key={`buy-${b.label}`}>
                  <td>{labels[b.label] ?? b.label}</td>
                  <td />
                  <td>
                    <Amount line={b} />
                  </td>
                </tr>
              ))}
              <tr>
                <td>
                  <strong>Total</strong>
                </td>
                <td>
                  <strong>{result.make_per_car ? formatMoney(result.make_per_car) : 'incomplet'}</strong>
                </td>
                <td>
                  <strong>{result.buy_per_car ? formatMoney(result.buy_per_car) : 'incomplet'}</strong>
                </td>
              </tr>
            </tbody>
          </table>
          {result.error && <p className="over small">{result.error}</p>}
          {result.rates.length > 0 && (
            <p className="muted small">
              Taux : {result.rates.map((r) => `1 ${r.from} = ${r.rate} ${r.to} (${r.source})`).join(' · ')}
            </p>
          )}
          {diff && (
            <p className="small">
              Écart (fabriquer − acheter) : <strong>{formatMoney(diff)}</strong> par voiture à {result.volume} voitures.{' '}
              {result.never_profitable
                ? 'Fabriquer ne devient jamais rentable (achat ≤ coût variable).'
                : result.break_even_volume !== null
                  ? `Fabriquer devient rentable à partir de ${result.break_even_volume} voitures.`
                  : 'Volume d\'équilibre non calculable.'}
            </p>
          )}
        </>
      )}
    </details>
  )
}

function Amount({ line }: { line: CostLine }) {
  return line.amount ? (
    <>{formatMoney(line.amount)}</>
  ) : (
    <span className="over" title={line.error ?? undefined}>
      manquant
    </span>
  )
}

