---
id: v2-lot-g
title: V2 lot G — corrections de la relecture indépendante du backend V2 (publication, immuabilité, budgets figés, perfs)
source: manual
source_ref: relecture indépendante du backend V2 (lots A, B, D, E, C) relayée par l'agent orchestrateur
created: 2026-11-17
status: in-progress
priority: P1
depends_on: [v2-lot-a, v2-lot-b, v2-lot-e]
branch: v2-lot-g
---

# V2 lot G — corrections de la relecture du backend V2

## Context

Une relecture indépendante du backend V2 (branche `v2/implementation` : lots A, B, D, E, C) a relevé un
problème bloquant (publication non atomique), des trous dans l'immuabilité des snapshots (AGENTS §3), des
réglages non figés (budgets), un contrôle qui passe au vert à tort, des doublons et doubles arrondis, une
validation manquante des pistes de réduction, du code mort et des temps de réponse > 1 s sur la Multipla
seedée (30 000 nœuds). Chaque point est corrigé ici avec, d'abord, un test qui le met en défaut (AGENTS §10).
Le lot F (front) travaille en parallèle : `web/` n'est pas touché, `internal/bom/budgets.go` non plus (il y
ajoute `BudgetStatus.Percent` et `budgetPercent`), le champ `percent` de `/tree` garde son nom et son format.

## Research Findings

### Relevant Files
- `internal/snapshot/snapshot.go` — `Publish`, `freezeCost`, `frozenCostInput`, `NewDraft`, `copyDraft`.
- `internal/snapshot/versions.go`, `compare.go` — versions d'une famille, `familyOf`, % du budget.
- `internal/bom/costfacts.go` (`draftCost`, `DraftCost`), `checks.go` (`checksInput`, `checkBudgets`),
  `ideas.go` (`rootID`, `RootBudget`, `validateIdea`), `analysis_fx.go` (`rootOf`, `rootBudget`, `costAsOf`,
  `budgetPercent`, `FXSensitivity`), `analysis_sourcing.go` (`rateGaps`, `sourcingFacts`),
  `costcalc.go` (`discountLabel`), `costmethod.go` (`resolveDiscount`).
- `internal/bom/store.go`, `bomstore/store.go`, `memstore_test.go` — accès aux brouillons.
- `migrations/00005` (garde transactionnelle des snapshots de prix, ADR 0016), `00012`–`00014`.
- `internal/httpapi/versions.go`, `analysis.go` — sortie des pourcentages.

### Existing Patterns
- Calcul pur dans le domaine (`ComputeTotals`, `RunChecks`, `ComputeBridge`), lectures derrière interfaces.
- Snapshots de prix fermés au commit de leur transaction (`creation_xid` + trigger, migration 00005).
- Tests d'intégration en transaction annulée (`testDB`) ou en schéma privé (`seedTestDB`).

### Constraints
- AGENTS §3 (snapshot = structure, quantités, réglages, prix et taux copiés ; immuable côté service et base),
  §4 (tests table-driven, erreurs sentinelles), §7 (modèle des snapshots → ADR), §11 (aucune migration ni ADR
  acceptée modifiée, aucun test affaibli).
- Nouvelle migration `00015` seulement ; ADR `0038` au statut Proposé.

## Requirements

### Must Have
- R1 Publication : contrôles, nœuds figés et faits de coût viennent d'une seule lecture du brouillon.
- R2 Base : aucun INSERT dans une table figée d'un snapshot hors de la transaction qui l'a créé ; rejeu du coût
  figé qui ne lit que les taux budget de l'année de calcul.
- R3 Budgets cibles figés à la publication ; `versions` et `NewDraft` lisent les budgets figés.
- R4 Contrôle `budgets` en `warn` (avec la raison) si un budget n'est pas évaluable (autre devise, coût manquant).
- R5 Une fonction de domaine pour la racine, le budget global, la date de calcul et le % du budget ; un seul
  arrondi (half-even) en sortie.
- R6 Pistes : brouillon lié de la même famille et différent de la BOM ; gain borné ; 400/422 cohérents.
- R7 `contract_not_started` réellement produit (contrat pas encore commencé à la date de calcul).
- R8 compare, fx-sensitivity, sourcing, versions < 1 s sur la Multipla seedée ; résultats inchangés.

### Should Have
- Mesures avant/après dans `state.md` ; ADR 0038 qui raconte les choix (lecture cohérente, garde, budgets figés).

### Won't Do
- Optimisation interne de `ComputeTotals` (allocations) : hors périmètre, on réduit le nombre d'appels.
- Front (`web/`), `internal/bom/budgets.go` (lot F).

