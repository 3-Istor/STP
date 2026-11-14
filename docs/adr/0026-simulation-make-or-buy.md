# 0026 — Simulation make-or-buy d'un sous-ensemble
Statut : Accepté — Date : 2026-11-14 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
make-or-buy (`docs/features.md:416–431`) : comparer fabriquer (coûts fixes, main-d'œuvre, logistique)
et acheter (prix Tier 1, coût rendu) pour un sous-ensemble ; afficher les deux coûts, l'écart et le
volume d'équilibre ; pas de décision automatique. Dépend d'ADR 0024 (heures) et 0025 (coût rendu).
Non dit : d'où vient le prix Tier 1 d'un sous-ensemble (un nœud sans pièce n'a pas d'offre), la période
des coûts fixes, le contenu de « logistique ».

## Options
1. Simulation **sans stockage** : `POST /api/v1/boms/{id}/nodes/{nodeID}/make-or-buy` avec coûts fixes,
   prix Tier 1 et logistique saisis ; le reste vient de l'arbre — aucun schéma ; rien n'est retrouvé ni
   figé au snapshot.
2. Scénarios stockés (`make_or_buy_scenarios`) liés au nœud, copiés au snapshot — retrouvables et
   reproductibles ; table et copie supplémentaires.

## Décision
**option 1** pour la première tranche (hypothèse → prototype, AGENTS §10) ; l'option 2 si
l'équipe veut conserver les scénarios. Évaluateur pur dans `pricing`.

## Conséquences
- Fabriquer, par voiture = coûts fixes ÷ volume + Σ coût rendu des pièces du sous-arbre
  + Σ heures × taux horaire + logistique saisie. Coûts fixes = montant total réparti sur le volume.
- Acheter, par voiture = prix Tier 1 saisi + son coût rendu (douane, transport, emballage d'ADR 0025
  avec l'origine saisie).
- Volume d'équilibre = ⌈coûts fixes ÷ (achat − coût variable interne)⌉ ; si achat ≤ variable interne :
  « jamais rentable », sans division.
- Tout montant reste (décimal, devise) ; taux manquant → erreur nommée. Résultat explicable (composantes).
- Aucune modification de la BOM : l'utilisateur décide (Won't : décision automatique).
