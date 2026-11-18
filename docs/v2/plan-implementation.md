# V2 — plan d'implémentation

Source : `docs/v2/vision-cible.md` (validée par le PO et Brian le 17/11/2026) et les maquettes « itération 4 »
(artefact « BOM Studio V2 — maquettes cibles »). Branche d'intégration : `v2/implementation`.
Chaque lot est un ticket `.specs/v2-lot-<x>/` (spec.md + state.md, format `.specs/AGENTS.md`) sur la branche `v2-lot-<x>`, est relu par un agent à contexte vierge, puis fusionné.

## Règles communes à tous les lots

- `AGENTS.md` fait foi : montants décimaux + devise (jamais `float`/`number`), domaine sans `net/http`,
  handlers fins, tests table-driven qui peuvent échouer, erreurs nommées, `slog`, pas de `console.log`.
- Toute décision du §7 d'AGENTS → une ADR **au statut Proposé** dans `docs/adr/` (numéros réservés ci-dessous).
- Migrations : nouvelles uniquement, numéros réservés ci-dessous. Jamais modifier une migration existante.
- Données de démo : `internal/sampledata` (seed idempotent). Toute donnée ajoutée pour la démo y est.
- Base locale : `source /home/claude/devdb.sh <base_du_lot>` (démarre Postgres si besoin, crée la base,
  exporte `DATABASE_URL`), puis `go tool goose -dir migrations postgres "$DATABASE_URL" up`.
- Définition de « terminé » : `go vet ./...`, `go tool golangci-lint run ./...`, `go test ./...` (avec
  `DATABASE_URL`), `cd web && npm ci && npm run lint && npx tsc -b && npx vite build` passent.
- Livrable AGENTS §9 dans le dernier message de commit du lot.
- Commits : `v2-lot-<x>: T<N> - <description>`, une intention par commit, trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Numéros réservés

| Lot | Migrations | ADR |
|---|---|---|
| A — coût complet | 00012 | 0031 méthode de calcul du coût |
| B — versions et publication | 00013 | 0032 comparaison et causes d'écart · 0033 contrôles avant publication et traçabilité |
| C — shell front | — | 0034 navigation en 5 étapes, routeur maison, thème |
| D — 3D | — | 0035 vue 3D par sous-ensemble, rendus simple et réaliste |
| E — analyses et profils | 00014 | 0036 profils de véhicule (pondérations) · 0037 anomalies en €/kg par famille |

(Les ADR 0031/0032 de la branche 3D d'Arthur ne sont pas sur `main` ; la nôtre 0035 les remplace.)

## Contrats d'API (figés pour paralléliser ; un lot peut ajouter des champs, jamais en retirer)

Tous sous `/api/v1`, JSON `snake_case`, montants `{"amount":"123.45","currency":"EUR"}`.

**Lot A**
- `GET /boms/{id}/cost-method` → `{"basis":"purchase|landed|full","fx":"budget|latest","program_volume":60000}`
- `PUT /boms/{id}/cost-method` même corps (brouillon seulement).
- Les totaux existants (`GET /boms/{id}/tree`, totaux de nœuds) suivent la méthode. L'explication des totaux
  porte `cost_method`.
- `GET /boms/{id}/nodes/{nodeId}/cost-breakdown` →
  `{"node_id":1,"currency":"EUR","method":{...},"lines":[{"kind":"price|fx|discount|customs|freight|packaging|tooling","label":"…","amount":{…},"source":"…","as_of":"RFC3339|null"}],"total":{…}}`
  Pour un sous-ensemble : lignes agrégées par `kind` sur les descendants.
- Snapshot : méthode, taux budget, tables douane/transport utilisées, outillage et volume programme figés.

**Lot B**
- `GET /compare?from=<ref>&to=<ref>` avec `<ref>` = `snapshot:<id>` ou `bom:<id>` →
  `{"from":{"label","totals":{cost,co2_kg,mass_kg,lead_time_days}},"to":{…},"delta":{…},
    "causes":[{"kind":"structure|offer_change|price|fx|discount|tooling","amount":{…},"explanation":"…","node_ids":[…]}],
    "changed_parts":[{"path":"…","before":"…","after":"…","delta":{…}}]}`
  La somme des `causes` = `delta.cost` exactement.
- `GET /boms/{id}/publication-checks` → `[{"code","level":"ok|warn|block","title","detail","node_ids":[]}]`
- Publication (route existante) accepte `{"note","proposed_by","validated_by","justifications":{"<code>":"…"}}` ;
  refus 409 si un contrôle `block` ; `warn` sans justification → 422. Champs conservés dans le snapshot.
- `GET|POST /boms/{id}/ideas`, `PATCH|DELETE /boms/{id}/ideas/{ideaId}` :
  `{"id","title","owner","estimated_gain":{…},"status":"idea|in_progress|to_validate|done|dropped","draft_bom_id":null}`
- `GET /boms/{id}/versions` → brouillons et snapshots de cette BOM, avec dates à la seconde, coût, % budget,
  snapshot de prix, proposé/validé par.

**Lot E**
- `GET /boms/{id}/analysis/fx-sensitivity?shock_percent=5` → `[{"currency","share_percent","impact":{…},"budget_percent_after"}]`
- `GET /boms/{id}/analysis/price-drivers?limit=10` → `[{"part_id","name","amplitude":{…}}]`
- `GET /boms/{id}/analysis/sourcing` → parts par pays, par fournisseur, poids des 5 premiers, pièces mono-source,
  contrats expirés encore utilisés, taux budget vs dernier taux.
- `GET /vehicle-profiles` (préréglages) ; `GET|PUT /boms/{id}/profile` `{"preset","weights":{"cost","co2","lead_time","mass","quality"}}`

## Lots et ordre

```
Vague 1 (parallèle) : A (back coût complet) · C (front shell) · D (3D)
Vague 2 (parallèle) : B (back versions)      · E (analyses + profils, back et front Chiffrer/Sourcer/Réglages)
Vague 3             : F (front Arbitrer, Publier, liste des BOM avec versions) · intégration 3D dans Structurer
Vague 4             : revue de conformité + revue qualité (agents à contexte vierge), e2e, captures, journal
```

## Écrans cibles (maquettes itération 4)

0 Toutes les BOM · 1 Structurer (explorateur, arbre + 3D, focus 3D) · 2 Chiffrer · 3 Sourcer (vue d'ensemble,
fournisseurs/offres/contrats) · 4 Arbitrer · 5 Publier. Colonne droite « Synthèse » commune (état, total, budget,
disponibilité, CO₂, masse, hors budget, barres par sous-système, prix utilisés sélectionnables, méthode), barre
d'état en bas avec bouton thème à gauche. Aucune fonction existante ne disparaît : elle change de place.
