import { useState, type FormEvent } from 'react'
import type { BOM } from '../api.ts'

type Props = {
  boms: BOM[]
  selectedID: number | null
  onSelect: (id: number) => void
  onCreate: (b: { name: string; currency: string; volume: number }) => Promise<void>
  onDelete: (b: BOM) => Promise<void>
}

// BomList : liste des BOM et création d'une BOM (nom, devise de référence, volume).
export function BomList({ boms, selectedID, onSelect, onCreate, onDelete }: Props) {
  const [name, setName] = useState('')
  const [currency, setCurrency] = useState('EUR')
  const [volume, setVolume] = useState('1000')

  async function submit(e: FormEvent) {
    e.preventDefault()
    await onCreate({ name: name.trim(), currency: currency.trim().toUpperCase(), volume: Number.parseInt(volume, 10) })
    setName('')
  }

  return (
    <nav className="bom-list" aria-label="BOM">
      <h2>BOM</h2>
      <ul>
        {boms.map((b) => (
          <li key={b.id} className={b.id === selectedID ? 'selected' : undefined}>
            <button type="button" className="link" onClick={() => onSelect(b.id)}>
              {b.name}
            </button>
            <span className="muted">
              {b.currency} · {b.volume} u.{b.price_snapshot_id !== null ? ' · figée' : ''}
            </span>
            <button
              type="button"
              className="icon"
              title="Supprimer la BOM"
              aria-label={`Supprimer ${b.name}`}
              onClick={() => void onDelete(b)}
            >
              ×
            </button>
          </li>
        ))}
        {boms.length === 0 && <li className="muted">Aucune BOM</li>}
      </ul>
      <form onSubmit={(e) => void submit(e)} className="stack">
        <input required placeholder="Nom de la BOM" value={name} onChange={(e) => setName(e.target.value)} />
        <div className="row">
          <input
            required
            aria-label="Devise"
            pattern="[A-Za-z]{3}"
            size={4}
            value={currency}
            onChange={(e) => setCurrency(e.target.value)}
          />
          <input
            required
            aria-label="Volume"
            type="number"
            min={1}
            step={1}
            value={volume}
            onChange={(e) => setVolume(e.target.value)}
          />
        </div>
        <button type="submit">Créer</button>
      </form>
    </nav>
  )
}
