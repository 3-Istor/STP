# 0014 — Conversion exacte par taux direct ou inverse
Statut : Proposé — Date : 2026-11-05 — Décideurs : en attente

## Contexte
`docs/features.md:203–214` autorise le taux inverse, interdit le pivot et demande sa provenance.
ADR 0004 impose l'arrondi uniquement en sortie. Un inverse périodique ne peut pas être porté
exactement par un décimal fini ; le moteur BOM actuellement sur NLPF-69 porte des Money décimaux.

## Options
1. Inverser en decimal avec précision fixée — petits changements ; arrondi intermédiaire nouveau.
2. Porter un montant rationnel exact jusqu'au résultat — aucune perte, même après somme/floor ;
   adapter explicitement la frontière de calcul BOM.
3. Refuser les inverses périodiques — simple ; ne respecte pas la conversion inverse exigée.

## Décision
**Proposition : option 2**, avec un type exact en complément de Money dans `internal/money`,
porté par `math/big.Rat` sans nouvelle dépendance ; pricing résout les cotations et leur sens.

## Conséquences
- Money et ses JSON existants restent décimaux, sans changement de contrat. Construire la valeur
  rationnelle à partir des décimaux exacts, conserver cette représentation lors des multiplications,
  additions, comparaisons et division entière de budget. Aucun float ni Div à précision implicite.
- Même devise : identité, sans inventer de taux ; sinon direct prioritaire, puis seule paire
  opposée inversée. Pas de chemin multi-paires ni de cotation inverse artificiellement enregistrée.
  Le résultat garde ID, paire, valeur décimale originale, source/date et sens de la conversion.
- En sortie seulement, quotient/reste entier et half-even aux unités mineures produisent un Money
  affichable, conforme à la règle ADR 0004 ; pas de changement de mode d'arrondi ni de mutation de cette ADR.
  Le total est arrondi après somme, jamais à partir des lignes déjà arrondies.
- Le moteur BOM doit agréger ces valeurs exactes dans le même calcul. Préparer un adapter temporaire
  qui arrondirait avant son appel n'est pas acceptable ; raccordement après accord avec Arthur.
- Tests : 100 USD × 0,92 = 92 EUR, direct prioritaire, inverse, absence de pivot, zéro/taux absent,
  JPY/KWD et >2^53. Trois fois 1 EUR avec USD→EUR=0,3 donnent ensemble 10 USD, pas 9,99.
- Retour arrière : conserver Money existant et désactiver le nouveau chemin sans réécrire les
  faits publiés ; une autre stratégie exige décision et version explicites. Hors-ligne : mêmes
  cotations figées, même résultat ; multi-source : provenance intacte. Aucun pivot anticipé.
