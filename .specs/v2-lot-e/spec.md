---
id: v2-lot-e
title: V2 lot E — analyses (change, pièces influentes, sourcing, anomalies €/kg) et profils de véhicule (backend)
source: manual
source_ref: docs/v2/plan-implementation.md (lot E)
created: 2026-11-17
status: review
priority: P2
depends_on: [v2-lot-a]
branch: v2-lot-e
---

# V2 lot E — analyses et profils de véhicule (backend Go)

## Context

Vision V2 §2 (audit métier) et §5 (« différenciant ») : l'étape **Chiffrer** montre la sensibilité au change,
les pièces dont le prix pèse le plus et les anomalies en €/kg par famille ; l'étape **Sourcer** montre l'origine
du coût par pays, le poids des 5 premiers fournisseurs, les pièces mono-source, les contrats expirés encore
utilisés et l'écart taux budget / dernier taux. L'onglet **Réglages** porte le profil du véhicule (pondérations
coût/CO₂/délai/masse/qualité) qui classe les alternatives. Maquettes `I4-4-Chiffrer.png`, `I4-5-Sourcer.png`,
`I4-1-Explorateur.dc.html`. Le lot A a livré le moteur de coût (méthode `basis × fx`, ADR 0031) : toutes les
analyses chiffrées réutilisent ce moteur (`ComputeTotals`) pour rester cohérentes au centime avec les totaux.
Pas de front dans ce lot.

## Research Findings

### Relevant Files
- `internal/bom/costcalc.go`, `costfacts.go`, `costmethod.go` — moteur et entrée du calcul (`DraftCost`).
- `internal/bom/strategy.go`, `suggest.go` — classement des alternatives (ADR 0028).
- `internal/bom/anomalies.go`, `internal/pricing/anomaly.go` — `AnomalyDetector`, `MedianDetector` (ADR 0027).
- `internal/pricing/statistics.go`, `pricingstore/history.go` — historique des cotations, fenêtre annuelle (ADR 0011).
- `internal/supplier` — fournisseurs, contrats, `EvaluateContracts`, `ContractStatus`.
- `internal/snapshot/snapshot.go`, `snapshotstore` — publication et gel, `NewDraft`.
- `internal/httpapi/router.go`, `boms.go`, `cost_method.go` — montage des routes, `writeError`.
- `internal/sampledata`, `seedstore/store.go` — seed idempotent (ADR 0012).

### Existing Patterns
- Calcul pur dans le domaine, lectures derrière interfaces (`With…` au câblage), sqlc par domaine.
- Variante de calcul = stratégie choisie à un seul endroit (`strategyFor`, `scoreOf`).
- Tables de snapshot protégées par `reject_mutation()` ; tests d'intégration sautés sans `DATABASE_URL`.

### Constraints
- AGENTS §3–§4 : décimal exact, devise portée, donnée manquante = erreur nommée, pas d'heure courante.
- Migration `00014` et ADR `0036`, `0037` seulement (statut Proposé).
- Le lot B modifie `router.go`, `boms.go` et `internal/sampledata` en parallèle : ajouts dans des fichiers
  séparés (`internal/httpapi/analysis.go`, `internal/bom/analysis_*.go`, `seedstore/profiles.go`).
- Contrats d'API du plan : on peut ajouter des champs, jamais en retirer.

## Requirements

### Must Have
- R1 `GET /api/v1/boms/{id}/analysis/fx-sensitivity?shock_percent=5` → tableau, par devise d'origine :
  `currency`, `share_percent`, `impact`, `budget_percent_after` ; impact = total rechiffré par le moteur avec le
  taux choqué − total de base (taux budget et dernier taux de cette devise × (1 + choc)).
