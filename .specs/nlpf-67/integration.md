# NLPF-67 — Proposition de contrats d'intégration (T1)

Statut : contrats retenus pour l'implémentation de NLPF-67, selon les ADR 0006–0010 acceptées
explicitement par Joe le 2026-11-04. Leur disponibilité technique reste à vérifier au fil des tâches.
Les interfaces de lecture actuelles restent compatibles ; aucun travail de NLPF-68/69 n'est repris ici.

## Répartition confirmée par les descriptions Jira

| Ticket | Assigné observé | Livraison attendue par NLPF-67 |
|---|---|---|
| [NLPF-67](https://3istors.atlassian.net/browse/NLPF-67) | Joe | Catalogue, fournisseurs, contrats, page, statistiques historiques et seed |
| [NLPF-68](https://3istors.atlassian.net/browse/NLPF-68) | Joe | Cotations/taux, historique initial, horloge, flux SSE, conversion |
| [NLPF-69](https://3istors.atlassian.net/browse/NLPF-69) | Arthur Presle | BOM, éditeur, quantités effectives, sélection contrat, totaux, snapshots BOM |
| [NLPF-70](https://3istors.atlassian.net/browse/NLPF-70) | Arthur Presle | Interfaces et schéma commun ; présents dans main, statut Jira encore To Do |

Consultation Jira le 2026-11-04 ; aucun ticket, commentaire ou lien modifié. Assignation constatée
ne signifie pas acceptation de ces contrats par l'équipe. Aucun travail repris sur une autre branche.

## Frontières et flux proposés

- `catalog.Catalog` garde `Part(ctx, id)` et `Offer(ctx, id)`. Les écritures passent par le nouveau
  service catalog, avec un repository interne. Pas d'ajout de méthodes d'écriture à l'interface
  consommée actuellement par BOM. Les lectures par ID rendent les archivés, les listes les excluent.
  Ajouts BOM : pièce puis offre sous FOR SHARE, vérification de archived_at dans la transaction ;
  archivage/suppression : même ordre sous FOR UPDATE. Une FK seule ne protège pas cet état.
- `supplier` ne lit pas l'arbre : son service évalue un choix de contrat pour une offre/quantité/date
  et retourne le palier applicable ou le motif d'absence. `bom` définit une interface étroite pour
  ce besoin, satisfaite par supplier au câblage. Les noms Go finaux sont locaux à l'implémentation.
- L'interface historique, distincte de `pricing.Pricing`, restitue les cotations d'une offre sur
  `[from, to]`, la dernière strictement avant from et la première connue si nécessaire. Chaque
  cotation porte id, offer_id, Money, source et quoted_at. Tri et ex æquo selon ADR 0008.
  Les requêtes PostgreSQL restent dans pricing ; le service statistique traite des cotations,
  jamais une réponse d'API externe. Pas de réimplémentation de l'ingestion du ticket NLPF-68.
- La composition fournisseur consulte catalog/supplier/pricing à travers les services ; le handler
  ne calcule ni coût ni statistique. Un prix manquant est une erreur par offre explicite dans une
  lecture composée, pas un zéro ni une disparition de la ligne.
- Le signal `GET /api/v1/quotes/stream` reste celui d'ADR 0003 : numéro de tick et instant simulé,
  puis relecture REST de la page. Le format précis du payload est à confirmer avec NLPF-68 avant
  T7 ; ce document ne déclare pas un endpoint SSE déjà disponible.
- BOM fournit la quantité effective par voiture (multiplication des parents), le volume, la date
  de calcul, le choix éventuel de contrat et les cotations choisies. Appliquer remise dans la devise
  d'origine, puis conversion déjà prévue par NLPF-68 et arrondi de sortie ADR 0004.
- Le seed attend un générateur pur d'historique et un mode d'insertion avec la transaction du
  chargement (ADR 0009). Une API distante non transactionnelle ne satisfait pas ce contrat.

## Contrats REST proposés pour le catalogue

Préfixe `/api/v1`, JSON snake_case, erreurs `{"error":"message explicite"}`.

| Route | Usage proposé |
|---|---|
| POST /parts, GET /parts/{id}, PATCH /parts/{id} | Créer/lire/modifier une pièce complète |
| GET /parts?name=…&category=…&supplier_id=…&zone=… | Recherche combinée, pièces distinctes |
| DELETE /parts/{id} | Supprimer si sans référence, sinon archiver ; résultat indique l'action |
| POST /parts/{id}/offers, GET /offers/{id}, PATCH /offers/{id} | Offres de la pièce ; pas de prix d'achat saisi |
| DELETE /offers/{id} | Suppression/archivage selon références et ADR 0006 |
| PUT /parts/{id}/alternatives/{other_id}, GET /parts/{id}/alternatives | Relation symétrique idempotente |
| POST /suppliers, GET /suppliers, GET/PATCH /suppliers/{id} | Fournisseurs et notes |
| POST /suppliers/{id}/contracts, GET /suppliers/{id}/contracts | Contrats, offres couvertes et paliers |
| GET/PATCH /contracts/{id} | Consultation/édition du contrat de travail |
| GET /suppliers/{id}/offers?as_of=… | Vue composée des offres, cotations, statistiques et séries |

Les routes du tableau sont relatives au préfixe, sauf le SSE explicitement complet.
Recherche proposée : sous-chaîne du nom insensible à la casse ; filtres catégorie/zone exacts,
AND entre filtres, ordre stable par id. Pas de déduplication de pièces par nom. Ce comportement
de recherche est proposé ici car docs/features.md:55 ne le détaille pas.

Nouvelles créations : 201 ; lectures/modifications : 200 ; identifiant absent : 404 ; JSON,
décimal numérique au lieu de chaîne, bornes ou dates invalides : 400. Conflit de sélection ou
référence empêchant une action : 409. Une opération idempotente ne crée pas de doublon.
PATCH absent = champ inchangé, null explicite d'un champ obligatoire = refus, pas remise à zéro.
Le DTO de liste donne une pièce et ses offres, pas une ligne pièce dupliquée par fournisseur.
Après première cotation, PATCH de devise/fournisseur différent renvoie 409 sans altérer l'offre
ni son historique : décision explicite de l'utilisateur du 2026-11-04. Créer une nouvelle offre et
archiver l'ancienne. Les bornes de sortie sont des objets Money montant/devise ; les entrées gardent
les chaînes price_min/price_max avec la devise de l'offre explicite.

## Données proposées

Pièce complète : `name`, `category`, `zone`, `mass_kg`, `co2_kg_per_unit`, `recycled_percent`,
`recyclability_percent`, `manufacturing_cost`, `production_days`, `labor_hours`, `hourly_rate`.
Montants : `{"amount":"10.00","currency":"EUR"}` ; autres décimaux : chaînes.
Délais et identifiants restent des entiers JSON ; les coûts peuvent avoir des devises différentes,
sans addition avant conversion. Aucun calcul humain dérivé stocké à la place de ses entrées.

Offre : `part_id`, `supplier_id`, `currency`, `price_min`, `price_max`, `lead_time_days`,
`quality_score`, `origin_country`. Bornes décimales dans la devise explicite de l'offre.
Fournisseur : `name`, `quality_score`, `trust_score`. Les notes sont des chaînes de 0 à 5.
Tous les champs économiques doivent être présents à la création. D'anciennes lignes incomplètes
signalent leurs champs manquants ; aucun enrichissement arbitraire pendant la migration.

Contrat : `supplier_id`, `starts_on`, `ends_on`, `conditions`, `offer_ids`,
`tiers: [{"min_quantity":"500","discount_percent":"5"}]`. Dates YYYY-MM-DD,
conditions textuelles informatives ; remise et quantité en chaînes. Pas de taux horaire implicite,
de devise implicite ni de contrat implicitement choisi parmi plusieurs actifs.

Statistiques d'une offre : `as_of`, `current_quote`, `reference_quote`, `period_start`,
`period_end`, `minimum`, `maximum`, `variation_percent`, `trend`, `history`, `error` éventuelle.
Le pourcentage de sortie est une chaîne à une décimale ; le ratio exact est calculé à partir des
prix de référence/courant renvoyés, sans précision globale implicite. Les montants min/max portent
la devise et les cotations portent date/source/id. Les coordonnées SVG ne sont pas des montants.

## Critères de disponibilité pour poursuivre

- T3/T4 : ADR 0006 acceptée, protocole d'ajout BOM/archivage partagé avec NLPF-69.
- T5/T8 : ADR 0007 acceptée ; sélection/quantité dans le contexte BOM et conversion disponibles.
- T6/T7 : ADR 0008 acceptée ; lecture d'historique, instant simulé et payload SSE coordonnés NLPF-68.
  Si le premier historique commence exactement à la borne annuelle, la variation est indisponible
  jusqu'à une référence strictement antérieure ; une cotation initiale supplémentaire doit être
  coordonnée et approuvée dans NLPF-68 pour changer ce résultat, jamais ajoutée implicitement ici.
- T7 : ADR 0010 acceptée, paquet de test autorisé, Node compatible et moteurs de test
  Chromium/Firefox/WebKit disponibles ; mêmes scénarios obligatoires sur les trois, profils mobiles
  et contrôles Safari réel tracés séparément. Aucun navigateur utilisateur imposé par le runner.
- T9 : ADR 0009 acceptée ; générateurs et services BOM/prix disponibles ; éditeur livré pour valider.

Ces contrats sont la cible d'intégration de NLPF-67. Leur indisponibilité ne justifie ni un remplacement silencieux
par des stubs en acceptation finale, ni l'ajout d'une feature hors périmètre de NLPF-67.
