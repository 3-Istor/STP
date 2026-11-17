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

## Decisions

## Blockers

Aucun.

## Verification Report(s)
