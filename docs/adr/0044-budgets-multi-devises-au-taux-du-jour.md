# 0044 — Budgets en plusieurs devises, plafond vérifié au taux du jour
Statut : Accepté — Date : 2026-11-17 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
ADR 0042 : la somme des sous-budgets d'un ensemble ne dépasse pas son budget, et **tous les budgets sont dans la
devise de la BOM**. Relecture de la PR #26 (relecteurs code et métier, vérifié) : `UpdateSettings` change la
devise de la BOM sans toucher aux budgets ; comme `CheckBudgetTree` juge tout l'arbre, un seul budget dans
l'ancienne devise bloque ensuite tout déplacement de nœud et toute pose ou tout retrait de budget.
Demande de l'équipe : **un sous-budget peut être dans une autre devise que l'ensemble (global en EUR, sous-budget
en JPY) ; il est converti pour le calcul, au taux du jour** — comme on le ferait en réel le jour où l'on pose le
budget (en démo, le jour simulé, ADR 0043). Règle de calcul absente de la spec → AGENTS §7.

## Options
1. **Une seule devise, changement de devise refusé** tant que la BOM a des budgets — simple ; pas de budget en devise.
2. **Conversion au taux budget annuel** (ADR 0031) — plafond stable toute l'année ; écarté par l'équipe : le
   besoin est de juger au jour de la saisie.
3. **Conversion au taux du jour** — chaque budget garde son montant et sa devise ; les sommes sont faites dans la
   devise de la BOM au dernier taux coté à l'instant courant (même source que les coûts en direct, ADR 0017).

## Décision
**Option 3.**
- **Écriture** (pose, retrait, déplacement) : refusée si elle fait dépasser un ensemble **qu'elle touche** (le nœud
  écrit et l'ensemble budgété au-dessus de lui, ou le nouvel ensemble d'accueil d'un nœud déplacé), sommes converties
  au taux du jour, exactes, sans arrondi intermédiaire (ADR 0004). Un dépassement ailleurs dans l'arbre ne bloque pas.
- **Message** : montants, taux, source et date utilisés (« 500 000 JPY × 0,00612, taux du 14/03/2026 = 3 060 EUR
  > 3 000 EUR »).
- **Taux absent** pour une paire : écriture refusée avec `ErrNoRate` nommé, jamais de budget ignoré ni de taux
  inventé (AGENTS §3).
- **Après coup** : le change peut faire dépasser une répartition déjà acceptée. Rien n'est bloqué ; l'ensemble
  est signalé (état `warn`/`over` avec la cause « change », taux et date), dans les couleurs existantes.
- **Changement de devise de la BOM** : permis ; les budgets gardent leur devise et sont convertis.
- Écran : le champ Budget de Structurer gagne le choix de la devise (défaut : celle de la BOM).

## Conséquences
- Facile : budgets dans la devise du fournisseur ou de l'usine ; plus de blocage après changement de devise.
- Difficile : un même plafond peut passer de vert à rouge sans écriture (change) — c'est voulu et dit. Avec la
  démo à 40 min par an, le taux change toutes les 6,5 s : deux saisies proches peuvent être jugées à deux taux.
- Une BOM aux prix figés juge ses budgets aux taux de son snapshot de prix (même source que ses coûts).
- Remplace ADR 0042 sur la règle « tous les budgets dans la devise de la BOM » et sur le contrôle de tout l'arbre ;
  le reste de 0042 est inchangé. Retour arrière : option 1.
- Pivot multi-devise / multi-usine : cette règle en est un premier pas (devise d'origine conservée, taux daté).
