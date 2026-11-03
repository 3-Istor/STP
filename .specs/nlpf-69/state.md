---
id: nlpf-69
status: in-progress
branch: nlpf-69
current_task: T7
started: 2026-11-02
updated: 2026-11-03
---

# State: BOM (C)

## Progress

T1-T6 terminées (6/9). Prochaine : T7 (domaine snapshot).

## Task Log

### T1: ADR 0006 (stockage snapshot de BOM) et ADR 0007 (règles de calcul des totaux)
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 737be8f
- **Notes**: Deux ADR au statut `Proposé`. 0007 tranche aussi, en plus de D4, le comportement d'un parent
  quand un prix/taux manque (pas de total de coût, liste des nœuds en défaut ; CO2 et délai restent calculés)
  et inclut la masse avec la même règle que le CO2 : à valider par l'humain.

### T2: Étendre `catalog.Part` et `pricing.Pricing` (+ stubs)
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 82548e3
- **Notes**: `Part` + `CO2Kg`, `MassKg`, `ProductionDays` (stub refuse les valeurs négatives).
  `Pricing` + `FreezePrices(ctx, offerIDs, currencies, at)`, `SnapshotOfferPrice`, `SnapshotRate`,
  sentinelle `ErrPriceSnapshotNotFound`. Stub rendu mutable (`AddQuote`, `AddRate`, mutex) pour simuler
  l'arrivée de cotations. Contrat d'interface partagé : à annoncer à Joe (NLPF-67, NLPF-68).

