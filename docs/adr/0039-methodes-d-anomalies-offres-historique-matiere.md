# 0039 — Trois méthodes d'anomalies de prix : offres, historique, matière
Statut : Accepté — Date : 2026-11-17 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
Les méthodes `median` (ADR 0027) et `per_kg` (ADR 0037) comparent une pièce à la médiane de sa catégorie dans
**tout le catalogue**. Les catalogues Multipla (série) et Valhalla (hypercar) y sont mélangés : sur la Valhalla,
125 pièces sur 191 sont « anormales » (rapports jusqu'à ×91) parce que la médiane vient de pièces d'une autre gamme.
Le cost engineer veut savoir si une pièce est **trop chère**, et par rapport à quoi (`docs/features.md`,
cost-anomalies). Décision d'architecture (AGENTS §7) : règle de calcul absente de la spec + colonne ajoutée
à une table partagée (`parts`).

## Options
1. **Ne rien faire** — aucune donnée nouvelle ; la Valhalla reste illisible.
2. **Trois méthodes en plus**, chacune une stratégie de `AnomalyDetector` choisie en un seul endroit
   (`anomalyDetectorFor`), sans toucher à `median` ni `per_kg` :
   - `offers` : coût de l'offre choisie ÷ médiane des offres actives **de la même pièce** (≥ 2 offres) ;
   - `history` : prix coté de l'offre choisie ÷ médiane de ses cotations des 12 derniers mois, **dans la devise
     de l'offre** (le change n'entre pas dans le verdict), au moins 5 cotations ;
   - `material` : prix au kg comparé à la médiane €/kg de la famille **catégorie × matière** (≥ 5 pièces).
3. **Gamme/véhicule sur les BOM** — règle le mélange mais pas l'hétérogénéité de matière ; non retenu ici.
4. **Régression should-cost** — plus juste, données de procédé absentes, peu explicable.

## Décision
**Option 2.** Elle répond à trois questions distinctes (« moins cher ailleurs ? », « plus cher qu'avant ? »,
« plus cher que ses pairs de même matière ? ») dont deux (`offers`, `history`) ne dépendent d'aucune population de pièces, donc pas du mélange entre
véhicules. `material` en dépend encore (le catalogue est partagé) : mesuré sur la Valhalla, 35 pièces sur 191 restent
signalées et 91 non évaluables (famille < 5 pièces) ; elle réduit l'hétérogénéité de matière, pas l'écart de gamme.
Seuils par défaut : `offers` 1,25 ; `history` 1,15 ; `material` 2 (paramètre `threshold` > 1, comme les autres).
Le défaut de `GET …/analysis/anomalies` **reste `per_kg`** : ADR 0037 est acceptée, on ne la réécrit pas.
Changer le défaut serait une ADR de remplacement, à décider par un humain.
Migration `00016` : `parts.material` (texte, NULL = non renseignée) ; lecture seule côté API, pas d'écriture
par l'API catalogue dans ce lot. Une matière absente rend la pièce « non évaluable » (`ErrNoMaterial`), jamais
« normale ».

## Conséquences
- Facile : comparer les méthodes (`?method=`), en ajouter une ; verdicts explicables (médiane, rapport, effectif).
- Difficile : `history` ne signale pas une pièce chère depuis toujours ; `offers` suppose ≥ 2 offres ;
  `material` dépend d'une donnée de catalogue à tenir à jour. Le jeu `demo-v4` change (matière) : base à recharger.
- BOM aux prix figés : `offers` et `history` lisent le direct → non évaluables (comme ADR 0027, 0037).
- Retour arrière : ne plus appeler ces méthodes ; la colonne `material` reste inoffensive (NULL).
- Pivot : multi-source de prix → `offers` s'enrichit naturellement ; carbone versionné → inchangé.
