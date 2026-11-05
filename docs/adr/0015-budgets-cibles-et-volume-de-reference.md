# 0015 — Budgets cibles et volume de référence
Statut : Proposé — Date : 2026-11-05 — Décideurs : en attente

## Contexte
`docs/features.md:22,265–275` : budgets, couleurs et volume finançable. Joe a confirmé les
budgets cibles **par voiture**, le budget de production total et le **coût unitaire au volume
de référence explicite**, conservé avec le résultat. Le schéma et les cas limites restent ouverts.

## Options
1. Colonnes dans boms/bom_nodes — peu de jointures ; table partagée modifiée à chaque extension.
2. Tables de paramètres liées aux brouillons et copies pour les nouveaux snapshots — ownership
   clair, pas de défaut métier injecté dans les anciennes données ; lectures supplémentaires.
3. Stocker uniquement le résultat calculé — simple ; perd la provenance et le recalcul reproductible.

## Décision
**Proposition : option 2**, paramètres possédés par BOM, évaluateurs purs dans pricing ;
conserver les trois arbitrages déjà confirmés. Nouvelles tables et raccordement snapshots
seulement après accord d'intégration, aucune donnée de snapshot publié modifiée.

## Conséquences
- Paramètres : budgets Money optionnels, seuil décimal 90 % par défaut dans [0,100[,
  mode manuel ou budget de production, volume de référence positif explicite. Absence ≠ zéro.
  Budgets non négatifs ; devise d'origine conservée si la devise BOM change, conversion au calcul
  avec les mêmes cotations/snapshot et provenance. Taux manquant → comparaison indisponible.
- Pour budget >0 : vert si coût < seuil×budget, orange jusqu'à budget inclus, rouge au-dessus.
  Budget zéro : coût zéro vert, coût positif rouge. Coût manquant → état indisponible, pas de couleur
  de réussite. Comparer sans arrondi d'affichage. Répartition : contrôler les budgets sans ancêtre
  budgété contre le global, puis les sous-budgets dans leur budget parent, sans double comptage.
- Volume = floor(budget production / coût exact par voiture au volume de référence). Pas
  d'itération des remises sur le volume déduit ; chaque recalcul conserve référence, coût, contrats,
  prix et taux utilisés. Ce résultat suit cette référence, pas une optimisation du meilleur volume.
- Coût nul/manquant → erreur nommée ; quotient 0 → « aucune voiture finançable », sans forcer 1
  ni stocker 0 dans boms.volume. Conserver le volume manuel positif et restituer séparément le
  volume effectif calculé. Hors capacité de l'entier BOM → erreur, pas de débordement. Publication
  en mode production sans volume effectif positif représentable refusée explicitement.
- Nouvelles publications figent paramètres et faits nécessaires ; ancien snapshot sans budget
  reste sans budget, sans backfill. Tests : seuils/égalité/zéro, répartition imbriquée, 66 voitures,
  taux manquant, changement de prix, remise au volume de référence et snapshots inchangés.
- Retour arrière : désactiver le mode budget en conservant paramètres et volume manuel ; pas de
  réécriture publiée. Hors-ligne : même contexte figé ; autres pivots derrière les frontières existantes.
