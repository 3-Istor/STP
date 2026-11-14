# 0027 — Détection des anomalies de coût
Statut : Accepté — Date : 2026-11-14 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
cost-anomalies (`docs/features.md:433–449`) : écart à la médiane des pièces de la même catégorie,
seuil paramétrable (3× par défaut), seulement si la catégorie compte au moins 5 pièces ; surbrillance
dans la BOM avec la raison ; remplacement futur par un modèle IA **derrière la même interface**.
Non dit : quel coût d'une pièce (une pièce a plusieurs offres, en plusieurs devises), la population
(« historique » : catalogue entier ou BOM), la médiane d'un nombre pair, où vit le seuil.

## Options
1. Interface `AnomalyDetector` dans `pricing`, implémentation `MedianDetector` pure — prête pour l'IA
   (AGENTS §3 « IA dans le produit ») ; aucun nouveau domaine.
2. Calcul dans le handler ou le front — rapide ; logique métier hors domaine (interdit, AGENTS §4).

## Décision
**option 1**, appelée sur l'arbre d'une BOM, aucune table nouvelle si le seuil est un
paramètre de requête.

## Conséquences
- Coût d'une pièce = prix de l'**offre choisie dans la BOM**, converti dans la devise de la BOM ; population
  = toutes les pièces non archivées du catalogue de la même catégorie, chacune à son offre la moins chère
  convertie.
- Médiane d'un nombre pair = moyenne exacte des deux valeurs centrales (décimal).
- Anomalie si coût ≥ seuil × médiane ; raison = « 3,4× la médiane de <catégorie> (n pièces) ».
- Moins de 5 pièces, prix ou taux manquant → « non évaluable » avec la raison, jamais « normal ».
- Seuil : paramètre de requête (`?threshold=3`), décimal > 1.
- Pas de correction automatique (Won't).