## Tasks

- [ ] **T1: Publication atomique**
  - Files: `internal/bom/draft.go` (nouveau), `store.go`, `bomstore/store.go`, `memstore_test.go`, `costfacts.go`,
    `checks.go`, `internal/snapshot/snapshot.go`, test `internal/snapshot/atomic_publish_integration_test.go`
  - Approach: `Store.Draft` lit BOM, arbre, stock, planning et budgets en une transaction en lecture seule
    REPEATABLE READ ; `DraftCost` en dérive ; `Publish` calcule contrôles, nœuds figés et faits de coût depuis ce
    seul `DraftCost` (rechiffré avec le snapshot de prix créé, sans relire le brouillon).
  - Done when: un test qui modifie le brouillon après la première lecture prouve que le snapshot publié est
    celui de cette lecture (échoue sur l'ancien code).
- [ ] **T2: Immuabilité complète (migration 00015) et année du taux budget au rejeu**
  - Files: `migrations/00015_snapshot_closure_and_budgets.sql`, `internal/snapshot/snapshot.go`, tests
  - Approach: `bom_snapshots.creation_xid` posé à l'insertion ; trigger d'instruction sur chaque table figée qui
    refuse une ligne d'un snapshot créé par une autre transaction (modèle 00005) ; `frozenBudgetRates` ne garde
    que l'année de `cost_as_of` et la devise de la BOM.
  - Done when: INSERT tardif refusé dans chaque table figée (schéma privé, transactions commitées) ; rejeu
    inchangé avec un taux d'une autre année.
- [ ] **T3: Budgets figés au snapshot**
  - Files: `00015`, `internal/snapshot/queries.sql`, `snapshotstore`, `snapshot.go`, `versions.go`
  - Approach: table `bom_snapshot_target_budgets` (clé de nœud figé), colonne `budgets_frozen` ; `versions` et
    `NewDraft` lisent les budgets figés (snapshot antérieur : budgets actuels de la BOM, comme avant).
  - Done when: modifier le budget de la BOM après publication ne change ni le % de V1 ni les budgets d'un
    brouillon créé depuis V1.
- [ ] **T4: Contrôle budgets non évaluable → warn**
  - Files: `internal/bom/checks.go`, `checks_test.go`
  - Done when: un budget `unavailable` donne `warn` avec la raison.
- [ ] **T5: Doublons et arrondi unique**
  - Files: `internal/bom/rootbudget.go` (nouveau), `analysis_fx.go`, `ideas.go`, `checks.go`, `costfacts.go`,
    `analysis_sourcing.go`, `snapshot/versions.go`, `compare.go`, `httpapi/versions.go`, `analysis.go`
  - Approach: `RootID`, `RootBudgetOf`, `CostAsOf`, `PercentOf` (exact) dans le domaine ; sortie
    `RoundBank` une seule fois.
  - Done when: 99,94996 % sort « 99.9 » ; tests des fonctions et de la sortie.
- [ ] **T6: Pistes de réduction : famille et bornes**
  - Files: `internal/bom/ideas.go`, `snapshot/versions.go` (famille), `cmd/server/main.go` (câblage), tests
  - Done when: brouillon d'une autre famille, BOM elle-même, gain ≥ 1e9 → `ErrInvalidInput` (400).
- [ ] **T7: `contract_not_started`**
  - Files: `internal/bom/costmethod.go`, `costmethod_test.go`
  - Done when: un contrat qui commence après la date de calcul donne le motif `contract_not_started`, sans remise.
- [ ] **T8: Performances**
  - Files: `analysis_fx.go`, `analysis_sourcing.go`, `snapshot/versions.go`, `compare.go`, `costfacts.go`
  - Approach: exposition par devise en une passe (coût linéaire en chaque taux), effet taux budget/dernier taux
    idem, un seul `DraftCost` par requête, famille lue une fois, contrats lus en une requête.
  - Done when: tests d'égalité avec le recalcul complet ; mesures avant/après < 1 s.
- [ ] **T9: Vérification et livrable §9**

## Verification

### Automated Checks
- `go vet ./...`, `go tool golangci-lint run ./...`, `go test -race ./...` (`DATABASE_URL`), goose up/reset/up
  (version 15), `go tool sqlc generate` sans écart.

### Manual Checks
- Mesures réelles avant/après sur la base seedée (`server seed`, demo-v4, seed 42), serveur :8088, API de prix :8098.

### Acceptance Criteria
- Un test par point qui échoue sur l'ancien code et passe sur le nouveau ; aucun test existant affaibli.
