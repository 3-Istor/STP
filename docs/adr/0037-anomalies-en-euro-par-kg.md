# 0037 — Anomalies de coût en prix au kg par famille
Statut : Accepté — Date : 2026-11-17 — Décideurs : Brian P. (accord explicite dans le chat)

## Contexte
Audit métier (vision V2 §2) : la détection d'ADR 0027 (prix unitaire ≥ 3 × la médiane de la catégorie) est
faible, car une catégorie mélange des pièces de masses très différentes (une vis et un bloc-cylindres). Le
cost engineer compare un **prix au kg par famille** (should-cost paramétrique minimal) et veut savoir **combien**
l'écart coûte par véhicule. Maquette `I4-4-Chiffrer` : pièce, famille, €/kg, médiane famille, surcoût estimé.

## Options
1. **Ne rien faire** : garder la médiane du prix unitaire — aucune donnée nouvelle ; faux positifs sur les
   pièces lourdes, aucun surcoût chiffré.
2. **Prix au kg comparé à la médiane €/kg de la famille** (catégorie du catalogue), nouvelle implémentation de
   l'interface `AnomalyDetector` d'ADR 0027 — même population, même conversion ; demande la masse de la pièce.
3. **Régression should-cost** (prix ~ masse × matière × procédé) — plus juste ; données de procédé absentes,
   résultat moins explicable.

## Décision
**Option 2**, en **stratégie** à côté de l'ancienne (AGENTS §4) : `pricing.PerKgDetector` et
`pricing.MedianDetector` implémentent la même interface ; la méthode (`per_kg` | `median`) est choisie au seul
endroit `anomalyDetectorFor`. Le nouvel appel `GET /boms/{id}/analysis/anomalies` vaut `per_kg` par défaut ;
`GET /boms/{id}/anomalies` (ADR 0027) reste inchangé.
- Prix au kg d'une pièce = coût unitaire de l'offre retenue converti (même source et mêmes taux qu'ADR 0027)
  ÷ masse unitaire (28 décimales). Population = pièces actives de la famille, chacune à son offre la moins chère.
- Anomalie si €/kg ≥ seuil × médiane €/kg de la famille ; seuil par défaut **2** (paramètre `threshold` > 1) ;
  au moins 5 pièces évaluables dans la famille, sinon « non évaluable » avec la raison.
- **Surcoût estimé** par véhicule = (coût unitaire − médiane €/kg × masse) × quantité réelle de la pièce dans
  la BOM (somme de ses nœuds) ; résultats classés par surcoût décroissant (égalité → id de pièce).
- **Masse absente ou nulle** → la pièce est exclue avec la raison `pricing.ErrNoMass`, jamais un €/kg à 0.

## Conséquences
- Facile : ajouter une méthode (une implémentation, un cas dans `anomalyDetectorFor`), comparer les deux.
- Difficile : une famille hétérogène en matière (plastique et acier dans « Carrosserie ») reste mal comparée :
  les familles du catalogue deviennent la vraie limite. Le surcoût est une estimation, pas une correction.
- BOM aux prix figés : la population de la famille n'est pas figée → non évaluable (comme ADR 0027).
- Retour arrière : `?method=median`. Pivot : multi-source de prix → inchangé (même lecture des prix) ;
  matières indexées → la médiane pourra être calculée par matière plutôt que par famille.
