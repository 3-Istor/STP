# 0028 — Stratégies de décision par nœud
Statut : Accepté — Date : 2026-11-14 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
node-strategies (`docs/features.md:451–466`) : stratégie (coût, délai, CO2, équilibré) attachée à un nœud ;
les suggestions d'alternatives sous ce nœud la respectent ; « équilibré » = moyenne à poids égaux des scores
normalisés 0–1 sur les alternatives comparées. Non dit : héritage par les sous-nœuds, normalisation quand
toutes les valeurs sont égales, quelles candidates sont comparées, figeage au snapshot.

## Options
1. Colonne `strategy TEXT NULL CHECK (IN ('cost','lead_time','co2','balanced'))` sur `bom_nodes`
   (+ `bom_snapshot_nodes`) — une colonne ; table partagée modifiée (AGENTS §7).
2. Table `bom_node_strategies (node_id, strategy)` + copie snapshot — `bom_nodes` intact ; jointure de plus.

## Décision
**option 1** : la stratégie est une propriété du nœud, lue avec l'arbre sans jointure.
Chaque stratégie est une implémentation d'une interface `Strategy` choisie à un seul endroit (AGENTS §4).

## Conséquences
- Héritage : une pièce suit la stratégie de son **ancêtre le plus proche** qui en a une ; aucune → pas
  de tri imposé (ordre par id).
- Candidates d'une pièce : la pièce elle-même et ses alternatives du catalogue, chacune avec chacune de
  ses offres ; coût = prix converti × quantité réelle, délai = livraison + production, CO2 = unitaire × quantité.
- Score = (v − min) ÷ (max − min), 0 = meilleur ; si max = min, score 0 pour toutes. Égalité → id d'offre.
- Candidate sans prix, taux ou attribut → exclue avec la raison affichée (pas de score inventé).
- Les suggestions ne modifient rien : l'utilisateur applique sur le brouillon via le PATCH de nœud existant.
- Retour arrière : migration down supprimant la colonne ; snapshots anciens = sans stratégie.
