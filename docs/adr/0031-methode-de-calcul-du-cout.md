# 0031 — Méthode de calcul du coût (base × change), ordre d'application et gel au snapshot
Statut : Proposé — Date : 2026-11-17 — Décideurs : (à compléter par un humain)

## Contexte
Vision V2 §2 et §5 (socle « coût complet et figé ») : le total d'une BOM = dernière cotation × quantité × dernier
taux. Les remises de contrat (`internal/supplier`, ADR 0010) et le coût rendu (ADR 0025, « les totaux restent au
prix d'achat ») sont hors totaux ; douane, transport, emballage et contrats ne sont pas figés au snapshot. L'expert
métier demande l'**outillage amorti** (outillage ÷ volume programme) et le **taux budget** de l'année, le dernier
taux servant de comparaison. Features : total-landed-cost, suppliers-and-contracts, multi-currency-costing.

## Options
1. **Ne rien faire** : totaux au prix d'achat, coût rendu à part — rien à migrer ; chiffre faux face à un Excel.
2. **Un booléen par composante** (remise, douane…) sur la BOM — souple ; 2⁵ combinaisons, `if` éparpillés.
3. **Méthode = `basis` × `fx`** choisie à un seul endroit (`bom.strategyFor`), stratégie AGENTS §4 —
   3 × 2 combinaisons nommées et testées ; une variante = un nouveau `basis`, pas un `if` de plus.

## Décision
**Option 3.** `basis` : `purchase` (calcul historique, cotation brute, **défaut** avec `latest` : aucune BOM
existante ne change), `landed` (= achat − remise + douane + transport + emballage), `full` (= landed + outillage
÷ volume programme). `fx` : `latest` (dernier taux du contexte de prix) ou `budget` (taux de l'année fixé par la
trésorerie, table `budget_rates`, paire directe seulement).
- **Ordre** : prix × quantité réelle → change → remise (% du prix converti) → douane (% du prix net de remise)
  → transport (au kg × masse × quantité) et emballage (par unité × quantité) → outillage. Exact en interne
  (seules les divisions non finies, outillage, sont à 28 décimales), arrondi ADR 0004 **en sortie seulement**.
- **Lignes** (`price|fx|discount|customs|freight|packaging|tooling`) additives dans la devise de la BOM. En
  `budget`, `price` est au dernier taux et `fx` = écart budget − dernier ; sans dernier taux, `price` est au
  taux budget et `fx` absente. Un sous-ensemble agrège ses descendants par `kind`.
- **Date de calcul** : brouillon = date de la cotation la plus récente utilisée (temps simulé, pas `time.Now`) ;
  snapshot = date figée (`cost_as_of`). Elle fixe l'état des contrats (bornes inclusives, ADR 0010) et l'année
  du taux budget.
- **Remise** : quantité = Σ quantités réelles de l'offre dans la BOM × `volume` de la BOM ; palier atteint sur
  toute la quantité (`supplier.EvaluateContracts`). Contrat expiré → non appliqué, motif `contract_expired`.
  Plusieurs contrats actifs → `ErrSelectionRequired` (aucun choix n'est encore stocké).
- **Donnée manquante = erreur nommée**, jamais 0 : `ErrNoBudgetRate`, `ErrNoTooling` (outillage NULL en `full` ;
  « pas d'outillage » se déclare à 0), `ErrNoProgramVolume`, `ErrNoCustomsDuty`… Le nœud n'a pas de coût.
- **Outillage** : par offre, réparti entre ses nœuds au prorata de la quantité réelle (Σ = outillage ÷ volume).
- **Budgets cibles** (ADR 0018, option 2) : table `bom_target_budgets`, budget de la racine = budget global.

## Conséquences
- Snapshot : méthode, volume programme, `cost_as_of`, taux budget de l'année et faits par offre (remise retenue
  avec contrat/palier/motif, douane, transport, emballage, outillage) sont **copiés** ; le snapshot rejoue le
  même coût au centime même si tables, contrats ou taux budget changent. Publication refusée si une composante
  de la méthode manque. Snapshot antérieur : méthode NULL → `purchase`/`latest`, comme avant (aucun backfill).
- Facile : comparer deux méthodes, expliquer une ligne, ajouter un `basis`. Difficile : un taux budget inversé
  ou via pivot (refusé, comme ADR 0020) ; la répartition d'outillage n'est pas exacte au-delà de 28 décimales.
- Retour arrière : `PUT cost-method` à `purchase`/`latest` ; les tables restent. Pivot multi-usine : la
  destination et le taux budget deviennent ceux de l'usine ; multi-source : inchangé (même adapter de prix).
