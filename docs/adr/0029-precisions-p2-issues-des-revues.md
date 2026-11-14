# 0029 — Précisions des features P2 issues des revues de code
Statut : Proposé — Date : 2026-11-14 — Décideurs : à compléter

## Contexte
Les relectures (une par feature P2) ont trouvé des cas que les ADR 0022 à 0028 ne tranchent pas, ou une
règle acceptée qui laissait un critère d'acceptation en défaut. Ces précisions sont **déjà codées et testées**
pour ne pas livrer un bug connu ; elles attendent une validation humaine (AGENTS §6, §8). Revenir en arrière
sur l'une d'elles = un commit ciblé, chacune a son test.

## Options
1. Accepter les précisions telles que codées (liste ci-dessous).
2. En refuser certaines : retour à la règle de l'ADR d'origine, en assumant le défaut signalé par la revue.

## Décision
**Proposition : option 1.**
- **Planning (0023)** : chemin critique et responsables = nœuds à **délai propre > 0**, sous-ensembles et racine
  compris ; une pièce en stock (délai propre nul) n'est plus listée. Sinon une cible impossible pouvait donner
  une liste vide. Jours comptés sans `time.Duration` (dates éloignées).
- **Stock (0022)** : un brouillon recréé depuis un snapshot recrée les nœuds par clé croissante, pour que la
  répartition du stock reste identique. Saisie bornée à `[0, 1e15[`, 6 décimales au plus, et pièce archivée refusée.
- **Stratégies (0028)** : appliquer une suggestion passe par `PATCH {part_id, offer_id}`, accepté seulement pour
  la pièce du nœud ou une **alternative déclarée**, non archivées. L'offre actuelle reste visible même archivée.
- **Anomalies (0027)** : BOM aux prix figés → toutes les pièces sont « non évaluables »
  (`ErrPopulationNotFrozen`), car son snapshot de prix ne contient pas la population. Une pièce dont une offre
  active est sans prix ou sans taux sort de la médiane et est comptée dans `missing`. Le ratio affiché est tronqué.
- **ETP (0024)** : base horaire bornée à `]0, 8 784]` h ; un snapshot antérieur affiche `ErrLaborNotFrozen`.
- **Coût rendu (0025)** : les tables de douane et de transport sont lues **en direct**, même pour une BOM figée.
  La réponse l'indique (`logistics_tables: "live"`) avec les valeurs retenues et les taux utilisés.
- **Make or buy (0026)** : un sous-ensemble sans pièce donne `ErrEmptySubAssembly`, pas des coûts nuls. Avec
  coûts fixes nuls et coût égal, le volume d'équilibre vaut 1. Des coûts fixes non convertibles ne masquent pas le coût d'achat.

## Conséquences
Questions encore ouvertes (aucun code écrit, une ADR chacune) :
- BOM figée : faut-il figer aussi la population des anomalies, les offres alternatives et les tables
  logistiques ? Cela change le modèle de snapshot (§7). Aujourd'hui : erreur nommée, ou tables en direct.
- Lecture groupée des prix (une seule frame) pour les anomalies et make-or-buy : il faut un nouveau port
  `Pricing` (§7). Aujourd'hui : une lecture par offre, lente sur un gros catalogue, et la population en direct
  peut mélanger deux instants.
- Seuil « orange = proche » du planning (§5) : absent de la spec.
- Le stock doit-il entrer dans le classement « délai » des stratégies ? Les catégories doivent-elles être
  comparées sans tenir compte de la casse ?
