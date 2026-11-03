import { useState, type FormEvent } from 'react'
import type { Explanation, NewNode, NodePatch, TreeNode } from '../api.ts'
import { partName, type Catalog } from '../catalog.ts'
import { formatDate, formatDecimal, formatMoney, multiplyDecimal } from '../money.ts'

type Props = {
  node: TreeNode
  explanation: Explanation
  volume: number
  catalog: Catalog
  readOnly: boolean
  isRoot: boolean
  onAdd: (n: NewNode) => Promise<boolean>
  onPatch: (p: NodePatch) => Promise<boolean>
  onStartMove: () => void
  onDelete: () => void
}

// NodePanel : détail d'un nœud à la demande (AGENTS §5) — totaux, sur quoi ils reposent, quantités
// commandées — et les actions du brouillon sur ce nœud.
export function NodePanel(p: Props) {
  const { node, explanation, volume, catalog, readOnly, isRoot } = p
  const t = node.totals
  const isPart = node.part_id !== null
  const title = isPart ? partName(catalog, node.part_id!) : node.name
  const offer = isPart ? catalog.byPart.get(node.part_id!)?.offers.find((o) => o.id === node.offer_id) : undefined

  return (
    <aside className="panel" aria-label="Détail du nœud">
      <h2>{title}</h2>
      <p className="muted">
        {isPart ? `Pièce #${node.part_id}, offre #${node.offer_id}${offer ? ` (${offer.supplier})` : ''}` : 'Sous-ensemble'}
      </p>

      {t && (
        <dl className="facts">
          <dt>Coût total</dt>
          <dd className={t.cost_error_code ? 'over' : undefined}>
            {t.cost ? formatMoney(t.cost) : (t.cost_error ?? '—')}
          </dd>
          {t.missing_cost.length > 0 && (
            <>
              <dt>Sans prix</dt>
              <dd className="over">nœuds {t.missing_cost.join(', ')}</dd>
            </>
          )}
          {t.own_cost && (
            <>
              <dt>Coût propre</dt>
              <dd>{formatMoney(t.own_cost)}</dd>
            </>
          )}
          <dt>Quantité réelle</dt>
          <dd>{formatDecimal(t.real_quantity)} par véhicule</dd>
          <dt>Quantité commandée</dt>
          {/* Produit fait par BigInt sur la chaîne décimale : pas de float (AGENTS §3). */}
          <dd>
            {formatDecimal(multiplyDecimal(t.real_quantity, volume))} pour {formatDecimal(String(volume))} véhicules
          </dd>
          <dt>CO2</dt>
          <dd>{formatDecimal(t.co2_kg, 2)} kg</dd>
          <dt>Masse</dt>
          <dd>{formatDecimal(t.mass_kg, 2)} kg</dd>
          <dt>Délai</dt>
          <dd>
            {t.lead_time_days} j (dont {t.own_lead_time_days} j propres)
          </dd>
        </dl>
      )}

      <details>
        <summary>Sur quoi reposent ces totaux</summary>
        <ul className="explain">
          <li>Devise : {explanation.currency}</li>
          <li>
            {explanation.price_snapshot_id === null
              ? 'Prix en direct (dernière cotation)'
              : `Prix figés : snapshot de prix #${explanation.price_snapshot_id}`}
          </li>
          {t?.quote && (
            <li>
              Cotation : {formatMoney(t.quote.price)} ({t.quote.source}, {formatDate(t.quote.quoted_at)})
            </li>
          )}
          {t?.rate && (
            <li>
              Taux : 1 {t.rate.from} = {t.rate.rate} {t.rate.to} ({t.rate.source}, {formatDate(t.rate.quoted_at)})
            </li>
          )}
          {!t?.quote &&
            explanation.rates.map((r) => (
              <li key={`${r.from}-${r.to}`}>
                Taux : 1 {r.from} = {r.rate} {r.to} ({r.source}, {formatDate(r.quoted_at)})
              </li>
            ))}
        </ul>
      </details>

      {!readOnly && (
        <>
          <EditFields key={`edit-${node.id}`} node={node} onPatch={p.onPatch} />
          <div className="row">
            {!isRoot && (
              <button type="button" onClick={p.onStartMove}>
                Déplacer…
              </button>
            )}
            {!isRoot && (
              <button type="button" className="danger" onClick={p.onDelete}>
                Supprimer
              </button>
            )}
          </div>
          <AddChild key={`add-${node.id}`} parentID={node.id} catalog={catalog} onAdd={p.onAdd} />
        </>
      )}
    </aside>
  )
}

