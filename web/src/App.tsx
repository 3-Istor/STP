import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, errorMessage, type BOM, type Snapshot } from './api.ts'
import { emptyCatalog, loadCatalog, type Catalog } from './catalog.ts'
import { CurrenciesContext, currencyChoices, loadCurrencies } from './currencies.ts'
import { AllBoms } from './home/AllBoms.tsx'
import { BomWorkspace } from './bom/BomWorkspace.tsx'
import { useBomData } from './bom/useBomData.ts'
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
  // Snapshots publiés depuis cet onglet : l'API ne liste pas encore les versions (lot B).
  const [published, setPublished] = useState<Snapshot[]>([])

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
          <AllBoms boms={boms} published={published} error={listError} onChanged={refresh} />
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
          published={published}
          onPublished={(s) => setPublished((p) => [...p.filter((x) => x.id !== s.id), s])}
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
  published,
  onPublished,
}: {
  route: Extract<Route, { page: 'bom' }>
  catalog: Catalog
  bar: BarProps
  onBOMChanged: () => void
  published: Snapshot[]
  onPublished: (s: Snapshot) => void
}) {
  const { state, error } = useBomData(route.target, catalog, onBOMChanged)
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
  const anomalyCount = data.anomalies.data?.nodes.filter((n) => n.status === 'anomaly').length ?? 0
  const badges: StepBadges = { structurer: { text: count(data.nodes.length) } }
  if (anomalyCount > 0) badges.chiffrer = { text: count(anomalyCount), level: 'warn' }
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
      <BomWorkspace route={route} data={data} error={error} published={published} onPublished={onPublished} />
    </Frame>
  )
}
