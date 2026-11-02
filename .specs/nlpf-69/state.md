---
id: nlpf-69
status: in-progress
branch: nlpf-69
current_task: T2
started: 2026-11-02
updated: 2026-11-02
---

# State: BOM (C)

## Progress

Execution in progress (T1 done).

## Task Log

### T1: ADR 0006 (stockage snapshot de BOM) et ADR 0007 (règles de calcul des totaux)
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 737be8f
- **Notes**: Deux ADR au statut `Proposé`. 0007 tranche aussi, en plus de D4, le comportement d'un parent
  quand un prix/taux manque (pas de total de coût, liste des nœuds en défaut ; CO2 et délai restent calculés)
  et inclut la masse avec la même règle que le CO2 : à valider par l'humain.

## Decisions

- 2026-11-02 — D1 : snapshot de BOM en tables relationnelles (ADR 0006 à rédiger) — choix humain.
- 2026-11-02 — D2 : étendre `catalog.Part` (CO2, masse, délai de production) + stub dès maintenant, à annoncer à Joe (NLPF-67) — choix humain.
- 2026-11-02 — D3 : brouillon depuis snapshot = nouvelle BOM avec `source_snapshot_id` — choix humain.
- 2026-11-02 — D4 : délai feuille = livraison de l'offre + production de la pièce (ADR 0007 à rédiger) — choix humain.
- 2026-11-02 — Défaut agent : virtualisation de l'arbre sans dépendance npm ; figer les prix via extension de `pricing.Pricing` + stub (implémentation base : NLPF-68).

## Blockers

- ADR 0006 et 0007 doivent passer à `Accepté` par un humain avant T3.

## Verification Report(s)

<empty — appended by /spec-verify>
