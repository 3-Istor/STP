# 0010 — Date de validité et sélection des remises
Statut : Accepté — Date : 2026-11-04 — Décideurs : Joe (accord explicite dans le chat)

## Contexte
`docs/features.md:230–246` fixe la quantité par voiture × volume, le choix entre contrats actifs
et la remise sur toute la quantité. Les bornes de validité et l'horloge ne sont pas définies.
NLPF-67 doit appliquer 5 % sur 1 000 pièces à 10 EUR = 9 500 EUR et expliquer les exclusions.

## Options
1. **Date métier fournie, bornes inclusives** — déterministe, compréhensible en dates calendaires ;
   l'appelant doit fournir la date et son contexte.
2. **Borne de fin exclusive** — facilite des contrats consécutifs sans chevauchement ; dernier jour
   de validité moins intuitif à saisir.
3. **Date réelle du serveur** — simple ; rend le résultat dépendant de l'heure et du déploiement.

## Décision
**Proposition : option 1**, car elle préserve la reproductibilité et l'usage de dates métier.
Un contrat est actif pour `starts_on ≤ as_of_date ≤ ends_on`, avec début ≤ fin ; la date est dérivée
en UTC de l'horloge simulée en mode direct et fournie au domaine. Aucun `time.Now()` métier.

## Conséquences
- Sans sélection, un seul contrat actif applicable est utilisé ; plusieurs exigent un choix explicite.
  Aucun contrat actif signifie prix sans remise avec motif, pas une erreur de prix manquant.
  Un contrat sélectionné devenu expiré/futur est exclu avec motif ; il n'est pas remplacé silencieusement.
- `conditions` est un texte à consulter, sans interprétation automatique. Conditions calculables
  supplémentaires exigeraient une spec. Les paliers et leur taux sont validés selon ADR 0009.
- Quantité commandée décimale = quantité effective par voiture × volume, y compris les quantités
  des parents de l'arbre. Appliquer le palier de seuil maximal atteint à toute la quantité :
  `prix × quantité × (1 − remise/100)`. Sous le premier seuil : remise nulle explicitement expliquée.
  Aucun cumul de contrats, aucune conversion préalable des montants d'origine ; arrondi ADR 0004
  uniquement à la sortie du calcul complet. Sans prix valide, aucun coût à zéro.
- Restituer contrat sélectionné/appliqué, quantité, palier, taux, date de calcul, cotation et
  motif d'exclusion. BOM orchestre conversion et totaux (NLPF-69/NLPF-68) ; supplier ne lit pas l'arbre.
- Tester veille/début/fin/lendemain et minuit UTC, futur, sélection ambiguë ou expirée,
  quantités 499/500/1 000 et quantités imbriquées, absence de prix et devises distinctes.
- Publication : NLPF-69 devra figer ces données selon le modèle de snapshot qu'il fera accepter ;
  cette ADR ne définit ni ne change ce modèle. Retour arrière par une nouvelle stratégie documentée,
  jamais recalcul d'un snapshot publié. Hors-ligne : mêmes entrées datées ; multi-source/multi-usine
  restent hors périmètre, sans horloge réseau dans le domaine ; carbone sans impact.
