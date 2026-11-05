# API catalogue et fournisseurs

Préfixe `/api/v1`, JSON en `snake_case`, erreurs `{"error":"message explicite"}`.
Lancer `make migrate` puis `make run` ; API locale sur `http://localhost:8080`.
Les exemples ci-dessous créent de vraies données de travail. Les identifiants sont rendus par l'API.

## Formats

- Montant : `{"amount":"10.125","currency":"EUR"}`. Valeur exacte et devise conservées ;
  arrondi half-even uniquement à l'affichage/publication, pas au stockage.
- Décimaux, notes et taux : chaînes (`"4.5"`, `"32.125"`), jamais nombres JSON.
- Notes : [0,5] ; matière recyclée/recyclabilité et remises : [0,100] ; jours : entiers ≥ 0.
- Dates de contrat : `YYYY-MM-DD`, bornes inclusives. Instant de lecture `as_of` : RFC3339 avec
  fuseau, normalisé UTC ; aucune horloge réelle utilisée pour les calculs.
- PATCH : champs absents conservés, `null` explicite refusé. Les attributs legacy manquants restent
  `null` à la lecture, sans valeur inventée.

## Routes

| Méthode et route | Résultat |
|---|---|
| POST /parts | Pièce complète créée, 201 + Location |
| GET /parts?name=…&category=…&zone=…&supplier_id=… | Pièces distinctes et offres actives ; filtres combinés par AND |
| GET/PATCH/DELETE /parts/{id} | Lecture, modification, suppression ou archivage explicite |
| POST /parts/{id}/offers | Offre créée, bornes saisies dans sa devise, aucun prix courant saisi |
| GET/PATCH/DELETE /offers/{id} | Lecture, modification, suppression ou archivage explicite |
| PUT /parts/{id}/alternatives/{other_id} | Lien symétrique idempotent, 204 |
| GET /parts/{id}/alternatives | Alternatives actives |
| GET/POST /suppliers | Liste ou création d'un fournisseur |
| GET/PATCH /suppliers/{id} | Détails/notes d'un fournisseur |
| GET/POST /suppliers/{id}/contracts | Liste ou création de contrats |
| GET/PATCH /contracts/{id} | Contrat, offres couvertes et paliers |
| GET /suppliers/{id}/contracts?as_of=… | Contrats avec status active/not_started/expired |
| GET /suppliers/{id}/offers?as_of=… | Offres de ce fournisseur, pièces et statistiques annuelles |

Recherche par sous-chaîne du nom, insensible à la casse ; catégorie et zone exactes ; ordre stable
par ID. Une pièce avec deux fournisseurs apparaît une fois avec deux offres. Une référence
(BOM, offre, alternative, cotation ou contrat selon la ressource) préserve la lecture par archivage :
DELETE rend `{"id":42,"archived":true}` ; sans référence, `archived:false` et suppression physique.
Le catalogue courant masque les archives, les BOM gardent la lecture par ID.

Une offre déjà cotée ne change plus de devise ou de fournisseur : réponse 409, création d'une autre
offre puis archivage de l'ancienne. Les anciennes cotations restent immuables.

## Exemples de corps JSON

Fournisseur (`POST /suppliers`) :

```json
{"name":"Équipementier exemple","quality_score":"4.5","trust_score":"4.25"}
```

Pièce (`POST /parts`) :

```json
{
  "name":"Support moteur", "category":"supports", "zone":"moteur",
  "mass_kg":"1.25", "co2_kg_per_unit":"3.125",
  "recycled_percent":"35", "recyclability_percent":"95",
  "manufacturing_cost":{"amount":"8.125","currency":"EUR"},
  "production_days":2, "labor_hours":"0.25",
  "hourly_rate":{"amount":"20.50","currency":"EUR"}
}
```

Offre (`POST /parts/{id}/offers`, remplacer supplier_id par l'identifiant obtenu) :

```json
{"supplier_id":1,"currency":"EUR","price_min":"9.50","price_max":"12.00","lead_time_days":3,"quality_score":"4.5","origin_country":"FR"}
```

Les bornes de sortie sont des objets Money montant/devise. Elles ne constituent pas une cotation.

Contrat (`POST /suppliers/{id}/contracts`, remplacer offer_ids) :

```json
{"starts_on":"2026-01-01","ends_on":"2026-12-31","conditions":"Livraison groupée","offer_ids":[1],"tiers":[{"min_quantity":"500","discount_percent":"5"}]}
```

Le palier de seuil maximal atteint s'applique à toute la quantité. Pour 1 000 unités à 10 EUR,
5 % dès 500 donnent 9 500 EUR. Un choix est exigé entre plusieurs contrats actifs ; un contrat
sélectionné devenu expiré est exclu avec motif, sans remplacement silencieux. Le service de calcul
est disponible pour l'intégration BOM ; il ne remplace pas le moteur de totaux de NLPF-69.

## Lecture des prix

La page renvoie `{as_of, offers:[{offer,part,statistics,error?}]}`. Sans paramètre, `as_of` observe
la date maximale des cotations persistées ; ce n'est pas une horloge simulée ni une ingestion.
Les statistiques rendent la cotation courante, la référence, leurs IDs/sources/dates, les extrema,
la variation textuelle, sa tendance et l'historique. Les cotations futures sont exclues de l'instant demandé.
Les extrema n'incluent pas une référence située avant la fenêtre annuelle.

Référence/fenêtre manquante : données disponibles conservées, autres champs à `null`, erreur explicite.
Prix entièrement absent : ligne conservée sans prix à zéro. Le flux live et sa notification restent
la responsabilité de NLPF-68. Les tests utilisent des cotations contrôlées, pas une API fournisseur réelle.
