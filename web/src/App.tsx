import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, errorMessage, type BOM } from './api.ts'
import { emptyCatalog, loadCatalog, type Catalog } from './catalog.ts'
import { CurrenciesContext, currencyChoices, loadCurrencies } from './currencies.ts'
import { AllBoms } from './home/AllBoms.tsx'
import { BomWorkspace } from './bom/BomWorkspace.tsx'
import { useBomData, type BomData } from './bom/useBomData.ts'
import { useLoad } from './bom/useLoad.ts'
import { formatDecimal } from './money.ts'
import { href, useRoute, type Route } from './router.ts'
import { StatusBar } from './shell/StatusBar.tsx'
import { TopBar, type StepBadges } from './shell/TopBar.tsx'
import SuppliersPage from './suppliers/SuppliersPage.tsx'
import { useTheme, type Theme } from './ui/theme.ts'
import './shell/shell.css'

const count = (n: number) => formatDecimal(String(n), 0)

// App : une seule structure pour tout BOM Studio (V2, ADR 0034) — barre du haut, contenu, barre d'état.
// L'écran courant vient de l'URL (router.ts).
export default function App() {
  const [route, go] = useRoute()
  const [theme, toggleTheme] = useTheme()
  const [boms, setBOMs] = useState<BOM[]>([])
  const [listError, setListError] = useState<string | null>(null)
  const [catalog, setCatalog] = useState<Catalog>(emptyCatalog)
  const [fxCurrencies, setFxCurrencies] = useState<string[]>([])

  const refresh = useCallback(() => {
    api
      .listBOMs()
      .then((b) => {
        setBOMs(b)
        setListError(null)
      })
      .catch((e: unknown) => setListError(errorMessage(e)))
  }, [])

  useEffect(() => {
    refresh()
    loadCatalog()
      .then(setCatalog)
      .catch((e: unknown) => setListError(errorMessage(e)))
    // Taux illisibles : le choix se limite aux devises des BOM existantes.
    loadCurrencies()
      .then(setFxCurrencies)
      .catch(() => setFxCurrencies([]))
  }, [refresh])

  const currencies = useMemo(() => currencyChoices(fxCurrencies, boms.map((b) => b.currency)), [fxCurrencies, boms])
  const bar = { theme, onToggleTheme: toggleTheme }

  let screen: ReactNode
  switch (route.page) {
    case 'boms':
      screen = (
        <Frame route={route} bar={{ ...bar, items: [`${count(boms.length)} BOM`], right: null }}>
          <AllBoms boms={boms} error={listError} onChanged={refresh} />
        </Frame>
      )
      break
    case 'suppliers':
      screen = (
        <Frame route={route} bar={{ ...bar, items: ['Fournisseurs, offres et contrats'], right: null }}>
          <div className="work">
            <main className="main4" id="main-content">
              <SuppliersPage selectedID={route.supplier} onSelect={(id) => go({ page: 'suppliers', supplier: id }, { replace: true })} />
            </main>
          </div>
        </Frame>
      )
      break
    case 'bom':
      screen = (
        <BomScreen
          key={`${route.target.kind}-${route.target.id}`}
          route={route}
          catalog={catalog}
          bar={bar}
          onBOMChanged={refresh}
          onPublished={refresh}
        />
      )
      break
    case 'not_found':
      screen = (
        <Frame route={route} bar={{ ...bar, items: [], right: null }}>
          <div className="work">
            <main className="main4" id="main-content">
              <section className="block">
                <h3>Page introuvable</h3>
                <div className="in">
                  <p>
                    Aucune page ne correspond à « {route.hash} ». <a href={href({ page: 'boms' })}>Toutes les BOM</a>
                  </p>
                </div>
              </section>
            </main>
          </div>
        </Frame>
      )
      break
  }

  return <CurrenciesContext value={currencies}>{screen}</CurrenciesContext>
}

type BarProps = { theme: Theme; onToggleTheme: () => void }

