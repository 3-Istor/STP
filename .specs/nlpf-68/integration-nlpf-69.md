# Raccordements restants de NLPF-68 — revue concrète pour T10

État du 2026-11-05 : capacités autonomes T1–T9 commitées sur `nlpf-68`,
produit `448d4b3`, suivi `2dcd47a`. ADR 0011–0016 acceptées par Joe.
NLPF-69 inspectée en lecture seule à `ff5cc69` (Arthur), main `532b627`.
Aucun checkout, reprise, fusion, cherry-pick, push, PR ou accès à la base d’équipe.

## Livrable disponible et raccordement précis

| Capacité prête | Consommateur réel à adapter | Preuve encore obligatoire |
|---|---|---|
| `SnapshotService.Create/Get`, `SelectSnapshotCoverage`, liens fermés au commit | Attachement/détachement du brouillon et publication NLPF-69 ; devises nécessaires aux budgets | Refus du vrai brouillon insuffisamment couvert, prix/taux publiés stables |
| `ResolveFX`, `ConvertMoney`, `money.Exact` | `internal/bom/totals.go` de NLPF-69 porte encore `*money.Money` dans Cost/OwnCost et `RateResult` décimal | Somme rationnelle dans le vrai moteur avant arrondi ; inverse périodique donnant exactement 10 USD |
| `EvaluateBudget`, `EvaluateAllocations`, `EstimateProduction` | Paramètres BOM, nouvelle persistance, routes et écrans ; faits copiés aux nouvelles publications | Cibles par voiture, répartition imbriquée, volume de référence et remises conservés, ancien publié inchangé |
| Source HTTP, runtime persistant et port seed transactionnel | Futur seed NLPF-67 et vrai éditeur | Même jeu initial Multipla, ~30 000 nœuds, seed répété sans doublons |
| Signal SSE après commit, UI fournisseurs et tests trois moteurs | Écran de BOM et budgets NLPF-69 | Prix/totaux/budgets actualisés, brouillon figé stable, aucune navigation |

Les 12 parcours réels et les 60 parcours UI contrôlés sont verts. Ils ne remplacent aucune
preuve BOM de ce tableau. Le job CI additif `web-live` est préparé ; aucune CI distante sur
cette révision n’est déclarée verte, aucun push n’a été autorisé.

## Incompatibilités concrètes conservées

- Notre historique contient `00002_catalogue_fournisseurs`, `00003_pricing_runtime` et
  `00004_price_snapshot_closure`. NLPF-69 contient une autre `00002_bom_snapshots`.
  Une fusion de fichiers suivie de goose up ne suffit pas : des bases déjà en version 2
  portent des schémas différents. Les snapshots publiés #207/#208 mentionnés par Arthur
  doivent rester intacts. Ne renuméroter ni réécrire les migrations appliquées.
- Les ADR acceptées 0006–0008 ont aussi des sens différents sur les deux branches.
  Une procédure de traçabilité commune doit garder les textes acceptés et leurs références.
- Le catalogue local a des attributs optionnels explicites ; les types NLPF-69 sont différents.
  Toute absence reste une erreur de calcul, jamais une valeur zéro injectée.
- L’interface `Pricing` NLPF-69 exige gel/lecture snapshot et une paire exacte. Un inverse
  rationnel ne peut pas devenir un taux décimal inventé dans `SnapshotRate`. Les ports étroits
  autonomes permettent de composer cette adaptation après validation du contrat commun.
- Publication NLPF-69 : contrats/remises et budgets ne sont pas encore figés. Leur modèle
  doit être validé puis ajouté pour les nouvelles publications, sans enrichir les anciennes.
- Serveur, routeur et App/CSS sont partagés ; conserver les tests des deux parcours et le
  protocole d’archivage/verrouillage pièce puis offre lors d’ajout/déplacement de nœud.

## Ordre de reprise proposé à l’équipe

1. Confirmer l’accord du propriétaire de NLPF-69 et la révision/base commune à utiliser.
2. Valider une procédure d’évolution des trois historiques (v1, v2 catalogue, v2 BOM) et la
   traçabilité des ADR ; préparer les décisions nouvelles au statut Proposé avant leur code.
3. Vérifier cette procédure sur bases jetables munies de fixtures d’anciens snapshots,
   par comparaison complète des IDs, prix, taux, contrats et faits conservés.
4. Raccorder le vrai moteur et la couverture ; ajouter paramètres/persistance/publication
   budgets/remises puis composer les écrans et le flux. Aucun second moteur de BOM.
5. Exécuter les preuves réelles du tableau et la matrice complète J1–J10/C1–C4, puis la CI
   sur la révision autorisée à être publiée. Revue humaine avant toute fusion.

## Autorisation manquante

`AGENTS.md` §13 : « Un agent ne reprend pas en cours la branche d’un autre sans que
l’humain propriétaire soit d’accord. » L’ADR 0016 acceptée autorise les capacités autonomes
et maintient cette condition. Aucun accord d’Arthur ni base coordonnée nouvelle n’a été reçu.
Le choix d’une procédure de migration commune reste aussi une décision à faire valider (§6–8).
Toutes les validations indépendantes continuent ; T10 et l’acceptation finale restent ouverts.
