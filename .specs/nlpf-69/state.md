---
id: nlpf-69
status: in-progress
branch: nlpf-69
current_task: T4
started: 2026-11-02
updated: 2026-11-02
---

# State: BOM (C)

## Progress

T1-T3 terminées (3/9). En attente avant T4 : choix store/pgx (voir Decisions).

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
- 2026-11-02 — `.specs/CLAUDE.md` (règles du skill spec) n'existe pas ; exécution faite avec `AGENTS.md`.

## Blockers

Aucun. (Blocage T3 levé le 2026-11-02 : ADR 0006 et 0007 acceptées par Arthur.presle.)

## Verification Report(s)

<empty — appended by /spec-verify>
