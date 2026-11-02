---
id: nlpf-70
status: blocked
branch: nlpf-70
current_task: T4
started: 2026-11-02
updated: 2026-11-02
---

# State: Socle commun (les 3 ensemble)

## Progress

T1–T3 faites (3/7). Exécution arrêtée avant T4 : l'ADR 0002 doit d'abord être acceptée par les 3.

## Task Log

### T1: Brouillon ADR 0002 — où vit le snapshot de prix
- **Status**: completed (brouillon) — acceptation humaine en attente
- **Time**: 2026-11-02
- **Commit**: 3939deb
- **Notes**: ADR au statut `Proposé` : option 1 (en-tête `price_snapshots` + liaisons vers les cotations
  immuables), portée par `pricing`. La partie « acceptée par les 3 » du Done when reste à faire par
  l'équipe ; elle bloque T4.

### T2: Brouillon ADR 0003 — canal temps réel
- **Status**: completed (brouillon) — acceptation humaine en attente
- **Time**: 2026-11-02
- **Commit**: 6926ff4
- **Notes**: ADR au statut `Proposé` : SSE (stdlib) portant un simple signal de tick ; l'UI relit les
  totaux en REST. Repli polling documenté. Acceptation par les 3 à faire.

### T3: Type `Money` et ses tests
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 6146d5e
- **Notes**: `internal/money` (Money, Currency, ErrCurrencyMismatch, ErrUnknownCurrency, ErrInvalidAmount,
  Add, Mul(decimal), Round half-even, JSON en chaîne). Tests table-driven verts (`-race`), lint 0 issue.
  Ajout au test de `0.005 + 0.002 = 0.007` : le cas `0.005 + 0.005` de la spec ne peut pas détecter un
  arrondi prématuré (0.01 dans les deux cas). ADR 0004 au statut `Proposé`. AGENTS §2 mentionne
  `internal/money`. `shopspring/decimal` en direct.

## Decisions

- 2026-11-02 — Q1 : `Money` vit dans `internal/money` (package transverse). Décideur : Arthur.presle.
- 2026-11-02 — Q2 : tables initiales = suppliers, parts, offers, price_quotes, fx_quotes, price_snapshots
  (+ liaisons selon ADR 0002), boms, bom_nodes. Décideur : Arthur.presle.
- 2026-11-02 — Q3 : arrondi half-even aux unités mineures ISO 4217, en sortie uniquement ; ADR 0004 à
  rédiger dans T3. Décideur : Arthur.presle.

- 2026-11-02 — Accord §11 : ajout de shopspring/decimal et pgx/v5 (directs) et sqlc (tool) approuvé.
  Décideur : Arthur.presle.
- 2026-11-02 — `.specs/CLAUDE.md` absent : exécution menée avec les règles d'`AGENTS.md` seules.
- 2026-11-02 — T1/T2 cochées une fois le brouillon livré ; la partie « acceptée par les 3 » de leur
  Done when reste suivie dans les critères d'acceptation et le blocker T4.
- 2026-11-02 — Devises : table fermée de devises prises en charge dans `internal/money` ; code inconnu =
  `ErrUnknownCurrency` (pas de 2 décimales par défaut). Consigné dans ADR 0004.
- 2026-11-02 — `Mul` prend un `decimal.Decimal` (quantités non entières possibles), pas un entier.

## Blockers

### T4: Migration initiale + triggers d'immutabilité
- **Error**: pas d'échec technique ; prérequis humain manquant.
- **Attempts**: 0 (non démarrée volontairement)
- **Analysis**: le schéma des snapshots de prix dépend de l'ADR 0002, encore au statut `Proposé`.
- **Suggested fix**: les 3 relisent `docs/adr/0002-stockage-snapshot-de-prix.md`, la passent à `Accepté`
  (ou choisissent une autre option) avec leurs noms, puis relancent `/spec-execute nlpf-70`.
  ADR 0003 et 0004 attendent aussi leur acceptation (non bloquantes pour T4).

## Verification Report(s)
