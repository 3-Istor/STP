---
id: v2-lot-a
status: review
branch: v2-lot-a
current_task: null
started: 2026-11-17
updated: 2026-11-17
---

# State: V2 lot A — coût complet

## Progress

All tasks completed. Ready for review (relecture humaine, ADR 0031 à accepter).
- Tasks: 8/8 completed
- Branch: `v2-lot-a`
- Commits: 9 (T6 en deux : correction ineffassign en commit séparé, pas de réécriture d'historique)

## Task Log

### T1: Ticket et ADR 0031
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: ADR 0031 au statut Proposé. Constat : les budgets cibles (target-costing) n'ont aucun stockage
  (ADR 0018 l'avait laissé en attente) : table ajoutée dans 00012 pour pouvoir les seeder.

### T2: Migration 00012 et requêtes sqlc
- **Status**: completed
- **Commit**: ee506e7
- **Notes**: `goose up/down/up` puis `up/reset/up` passent ; `sqlc generate` à jour (modèles catalogdb/supplierdb
  régénérés : colonnes ajoutées à `offers`).

### T3: Calcul pur selon la méthode
- **Status**: completed
- **Commit**: 6bcfb03
- **Notes**: `strategyFor` seul point de choix. Libellés et sources construits à la demande : benchmark
  `BenchmarkTotals30k` 59 ms → 89 ms/op (lignes par pièce et totaux par nature) ; avec libellés formatés au
  calcul c'était 133 ms, d'où la construction paresseuse.

### T4: Faits pour un brouillon
- **Status**: completed
- **Commit**: 36ef7e3
- **Notes**: lecture par lot (une requête paramètres d'offre, deux listes douane/transport, contrats une fois par
  fournisseur). `Totals` passe par `DraftCost`.

### T5: Gel et relecture au snapshot
- **Status**: completed
- **Commit**: 5604596
- **Notes**: la publication rechiffre avec le snapshot de prix créé (`DraftCostWithPrices`) pour figer
  exactement ce qui est publié ; `Frozen` interdit tout recalcul de remise à la relecture.

### T6: API
- **Status**: completed
- **Commits**: cd46d69, bdd0a4a
- **Notes**: bug trouvé par le test d'intégration : la validation enveloppait `ErrNoProgramVolume` (409) et
  `ErrInvalidInput` (400) → corrigé (400). Route bonus : ventilation d'un nœud de snapshot.

### T7: Données de démo
- **Status**: completed
- **Commit**: 0550106
- **Notes**: mesuré sur une base seedée (`stp_lot_a_seed`, serveur :8081, API de prix :8091) : délai
  Multipla 58 j → date cible 2026-03-02 (60 j, marge 2). Totaux Multipla sur un même snapshot de prix
  (temps simulé 2026-01-09) : purchase/latest 25 590,27 € ; landed/latest 25 663,59 € ; full/latest
  25 701,41 € ; full/budget 25 825,85 €. Seed relancé : `created=false`, mêmes comptes.

### T8: Vérification
- **Status**: completed
- **Notes**: voir rapport ci-dessous.

## Decisions

- D1 (seul, à valider) : `purchase` = calcul historique (cotation brute, sans remise) pour que le défaut
  `purchase`/`latest` ne change aucun total existant ; la remise entre à partir de `landed`.
- D2 (seul, à valider) : quantité comparée aux paliers = Σ quantités réelles de l'offre dans la BOM × `volume`
  de la BOM (pas le volume programme).
- D3 (seul, à valider) : outillage NULL en `full` → `ErrNoTooling` (pas de total) ; « pas d'outillage » se
  déclare à 0. Le seed déclare donc 0 explicitement sur les offres sans outillage.
- D4 (seul, à valider) : date de calcul d'un brouillon = cotation la plus récente utilisée (temps simulé).
- D5 (seul, à valider) : en `budget`, ligne `price` au dernier taux + ligne `fx` = écart budget − dernier.
- D6 (seul, à valider) : budgets cibles stockés par nœud (`bom_target_budgets`), racine = budget global.
- D7 (seul, à valider) : publication refusée (`ErrIncompleteCost`, 409) si la méthode ne donne pas de coût
  complet ; la remise retenue est figée par offre (contrat, palier, motif), pas les contrats eux-mêmes.
- D8 (seul, à valider) : taux budget en paire directe seulement (pas d'inverse ni de pivot) ; seed : taux
  vers EUR et vers GBP. Douane/transport seedés vers IT (Multipla) et GB (Valhalla), valeurs fictives.
- D9 (seul, à valider) : outillage réparti entre les nœuds d'une même offre au prorata de la quantité réelle,
  division à 28 décimales (seule division non finie).
- D10 (seul) : `SIM_DATASET_VERSION` par défaut `demo-v4` (le contenu généré change ; une base `demo-v3`
  existante reçoit un second jeu, cf. README).

## Blockers

Aucun.

## Verification Report(s)

### 2026-11-17 — T8 (exécuté localement, base Postgres `stp_lot_a`)
| Check | Résultat |
|---|---|
| `go vet ./...` | PASS |
| `go tool golangci-lint run ./...` | PASS — `0 issues.` |
| `go test -race -count=1 ./...` (avec `DATABASE_URL`) | PASS — 18 paquets `ok`, aucun échec |
| `go mod tidy -diff` | PASS |
| `go tool sqlc generate` puis `git status` | PASS — aucun écart |
| `goose up` / `reset` / `up` (base neuve `stp_lot_a_reset`) | PASS — version 12 |
| `cd web && npm ci && npm run lint && npx tsc -b && npx vite build` | PASS — 0 vulnérabilité, oxlint OK, build OK |
| CI GitHub | NON EXÉCUTÉE (rien n'est poussé) |

Contrôle manuel (serveur :8081, API de prix :8091, base `stp_lot_a_seed`, arrêtés ensuite) : ventilation
« Porte avant nue » (nœud 8438, offre Arden) en full/budget ; au temps simulé 2026-02-27 remise 9,7 % (palier
≥ 500), au 2026-04-04 « Contrat expiré : remise non appliquée » (0,00 €), total 134,33 €.

Acceptance : AC1 `TestCostMethodTotals`, `TestDefaultMethodIsHistoricalCalculation`, `TestServiceTotalsFollowCostMethod` ;
AC2 `TestCostMethodMissingData`, cas « contrat expiré » ; AC3 `TestServiceCostBreakdown`, `TestCostMethodRoutes` ;
AC4 `TestSnapshotReplaysFullCostAfterTablesChange` ; AC5 `TestRunSeedTwiceWithSameParametersAddsNothing`
(+ `checkMultiplaPilot`), `TestDatasetInvariants`.