- R2 `GET /api/v1/boms/{id}/analysis/price-drivers?limit=10` → tableau `part_id`, `name`, `amplitude` :
  (max − min des cotations de l'offre retenue sur la fenêtre annuelle ADR 0011 finissant à la date de calcul)
  × quantité réelle, converti au taux de la méthode, trié décroissant ; pièces non évaluables listées après,
  avec la raison.
- R3 `GET /api/v1/boms/{id}/analysis/sourcing` → part du coût par pays d'origine et par devise, par fournisseur
  (avec état du contrat), poids des 5 premiers, pièces mono-source (une seule offre active), contrats expirés
  encore utilisés (pièces, remise perdue), taux budget vs dernier taux par devise et effet sur la voiture.
- R4 Anomalies €/kg par famille : `pricing.PerKgDetector` à côté de `MedianDetector`, choix au seul endroit
  `anomalyDetectorFor` ; `GET /api/v1/boms/{id}/analysis/anomalies?method=per_kg|median&threshold=2`, résultats
  par pièce classés par surcoût ; masse absente ou nulle → exclue avec la raison. ADR 0037.
- R5 Profils : `GET /api/v1/vehicle-profiles`, `GET|PUT /api/v1/boms/{id}/profile`
  `{preset, weights:{cost,co2,lead_time,mass,quality}}` (entiers, somme 100) ; le classement des alternatives
  utilise les poids ; stratégies par nœud inchangées ; profil figé au snapshot, repris par un nouveau brouillon.
  Migration 00014 (Up/Down). ADR 0036.
- R6 Données de démo : Multipla en profil « familiale ».

### Should Have
- `GET /api/v1/snapshots/{id}/profile` (profil figé d'un snapshot).

### Won't Do
- Front (Chiffrer, Sourcer, Réglages). Indexation matières (« Aluminium +10 % ») : pas de données d'indices.
- Analyses sur un snapshot publié (brouillon seulement).

## Tasks

- [x] T1 Ticket, ADR 0036 et 0037 (Proposé).
  Files: `.specs/v2-lot-e/*`, `docs/adr/0036-*.md`, `docs/adr/0037-*.md`. Done when: relus, cohérents avec le plan.
- [x] T2 Migration 00014 et requêtes sqlc des profils (brouillon et snapshot).
  Files: `migrations/00014_vehicle_profiles.sql`, `internal/bom/queries.sql`, `internal/snapshot/queries.sql`, code
  généré. Approach: tables dédiées, CHECK somme = 100, triggers d'immutabilité. Done when: goose up/down/up, sqlc à jour.
- [x] T3 Profils dans le domaine et classement pondéré.
  Files: `internal/bom/profile.go`, `strategy.go`, `suggest.go`, `bomstore/profiles.go`, tests.
  Approach: `strategyWeights` seul point de choix, score pondéré ; priorité nœud > profil > aucun.
  Done when: tests existants verts, tests table-driven des poids, préréglages et validation.
- [x] T4 Gel du profil au snapshot et reprise par un nouveau brouillon.
  Files: `internal/snapshot/snapshot.go`, `snapshotstore/store.go`, test d'intégration.
- [x] T5 Sensibilité au change. Files: `internal/bom/analysis.go`, `analysis_fx.go`, tests.
  Done when: un test prouve impact = rechiffrage exact et part = exposition ÷ total.
- [x] T6 Pièces qui pèsent le plus. Files: `internal/bom/analysis_drivers.go`, tests.
- [x] T7 Sourcing. Files: `internal/bom/analysis_sourcing.go`, tests.
- [x] T8 Anomalies €/kg. Files: `internal/pricing/anomaly.go`, `anomaly_perkg.go`, `internal/bom/anomalies.go`,
  `analysis_anomalies.go`, tests.
- [x] T9 API. Files: `internal/httpapi/analysis.go`, `profiles.go`, une ligne dans `router.go`, JSON des
  suggestions, câblage `cmd/server/main.go`, tests.
- [x] T10 Données de démo : Multipla « familiale ». Files: `internal/sampledata/profiles.go`,
  `seedstore/profiles.go`, test d'intégration du seed.
- [x] T11 Vérification : vet, lint, tests, goose, sqlc ; réponses réelles sur la Multipla seedée.

## Verification

### Automated Checks
- `go vet ./...`, `go tool golangci-lint run ./...`, `go test ./...` (avec `DATABASE_URL`).
- `goose up / reset / up` sur une base neuve, `go tool sqlc generate` sans écart.

### Manual Checks
- Serveur sur la base seedée : réponses des trois analyses et des profils sur la Multipla collées dans state.md.

### Acceptance Criteria
- AC1 Σ des impacts de change cohérent avec le moteur : impact = total(taux choqué) − total, exact.
- AC2 Les stratégies par nœud classent comme avant (tests d'ADR 0028 inchangés et verts).
- AC3 Profil invalide (somme ≠ 100, poids négatif, préréglage inconnu) → 400.
- AC4 Le snapshot garde le profil de la publication même si le brouillon change ensuite.
- AC5 Pièce sans masse → exclue des anomalies €/kg avec la raison, jamais 0.
