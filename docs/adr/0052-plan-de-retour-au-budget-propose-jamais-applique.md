# 0052 — Plan de retour au budget : proposer, ne jamais appliquer
Statut : Proposé — Date : 2026-11-18 — Décideurs : (à compléter par un humain)

## Contexte
Un sous-système peut dépasser son budget cible (ADR 0018, 0031). L'utilisateur veut voir les actions qui l'y ramènent
(autre fournisseur, autre matière). `docs/features.md` écarte « l'optimisation automatique de la BOM pour tenir le
budget » (Won't) : la décision reste humaine (« l'IA produit, l'humain garantit »). `GET …/suggestions` ne répond
que pour une pièce ; sous « Carrosserie et ouvrants » (BOM #1) il y a 2 705 pièces.

## Options
1. Ne rien faire — l'utilisateur ouvre les suggestions pièce par pièce : inutilisable à cette échelle.
2. Optimiseur qui modifie la BOM jusqu'à tenir le budget — contredit la spec, change les coûts sans décision humaine,
   et masque les arbitrages qualité, délai et CO2.
3. Plan en lecture seule : `GET /api/v1/boms/{id}/nodes/{nodeID}/budget-plan` propose des actions ; l'humain applique
   chacune avec l'appel existant qui change l'offre ou la pièce d'un nœud, puis le plan est relu.

## Décision
**À trancher par un humain** : la règle de sélection ci-dessous (économie d'achat seule, qualité et délai montrés mais
pas filtrés). Recommandation : l'accepter ; si non, il faut décider quels critères bloquent une action (qualité
minimale, délai maximal), ce qui ajoute des seuils à régler.

Option 3 : le plan propose, il n'applique jamais. Règle de sélection :
- par pièce du sous-arbre, la candidate (autre offre de la pièce ou pièce alternative active, mêmes candidates que
  `suggestions`) au **prix d'achat × quantité réelle** le plus bas, strictement inférieur à l'actuelle ; égalité : plus
  petit id d'offre ; candidate exclue (prix, taux ou attribut manquant) ignorée, jamais comptée comme gratuite ;
- la même substitution (offre avant → offre après) sur plusieurs nœuds forme une action ;
- tri par gain par véhicule décroissant (égalité : offre cible, puis premier nœud), cumul jusqu'à couvrir le
  dépassement, puis 3 actions de plus ; au plus 20 actions ;
- rien ne suffit : `covered` faux et `remaining` dit ce qui reste à trouver ;
- le gain est sur le prix d'achat quelle que soit la méthode de coût (rendu, complet) : une estimation, le vrai effet
  se lit sur le coût recalculé après application ;
- plafond de 5 000 pièces par plan (`plan_too_large`, 422) : au-delà, demander un sous-ensemble.

## Conséquences
Facile : une vue unique des leviers d'un sous-système, sans écriture. Difficile : le gain ne tient pas compte des
interactions (quantités minimales, contrats, coûts rendus) ; les actions sont indépendantes et leur somme est
indicative. Retour en arrière : route en lecture seule, retirer la route et le panneau ; aucune donnée ni migration.
Impact si le pivot arrive : hors-ligne, multi-source de prix, carbone versionné et multi-usine ne changent pas la règle ;
le plan lit les mêmes prix que les totaux du brouillon (snapshot de prix compris).
