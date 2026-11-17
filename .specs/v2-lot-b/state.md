---
id: v2-lot-b
status: in-progress
branch: v2-lot-b
current_task: T2
started: 2026-11-17
updated: 2026-11-17
---

# State: V2 lot B — versions et publication

## Progress

- Tasks: 1/9 completed
- Branch: `v2-lot-b` (contient lots A et D)

## Task Log

### T1: Ticket et ADR 0032, 0033
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: base `stp_lot_b` migrée (version 12), tests de référence verts avant toute modification.

## Decisions

- D1 (seul, à valider) : ordre du pont `structure → offer_change → price → fx → discount → logistics → tooling →
  method` ; ajout des causes `logistics` et `method` (« ce que la méthode exige »). Causes à 0 omises.
- D2 (seul) : appariement des nœuds par chemin (nom de sous-ensemble / id de pièce, rang parmi les frères
  identiques) : aucune colonne ajoutée à `bom_nodes`. Remplacer une pièce par une autre (autre id) = retirée +
  ajoutée (`structure`), pas `offer_change`.
- D3 (seul) : un fait absent d'un côté est pris de l'autre ; l'outillage d'une nouvelle offre tombe dans
  `offer_change` (la maquette I4-6 le montrait à part, elle est « illustrative »).
- D4 (seul) : `prices_frozen` est un `warn` (publier en prix en direct crée un snapshot de prix, c'est un choix
  à justifier) ; justifications pour un contrôle non `warn` ignorées (non stockées).

## Blockers

Aucun.

## Verification Report(s)

(à venir)