### T3: Migration 00002 — colonnes brouillon et tables de snapshot de BOM
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 0e6c472
- **Notes**: `bom_nodes.production_days`, `boms.price_snapshot_id` / `source_snapshot_id`, `bom_snapshots`
  (FK `bom_id` sans cascade, `price_snapshot_id` NOT NULL) et `bom_snapshot_nodes` (PK `(snapshot_id, node_key)`,
  FK parent **différée** pour insérer dans n'importe quel ordre, une seule racine, CHECK : un nœud pièce a
  CO2/masse/délai de livraison figés, un sous-ensemble n'en a pas). Triggers `reject_mutation`.
  `goose up/reset/up` vérifié sur une base jetable (`bomstudio_t3_check`, supprimée ensuite) : `reset` sur la
  base de dev a été refusé par le garde-fou (destruction des données locales). Test d'intégration : refus
  UPDATE/DELETE/TRUNCATE (23001), DELETE d'une BOM publiée (23503), brouillon toujours modifiable, lignes figées invalides refusées.

### T4: Accès données BOM (sqlc) et opérations d'arbre
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 52d0865
- **Notes**: Option (a) : `internal/bom/store.go` = interface `Store`, types `BOM`/`Node`, sentinelles
  (`ErrBOMNotFound`, `ErrNodeNotFound`, `ErrCycle`, `ErrRootNode`, `ErrHasSnapshots`, `ErrNotAPart`) ;
  implémentation pgx dans `internal/bom/bomstore` (au lieu de `internal/bom/store.go` prévu par la spec),
  requêtes `internal/bom/queries.sql` → `internal/bom/bomdb`. `MoveNode` verrouille la ligne `boms`
  (`FOR UPDATE`) avant la CTE pour que deux déplacements concurrents ne créent pas de cycle.
  Ajouts hors liste de la spec, utiles à T6/T8 : `SetNodeQuantity`, `SetNodeProductionDays`, `SetPriceSnapshot`.
  sqlc refuse `EXISTS (SELECT … FROM cte)` : contrôle écrit avec `bool_or`. Modèles `catalogdb`/`pricingdb`
  régénérés (T3 les avait laissés périmés : la CI « code sqlc à jour » aurait échoué).
  Deadlock 40P01 entre le `TRUNCATE bom_snapshots CASCADE` du test T3 et les nouveaux tests quand les paquets
  tournent en parallèle : verrou `pg_advisory_xact_lock(69)` dans le `testDB` des deux paquets (5 runs verts).
  Mutation vérifiée : sans le contrôle de cycle, 4 cas de `TestMoveNode` échouent.

### T5: Calcul des totaux en Go (pur, sans base) + benchmark
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: b0cfcb2
- **Notes**: Signature `ComputeTotals(TotalsInput)` (struct au lieu de 4 paramètres) avec `CalcNode` détaché
  du brouillon, pour que T7 calcule un snapshot avec ses attributs figés. Les 4 exemples de R6, `ErrNoQuote`,
  `ErrNoRate`, taux de la mauvaise paire, absence d'arrondi intermédiaire, déterminisme et explication
  (taux utilisés triés, dates, source) testés. Benchmark : ~35 ms/op pour 30 000 nœuds.
  Choix agent à valider : une pièce qui a des enfants rend l'arbre invalide (`ErrInvalidTree`) plutôt que
  d'inventer une règle de cumul ; T6 devra refuser d'ajouter/déplacer un nœud sous une pièce.

### ADR 0008 : adaptation de `ComputeTotals` (préalable à T6)
- **Status**: completed
- **Time**: 2026-11-03
- **Commit**: 68251ba (ADR acceptée : 388af69)
- **Notes**: Rejet `ErrInvalidTree` « part node has children » retiré ; chaque nœud = contribution propre
  (pièce) + enfants. Nouvelle erreur de coût : une pièce sans prix garde son `ErrNoQuote`/`ErrNoRate` même si
  ses enfants en manquent aussi, et se liste en tête de `MissingCost`. `NodeTotals` gagne `OwnCost` et
  `OwnLeadTimeDays` (panneau latéral, ADR 0008 « Conséquences »). Tests : exemple Roue/Écrou de l'ADR
  (105 €/roue, 420 €, 20 écrous, 35 j), prix manquant sur parent, enfant, les deux, pièce racine.
  Benchmark 30k : ~28 ms/op.

### T6: Service BOM — CRUD, arbre, prix en direct ou figés, totaux
- **Status**: completed
- **Time**: 2026-11-03
- **Commit**: 38fe974
- **Notes**: `NewService(store, catalog, pricing)`. Sentinelles ajoutées : `ErrInvalidInput`, `ErrOfferNotForPart`.
  `FreezePrices(ctx, bomID, at)` (instant fourni par l'appelant) / `UnfreezePrices`. `Totals` renvoie
  `TreeTotals{BOM, Nodes, Totals}` pour T8. Tests sur un `Store` en mémoire (`memstore_test.go`, fichier de test
  hors liste de la spec). Mutations vérifiées : sans contrôle offre/pièce, toujours en direct, ou production du
  catalogue ignorée → au moins un test échoue. Tests d'intégration Postgres **sautés en local** (la base sur
  :5432 refuse l'utilisateur `bom`, aucun conteneur du projet lancé) : T6 ne touche pas la base, à rejouer en CI.

## Decisions

- 2026-11-02 — D1 : snapshot de BOM en tables relationnelles (ADR 0006 à rédiger) — choix humain.
- 2026-11-02 — D2 : étendre `catalog.Part` (CO2, masse, délai de production) + stub dès maintenant, à annoncer à Joe (NLPF-67) — choix humain.
- 2026-11-02 — D3 : brouillon depuis snapshot = nouvelle BOM avec `source_snapshot_id` — choix humain.
- 2026-11-02 — D4 : délai feuille = livraison de l'offre + production de la pièce (ADR 0007 à rédiger) — choix humain.
- 2026-11-02 — Défaut agent : virtualisation de l'arbre sans dépendance npm ; figer les prix via extension de `pricing.Pricing` + stub (implémentation base : NLPF-68).
- 2026-11-02 — T2 (validé par Arthur.presle) : `currencies` de `FreezePrices` = devises **cibles** ; on fige le taux
  de chaque devise d'offre vers chacune d'elles (paire exacte).
- 2026-11-02 — T2 (validé par Arthur.presle) : `FreezePrices` **refuse** (rien n'est créé) s'il manque un prix ou un
  taux, et liste tout ce qui manque (`ErrNoQuote` / `ErrNoRate` joints). Alternative écartée : figer
  partiellement, ce qui graverait un trou dans un snapshot immuable.
- 2026-11-02 — T2 : le stub copie les cotations courantes et n'utilise pas `at` (pas d'historique) ;
  l'implémentation base (NLPF-68) devra figer la dernière cotation `quoted_at <= at`.
- 2026-11-02 — Point à trancher avant T4 : `internal/bom/service_test.go` contient
  `TestDomainHasNoForbiddenDependency`, qui interdit `github.com/jackc/pgx` dans les dépendances du package
  `internal/bom`. Or T4 place `store.go` (pgx + sqlc) dans ce même package. Options : (a) mettre le store
  dans un sous-package `internal/bom/bomstore` (le domaine reste sans pgx, le test inchangé) ;
  (b) restreindre le test (affaiblir un test : interdit sans accord, AGENTS §11). L'agent propose (a).
- 2026-11-02 — ADR 0006 et 0007 passées à `Accepté` par Arthur.presle (y compris, pour 0007, le parent sans
  total de coût quand un prix/taux manque et la masse traitée comme le CO2).
- 2026-11-02 — Point avant T4 tranché par Arthur.presle : option (a), store dans `internal/bom/bomstore`.
- 2026-11-02 — `.specs/CLAUDE.md` (règles du skill spec) n'existe pas ; exécution faite avec `AGENTS.md`.
- 2026-11-03 — Point avant T6 tranché par Arthur.presle : une pièce peut avoir des enfants (option (b), « sous tout
  nœud »). Règle de cumul rédigée en ADR 0008 (`Proposé`) : pièce parent = sa contribution propre + celle de ses
  enfants ; délai = max(enfants) + (livraison + production). Exemple : Roue 4 × 100 € + Écrou 5 × 1 € → 105 €/roue,
  420 €, 20 écrous, 35 j. Conséquence pour T6 : retirer le rejet `ErrInvalidTree` « part node has children » de
  `ComputeTotals` (T5) et ajouter les tests de l'exemple ; le service n'a rien à refuser.
- 2026-11-03 — ADR 0008 passée à `Accepté` par Arthur.presle.
- 2026-11-03 — T6, choix agent à valider : les jours de production d'une **pièce** viennent du catalogue
  (`Part.ProductionDays`, ADR 0007 règle 4) ; le service refuse (`ErrInvalidInput`) de les saisir sur un nœud
  pièce plutôt que de stocker une valeur que le calcul ignorerait.
- 2026-11-03 — T6, choix agent à valider : figer une BOM déjà figée la fige à nouveau sur les cotations
  courantes (nouveau snapshot de prix) ; l'ancien snapshot de prix reste en base, immuable.
- 2026-11-03 — T6, choix agent : `SetNodeOffer` / `SetNodeProductionDays` chargent l'arbre (une requête) pour
  connaître le type du nœud, faute de `GetNode` dans l'interface `Store` ; à remplacer si le coût se voit.
- 2026-11-03 — T6 : seuls `ErrNoQuote` / `ErrNoRate` deviennent une erreur sur le nœud ; un snapshot de prix
  introuvable, une pièce ou une offre absente du catalogue, ou une panne font échouer `Totals` (rien caché).

## Blockers

Aucun. (Blocage T6 levé le 2026-11-03 : ADR 0008 acceptée par Arthur.presle. Blocage T3 levé le 2026-11-02 :
ADR 0006 et 0007 acceptées par Arthur.presle.)

## Verification Report(s)

<empty — appended by /spec-verify>
