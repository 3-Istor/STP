# 0024 — Calcul des ETP par sous-système
Statut : Accepté — Date : 2026-11-14 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
workforce-fte (`docs/features.md:343–357`) : ETP = heures de main-d'œuvre par voiture × volume ÷ base
horaire annuelle paramétrable ; vue par sous-système. Les heures par pièce existent (`parts.labor_hours`).
Non dit : valeur par défaut de la base horaire, heures d'un sous-ensemble (assemblage), ce qu'est un
« sous-système », arrondi d'affichage.

## Options
1. Base horaire en constante (1 607 h) — rien à stocker ; non paramétrable, contraire à la spec.
2. Base horaire **paramètre de la BOM** (colonne ou table de réglages, copiée au snapshot), défaut 1 607 h
   à la création — paramétrable et reproductible ; une donnée de plus dans le snapshot.
3. Base horaire en paramètre de requête — pas de schéma ; non figée à la publication.

## Décision
**option 2**, calcul pur dans `bom` à côté des totaux, en décimal (jamais `float`).

## Conséquences
- Heures d'un nœud = `labor_hours` de la pièce × quantité réelle + somme des enfants ; un sous-ensemble
  n'a pas d'heures propres (pas de champ). Heures manquantes sur une pièce → erreur nommée remontée,
  comme pour le coût (ADR 0007 règle 5), jamais zéro.
- Sous-système = **chaque nœud** (l'arbre l'affiche déjà) ; la vue « par sous-système » liste les
  enfants directs de la racine.
- Volume = `boms.volume` (volume manuel) ; le volume effectif d'ADR 0018 n'est pas utilisé tant qu'il
  n'est pas implémenté.
- ETP exact en interne ; affichage arrondi à 2 décimales (half-even), uniquement en sortie.
- Test d'acceptation : 16,07 h × 100 ÷ 1 607 = 1 ETP exactement.
- Retour arrière : supprimer le réglage ; aucun total existant modifié.