function EditFields({ node, onPatch }: { node: TreeNode; onPatch: (p: NodePatch) => Promise<boolean> }) {
  const [qty, setQty] = useState(node.quantity)
  const [days, setDays] = useState(String(node.production_days))
  const isPart = node.part_id !== null
  return (
    <div className="stack">
      {node.parent_id !== null && (
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault()
            void onPatch({ quantity: qty.trim() })
          }}
        >
          <label>
            Quantité <input aria-label="Quantité" inputMode="decimal" size={6} value={qty} onChange={(e) => setQty(e.target.value)} />
          </label>
          <button type="submit" disabled={qty.trim() === node.quantity}>
            OK
          </button>
        </form>
      )}
      {/* Les jours de production d'une pièce viennent du catalogue (choix T6) : seulement pour un sous-ensemble. */}
      {!isPart && (
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault()
            void onPatch({ production_days: Number.parseInt(days, 10) })
          }}
        >
          <label>
            Jours de production{' '}
            <input aria-label="Jours de production" type="number" min={0} step={1} value={days} onChange={(e) => setDays(e.target.value)} />
          </label>
          <button type="submit" disabled={days === String(node.production_days)}>
            OK
          </button>
        </form>
      )}
    </div>
  )
}

function AddChild({
  parentID,
  catalog,
  onAdd,
}: {
  parentID: number
  catalog: Catalog
  onAdd: (n: NewNode) => Promise<boolean>
}) {
  const [kind, setKind] = useState<'part' | 'group'>(catalog.available ? 'part' : 'group')
  const [name, setName] = useState('')
  const [partID, setPartID] = useState(catalog.parts[0]?.id ?? 0)
  const part = catalog.byPart.get(partID)
  const [offerID, setOfferID] = useState(part?.offers[0]?.id ?? 0)
  const [qty, setQty] = useState('1')
  const [days, setDays] = useState('0')

  async function submit(e: FormEvent) {
    e.preventDefault()
    const ok =
      kind === 'part'
        ? await onAdd({ parent_id: parentID, part_id: partID, offer_id: offerID, quantity: qty.trim() })
        : await onAdd({
            parent_id: parentID,
            name: name.trim(),
            quantity: qty.trim(),
            production_days: Number.parseInt(days, 10),
          })
    if (ok) setName('')
  }

  return (
    <form className="stack add" onSubmit={(e) => void submit(e)}>
      <h3>Ajouter dessous</h3>
      <div className="row" role="radiogroup">
        <label>
          <input
            type="radio"
            aria-label="Pièce"
            checked={kind === 'part'}
            disabled={!catalog.available}
            onChange={() => setKind('part')}
          />{' '}
          Pièce
        </label>
        <label>
          <input type="radio" aria-label="Sous-ensemble" checked={kind === 'group'} onChange={() => setKind('group')} /> Sous-ensemble
        </label>
      </div>
      {kind === 'part' ? (
        <>
          <select
            aria-label="Pièce"
            value={partID}
            onChange={(e) => {
              const id = Number(e.target.value)
              setPartID(id)
              setOfferID(catalog.byPart.get(id)?.offers[0]?.id ?? 0)
            }}
          >
            {catalog.parts.map((pt) => (
              <option key={pt.id} value={pt.id}>
                {pt.name}
              </option>
            ))}
          </select>
          <select aria-label="Offre" value={offerID} onChange={(e) => setOfferID(Number(e.target.value))}>
            {part?.offers.map((o) => (
              <option key={o.id} value={o.id}>
                {o.supplier} · {o.price ? formatMoney(o.price) : 'sans prix'} · {o.lead_time_days} j
              </option>
            ))}
          </select>
        </>
      ) : (
        <>
          <input required placeholder="Nom du sous-ensemble" value={name} onChange={(e) => setName(e.target.value)} />
          <label>
            Jours de production{' '}
            <input aria-label="Jours de production" type="number" min={0} step={1} value={days} onChange={(e) => setDays(e.target.value)} />
          </label>
        </>
      )}
      <label>
        Quantité <input aria-label="Quantité" required inputMode="decimal" size={6} value={qty} onChange={(e) => setQty(e.target.value)} />
      </label>
      <button type="submit">Ajouter</button>
    </form>
  )
}
