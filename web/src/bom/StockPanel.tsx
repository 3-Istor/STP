import { useState } from 'react'
import { api, errorMessage, type StockLine, type TreeNode } from '../api.ts'
import { decimalInput, formatDecimal, multiplyDecimal, sameDecimal } from '../money.ts'

type Props = {
  node: TreeNode // une pièce
  stock: StockLine[]
  volume: number
  readOnly: boolean
  onSetStock?: (partID: number, quantity: string) => Promise<boolean>
}

// StockPanel : stock de la pièce propre à cette BOM (ADR 0022), couverture calculée par l'API, et à la demande
// les alternatives du catalogue qui ont du stock dans cette BOM (suggestion, rien n'est remplacé).
export function StockPanel({ node, stock, volume, readOnly, onSetStock }: Props) {
  const partID = node.part_id!
  const own = stock.find((s) => s.part_id === partID)?.quantity ?? '0'
  const [qty, setQty] = useState(own)
  const t = node.totals
  return (
    <div className="stack">
      <dl className="facts">
        <dt>Stock</dt>
        <dd className={t?.in_stock ? 'ok' : undefined}>
          {t?.stock_covered === null || t?.stock_covered === undefined
            ? 'aucun'
            : t.in_stock
              ? `en stock (${formatDecimal(t.stock_covered)} couvertes, délai propre nul)`
              : `${formatDecimal(t.stock_covered)} couvertes sur ${formatDecimal(multiplyDecimal(t.real_quantity, volume))}`}
        </dd>
      </dl>
      {!readOnly && onSetStock && (
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault()
            void onSetStock(partID, decimalInput(qty))
          }}
        >
          <label>
            Stock de la pièce{' '}
            <input aria-label="Stock de la pièce" inputMode="decimal" size={6} value={qty} onChange={(e) => setQty(e.target.value)} />
          </label>
          <button type="submit" disabled={sameDecimal(decimalInput(qty), own)}>
            OK
          </button>
        </form>
      )}
      <StockedAlternatives partID={partID} stock={stock} />
    </div>
  )
}

function StockedAlternatives({ partID, stock }: { partID: number; stock: StockLine[] }) {
  const [alts, setAlts] = useState<{ id: number; name: string; quantity: string }[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  return (
    <details
      onToggle={(e) => {
        if (!(e.currentTarget as HTMLDetailsElement).open || alts !== null) return
        setError(null)
        api
          .alternatives(partID)
          .then((parts) => {
            const inStock = new Map(stock.map((s) => [s.part_id, s.quantity]))
            setAlts(parts.filter((p) => inStock.has(p.id)).map((p) => ({ id: p.id, name: p.name, quantity: inStock.get(p.id)! })))
          })
          .catch((err: unknown) => setError(errorMessage(err)))
      }}
    >
      <summary>Alternatives en stock</summary>
      {error ? (
        <p className="error">{error}</p>
      ) : alts === null ? (
        <p className="muted">Chargement…</p>
      ) : alts.length === 0 ? (
        <p className="muted">Aucune alternative déclarée n'a de stock dans cette BOM.</p>
      ) : (
        <ul className="explain">
          {alts.map((a) => (
            <li key={a.id}>
              {a.name} (#{a.id}) : {formatDecimal(a.quantity)} en stock
            </li>
          ))}
        </ul>
      )}
    </details>
  )
}
