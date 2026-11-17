# 0047 — La copie d'un brouillon refuse un budget figé incohérent
Statut : Proposé — Date : 2026-11-17 — Décideurs : à valider par Arthur.presle

## Contexte
Relecture de la PR #26 : `snapshot.NewDraft` recopiait les budgets figés d'un snapshot par `SetTargetBudget`, qui ne
juge rien. Un snapshot dont un sous-budget dépasse son ensemble (donnée écrite avant la règle de l'ADR 0042, ou hors
service) produisait un brouillon que le service aurait refusé d'écrire. Décision de l'équipe dans le chat : « la copie
échoue et signale pourquoi ». Contrainte : l'ADR 0044 dit qu'un dérapage de change n'est pas un refus mais un
avertissement de répartition (`AllocationWarnings`), et un snapshot publié est immuable : un refus à la copie est
définitif, on ne peut pas le corriger.

## Options
1. **Ne rien faire** — un brouillon peut naître d'un état incohérent, sans signal.
2. **Refuser tout budget que la règle ne juge pas aux taux du snapshot** — bloque aussi les dérapages de change et
   les budgets en devise sans taux figé : un snapshot devient impossible à rouvrir pour une cause que l'ADR 0044
   classe en avertissement. Écartée (constaté par deux relecteurs, test `TestNewDraftKeepsFrozenBudgetsThat…`).
3. **Refuser seulement l'incohérence sans change** — dépassement entre budgets dans la devise de la BOM ; tout
   ensemble dont le verdict dépend d'un taux est copié et laissé à `AllocationWarnings`.

## Décision (proposée)
**Option 3.** `bom.CheckFrozenBudgets(nodes, budgets, devise)` : fonction pure (aucun taux, aucune horloge, même entrée
même verdict) appelée par `NewDraft` **avant** `CreateBOM` ; un refus renvoie `ErrBudgetExceedsParent` (409,
`budget_exceeds_parent`) avec le nœud et les montants, et ne crée aucun brouillon. Un snapshot antérieur à la
migration 00015 (`BudgetsFrozen` faux) n'est pas vérifié : ses budgets sont les budgets vivants de sa BOM.

## Conséquences
- Facile : un brouillon ne naît plus d'un état que le service aurait refusé ; la raison est nommée à l'écran.
- Difficile : un snapshot incohérent ne peut plus être rouvert tel quel ; l'issue (copier sans budgets, ou avec
  avertissement) reste à décider par un humain si le cas se présente. `Publish` ne juge toujours pas les budgets
  avant de les figer : c'est la cause racine, à traiter à part.
- Un état « non vérifié » existe pour les snapshots anciens (`BudgetsFrozen` faux).
- Retour arrière : retirer l'appel dans `NewDraft` (aucune migration).
