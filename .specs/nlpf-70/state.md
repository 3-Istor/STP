---
id: nlpf-70
status: in-progress
branch: nlpf-70
current_task: T3
started: 2026-11-02
updated: 2026-11-02
---

# State: Socle commun (les 3 ensemble)

## Progress

Spec created. Q1–Q3 tranchées et dépendances approuvées : prêt pour exécution
(T4 attend l'acceptation de l'ADR 0002).

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

## Decisions

- 2026-11-02 — Q1 : `Money` vit dans `internal/money` (package transverse). Décideur : Arthur.presle.
- 2026-11-02 — Q2 : tables initiales = suppliers, parts, offers, price_quotes, fx_quotes, price_snapshots
  (+ liaisons selon ADR 0002), boms, bom_nodes. Décideur : Arthur.presle.
- 2026-11-02 — Q3 : arrondi half-even aux unités mineures ISO 4217, en sortie uniquement ; ADR 0004 à
  rédiger dans T3. Décideur : Arthur.presle.

- 2026-11-02 — Accord §11 : ajout de shopspring/decimal et pgx/v5 (directs) et sqlc (tool) approuvé.
  Décideur : Arthur.presle.

## Blockers

- T4 bloquée tant que l'ADR 0002 n'est pas acceptée par les 3.

## Verification Report(s)
