---
id: v2-lot-b
title: V2 lot B — versions et publication (comparaison et pont de coût, contrôles, publication tracée, pistes, versions)
source: manual
source_ref: docs/v2/plan-implementation.md (lot B)
created: 2026-11-17
status: review
priority: P1
depends_on: [v2-lot-a]
branch: v2-lot-b
---

# V2 lot B — versions et publication (backend Go)

## Context

Vision V2 §2 : le pont de coût entre deux versions est la priorité 1 de l'audit métier, les contrôles avant
publication sont la revue de jalon, le suivi des pistes de réduction ferme l'écart à la cible. §8 retient l'option
A (mono-utilisateur + traçabilité : noms saisis, note et justifications figées dans le snapshot). Le lot A a
livré le coût complet (méthode `basis × fx`, faits figés au snapshot) : le pont doit rechiffrer avec ce même
moteur. Maquettes : I4-6 Arbitrer, I4-7 Publier, I4-0 Toutes les BOM. Pas de front dans ce lot (lot F).

## Research Findings

### Relevant Files
- `internal/bom/totals.go`, `costcalc.go`, `costmethod.go` — `ComputeTotals` pur, coût selon la méthode,
  `CostInput.Frozen` + `OfferCost.Discount` : remise figée réutilisable pour rechiffrer des états mixtes.
