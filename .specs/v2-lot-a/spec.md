---
id: v2-lot-a
title: V2 lot A — coût complet (remises, coût rendu, outillage amorti, taux budget) figé au snapshot
source: manual
source_ref: docs/v2/plan-implementation.md (lot A)
created: 2026-11-17
status: review
priority: P1
depends_on: []
branch: v2-lot-a
---

# V2 lot A — coût complet

## Context

Aujourd'hui le total d'une BOM = dernière cotation × quantité réelle × dernier taux (ADR 0007). Les remises de
contrat (`internal/supplier`, ADR 0010) et le coût rendu (douane, transport, emballage, ADR 0025) sont calculés à
part et absents des totaux ; les tables douane/transport et les contrats ne sont pas figés au snapshot, qui ne
peut donc pas rejouer un coût rendu. L'expert métier (vision V2 §2) demande l'outillage amorti (outillage ÷
volume programme) et le taux budget de l'année (trésorerie), le dernier taux servant de comparaison. Vision §5 :
« coût complet et figé avec cascade explicative » est dans le socle.

## Research Findings

### Relevant Files
- `internal/bom/totals.go` — `ComputeTotals` (pur, O(n)), `partCost` = cotation × quantité réelle × taux.
- `internal/bom/landed.go`, `internal/pricing/landed.go` — coût rendu pièce par pièce (ADR 0025), erreurs
  nommées `ErrNoCustomsDuty`, `ErrNoFreightRate`, `ErrNoPackaging`, `ErrNoDestination`.
- `internal/supplier/evaluation.go` — `EvaluateContracts` (pur) : palier atteint sur toute la quantité,
  contrats actifs à `as_of` (bornes inclusives), `ErrSelectionRequired`.
- `internal/bom/service.go` — `Totals`, `totalsInput`, `LoadPrices` (lecture par lot).
- `internal/snapshot/snapshot.go` — `Publish` (gel), `Get` (relecture avec le snapshot de prix).
- `internal/httpapi/boms.go`, `boms_json.go` — `GET /boms/{id}/tree`, `explanation`.
- `internal/sampledata/*`, `seedstore/store.go` — seed idempotent par `dataset_version` (ADR 0012, 0021).
- `internal/pricing/budget.go` — `EvaluateBudget` (pur) ; **aucun stockage des budgets cibles** n'existe
  (ADR 0018 : « nouvelles tables seulement après accord »).

### Existing Patterns
- Domaine pur + stores derrière interfaces (bomstore, pricingstore, snapshotstore, sqlc par domaine).
- Erreurs sentinelles, tests table-driven, tests d'intégration Postgres sautés sans `DATABASE_URL`.
- Snapshots : copies dans des tables `bom_snapshot_*` protégées par `reject_mutation()`.

### Constraints
- AGENTS §3 : jamais de float, donnée manquante = erreur nommée, aucun calcul ne dépend de l'heure courante.
- AGENTS §4 : la variante de calcul est une stratégie choisie à un seul endroit.
- Migrations : `00012` seulement ; ADR : `0031` au statut Proposé.
- Défaut `purchase` + `latest` : les totaux existants ne bougent pas.
- Temps simulé : `SIM_AS_OF=2026-01-01` ; un contrat expiré au 2026-03-31 ne l'est qu'une fois l'horloge
  simulée au-delà de cette date.

## Requirements

### Must Have
- R1 : méthode `basis` (purchase | landed | full) × `fx` (budget | latest) par BOM, `program_volume`,
  `GET/PUT /api/v1/boms/{id}/cost-method` (PUT : brouillon seulement, full exige `program_volume`).
- R2 : totaux (`/tree`, totaux de nœuds) selon la méthode, calculés en Go en remontant l'arbre ; l'explication
  porte `cost_method`.
- R3 : remise de contrat au palier atteint (quantité × volume de la BOM) ; contrat expiré à la date de calcul
  non appliqué ; date de calcul = cotation la plus récente (brouillon) ou date figée (snapshot).
- R4 : taux budget de l'année (table), absent → `ErrNoBudgetRate` ; outillage NULL en full → `ErrNoTooling`.
- R5 : `GET /api/v1/boms/{id}/nodes/{nodeId}/cost-breakdown` : lignes par `kind` avec source et date ; agrégé
  pour un sous-ensemble.
- R6 : le snapshot fige méthode, taux budget, faits douane/transport/emballage/outillage/remise, volume
  programme et date ; il rejoue le même coût au centime après modification des tables.
