// Page de démo isolée (composant ; montage dans main.tsx) de la vue 3D (lot D, ADR 0035), servie par Vite en dev : /vehicle3d-demo.html.
// Paramètres : ?bom=<id> (API) ou ?fixture=multipla|valhalla, &mode=simple|realistic, &color=budget|system,
// &explode=0..1, &select=<id de nœud>, &fakeBudget=1 (statuts budget FICTIFS, pour voir la coloration).
import { useEffect, useMemo, useState } from 'react'
import { Vehicle3D, type BudgetStatus, type ColorBy, type RenderMode, type Vehicle3DNode } from '../index.tsx'
import multipla from './multipla-nodes.json' with { type: 'json' }
import valhalla from './valhalla-nodes.json' with { type: 'json' }
import '../../../index.css'
import './demo.css'

const q = new URLSearchParams(location.search)
const FIXTURES: Record<string, { bom_id: number; bom_name: string; nodes: Vehicle3DNode[] }> = { multipla, valhalla }

/** Statuts budget de démonstration, affichés comme FICTIFS : l'API ne fournit pas encore de statut par nœud. */
function fakeStatus(name: string): BudgetStatus | null {
  if (/^(porte avant|moteur |habitacle)/i.test(name)) return 'over'
  if (/^(capot|train avant|roue avant|porte arri|rang 2)/i.test(name)) return 'warn'
  if (/^(coffre|faisceau de pavillon)$/i.test(name)) return null
  return 'ok'
}

export function Demo() {
  const [bomId, setBomId] = useState(Number(q.get('bom') ?? 1))
  const fixture = q.get('fixture')
  const [boms, setBoms] = useState<{ id: number; name: string }[]>([])
  const [loaded, setLoaded] = useState<{ bomId: number; nodes?: Vehicle3DNode[]; error?: string } | null>(null)
  const [mode, setMode] = useState<RenderMode>((q.get('mode') as RenderMode) ?? 'simple')
  const [colorBy, setColorBy] = useState<ColorBy>((q.get('color') as ColorBy) ?? 'material')
  const [explode, setExplode] = useState(Number(q.get('explode') ?? 0))
  const [fake, setFake] = useState(q.get('fakeBudget') === '1')
  const [selected, setSelected] = useState<number | null>(q.get('select') ? Number(q.get('select')) : null)

  useEffect(() => {
    if (fixture) return
    fetch('/api/v1/boms').then(r => r.json()).then(setBoms, () => setBoms([]))
  }, [fixture])

  useEffect(() => {
    if (fixture) return
    fetch(`/api/v1/boms/${bomId}/tree`)
      .then(async r => { if (!r.ok) throw new Error(`${r.status} ${(await r.json()).error ?? ''}`); return r.json() })
      .then((t: { nodes: Vehicle3DNode[] }) => setLoaded({ bomId, nodes: t.nodes.map(({ id, parent_id, name, part_id, quantity }) => ({ id, parent_id, name, part_id, quantity })) }))
      .catch((e: Error) => setLoaded({ bomId, error: `Arbre de la BOM ${bomId} indisponible : ${e.message}` }))
  }, [bomId, fixture])

  const current = loaded?.bomId === bomId ? loaded : null
  const fixtureNodes = fixture ? FIXTURES[fixture]?.nodes ?? [] : null
  const raw = fixtureNodes ?? current?.nodes ?? null
  const error = fixture ? null : current?.error ?? null

  const nodes = useMemo(() => raw && (fake ? raw.map(n => ({ ...n, budget_status: fakeStatus(n.name) })) : raw), [raw, fake])
  const outline = useMemo(() => {
    if (!nodes) return []
    const root = nodes.find(n => n.parent_id === null)
    const subs = (id: number) => nodes.filter(n => n.parent_id === id && n.part_id === null)
    return root ? subs(root.id).map(s => ({ system: s, children: subs(s.id) })) : []
  }, [nodes])
  const selectedName = nodes?.find(n => n.id === selected)?.name

  return (
    <div className="demo">
      <header className="demo-bar">
        <strong>Démo vue 3D</strong>
        {fixture ? <span>Fichier de nœuds : {FIXTURES[fixture]?.bom_name} (exporté de l’API)</span> : (
          <label>BOM <select value={bomId} onChange={e => { setBomId(Number(e.target.value)); setSelected(null) }}>
            {boms.map(b => <option key={b.id} value={b.id}>{b.name}</option>)}
          </select></label>
        )}
        <span className="demo-seg">Rendu
          <button aria-pressed={mode === 'simple'} onClick={() => setMode('simple')}>Simple</button>
          <button aria-pressed={mode === 'bound'} onClick={() => setMode('bound')}>Composants</button>
        </span>
        <span className="demo-seg">Colorer
          <button aria-pressed={colorBy === 'material'} onClick={() => setColorBy('material')}>Matériaux</button>
          <button aria-pressed={colorBy === 'budget'} onClick={() => setColorBy('budget')}>Budget</button>
          <button aria-pressed={colorBy === 'system'} onClick={() => setColorBy('system')}>Système</button>
        </span>
        <label>Éclaté <input type="range" min={0} max={1} step={0.05} value={explode} onChange={e => setExplode(Number(e.target.value))} /></label>
        <label><input type="checkbox" checked={fake} onChange={e => setFake(e.target.checked)} /> statuts budget <b>fictifs</b> (démo)</label>
        <span className="demo-sel">Sélection : {selectedName ? `${selectedName} (#${selected})` : '—'}</span>
      </header>
      <aside className="demo-tree" aria-label="Sous-ensembles">
        {error ? <p role="alert">{error}</p> : null}
        {!nodes && !error ? <p>Chargement de l’arbre…</p> : null}
        {outline.map(({ system, children }) => (
          <div key={system.id}>
            <button className={selected === system.id ? 'on sys' : 'sys'} onClick={() => setSelected(system.id)}>{system.name}</button>
            {children.map(c => (
              <button key={c.id} className={selected === c.id ? 'on' : ''} onClick={() => setSelected(c.id)}>{c.name}</button>
            ))}
          </div>
        ))}
      </aside>
      <main className="demo-view">
        {nodes ? (
          <Vehicle3D bomId={fixture ? FIXTURES[fixture]?.bom_id ?? 0 : bomId} nodes={nodes} selectedNodeId={selected}
            onSelectNode={setSelected} mode={mode} colorBy={colorBy} explode={explode} onResetView={() => setExplode(0)} />
        ) : null}
      </main>
    </div>
  )
}