- `internal/bom/costfacts.go` — `DraftCost` (entrée + totaux d'un brouillon), `CostBreakdown`.
- `internal/bom/budgets.go` — `EvaluateTargetBudgets` (états ok/warn/over/unavailable).
- `internal/bom/planning.go` — `PlanSchedule` (`TargetMet`).
- `internal/snapshot/snapshot.go` — `Publish`, `Get` (relecture avec faits figés), `NewDraft` (lien
  `source_snapshot_id`, copie par clé croissante : l'ordre des frères est conservé).
- `internal/httpapi/boms.go` — route `POST /boms/{id}/publish`, `statusOf`.
- `internal/sampledata`, `seedstore`, `cmd/server/seed.go` — seed SQL idempotent par `dataset_version`.

### Existing Patterns
- Domaine pur derrière interfaces ; sqlc par domaine ; tests d'intégration Postgres sautés sans `DATABASE_URL`.
- Snapshots immuables : `reject_mutation()` sur UPDATE/DELETE/TRUNCATE ; `ADD COLUMN` ne déclenche pas les triggers.

### Constraints
- Migration `00013` seulement ; ADR `0032` et `0033` au statut Proposé.
- Aucune dépendance ajoutée. Pas de `web/`.
- Le contenu généré du jeu (`Dataset`, son empreinte) ne doit pas changer : une base `demo-v4` déjà chargée
  doit pouvoir recevoir les versions de démo sans conflit.

## Requirements

### Must Have
- R1 `GET /api/v1/compare?from=&to=` (`snapshot:<id>` | `bom:<id>`) : totaux des deux côtés (coût selon la
  méthode, CO₂, masse, délai), écart, pièces modifiées (chemin, avant/après, delta), causes (`structure`,
  `offer_change`, `price`, `fx`, `discount`, `logistics`, `tooling`, `method`) dont la somme = écart exactement,
  chacune avec une phrase d'explication et ses nœuds. ADR 0032.
- R2 `GET /api/v1/boms/{id}/publication-checks` : `{code, level, title, detail, node_ids}` ; règles dans le
  domaine, une fonction par règle, testées.
- R3 publication tracée : `{note, proposed_by, validated_by, justifications}` ; 409 si `block`, 422 si `warn`
  sans justification ; figé dans le snapshot (migration 00013, immuable). ADR 0033.
- R4 pistes de réduction : `GET|POST /boms/{id}/ideas`, `PATCH|DELETE /boms/{id}/ideas/{ideaId}` ; `GET` rend
  `{ideas, open_total, gap_to_target, remaining}`.
- R5 `GET /api/v1/boms/{id}/versions` : brouillons et snapshots de la famille (lien `source_snapshot_id`),
  dates à la seconde, coût selon la méthode, % du budget, snapshot de prix, proposé/validé par, note.
- R6 seed Multipla : snapshot V1 (« Jalon de lancement », Arthur P., Joe), brouillon B depuis V1 (portes avant nues
  chez un autre fournisseur, haut-parleurs ajoutés aux 4 portes), 5 pistes (maquette I4-6) ; idempotent.

### Should Have
- S1 `GET /api/v1/boms` enrichi sans casser le contrat (champs ajoutés seulement) — **non fait** : `source_snapshot_id`
  suffit à grouper, `GET /boms/{id}/versions` donne libellés, coûts et trace (voir state.md D9).
- S2 `GET /api/v1/snapshots/{id}` expose note, proposé/validé par et contrôles figés.

### Won't Do
- Front (lot F), export PDF/Excel, comptes utilisateurs (ADR 0033), pont CO₂ décomposé.
- Proposé par sur un brouillon (pas de stockage ; seul le snapshot le porte).

## Tasks

- [x] T1: Ticket et ADR 0032, 0033
  - **Files**: `.specs/v2-lot-b/*`, `docs/adr/0032-*.md`, `docs/adr/0033-*.md`
  - **Approach**: ordre du pont, appariement, règles des contrôles, traçabilité.
  - **Done when**: ADR Proposé, tâches listées.
- [x] T2: Migration 00013 et requêtes sqlc
  - **Files**: `migrations/00013_versions_and_publication.sql`, `internal/{bom,snapshot}/queries.sql`, code généré
  - **Approach**: colonnes note/proposed_by/validated_by de `bom_snapshots`, `bom_snapshot_checks` immuable,
    `bom_reduction_ideas`, `seed_demo_steps`.
  - **Done when**: goose up/reset/up, sqlc à jour, tests existants verts.
- [x] T3: Pont de coût pur
  - **Files**: `internal/bom/bridge.go`, `bridge_test.go`
  - **Approach**: appariement par chemin, 9 états, `ComputeTotals` à chaque marche, contrôle des extrémités.
  - **Done when**: somme = écart au centime (cause isolée, combinées, devises différentes), ordre testé.
- [x] T4: Contrôles avant publication
  - **Files**: `internal/bom/checks.go`, `checks_test.go`
  - **Approach**: une fonction pure par règle sur une entrée construite par le service.
  - **Done when**: chaque règle ok/warn/block testée.
- [x] T5: Publication tracée
  - **Files**: `internal/snapshot/snapshot.go`, `snapshotstore`, tests d'intégration
  - **Approach**: contrôles recalculés à la publication, refus nommés, copie dans le snapshot.
  - **Done when**: 409/422 par erreur nommée, trace relue, immuabilité testée.
- [x] T6: Comparaison, versions, pistes (services)
  - **Files**: `internal/snapshot/compare.go`, `versions.go`, `internal/bom/ideas.go`, stores
  - **Approach**: références `snapshot:`/`bom:`, explications avec noms, famille de versions, pistes CRUD.
  - **Done when**: tests de service (intégration) verts.
- [x] T7: API
  - **Files**: `internal/httpapi/versions.go`, `boms.go`, tests
  - **Done when**: routes testées (codes et formes JSON).
- [x] T8: Données de démo
  - **Files**: `internal/sampledata/versions.go`, `seedstore`, `cmd/server/seed.go`
  - **Done when**: seed deux fois idempotent ; comparaison V1 → B non vide sur la base seedée.
- [x] T9: Vérification et livrable §9
  - **Files**: `.specs/v2-lot-b/state.md`
  - **Done when**: vet, lint, tests, goose, sqlc ; réponses réelles `compare` et `publication-checks` collées.

## Verification

### Automated Checks
- `go vet ./...`, `go tool golangci-lint run ./...`, `go test ./...` avec `DATABASE_URL`
- `goose up / reset / up`, `go tool sqlc generate` sans écart

### Manual Checks
- Seed, serveur :8084 + API de prix :8094 ; `GET /compare?from=snapshot:V1&to=bom:B`, `GET /boms/B/publication-checks`.

### Acceptance Criteria
- [x] AC1 (R1) : Σ causes = delta.cost exactement ; causes non vides sur V1 → B.
- [x] AC2 (R2, R3) : un `block` refuse (409), un `warn` non justifié refuse (422), la trace est dans le snapshot.
- [x] AC3 (R4) : reste à trouver = écart à la cible − pistes ouvertes.
- [x] AC4 (R5) : V1 et B listés ensemble, dates à la seconde.
- [x] AC5 (R6) : seed idempotent.
