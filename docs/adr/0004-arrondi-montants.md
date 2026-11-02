# 0004 — Arrondi des montants : half-even, aux unités mineures, en sortie seulement
Statut : Proposé — Date : 2026-11-02 — Décideurs : à compléter (règle tranchée en cadrage par Arthur.presle)

## Contexte
AGENTS §3 impose un « arrondi explicite aux unités mineures de la devise, uniquement en sortie », sans dire
lequel. `docs/features.md` n'en parle pas non plus (multi-currency-costing : « 100 USD × 0,92 = 92,00 € »).
C'est une règle de calcul absente de la spec, donc une décision du §7. Elle conditionne la reproductibilité
des totaux (même entrée = même résultat) et l'écart entre la somme des lignes et le total d'une BOM.
Implémentée par `internal/money` (NLPF-70 T3).

## Options
1. **Half-up (arrondi « commercial »)** — 0,125 → 0,13. + intuitif. − biais systématique vers le haut :
   sur des milliers de lignes, les écarts s'accumulent dans le même sens.
2. **Half-even (arrondi bancaire)** — 0,125 → 0,12 ; 0,135 → 0,14. + pas de biais statistique, norme
   IEEE 754 et usage comptable courant. − surprend à la lecture (« pourquoi 0,12 ? »).
3. **Troncature** — + simple. − biais vers zéro, perd de la valeur à chaque ligne.
4. **Stocker en unités mineures entières (centimes)** — + aucun arrondi « en sortie » à décider.
   − arrondit dès la saisie et à chaque multiplication (taux, remises) : contraire au « en sortie seulement ».

## Décision
**Option 2 : half-even, aux unités mineures ISO 4217 de la devise (EUR/USD 2, JPY 0, KWD 3), appliqué
uniquement par `Money.Round()`**, en sortie (affichage, publication). Les calculs (`Add`, `Mul`,
conversion) restent exacts en `decimal.Decimal` ; on arrondit le total, pas chaque ligne avant de sommer.

## Conséquences
- Facile : un seul point d'arrondi, testé (`internal/money/money_test.go`) ; changer de règle = changer
  une ligne et ses tests.
- Difficile : la somme des lignes arrondies affichées peut différer du total arrondi d'une unité mineure ;
  l'UI doit l'accepter (le total fait foi) plutôt que recalculer en `number`.
- Les devises prises en charge sont celles de la table de `internal/money` ; une devise inconnue est une
  erreur nommée (`ErrUnknownCurrency`), jamais un arrondi à 2 par défaut.
- Retour arrière : passer `RoundBank` à `Round` (half-up) ; les snapshots publiés gardent leurs montants
  exacts, seul l'affichage change.
- Pivot multi-devise : chaque devise arrondit selon ses propres unités mineures ; la conversion se fait
  avant l'arrondi, jamais après.
