import { bomRoute, href, STEP_LABELS, STEPS, type Route, type Step, type Target } from '../router.ts'

export type StepBadges = Partial<Record<Step, { text: string; level?: 'warn' | 'over' | 'ok' }>>

type Props = {
  route: Route
  bom: { target: Target; name: string; label: string } | null // BOM ouverte : nom et « brouillon » / « snapshot #N »
  badges: StepBadges
}

// TopBar : barre du haut sombre des maquettes I4 — marque, fil d'Ariane, 5 étapes (vision-cible §4).
// Les étapes sont de vrais liens (#/boms/1/chiffrer?node=…) : la sélection de nœud suit d'une étape à l'autre.
export function TopBar({ route, bom, badges }: Props) {
  const current = route.page === 'bom' ? route.step : null
  return (
    <header className="top">
      <a className="brand" href={href({ page: 'boms' })}>
        <span className="logo" aria-hidden="true">
          B
        </span>
        BOM Studio
      </a>
      <nav className="crumbs" aria-label="Fil d’Ariane">
        {bom ? (
          <>
            <a href={href({ page: 'boms' })}>Toutes les BOM</a> <span className="sep">/</span> <b>{bom.name}</b>{' '}
            <span className="sep">·</span> <span>{bom.label}</span>
          </>
        ) : route.page === 'suppliers' ? (
          <>
            <a href={href({ page: 'boms' })}>Toutes les BOM</a> <span className="sep">/</span> <b>Fournisseurs</b>
          </>
        ) : (
          <b>Toutes les BOM</b>
        )}
      </nav>
      {bom && route.page === 'bom' && (
        <nav className="mods" aria-label="Étapes">
          {STEPS.map((s, i) => (
            <span key={s} className="mod-wrap">
              {i > 0 && (
                <span className="sep" aria-hidden="true">
                  ›
                </span>
              )}
              <a
                className={`mod ${s === current ? 'on' : ''}`}
                aria-current={s === current ? 'page' : undefined}
                href={href(bomRoute(bom.target, s, { node: route.node }))}
              >
                <span className="n">{i + 1}</span>
                <b>{STEP_LABELS[s]}</b>
                {badges[s] && <span className={`bdg ${badges[s]!.level ?? ''}`}>{badges[s]!.text}</span>}
              </a>
            </span>
          ))}
        </nav>
      )}
    </header>
  )
}
