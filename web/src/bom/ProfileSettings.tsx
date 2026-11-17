import { useState } from 'react'
import { api, type VehicleProfile, type Weights } from '../api.ts'
import { useLoad } from './useLoad.ts'
import { usePresets, WEIGHT_KEYS } from './profile.ts'
import type { BomData } from './useBomData.ts'

// ProfileSettings : profil de véhicule (maquette I4-1, onglet « Réglages de calcul », ADR 0036). Les
// préréglages imposent leurs poids ; « Personnalisé » laisse régler les cinq poids (entiers, somme 100,
// validée par l'API). Le profil classe les alternatives proposées (étape Arbitrer) quand aucune stratégie de
// nœud ne s'applique ; il ne change aucun total. Enregistré par PUT /boms/{id}/profile puis relecture.
export function ProfileSettings({ data }: { data: BomData }) {
  const { presets, error: presetsError } = usePresets()
  const target = data.target
  const current = useLoad(`${target.kind}-${target.id}-${data.version}`, () =>
    target.kind === 'draft' ? api.profile(target.id) : api.snapshotProfile(target.id),
  )
  if (presetsError) return <p className="error small">Profils indisponibles : {presetsError}</p>
  if (!presets || current.status === 'loading') return <p className="muted small">Chargement du profil…</p>
  if (current.status === 'error') return <p className="error small">Profil indisponible : {current.error}</p>
  return <ProfileEditor key={JSON.stringify(current.data)} data={data} profile={current.data} presets={presets} />
}

function ProfileEditor({ data, profile, presets }: { data: BomData; profile: VehicleProfile; presets: NonNullable<ReturnType<typeof usePresets>['presets']> }) {
  const readOnly = data.readOnly
  const custom = presets.find((p) => p.custom)
  const [weights, setWeights] = useState<Weights | null>(profile.weights)
  const edited = weights !== null && (profile.weights === null || WEIGHT_KEYS.some((k) => weights[k.key] !== profile.weights![k.key]))
  const chosen = presets.find((p) => p.preset === profile.preset)
  const sum = weights ? WEIGHT_KEYS.reduce((n, k) => n + weights[k.key], 0) : 0
  const save = (preset: string, w: Weights) => void data.run(() => api.setProfile(data.target.id, { preset, weights: w }))
  return (
    <section className="profile-settings" aria-label="Profil de véhicule">
      <h3 className="h3">Profil de véhicule</h3>
      <div className="chips" role="radiogroup" aria-label="Préréglage">
        {presets.map((p) => (
          <button
            key={p.preset}
            type="button"
            role="radio"
            aria-checked={profile.preset === p.preset}
            className={`chip ${profile.preset === p.preset ? 'on' : ''}`}
            disabled={readOnly || data.busy}
            title={p.hint}
            onClick={() => {
              if (p.custom) setWeights(weights ?? p.weights)
              else if (p.weights && profile.preset !== p.preset) save(p.preset, p.weights)
            }}
          >
            {p.label}
          </button>
        ))}
      </div>
      <p className="muted small">
        {profile.preset === null
          ? 'Aucun profil : les alternatives suivent la stratégie du nœud, sinon l’ordre des offres.'
          : (chosen?.hint ?? profile.preset)}
      </p>
      {weights && (
        <div className="weights">
          {WEIGHT_KEYS.map((k) => (
            <label key={k.key} className="sl">
              <span>{k.label}</span>
              <input
                type="range"
                min={0}
                max={100}
                step={5}
                value={weights[k.key]}
                disabled={readOnly || data.busy}
                aria-label={`Poids ${k.label}`}
                onChange={(e) => setWeights({ ...weights, [k.key]: Number.parseInt(e.target.value, 10) })}
              />
              <b>{weights[k.key]}</b>
            </label>
          ))}
          <p className={`small ${sum === 100 ? 'muted' : 't-warn'}`}>Somme des poids : {sum} (100 attendu, vérifié par le serveur)</p>
          {edited && !readOnly && custom && (
            <div className="row">
              <button type="button" className="btn pri" disabled={data.busy} onClick={() => save(custom.preset, weights)}>
                Enregistrer en « {custom.label} »
              </button>
              <button type="button" className="btn" onClick={() => setWeights(profile.weights)}>
                Annuler
              </button>
            </div>
          )}
        </div>
      )}
      <p className="muted small">
        {readOnly ? 'Profil figé dans ce snapshot.' : 'Classe les alternatives proposées (étape Arbitrer). Rien ne change dans la BOM sans votre accord.'}
      </p>
    </section>
  )
}