function Frame({
  route,
  bom = null,
  badges = {},
  bar,
  children,
}: {
  route: Route
  bom?: Parameters<typeof TopBar>[0]['bom']
  badges?: StepBadges
  bar: BarProps & { items: string[]; right: string | null }
  children: ReactNode
}) {
  return (
    <div className="app">
      <a className="skip" href="#main-content">
        Aller au contenu
      </a>
      <TopBar route={route} bom={bom} badges={badges} />
      {children}
      <StatusBar {...bar} />
    </div>
  )
}

function BomScreen({
  route,
  catalog,
  bar,
  onBOMChanged,
  onPublished,
}: {
  route: Extract<Route, { page: 'bom' }>
  catalog: Catalog
  bar: BarProps
  onBOMChanged: () => void
  onPublished: () => void
}) {
  const { state, error } = useBomData(route.target, catalog, onBOMChanged)
  const badgeData = useBadgeData(state.status === 'ready' ? state.data : null)
  if (state.status !== 'ready') {
    const label = route.target.kind === 'draft' ? 'brouillon' : `snapshot #${route.target.id}`
    return (
      <Frame route={route} bom={{ target: route.target, name: '…', label }} bar={{ ...bar, items: [], right: null }}>
        <div className="work">
          <main className="main4" id="main-content">
            {state.status === 'error' ? (
              <p role="alert" className="error">
                {state.error}
              </p>
            ) : (
              <p className="muted">Chargement…</p>
            )}
          </main>
        </div>
      </Frame>
    )
  }
  const data = state.data
  const badges = stepBadges(data.nodes.length, badgeData)
  const label = data.snapshot ? `snapshot #${data.snapshot.id}` : 'brouillon'
  return (
    <Frame
      route={route}
      bom={{ target: route.target, name: data.name, label }}
      badges={badges}
      bar={{
        ...bar,
        items: [`${count(data.nodes.length)} nœuds`, data.snapshot ? `Snapshot #${data.snapshot.id} · lecture seule` : `Brouillon #${data.target.id}`],
        right: `${data.currency} · ${count(data.volume)} véhicules`,
      }}
    >
      <BomWorkspace route={route} data={data} error={error} onPublished={onPublished} />
    </Frame>
  )
}

// Données des badges d'étape, relues avec l'arbre (brouillon seulement) : anomalies €/kg (Chiffrer, ADR 0037)
// et contrôles avant publication (Publier « ok sur total », Sourcer « contrat expiré », ADR 0033). Une
// lecture en échec retire le badge : aucun chiffre par défaut.
function useBadgeData(data: BomData | null) {
  const draft = data && !data.readOnly ? data.target.id : null
  const key = draft === null ? null : `${draft}-${data!.version}`
  const anomalies = useLoad(key, () => api.partAnomalies(draft!))
  const checks = useLoad(key, () => api.publicationChecks(draft!))
  return {
    anomalyCount: anomalies.status === 'ready' ? anomalies.data.anomaly_count : null,
    checks: checks.status === 'ready' ? checks.data : null,
  }
}

function stepBadges(nodes: number, b: ReturnType<typeof useBadgeData>): StepBadges {
  const badges: StepBadges = { structurer: { text: count(nodes), title: 'nœuds' } }
  if (b.anomalyCount !== null && b.anomalyCount > 0) badges.chiffrer = { text: count(b.anomalyCount), level: 'warn', title: 'anomalies €/kg' }
  if (b.checks) {
    const ok = b.checks.filter((c) => c.level === 'ok').length
    const level = b.checks.some((c) => c.level === 'block') ? 'over' : ok < b.checks.length ? 'warn' : 'ok'
    badges.publier = { text: `${ok}/${b.checks.length}`, level, title: 'contrôles avant publication passés' }
    const expired = b.checks.find((c) => c.code === 'expired_contracts' && c.level !== 'ok')
    if (expired) badges.sourcer = { text: '!', level: 'over', title: `${expired.title} : ${expired.detail}` }
  }
  return badges
}
