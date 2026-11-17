---
id: v2-lot-g
status: in-progress
branch: v2-lot-g
current_task: T1
started: 2026-11-17
updated: 2026-11-17
---

# State: V2 lot G — corrections de la relecture du backend V2

## Progress

- Tasks: 0/9 completed
- Branch: `v2-lot-g` (contient les lots A, B, D, E, C) ; rien n'est poussé

## Task Log

### T0: Référence avant modification
- **Status**: completed
- **Notes**: base `stp_lot_g` migrée (version 14), `go test -count=1 ./...` vert avant toute modification.
  Base seedée `stp_lot_g_seed` (demo-v4, seed 42, API de prix :8098), mesures « avant » ci-dessous.

### T1: Publication atomique
- **Status**: completed
- **Notes**: test d'abord : `TestPublishReadsTheDraftOnce` (store qui modifie le brouillon après la première
  lecture) échouait sur l'ancien code (Roue ×10 et Écrou ajouté figés alors que les contrôles avaient vu ×4).
  Correction : `bom.Store.Draft` (BOM, arbre, stock, planning, budgets dans une transaction REPEATABLE READ en
  lecture seule ; savepoint si le store est câblé sur une transaction), `DraftCost` en dérive,
  `PublicationChecksOf(dc)`, `CostOfDraft(draft, prix figés)`. `Publish` : une lecture, contrôles, nœuds,
  stock, planning, réglages et faits de coût du même état. En prix en direct, le même état est rechiffré avec
  le snapshot de prix créé (deux calculs, aucune relecture du brouillon) ; prix figés : un seul calcul.

### T2: Immuabilité complète (00015) et année du taux budget au rejeu
- **Status**: completed
- **Notes**: tests d'abord : `TestPublishedSnapshotRejectsLateInserts` (schéma privé, snapshot commité, INSERT
  tardif dans les 7 tables figées) acceptait les 7 INSERT ; `TestFrozenCostInputKeepsTheBudgetRatesOfTheCostYear`
  prenait le taux 2027 (et un taux vers GBP). Migration 00015 : `creation_xid` + trigger d'instruction
  `guard_bom_snapshot_rows` (table de transition) sur les 7 tables, table `bom_snapshot_target_budgets` et colonne
  `budgets_frozen` (utilisées par T3). Première version avec une FK vers `bom_snapshot_nodes` : le TRUNCATE des
  nœuds échouait en 0A000 au lieu de 23001 (test existant) → FK vers `bom_snapshots` seulement. ADR 0038 (Proposé).

### T3: Budgets figés au snapshot
- **Status**: completed
- **Notes**: test d'abord : `TestSnapshotBudgetsAreFrozen` (budget de la BOM passé de 450 à 1 000 € après V1) :
  V1 affichait 38,8 % au lieu de 86,22 % et le brouillon issu de V1 recevait 1 000 € + un budget ajouté
  après coup. `Publish` copie `dc.Budgets` (même lecture que le reste) dans `bom_snapshot_target_budgets`,
  `budgets_frozen = true` ; `Versions` et `NewDraft` lisent `frozenBudgets` (snapshot publié avant 00015 :
  budgets actuels de la BOM, comportement précédent, dit dans le code). La date cible était déjà figée
  (`start_on`/`target_on`, ADR 0023). Cas UPDATE/DELETE/TRUNCATE des budgets figés ajoutés à
  `TestBomSnapshotRejectsMutations`. `go test -count=1 ./...` vert.

### T4: Contrôle budgets non évaluable → warn
- **Status**: completed
- **Notes**: tests d'abord (2 cas de `TestPublicationChecks` + `TestCheckBudgetsNamesTheUnavailableReason`) : le
  contrôle était `ok` (« 1 budget(s) »). Un budget `unavailable` (autre devise, coût manquant), racine comprise,
  passe le contrôle en `warn` avec le nœud et la raison dans le détail (justification exigée à la publication).

### T5: Doublons et arrondi unique
- **Status**: completed
- **Notes**: test d'abord : `TestVersionBudgetPercentIsExact` (99,94996 % arrondi à 99,95 dans le domaine, puis
  « 100.0 » en sortie). `internal/bom/rootbudget.go` : `PercentOf` (exact, 28 décimales), `BudgetPercent`,
  `RootBudgetOf`, `GlobalBudget`, `CalcRootID`, `QuotesAsOf`, `CostAsOf` ; `RootID` dans `draft.go`. Supprimés :
  `rootOf` (analysis_fx, versions), `rootID` (ideas, snapshot), `rootBudget`/`Service.RootBudget` (relisaient l'arbre),
  `budgetPercent` (analysis_fx), `costAsOf` et les deux autres déductions de la date de calcul (costfacts, checks).
  Sortie : `httpapi.percentOut` (half-even, une fois) pour `budget_percent` des versions et les `*_percent` des
  analyses (`pct`, était half-away) ; textes figés (contrôle budgets, explication de la comparaison) : `PercentOf`
  puis `RoundBank` une fois. Tests : `TestPercentOf`, `TestBudgetPercentAndGlobalBudget`, `TestCostAsOf`,
  `TestPercentOut`. Le champ `percent` de `/tree` (lot F, `budgets.go`) n'est pas touché ; après fusion, son
  `budgetPercent` (DivRound 17 puis RoundBank 1) peut appeler `bom.PercentOf` (même résultat sauf à la 17e décimale).

## Decisions

## Blockers

Aucun.

## Verification Report(s)
