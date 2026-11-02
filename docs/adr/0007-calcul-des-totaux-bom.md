# 0007 — Règles de calcul des totaux d'une BOM (coût, CO2, délai)
Statut : Proposé — Date : 2026-11-02 — Décideurs : —

## Contexte
bom-tree-editor (`docs/features.md` l. 94-121) : totaux par nœud en remontant l'arbre ; quantités multipliées
le long du chemin ; coût converti dans la devise de la BOM **puis** additionné ; délai = délai le plus long
des enfants + production du nœud. La spec ne dit pas ce qu'est le délai d'une **pièce** (feuille), ni ce que
devient le total d'un parent quand un prix ou un taux manque. Ce sont des règles de calcul absentes de la spec
(AGENTS §7). Choix D4 exprimé par l'humain au cadrage de NLPF-69, reste à l'acter. Cible : 30 000 nœuds < 1 s.

## Options
Délai d'une feuille :
1. **Livraison de l'offre + production de la pièce** (`offers.lead_time_days + parts.production_days`).
   + reflète le chemin réel (attendre la pièce, puis la produire / préparer) ; changer d'offre change le délai.
   − deux sources à figer au snapshot.
2. **Livraison seule** — + simple. − la production d'une pièce n'est jamais comptée.
3. **max(livraison, production)** — − suppose les deux en parallèle, ce que rien n'indique.

Total d'un parent quand un descendant n'a ni prix ni taux :
A. **Pas de total de coût** : le parent porte l'erreur et la liste des nœuds en défaut. + rien de faux affiché.
B. **Total partiel marqué « incomplet »** — − un chiffre faux finit toujours recopié quelque part.

## Décision
**Option 1 et option A**, avec ces règles, dans cet ordre :
1. Quantité réelle d'un nœud = produit exact (`decimal`) des quantités de la racine au nœud.
2. Coût d'une feuille = prix de l'offre (dernière cotation, ou snapshot de prix du brouillon / du snapshot)
   × quantité réelle, **converti** dans la devise de la BOM avec le taux de la paire exacte (pas de
   conversion si même devise, pas d'inversion de taux : NLPF-68). Coût d'un parent = somme exacte des coûts
   de ses enfants (`money.Add`, même devise garantie). Aucun arrondi en cours de calcul : arrondi half-even
   en sortie seulement (ADR 0004).
3. CO2 (et masse) d'une feuille = valeur de la pièce × quantité réelle ; d'un parent = somme des enfants.
4. Délai d'une feuille = livraison + production ; d'un parent = max(délai des enfants) + sa production ;
   un sous-ensemble vide = sa production. La quantité n'influe pas sur le délai.
5. Prix manquant → `ErrNoQuote`, taux manquant → `ErrNoRate`, sur la feuille ; chaque ancêtre n'a pas de coût
   et liste les nœuds en défaut. CO2 et délai restent calculés (ils ne dépendent pas du prix).
6. Déterminisme : enfants parcourus par `id` croissant ; chaque taux et chaque prix est lu une seule fois par
   calcul (pas deux valeurs pour la même paire) ; l'explication liste la source des prix (en direct ou id
   du snapshot de prix), la devise et les taux utilisés avec leur date, triés par paire.

## Conséquences
- Facile : calcul pur en Go (`internal/bom/totals.go`), testable sans base, un parcours post-ordre en O(n).
- Facile : chaque total est explicable (taux et dates listés) et reproductible (même entrée = même sortie).
- Difficile : un seul prix manquant masque le coût de toute la branche jusqu'à la racine ; c'est voulu,
  l'UI montre où est le trou.
- Remises et contrats (features à venir) s'insèreront entre 2 et la somme : nouvelle ADR à ce moment.
- Retour arrière : la règle 4 tient en une fonction ; passer à l'option 2 = une ligne et ses tests.
- Pivot : carbone versionné → la règle 3 prend le facteur de la version figée ; multi-devise → la règle 2
  gagne la conversion par devise pivot (NLPF-68) sans changer l'ordre ; hors-ligne → le calcul ne lit que
  des valeurs déjà chargées, il ne dépend ni du réseau ni de l'heure.
