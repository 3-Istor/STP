---
id: nlpf-69
status: blocked
branch: nlpf-69
current_task: T3
started: 2026-11-02
updated: 2026-11-02
---

# State: BOM (C)

## Progress

T1-T2 terminées (2/9). Bloqué avant T3 : ADR 0006 et 0007 à accepter par un humain.

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

## Decisions

- 2026-11-02 — D1 : snapshot de BOM en tables relationnelles (ADR 0006 à rédiger) — choix humain.
- 2026-11-02 — D2 : étendre `catalog.Part` (CO2, masse, délai de production) + stub dès maintenant, à annoncer à Joe (NLPF-67) — choix humain.
- 2026-11-02 — D3 : brouillon depuis snapshot = nouvelle BOM avec `source_snapshot_id` — choix humain.
- 2026-11-02 — D4 : délai feuille = livraison de l'offre + production de la pièce (ADR 0007 à rédiger) — choix humain.
- 2026-11-02 — Défaut agent : virtualisation de l'arbre sans dépendance npm ; figer les prix via extension de `pricing.Pricing` + stub (implémentation base : NLPF-68).
- 2026-11-02 — T2 (agent, à valider) : `currencies` de `FreezePrices` = devises **cibles** ; on fige le taux
  de chaque devise d'offre vers chacune d'elles (paire exacte).
- 2026-11-02 — T2 (agent, à valider) : `FreezePrices` **refuse** (rien n'est créé) s'il manque un prix ou un
  taux, et liste tout ce qui manque (`ErrNoQuote` / `ErrNoRate` joints). Alternative écartée : figer
  partiellement, ce qui graverait un trou dans un snapshot immuable.
- 2026-11-02 — T2 : le stub copie les cotations courantes et n'utilise pas `at` (pas d'historique) ;
  l'implémentation base (NLPF-68) devra figer la dernière cotation `quoted_at <= at`.
- 2026-11-02 — Point à trancher avant T4 : `internal/bom/service_test.go` contient
  `TestDomainHasNoForbiddenDependency`, qui interdit `github.com/jackc/pgx` dans les dépendances du package
  `internal/bom`. Or T4 place `store.go` (pgx + sqlc) dans ce même package. Options : (a) mettre le store
  dans un sous-package `internal/bom/bomstore` (le domaine reste sans pgx, le test inchangé) ;
  (b) restreindre le test (affaiblir un test : interdit sans accord, AGENTS §11). L'agent propose (a).
- 2026-11-02 — `.specs/CLAUDE.md` (règles du skill spec) n'existe pas ; exécution faite avec `AGENTS.md`.

## Blockers

### T3: Migration 00002 — colonnes brouillon et tables de snapshot de BOM
- **Error**: condition de la spec (T1 « Done when ») : ADR 0006 et 0007 doivent être acceptées par un
  humain avant T3. Elles sont au statut `Proposé`.
- **Attempts**: 0 (blocage volontaire, AGENTS §6 et §8)
- **Analysis**: T3 implémente ADR 0006 ; T5 implémente ADR 0007.
- **Suggested fix**: relire `docs/adr/0006-stockage-snapshot-de-bom.md` et `docs/adr/0007-calcul-des-totaux-bom.md`,
  passer `Statut : Accepté` et remplir `Décideurs`, trancher le point store/pgx (Decisions), puis relancer
  `/spec-execute nlpf-69`.

## Verification Report(s)

<empty — appended by /spec-verify>
