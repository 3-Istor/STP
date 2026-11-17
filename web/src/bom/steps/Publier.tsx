import { useState } from 'react'
import { api, errorMessage, type Snapshot } from '../../api.ts'
import { formatDateTime, formatMoney } from '../../money.ts'
import { bomRoute, href } from '../../router.ts'
import { Pending } from '../../shell/Pending.tsx'
import type { BomData } from '../useBomData.ts'

type Props = {
  data: BomData
  published: Snapshot[] // snapshots publiés connus de cette session (aucune liste côté API avant le lot B)
  onPublished: (s: Snapshot) => void
  onOpenDraft: (bomID: number) => void
}

// Publier (maquette I4-7) : « Est-ce prêt ? ». Fonctions existantes : publier un brouillon en snapshot
// immuable, ouvrir un snapshot, créer un nouveau brouillon depuis un snapshot. Contrôles avant publication,
// note de décision, proposé / validé par et liste complète des versions : lot B.
export function Publier({ data, published, onPublished, onOpenDraft }: Props) {
  const [done, setDone] = useState<Snapshot | null>(null)
  const [error, setError] = useState<string | null>(null)
  const cost = data.root?.totals?.cost ?? null

  if (data.snapshot) {
    const s = data.snapshot
    return (
      <>
        <section className="block" aria-label="Snapshot publié">
          <h3>Snapshot #{s.id} · publié le {formatDateTime(s.published_at)}</h3>
          <div className="in stack">
            <p className="muted">
              Lecture seule : structure, quantités, réglages, prix (snapshot de prix #{s.price_snapshot_id}) et taux figés à la publication.
            </p>
            <div className="row">
              <button
                type="button"
                className="btn pri"
                onClick={() => {
                  setError(null)
                  api
                    .newDraft(s.id)
                    .then((b) => onOpenDraft(b.id))
                    .catch((e: unknown) => setError(errorMessage(e)))
                }}
              >
                Nouveau brouillon depuis ce snapshot
              </button>
            </div>
            {error && <p role="alert" className="error">{error}</p>}
          </div>
        </section>
        <Pending title="Note de décision, proposé par, validé par" lot="lot B" />
      </>
    )
  }

  const bom = data.bom!
  const mine = published.filter((p) => p.bom_id === bom.id)
  return (
    <>
      <Pending title="Contrôles avant publication" lot="lot B">
        Offres actives, conversions, prix figés, date cible, tables douane et transport, budgets : chaque contrôle bloquant ou à justifier.
      </Pending>
      <section className="block" aria-label="Publication">
        <h3>Publier ce brouillon</h3>
        <div className="in stack">
          <p className="muted">
            Publier crée un snapshot immuable : structure, quantités, réglages, prix fournisseurs et taux de change copiés maintenant. Coût actuel :{' '}
            <b>{cost ? formatMoney(cost) : 'incomplet'}</b>.
          </p>
          <div className="row">
            <button
              type="button"
              className="btn pri"
              disabled={data.busy}
              onClick={() =>
                void data.run(async () => {
                  const s = await api.publish(bom.id)
                  setDone(s)
                  onPublished(s)
                })
              }
            >
              Publier
            </button>
          </div>
          {done && (
            <p role="status" className="notice">
              Snapshot #{done.id} publié ({formatMoney(cost)}).{' '}
              <a href={href(bomRoute({ kind: 'snapshot', id: done.id }, 'publier'))}>Ouvrir</a>
            </p>
          )}
        </div>
      </section>
      <section className="block" aria-label="Versions">
        <h3>Versions</h3>
        <table>
          <thead>
            <tr>
              <th>Version</th>
              <th>Date</th>
              <th>Prix utilisés</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <tr className="sel">
              <td>
                <b>{bom.name}</b> · brouillon
              </td>
              <td>créé le {formatDateTime(bom.created_at)}</td>
              <td>{bom.price_snapshot_id === null ? 'en direct' : `snapshot de prix #${bom.price_snapshot_id}`}</td>
              <td>en cours</td>
            </tr>
            {bom.source_snapshot_id !== null && (
              <tr>
                <td>snapshot #{bom.source_snapshot_id}</td>
                <td className="muted">source de ce brouillon</td>
                <td />
                <td>
                  <a href={href(bomRoute({ kind: 'snapshot', id: bom.source_snapshot_id }, 'publier'))}>Ouvrir</a>
                </td>
              </tr>
            )}
            {mine.map((s) => (
              <tr key={s.id}>
                <td>snapshot #{s.id}</td>
                <td>publié le {formatDateTime(s.published_at)}</td>
                <td>snapshot de prix #{s.price_snapshot_id}</td>
                <td>
                  <a href={href(bomRoute({ kind: 'snapshot', id: s.id }, 'publier'))}>Ouvrir</a>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="muted small in">Seuls les snapshots publiés depuis cet onglet ou à l’origine du brouillon sont listés : la liste complète des versions arrive avec le lot B.</p>
      </section>
    </>
  )
}
