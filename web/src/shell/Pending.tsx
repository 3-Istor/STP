import type { ReactNode } from 'react'

// Pending : encart des maquettes I4 dont les données n'existent pas encore. Il dit quel lot les livre,
// au lieu d'afficher des chiffres inventés.
export function Pending({ title, lot, children }: { title: string; lot: string; children?: ReactNode }) {
  return (
    <section className="block pending" aria-label={title}>
      <h3>
        {title} <span className="illu">contenu complet livré par le {lot}</span>
      </h3>
      {children && <div className="in muted">{children}</div>}
    </section>
  )
}