- R7 : seed Multipla : full + budget, volume programme 60 000, taux budget 2026, outillage, budgets cibles,
  date cible ≈ disponibilité + 2 j, contrat Arden expiré au 2026-03-31, tables douane/transport/emballage.

### Should Have
- S1 : `GET /api/v1/boms/{id}/budgets` : budgets cibles par nœud comparés au coût de la méthode.

### Won't Do
- Saisie des taux budget, de l'outillage et des budgets cibles par l'API (seed et base seulement).
- Sélection explicite d'un contrat parmi plusieurs actifs (erreur nommée à la place).
- Front (le lot C/F affichera la cascade) ; budgets figés au snapshot (lot B, versions et % budget).

## Tasks

- [x] T1: Ticket et ADR 0031
  - **Files**: `.specs/v2-lot-a/spec.md`, `.specs/v2-lot-a/state.md`, `docs/adr/0031-methode-de-calcul-du-cout.md`
  - **Approach**: options, décision, ordre, arrondi, gel au snapshot.
  - **Done when**: ADR au statut Proposé, tâches listées.
- [x] T2: Migration 00012 et requêtes sqlc
  - **Files**: `migrations/00012_cost_method.sql`, `internal/*/queries.sql`, code `*db` régénéré
  - **Approach**: colonnes boms/offers/bom_snapshots, tables budget_rates, bom_target_budgets,
    bom_snapshot_budget_rates, bom_snapshot_offer_costs (immuables).
  - **Done when**: `goose up/down/up` passe, `sqlc generate` à jour, tests existants verts.
- [x] T3: Calcul pur selon la méthode
  - **Files**: `internal/bom/costmethod.go`, `internal/bom/totals.go`, tests table-driven
  - **Approach**: `strategyFor` (un seul endroit), composantes par pièce, agrégation par `kind`.
  - **Done when**: tests par méthode et par ligne (contrat expiré, taux budget absent, outillage ÷ volume,
    arrondi) verts ; tests existants inchangés et verts.
- [x] T4: Lecture des faits pour un brouillon et stockage de la méthode
  - **Files**: `internal/bom/service.go`, `internal/bom/costfacts.go`, `bomstore`, `pricingstore`, `cmd/server`
  - **Approach**: méthode et budgets dans bomstore ; outillage, emballage, taux budget dans pricingstore ;
    contrats via `supplier.Service.ListContracts`.
  - **Done when**: `Service.Totals` suit la méthode ; tests de service verts.
- [x] T5: Gel et relecture au snapshot
  - **Files**: `internal/snapshot/*`, `snapshotstore`
  - **Approach**: publier fige les faits utilisés ; relire reconstruit le calcul sans lire les tables vivantes.
  - **Done when**: test d'intégration de reproductibilité (tables douane/transport et taux budget modifiés après
    publication → même coût au centime).
- [x] T6: API
  - **Files**: `internal/httpapi/boms*.go`, `cost_method.go`, tests d'intégration
  - **Approach**: handlers fins, codes d'erreur nommés, `cost_method` dans l'explication.
  - **Done when**: tests HTTP des nouvelles routes verts.
- [x] T7: Données de démo Multipla
  - **Files**: `internal/sampledata/*`, `seedstore/store.go`, `Makefile`, `compose.yaml`, `README.md`
  - **Approach**: nouvelles données générées, chargées dans la même transaction, version `demo-v4`.
  - **Done when**: seed deux fois idempotent ; Multipla full/budget a un total.
- [x] T8: Vérification et livrable §9
  - **Files**: `.specs/v2-lot-a/state.md`
  - **Done when**: vet, lint, tests, front (`npm ci`, `tsc -b`) verts et reportés.

## Verification

### Automated Checks
- `go vet ./...`, `go tool golangci-lint run ./...`, `go test ./...` avec `DATABASE_URL`
- `cd web && npm ci && npx tsc -b`

### Manual Checks
- Seed, puis `GET /api/v1/boms/{multipla}/nodes/{porte avant nue}/cost-breakdown` et totaux en
  purchase/latest vs full/budget.

### Acceptance Criteria
- [x] AC1 (R1, R2) : la méthode change les totaux ; défaut = totaux historiques.
- [x] AC2 (R3, R4) : contrat expiré non appliqué ; taux budget absent et outillage NULL → erreurs nommées.
- [x] AC3 (R5) : breakdown d'une pièce et d'un sous-ensemble ; Σ lignes = total.
- [x] AC4 (R6) : snapshot reproductible au centime après modification des tables.
- [x] AC5 (R7) : seed idempotent, Multipla chiffrée en full/budget.
