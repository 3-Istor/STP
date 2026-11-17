---
id: v2-lot-d
status: in-progress
branch: v2-lot-d
current_task: T2
started: 2026-11-17
updated: 2026-11-17
---

# State: V2 lot D — vue 3D par sous-ensemble

## Progress

- Tasks: 1/7 completed
- Branch: `v2-lot-d`

## Task Log

### T1: Ticket et ADR
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: spec, état, ADR 0035 au statut Proposé (décideur à compléter par un humain).

## Decisions

- **Slot (emplacement) comme pivot** entre BOM et 3D : une table nom → slot partagée par les deux rendus
  (ADR 0035, vision §6 « les trois partagent la même correspondance »).
- **Statut budget** : lu sur un champ optionnel `budget_status` des nœuds ; l'API actuelle ne le fournit pas
  → coloration par système avec un avis visible. Aucun calcul budget côté front (ADR 0018 appartient au back).

## Blockers

Aucun.

## Verification Reports
