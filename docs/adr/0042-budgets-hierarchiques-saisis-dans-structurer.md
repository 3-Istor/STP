# 0042 — Budgets cibles saisis dans Structurer, plafonnés par l'ensemble parent
Statut : Accepté — Date : 2026-11-17 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
budget-targets (`docs/features.md:259–275`) : *Must* « saisir un budget cible global » et « le répartir par
sous-système ». Aujourd'hui le domaine sait enregistrer un budget (`Service.SetTargetBudget`), mais aucune route
ni écran ne le permet : seuls les budgets du jeu de démo existent. Demande de l'équipe : poser le budget d'un nœud
dans l'onglet Structurer, et **si un ensemble a 3 000 €, la somme de ses sous-ensembles ne peut pas dépasser 3 000 €**.
La spec dit seulement « SI la somme des budgets par sous-système dépasse le budget global ALORS le système DOIT
avertir » (`docs/features.md:274`) : la règle demandée est plus stricte (refus) et s'applique à tous les niveaux.
Règle de calcul absente de la spec + nouveau contrat d'API → décision d'architecture (AGENTS §7).

## Options
1. **Ne rien faire** — budgets non saisissables ; le *Must* reste ouvert.
2. **Avertir seulement** (lettre de la spec) — la saisie passe toujours, l'écran signale le dépassement ;
   la répartition peut rester incohérente.
3. **Refuser** — un budget qui ferait dépasser la somme des sous-budgets d'un ensemble est refusé
   (`ErrBudgetExceedsParent`, 409), à tous les niveaux de l'arbre.

## Décision
**Option 3**, demandée explicitement par l'équipe. Règle : pour chaque nœud qui a un budget, la somme des budgets
de ses **descendants budgétés les plus proches** (on descend jusqu'au premier nœud budgété de chaque branche ;
un nœud sans budget est transparent) doit être ≤ son budget. Égalité permise. Tous les budgets d'une BOM sont dans
la devise de la BOM (sinon refus : additionner deux devises est une erreur, AGENTS §3).
La règle est vérifiée dans la **même transaction** que l'écriture, BOM verrouillée, à chaque :
pose/modification de budget, retrait de budget (les sous-budgets remontent alors vers l'ensemble au-dessus) et
déplacement de nœud. Le jeu de démo la respecte déjà (racine 26 500 € = Σ des six sous-systèmes).
API : `PUT /api/v1/boms/{id}/nodes/{node_id}/budget` `{"amount":"3000.00","currency":"EUR"}` et
`DELETE` sur la même route (rétrocompatible : ajouts). Écran : champ « Budget » dans le panneau du nœud,
onglet Structurer, brouillon seulement.

## Conséquences
- Facile : une répartition toujours cohérente ; l'erreur dit quel ensemble serait dépassé, de combien.
- Difficile : pour baisser un ensemble sous la somme de ses sous-budgets, il faut d'abord baisser ces derniers.
  L'avertissement de la spec (somme ≤ global) devient un refus : `docs/features.md` n'est pas modifié ici,
  à aligner par l'équipe si l'ADR est acceptée. Le coût réel peut toujours dépasser un budget (c'est le rôle des
  couleurs vert / orange / rouge), seule la **répartition** est plafonnée.
- Retour arrière : ne plus appeler la vérification (un seul endroit, `CheckBudgetTree`) → option 2.
- Pivot multi-devise : il faudra convertir les budgets à un taux daté avant de les additionner.
