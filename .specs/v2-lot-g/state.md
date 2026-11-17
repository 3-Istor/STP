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

### T6: Pistes de réduction : famille et bornes
- **Status**: completed
- **Notes**: tests d'abord (`TestReductionIdeas`) : brouillon lié = la BOM elle-même, brouillon d'une autre famille
  et gain de 1e9 étaient acceptés. `validateIdea` : brouillon ≠ BOM, existant, de la même famille
  (`bom.DraftFamily`, satisfait par `snapshot.Service.InSameFamily`, câblé dans `domainServices`), 0 ≤ gain < 1e9 ;
  toutes ces erreurs sont `ErrInvalidInput` → 400 ; famille non câblée → `ErrIdeasUnavailable` (503), jamais
  accepté en silence (`TestIdeaDraftLinkNeedsTheFamilyCheck`). Cas 400 ajoutés au test HTTP. Le test existant liait
  la piste de B à B : elle est maintenant liée à A (même famille), seule correction de fixture.
  **Seed** : les 5 pistes de démo étaient sur B et la piste chiffrée liée à B (refusé désormais, et son gain
  compterait deux fois dans le reste à trouver de B) : elles sont créées sur la Multipla (écart à son budget,
  comme la maquette I4-6 : écart 826, pistes 640), la piste « Portes avant nues » liée au brouillon B. Vérifié
  dans `checkDemoVersions` (5 sur la Multipla, 1 liée à B, 0 auto-liée). Impact front (lot F) : la branche
  « dans ce brouillon » de `Ideas.tsx` ne se produit plus ; les pistes de la démo s'affichent sur la Multipla.

### T7: `contract_not_started` produit réellement
- **Status**: completed
- **Notes**: test d'abord (`TestCostMethodTotals`, date de calcul 2025-12-31, contrat du 2026-01-01) : motif
  `no_active_contract`. `resolveDiscount` nomme le contrat à venir le plus proche (`contract_not_started`,
  remise 0, `ContractID` du contrat) quand aucun contrat n'est actif ni expiré ; un contrat expiré reste prioritaire
  (cas « expiré + renouvellement à venir » testé : `contract_expired`, cohérent avec le contrôle bloquant
  `expired_contracts`). Le libellé « Contrat pas encore actif : remise non appliquée » n'est plus du code mort.

### T8: Performances
- **Status**: completed (compare au seuil : médiane 1,0 s, premier appel à froid 1,35 s)
- **Notes**: tests d'égalité d'abord, l'implémentation d'avant gardée comme référence dans le test :
  `TestFXSensitivityEqualsFullRecompute` et `TestRateGapsEqualFullRecompute` (6 méthodes × 1 ou 2 devises
  étrangères, faits en GBP, taux budget sans dernier taux, 2 chocs) ; `TestOfferContractsReadAllAtOnce`,
  `TestPartLinesSharedBetweenIdenticalNodes`, cas `ListAllContracts` dans le test du store fournisseurs.
  Changements : (1) `ComputeFXSensitivity` en une passe (`exposureByCurrency` : chaque ligne propre rattachée à la
  devise qu'elle convertit ; impact = exposition × choc) au lieu de 2 recalculs par devise ; (2) `rateGaps` : un
  calcul à taux 1 donne K(c), effet = K(c) × (dernier − budget) au lieu de 1 + n recalculs ; (3) pont de coût :
  marches indépendantes chiffrées en parallèle (`bom.parallel`, calcul pur seulement) ; (4) `Compare` ne relit pas
  la famille si « vers » y est déjà ; (5) `ComputeTotals` : capacité des lignes fixée (la recopie de lignes lourdes
  prenait la moitié du pont), taux/cotations/remises partagés par pointeur, totaux par nature accumulés en décimaux,
  lignes d'une pièce calculées une fois par (offre, masse, quantité réelle) ; (6) contrats lus en une requête
  (`supplier.ListAllContracts`, interface facultative `bom.AllContracts`) pour la méthode et le sourcing ;
  (7) `Versions` : budgets de la même lecture (plus de relecture de l'arbre par `RootBudget`, T5).
  **Résultats inchangés** : sur deux copies identiques de la base seedée (version 14 pour l'ancien code, 15 pour le
  nouveau, sans worker de prix), dump de fx-sensitivity, sourcing, compare bom:1→bom:3, de toutes les lignes de
  coût des 29 977 nœuds de la Multipla et des versions : 158 803 lignes identiques sauf 3 (effets taux budget/dernier
  taux de valeur égale, zéros de fin en plus : `-42.2301300553777602600982721912001200000000000000` contre
  `…12000`). `GET /snapshots/1` (30 000 nœuds, coût complet) octet pour octet identique avant/après.

  Mesures HTTP réelles (Multipla seedée, demo-v4 seed 42, serveur :8088, 3 appels, secondes, machine 2 cœurs) :

  | Route | Avant | Après |
  |---|---|---|
  | `compare?from=snapshot:1&to=bom:3` | 3,17 / 3,10 / 4,07 | 1,35 / 1,00 / 0,88 |
  | `compare?from=bom:1&to=bom:3` | 2,24 / 3,20 / 1,75 | 0,74 / 0,73 / 0,71 |
  | `analysis/fx-sensitivity?shock_percent=5` | 1,52 / 1,54 / 1,39 | 0,19 / 0,24 / 0,22 |
  | `analysis/sourcing` | 1,21 / 1,52 / 1,14 | 0,47 / 0,32 / 0,30 |
  | `versions` (3 versions) | 0,95 / 1,08 / 1,21 | 0,90 / 0,74 / 0,63 |
  | `tree` | 0,63 / 0,59 / 0,57 | 0,48 / 0,51 / 0,64 |
  | `publication-checks` | 0,35 / 0,36 / 0,34 | 0,20 / 0,32 / 0,31 |
  | `snapshots/1` | 0,71 / 0,51 / 0,60 | 0,56 / 0,52 / 0,42 |
  | `POST publish` (bom 1, prix en direct, copie de la base) | 1,84 | 1,30 |

  Publication en prix en direct : toujours deux calculs (prix en direct pour les contrôles, prix figés pour le gel),
  chacun moins cher ; prix figés : un seul.

## Decisions

## Blockers

Aucun.

## Verification Report(s)
