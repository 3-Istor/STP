# 0045 — Le budget global reste dans la devise de la BOM
Statut : Proposé — Date : 2026-11-17 — Décideurs : (à renseigner par l'humain qui accepte)

## Contexte
ADR 0044 permet un budget dans n'importe quelle devise, converti au taux du jour. Relecture de la PR #26 (tours 1
et 2, relecteurs code et métier) : dès que le **budget global** (racine) est dans une autre devise que la BOM, trois
lecteurs doivent le convertir — écart des pistes de réduction, sensibilité au change, % du budget des versions — et
chacun ouvre un problème : taux non exposé dans trois réponses `/api/v1`, arrondi intermédiaire du montant converti,
choc de change qui déplace le coût mais pas le budget (dépassement affirmé à tort). L'équipe choisit de borner la
règle plutôt que d'empiler des correctifs.

## Options
1. **Garder 0044 tel quel** : exposer le taux dans trois routes, calculer en rationnels exacts, choisir une règle de
   choc pour le budget — plus de contrat et de code, risques déjà vus en relecture.
2. **Budget global dans la devise de la BOM ; sous-budgets dans toute devise** (0044 pour les ensembles).
3. **Revenir à une seule devise pour tous les budgets** (0042) : perd le besoin exprimé (sous-budget en JPY).

## Décision
**Option 2.** Précise ADR 0044 (qui reste valable pour les sous-ensembles) :
- un budget posé sur la **racine** dans une autre devise que la BOM est refusé (`ErrInvalidInput`, raison nommée) ;
- **changer la devise de la BOM** est refusé tant qu'elle a un budget global dans une autre devise que la nouvelle
  (`ErrInvalidInput`) : on retire d'abord le budget global ; les sous-budgets gardent leur devise et restent convertis ;
- ces deux règles sont jugées **sous le verrou de la BOM**, avec la devise relue dans la transaction de l'écriture
  (comme le contrôle de cycle) : deux requêtes simultanées ne peuvent pas les contourner ;
- pistes, sensibilité au change et versions lisent le budget global sans conversion ; un budget global hérité dans une
  autre devise (données antérieures) reste « non converti », avec la raison, jamais comparé ;
- l'avertissement de répartition distingue **dépassement** (change) et **répartition non vérifiable** (taux absent),
  et ne vise que les ensembles qui ont au moins un sous-budget.

## Conséquences
- Facile : le budget global, base de l'écart et des pourcentages, n'a jamais de taux caché ; pas de changement de
  contrat pour les pistes, la sensibilité au change et les versions.
- Difficile : une BOM qui change de devise doit d'abord retirer son budget global.
- Retour arrière : autoriser à nouveau une devise étrangère sur la racine (0044 intégral) en traitant les trois
  constats de relecture.
