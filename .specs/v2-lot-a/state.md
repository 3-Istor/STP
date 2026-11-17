---
id: v2-lot-a
status: in-progress
branch: v2-lot-a
current_task: T2
started: 2026-11-17
updated: 2026-11-17
---

# State: V2 lot A — coût complet

## Progress

- Tasks: 1/8 completed
- Branch: `v2-lot-a`

## Task Log

### T1: Ticket et ADR 0031
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: ADR 0031 au statut Proposé. Constat : les budgets cibles (target-costing) n'ont aucun stockage
  (ADR 0018 l'avait laissé en attente) : table ajoutée dans 00012 pour pouvoir les seeder.

## Decisions

- D1 (seul, à valider) : `purchase` = calcul historique (cotation brute, sans remise) pour que le défaut
  `purchase`/`latest` ne change aucun total existant ; la remise entre à partir de `landed`.
- D2 (seul, à valider) : quantité comparée aux paliers = Σ quantités réelles de l'offre dans la BOM × `volume`
  de la BOM (pas le volume programme).
- D3 (seul, à valider) : outillage NULL en `full` → `ErrNoTooling` (pas de total) ; « pas d'outillage » se
  déclare à 0. Le seed déclare donc 0 explicitement sur les offres sans outillage.
- D4 (seul, à valider) : date de calcul d'un brouillon = cotation la plus récente utilisée (temps simulé).
- D5 (seul, à valider) : en `budget`, ligne `price` au dernier taux + ligne `fx` = écart budget − dernier.
- D6 (seul, à valider) : budgets cibles stockés par nœud (`bom_target_budgets`), racine = budget global.

## Blockers

Aucun.

## Verification Report(s)

(à venir)
