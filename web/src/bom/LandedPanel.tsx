import { useRef, useState } from 'react'
import { CurrencySelect } from '../CurrencySelect.tsx'
import { api, errorMessage, type LandedComparison, type LandedComponent, type OfferLanded, type TreeNode } from '../api.ts'
import { type Catalog } from '../catalog.ts'
import { decimalInput, formatMoney } from '../money.ts'

type Props = {
  bomID: number
  node: TreeNode // une pièce
  catalog: Catalog
  destination: string | null
}

const rows: { key: 'purchase' | 'customs' | 'freight' | 'packaging'; label: string }[] = [
  { key: 'purchase', label: 'Achat' },
  { key: 'customs', label: 'Douane' },
  { key: 'freight', label: 'Transport' },
  { key: 'packaging', label: 'Emballage' },
]

// LandedPanel : coût total rendu d'une unité, offres de la pièce côte à côte (ADR 0025). Le calcul est fait
// par l'API ; une composante manquante se renseigne sur place (table de douane, de transport, emballage).
export function LandedPanel({ bomID, node, catalog, destination }: Props) {
  const offers = catalog.byPart.get(node.part_id!)?.offers ?? []
  const [selected, setSelected] = useState<number[]>(() => {
    const others = offers.filter((o) => o.id !== node.offer_id).map((o) => o.id)
    return [node.offer_id!, ...others].slice(0, 2)
  })
  const [data, setData] = useState<LandedComparison | null>(null)
  const [error, setError] = useState<string | null>(null)
  // Seule la dernière requête lancée est affichée (cocher/décocher vite ne mélange pas les sélections).
  const request = useRef(0)
  const load = (ids: number[]) => {
    const req = ++request.current
    setError(null)
    if (ids.length === 0) {
      setData(null)
      return
    }
    api
      .landedCost(bomID, node.part_id!, ids)
      .then((d) => {
        if (req === request.current) setData(d)
      })
      .catch((e: unknown) => {
        if (req === request.current) setError(errorMessage(e))
      })
  }
  // Offres choisies absentes du catalogue actif (offre actuelle archivée) : décochables quand même.
  const extra = selected.filter((id) => !offers.some((o) => o.id === id))
  const toggle = (id: number) => {
    const next = selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]
    setSelected(next)
    load(next)
  }
  const fill = (action: () => Promise<unknown>) => {
    action()
      .then(() => load(selected))
      .catch((e: unknown) => setError(errorMessage(e)))
  }
  const supplier = (offerID: number) => offers.find((o) => o.id === offerID)?.supplier ?? `offre #${offerID}`

  return (
    <details
      onToggle={(e) => {
        if ((e.currentTarget as HTMLDetailsElement).open && data === null) load(selected)
      }}
    >
      <summary>Coût rendu (comparer les origines)</summary>
      {destination === null && <p className="over small">Renseignez le pays de destination de la BOM pour la douane et le transport.</p>}
      <div className="row small">
        {offers.map((o) => (
          <label key={o.id}>
            <input type="checkbox" checked={selected.includes(o.id)} onChange={() => toggle(o.id)} /> {o.supplier} ({o.origin_country})
          </label>
        ))}
        {extra.map((id) => (
          <label key={id}>
            <input type="checkbox" checked onChange={() => toggle(id)} /> offre #{id} (archivée)
          </label>
        ))}
      </div>
      {error && <p className="error">{error}</p>}
      {data && (
        <table className="compact">
          <thead>
            <tr>
              <th />
              {data.offers.map((o) => (
                <th key={o.offer_id}>
                  {supplier(o.offer_id)} {o.origin_country}→{o.destination_country ?? '?'}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.key}>
                <td>{r.label}</td>
                {data.offers.map((o) => (
                  <td key={o.offer_id}>
                    <Cell component={o[r.key]} offer={o} currency={data.currency} onFill={fill} />
                  </td>
                ))}
              </tr>
            ))}
            <tr>
              <td>
                <strong>Total rendu</strong>
              </td>
              {data.offers.map((o) => (
                <td
                  key={o.offer_id}
                  title={[
                    o.error ?? '',
                    o.duty_percent !== null ? `douane ${o.duty_percent} %` : '',
                    o.freight_per_kg ? `transport ${formatMoney(o.freight_per_kg)}/kg` : '',
                    ...o.rates.map((r) => `1 ${r.from} = ${r.rate} ${r.to} (${r.source})`),
                    'tables logistiques en direct',
                  ]
                    .filter(Boolean)
                    .join('\n')}
                >
                  <strong className={o.total ? undefined : 'over'}>{o.total ? formatMoney(o.total) : 'incomplet'}</strong>
                </td>
              ))}
            </tr>
          </tbody>
        </table>
      )}
    </details>
  )
}

// Cell : montant, ou raison de l'absence avec, pour une table manquante, un champ pour la renseigner.
function Cell({
  component,
  offer,
  currency: bomCurrency,
  onFill,
}: {
  component: LandedComponent
  offer: OfferLanded
  currency: string
  onFill: (a: () => Promise<unknown>) => void
}) {
  const [value, setValue] = useState('')
  const [currency, setCurrency] = useState(bomCurrency)
  if (component.amount) return <>{formatMoney(component.amount)}</>
  const dest = offer.destination_country
  const editable =
    (component.error_code === 'no_customs_duty' || component.error_code === 'no_freight_rate') && dest !== null
      ? true
      : component.error_code === 'no_packaging'
  if (!editable) {
    return (
      <span className="over" title={component.error ?? undefined}>
        manquant
      </span>
    )
  }
  const submit = () => {
    const v = decimalInput(value)
    if (v === '') return
    const money = { amount: v, currency }
    if (component.error_code === 'no_customs_duty') onFill(() => api.setCustomsDuty(offer.origin_country, dest!, v))
    else if (component.error_code === 'no_freight_rate') onFill(() => api.setFreightRate(offer.origin_country, dest!, money))
    else onFill(() => api.setPackaging(offer.offer_id, money))
  }
  return (
    <form
      className="row"
      title={component.error ?? undefined}
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <input aria-label="Valeur" inputMode="decimal" size={5} value={value} onChange={(e) => setValue(e.target.value)} />
      {component.error_code === 'no_customs_duty' ? (
        <span>%</span>
      ) : (
        <>
          <CurrencySelect aria-label="Devise" value={currency} onChange={setCurrency} />
          {component.error_code === 'no_freight_rate' && <span>/kg</span>}
        </>
      )}
      <button type="submit" className="link">
        OK
      </button>
    </form>
  )
}
