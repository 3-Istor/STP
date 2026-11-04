---
id: nlpf-67
title: Catalogue et fournisseurs — pièces, offres, contrats, historique et données de démonstration
source: jira
source_ref: https://3istors.atlassian.net/browse/NLPF-67
created: 2026-11-04
status: in-progress
priority: P2
depends_on: [nlpf-70]
branch: nlpf-67
---

# Catalogue et fournisseurs (NLPF-67)

## Context

NLPF-67 doit rendre le catalogue de pièces et d'offres exploitable par API, appliquer les contrats
fournisseurs et présenter leurs offres avec leur historique de prix. Le ticket inclut aussi un jeu de
données déterministe permettant d'ouvrir la Fiat Multipla et ses totaux dans l'éditeur de BOM.
Le 2026-11-04, l'utilisateur a confirmé ce périmètre complet, avec des dépendances BOM/prix explicitement
bloquantes, et les échelles : notes de qualité/confiance de 0 à 5, recyclabilité de 0 à 100 %.
L'utilisateur a ensuite accepté explicitement les ADR 0006–0010 et demandé de poursuivre
l'implémentation le 2026-11-04 ; leurs choix guident désormais cette exécution.

## Research Findings

### Relevant Files

- `docs/features.md:40–68` : part-catalog ; attributs, offres, recherche, alternatives et archivage.
- `docs/features.md:124–161` : live-price-feed ; cotations immuables, temps simulé, historique et signal.
- `docs/features.md:218–250` : fournisseurs, contrats, paliers, statistiques et page fournisseur.
- `docs/features.md:279–298` : sample-dataset ; graine, idempotence et BOM jusqu'à environ 30 000 nœuds.
- `internal/catalog/catalog.go`, `stub.go`, `stub_test.go`, `queries.sql` : types Part/Offer minimaux,
  interface de lecture consommée par BOM, stub déterministe et deux requêtes de pièces seulement.
- `internal/pricing/pricing.go`, `queries.sql` : prix courant et taux ; pas d'interface d'historique ni
  de statistiques. LatestPriceQuote départage déjà par quoted_at puis id décroissants.
- `internal/bom/service.go` : lecture d'une offre et de sa cotation ; aucun calcul d'arbre ni de remise.
- `internal/supplier/doc.go` : domaine réservé, sans service ni contrat implémenté.
- `migrations/00001_init.sql`, `sqlc.yaml` : tables minimales, FK BOM→pièce/offre, cotations immuables ;
  générateurs catalogdb et pricingdb, pas de supplierdb. Ne pas modifier cette migration existante.
- `internal/money/money.go` : Money décimal, devise validée, JSON en chaîne et half-even explicite.
- `internal/pricing/immutability_integration_test.go` : intégration PostgreSQL en transaction annulée.
- `internal/httpapi/router.go`, `cmd/server/main.go` : uniquement /healthz ; serveur sans pool DB métier.
- `web/src/App.tsx`, `web/package.json` : écran de santé ; React sans bibliothèque graphique ni outil
  de tests navigateur déclaré. Le projet utilise Vite 8 et la CI Node 24 : Node 18 suffisant pour
  Spec-Drum ne prouve pas que le frontend peut être construit avec cette version.
- `Makefile`, `.github/workflows/ci.yml` : lint, tests Go, sqlc, migrations, build web et smoke Docker ;
  pas de cible seed. ADR 0002–0005 acceptées : stockage pricing, SSE, arrondi et API de prix séparée.

### Existing Patterns

- Domaines existants `catalog`, `supplier`, `pricing`, `bom` conservés ; handlers HTTP fins et
  composition dans cmd/server. Les propositions d'interfaces ci-dessous restent à approuver.
- SQL typé par sqlc, pgx/v5 et shopspring/decimal déjà disponibles ; aucun besoin établi de dépendance
  de production supplémentaire. Code généré uniquement par sqlc.
- Tests Go table-driven, sentinelles avec errors.Is et erreurs contextualisées ; horloge explicite
  pour les calculs. Le temps de référence des prix est le temps simulé, pas time.Now().
- Money et tous les décimaux du catalogue transitent comme chaînes JSON/TS ; aucune conversion
  monétaire en number. Formats et noms JSON snake_case.
- Le ticket est « To Do », assigné à Joe, priorité Jira Medium ; mapping proposé vers P2 pour cette
  spec. Les trois features fonctionnelles sont P1 : cette différence de classement est explicite.
- Aucune spec nlpf-67 existante, branche initiale main et arbre de travail propre lors de la recherche.
  La spec nlpf-70 est restée draft malgré ses tâches cochées : ne pas déduire son acceptation de ce statut.

### Constraints

#### Dépendances de livraison

La consultation Jira du 2026-11-04 identifie NLPF-68 (Prix & devises, Joe) et NLPF-69
(BOM, Arthur Presle), en plus du socle NLPF-70. Aucun lien de dépendance Jira n'est créé.
Leurs contrats proposés et responsabilités figurent dans [integration.md](integration.md) ;
ces propositions restent à confirmer, elles ne valent pas accord des propriétaires.

| Dépendance | État observé | Bloque |
|---|---|---|
| Socle NLPF-70 | Types, schéma et ADR présents ; CI courante non consultée | Vérification de compatibilité T2/T3 |
| live-price-feed (NLPF-68) | Interfaces/stubs et tables présents ; pas d'historique public, d'horloge ni de SSE opérationnels | T6/T7/T9 et validation finale |
| bom-management / bom-tree-editor (NLPF-69) | Tables présentes ; services, totaux et éditeur absents | Intégration T8 et ouverture Multipla T9 |
| multi-currency-costing (NLPF-68) | Pas de service de conversion observé | Totaux reproductibles de la Multipla multidevise |
| contrats de publication des BOM | Snapshots BOM non implémentés | Préserver ultérieurement les contrats/remises figés ; ne pas inventer leur stockage ici |

Le catalogue peut être livré en premier et testé avec des fixtures de cotations ; ces fixtures ne
remplacent pas le flux réel ni l'éditeur pour déclarer NLPF-67 terminé. Ne pas absorber implicitement
l'implémentation de ces autres features. Pas d'autre modèle de voiture exigé dans cette première livraison.

#### Décisions d'architecture acceptées le 2026-11-04

Les paragraphes D1–D5 ci-dessous décrivent le cadrage initial. Les ADR 0006–0010 sont désormais
Accepté sur instruction explicite de Joe ; leurs textes font foi pour les choix retenus.

- **D1 — Modèle partagé et interfaces (AGENTS §6–8).** Proposition : enrichir parts/offers/suppliers,
  ajouter archivage, liens d'alternatives symétriques, contrats, paliers et associations contrat–offre ;
  persistance dans les domaines existants. `catalog` garde les pièces/offres, `supplier` les contrats et
  notes fournisseur, `pricing` les cotations/statistiques ; `bom` orchestre coût et remise derrière une
  interface, sans accès SQL aux contrats. Alternative : service de lecture composite dans la couche
  de composition, avec statistiques exposées par pricing ; éviter les imports cycliques. Comparer ces
  frontières et la représentation des liens dans une ADR proposée, puis attendre l'accord humain.
  Les chemins de nouveaux fichiers ci-dessous sont prévisionnels, pas une frontière déjà décidée.
- **D2 — Dates de contrats (features l. 230, 245).** La spec ne précise ni les bornes ni l'horloge
  d'expiration. Proposition à confirmer : dates calendaires inclusives et date métier fournie par le
  contexte de calcul, alignée sur l'horloge simulée en mode direct ; à expiration, remise refusée et
  motif renvoyé. Alternative : borne de fin exclusive ou date civile réelle. Aucun choix implicite.
- **D3 — Statistiques (l. 233, 248, 250).** Calcul attendu : (courant − référence) / référence × 100,
  avec la dernière cotation strictement antérieure à la date d'il y a 12 mois ; pour moins de 12 mois,
  première cotation et période réelle affichée. Restent à approuver : anniversaire au 29 février,
  cas d'un historique long sans cotation antérieure à la borne, précision/arrondi du pourcentage,
  et cohortes de cotations au même instant. Proposition : borne calendaire rabattue au dernier jour
  du mois, absence de référence signalée, départage quoted_at/id existant, affichage half-even à une
  décimale sans arrondir les valeurs de calcul. Min/max sur la fenêtre, référence extérieure séparée.
- **D4 — Seed idempotent sans perte (l. 290–297).** Proposition : identités stables dans un espace de
  données de démonstration et transaction d'insertion/réconciliation, sans purge des tables métier.
  Alternative : base de démonstration dédiée recréable explicitement. En cas de collision ou de données
  d'exemple modifiées, proposition de refus explicite plutôt qu'écrasement. Documenter la stratégie
  et ses éventuels changements de schéma ; aucun UPDATE/DELETE des cotations/snapshots immuables.
- **D5 — Tests UI.** Le ticket demande un test automatique pour chaque critère, mais aucun harness
  navigateur n'est installé. Comparer un outil navigateur déjà disponible dans l'environnement et
  l'ajout explicite d'un runner tel que Playwright en dépendance de développement. Choix et éventuelle
  dépendance soumis à accord avant ajout ; ne pas déclarer les comportements UI vérifiés sur la seule
  base de tests de services. Les commandes exactes seront fixées avant T7.
  Précision utilisateur du 2026-11-04 : application utilisable quel que soit le navigateur.
  L'ADR 0010 révisée prévoit les trois moteurs Chromium/Firefox/WebKit obligatoires, ordinateur et
  mobile, avec versions testées consignées et contrôle Safari réel distinct de l'émulation.
- **Décision confirmée** : notes offre/fournisseur de 0 à 5 et recyclabilité de 0 à 100 %, en décimal.
  Matière recyclée également en pourcentage. Ces bornes inclusives seront testées, sans valeur par défaut
  inventée ni conversion entre les deux échelles.

Une ou plusieurs nouvelles ADR au prochain numéro disponible devront documenter les décisions
retenues. L'agent les laisse Proposé ; seul un humain renseigne Décideurs et Accepté. La présente
planification n'autorise aucune dépendance, modification de migration existante ou ADR acceptée.

Correspondances : D1→ADR 0006, D2→0007, D3→0008, D4→0009, D5→0010,
toutes acceptées par Joe le 2026-11-04. D3 précise le ratio exact pour ne pas introduire l'arrondi implicite de
decimal.Div ; D4 précise les identités logiques et le périmètre initial des cotations.
Les règles supplémentaires (legacy incomplet, archivés, paliers, recherche) sont explicites
dans ces ADR et integration.md. L'acceptation résulte du message humain « accepte toutes les adere
et continue », pas d'une déduction faite à l'invocation précédente.

## Requirements

### Must Have

- **R1 — Pièces complètes.** Créer, lire et modifier par API nom, catégorie, zone, masse, CO2,
  matière recyclée, recyclabilité, coût de fabrication avec devise, délai de production, heures de
  travail et taux horaire avec devise. Conservation numérique exacte en base/Go/JSON ; décimaux sous
  forme de chaînes, y compris CO2 et pourcentages. Validation explicite des champs et échelles.
- **R2 — Offres et recherche.** Une pièce partagée par deux fournisseurs reste une pièce avec deux
  offres. Chaque offre porte fournisseur, devise, bornes mini/maxi, délai, qualité et origine ISO 3166.
  Refuser mini > maxi avec erreur métier explicite ; conserver les devises d'origine. Rechercher par
  nom et filtrer par catégorie, fournisseur et zone ; prix courant lu dans les cotations, jamais saisi
  dans le catalogue ni remplacé par zéro si manquant.
  Décision utilisateur du 2026-11-04 : après une première cotation, refuser tout changement de
  devise ou fournisseur avec conflit explicite ; créer une autre offre et archiver l'ancienne.
- **R3 — Alternatives et archivage.** Déclarer/lire A↔B sans doublons ; suppression d'une pièce ou
  offre utilisée dans une BOM transformée en archivage. Exclusion du catalogue courant, lecture par
  identifiant conservée pour les BOM. Vérifier référence et suppression/archivage dans la même
  transaction ; un lien concurrent de BOM ne doit pas rendre la pièce illisible.
- **R4 — Fournisseurs et contrats.** Liste fournisseurs, notes qualité/confiance, offres et contrats
  datés avec conditions et paliers. Choix explicite si plusieurs contrats couvrent une offre ; quantité
  commandée = quantité par voiture × volume. Palier atteint appliqué à toute la quantité, sans cumul
  automatique de contrats ; contrat expiré indiqué et remise exclue. Référence temporelle explicite.
  L'exemple 1 000 × 10 EUR avec 5 % dès 500 donne 9 500 EUR via le calcul BOM, avec provenance.
- **R5 — Historique et statistiques.** Courant, min/max, variation sur 12 mois et séries datées par
  offre, dans la devise d'origine et au temps simulé ; calcul décimal côté Go. Référence antérieure
  à la borne annuelle, repli à première cotation pour historique récent avec période réelle. Données
  absentes ou incohérentes signalées ; chaque résultat restitue dates, référence et provenance utilisées.
- **R6 — UI fournisseur.** Onglet et page limités aux offres du fournisseur choisi : contrats,
  prix, extrêmes, sparkline annuelle et variation (+10,0 % rouge pour 100→110, baisse verte, stable
  neutre). Panneau détaillé avec courbe et date/prix au survol. Rafraîchissement REST sur signal SSE
  conformément à ADR 0003, sans rechargement ; états chargement, erreur, absence d'historique explicites.
  Couleurs sémantiques --ok/--over et libellés, sans code couleur concurrent.
  Compatibilité avec les navigateurs modernes, sans dépendance à Chrome ; détails utilisables
  au toucher/clic/clavier sur mobile autant qu'au survol sur ordinateur.
- **R7 — Démonstration déterministe.** `make seed` avec graine et date de référence explicites produit
  le même contenu sans doublons à chaque répétition, en préservant les données existantes. Catalogue
  multidevise, plusieurs fournisseurs/offres, alternatives, contrats/paliers et BOM Fiat Multipla
  réaliste jusqu'à environ 30 000 nœuds. Réutiliser le générateur d'historique live-price-feed ; ne pas
  créer un simulateur concurrent. La Multipla s'ouvre dans l'éditeur avec totaux calculés et provenance.
- **R8 — Preuves.** Chaque critère Jira a un test automatique identifiable ; règles monétaires testées
  aux limites, persistance réelle et UI comprise. lint/tests/build/sqlc/migrations et CI verts avant
  clôture. Documenter fichiers/responsabilités, panne prévisible, test associé et contrôles non exécutés.

### Should Have

- **S1** : graphiques et liste fournisseur utilisables au clavier et avec texte alternatif des valeurs.
- **S2** : mesurer le temps et la taille du seed complet, ainsi que le coût des lectures fournisseur ;
  ne pas ajouter de cache transverse sans besoin mesuré et décision humaine.

### Won't Do

- Changer docs/features.md, les ADR acceptées ou migrations déjà commitées.
- Authentification, ERP, signature électronique, conversion d'affichage optionnelle des prix fournisseurs,
  alertes, import CSV, photos et modèles 3D.
- Réimplémenter dans ce ticket le simulateur, la conversion, l'éditeur ou les snapshots de BOM ; leurs
  interfaces/intégrations sont des dépendances et leur absence bloque la livraison complète.
- Ajouter une bibliothèque de graphiques par défaut ; SVG/HTML locaux proposés, avec conversion des
  décimaux en coordonnées visuelles isolée de tout calcul/formatage monétaire.
- Purger une base, modifier des snapshots publiés, commiter, pousser ou ouvrir une PR pendant ce plan.

## Tasks

- [x] T1: Faire valider les décisions et les contrats d'intégration (R1–R8)
  - **Files**: `docs/adr/0006-modele-catalogue-et-contrats.md`,
    `0007-validite-et-application-des-remises.md`, `0008-statistiques-annuelles-des-cotations.md`,
    `0009-chargement-deterministe-des-donnees-de-demo.md`, `0010-tests-navigateur-reproductibles.md`
    dans le même dossier ; cette spec, state.md et integration.md.
  - **Approach**: documenter options D1–D5, modèles/API proposés et responsabilités ; consigner les
    échelles confirmées. Identifier les tickets/owners prix et BOM et leurs livrables, sans reprendre
    leurs branches. Définir les commandes du harness UI choisi. Aucune migration/code avant accord.
  - **Done when**: accord explicite sur les décisions, ADR acceptées par l'humain et contrats
    d'intégration disponibles ; dépendances non livrées encore visibles comme blocages de tâches.

- [x] T2: Protéger le socle avant extension (R1–R4, R8)
  - **Files**: `internal/catalog/stub_test.go`, `internal/bom/service_test.go`,
    `internal/httpapi/router_test.go`, nouveaux tests de compatibilité ciblés si nécessaires.
  - **Approach**: exécuter les tests actuels et compléter uniquement les garanties manquantes :
    Part/Offer par ID, cotation absente → ErrNoQuote, devise et JSON Money exacts, /healthz conservé.
    Ces tests doivent passer avant toute modification des types/interfaces consommés par BOM.
  - **Done when**: tests de caractérisation verts, échecs observables si une garantie est cassée ;
    état initial des contrôles consigné, sans affaiblir de tests existants.

- [x] T3: Étendre la persistance catalogue/fournisseurs approuvée (R1–R4, R7, R8)
  - **Files**: nouvelle `migrations/00002_catalogue_fournisseurs.sql` (numéro à revalider),
    `internal/catalog/queries.sql`, nouveaux `internal/supplier/queries.sql`, `sqlc.yaml`,
    code généré `internal/{catalog,supplier}/*db/`, tests d'intégration associés.
  - **Approach**: appliquer uniquement D1 approuvée : NUMERIC pour attributs/notes/remises,
    monnaies en paires, archivage, liens symétriques, contrats/paliers ; préserver les FK et données
    du socle. Régénérer sqlc, vérifier les anciennes fixtures et l'immuabilité. Ne pas imposer de
    défaut métier arbitraire lors de l'ajout d'attributs aux pièces existantes : stratégie dans D1.
  - **Done when**: up/down/up sur base jetable fonctionne, migration des données minimales existantes
    couverte, contraintes et aller-retour décimal testés, génération stable ; aucune migration ancienne modifiée.

- [x] T4: Livrer le catalogue par API, alternatives et archivage (R1–R3, R8)
  - **Files**: `internal/catalog/catalog.go`, nouveaux `service.go`, `repository.go`, tests associés,
    `internal/httpapi/catalog.go`, `internal/httpapi/router.go`, `cmd/server/main.go`.
  - **Approach**: modèle complet, service/repository dans catalog et pool pgx dans le câblage ;
    endpoints /api/v1 pour pièces/offres/recherche/alternatives selon contrat T1. Erreurs métier
    traduites en JSON explicite ; lecture des archivés conservée. Protéger la course suppression/
    référence BOM en transaction ; les cotations existantes peuvent aussi empêcher une suppression
    physique d'offre : politique conforme aux FK à documenter dans D1, jamais de cascade destructrice.
  - **Done when**: tests HTTP + PostgreSQL prouvent round-trip précis, rejet mini>maxi, devise USD
    conservée, une pièce/deux offres, tous les filtres, symétrie et BOM lisible après archivage.

- [x] T5: Livrer fournisseurs, contrats et calcul de remise isolé (R4, R8)
  - **Files**: nouveaux `internal/supplier/{supplier,service,contract,repository}.go`, tests associés,
    `internal/httpapi/supplier.go`, `internal/httpapi/router.go`, `cmd/server/main.go`.
  - **Approach**: API fournisseurs/contrats, sélection du contrat applicable, paliers en décimal et
    date fournie par l'appelant. Déterminer le palier atteint et expliquer prix de base/quantité/
    remise/contrat, sans effectuer encore le calcul des totaux d'arbre. Appliquer les choix D1/D2.
  - **Done when**: tests table-driven couvrent 499/500/1 000 unités, exact 9 500 EUR, avant/pendant/
    après validité et bornes de dates, plusieurs contrats sans choix et notes hors de [0,5].

- [ ] T6: Exposer l'historique et les statistiques de prix (R5, R8)
  - **Files**: `internal/pricing/pricing.go`, `internal/pricing/queries.sql`, nouveaux
    `history.go`, `statistics.go`, tests associés, code sqlc et handlers selon contrat T1.
  - **Approach**: extension coordonnée avec le propriétaire live-price-feed, requêtes filtrées par
    offre/fournisseur et fenêtre temporelle ; obtenir la cotation antérieure nécessaire en plus de
    la série annuelle. Calculer en Go sans heure implicite ni arrondi intermédiaire ; appliquer D3.
  - **Done when**: tests prouvent 100→110 = +10 %, baisse/stabilité, moins de 12 mois avec période,
    référence hors fenêtre distincte des extrema, absence de prix, fenêtre sans données, 29 février
    et absence d'offres d'un autre fournisseur. Bloqué pour intégration si le contrat prix manque.

- [ ] T7: Construire la page fournisseur et son actualisation (R6, R8, S1)
  - **Files**: `web/src/App.tsx`, nouveaux `web/src/{suppliers,components,api}/` ciblés,
    `web/src/index.css`, tests UI et configuration uniquement après D5 approuvée.
  - **Approach**: onglet liste/détail, contrats, sparkline SVG et panneau détaillé ; texte monétaire
    formaté depuis des chaînes avec Intl.NumberFormat sans passage par Number/parseFloat. Ne pas
    recalculer variation/remise dans le navigateur. Isoler les coordonnées graphiques. S'abonner au
    signal SSE existant et relire REST ; nettoyer la connexion et éviter les réponses périmées lors
    d'un changement de fournisseur. Respecter états d'erreur et lisibilité au clavier.
  - **Done when**: tests UI automatiques du filtrage, +10,0 % rouge, baisse/stabilité, panneau/survol,
    nouvelles cotations sans reload et prix manquant sur Chromium/Firefox/WebKit ; profils mobiles,
    formatage exact et reconnexion SSE couverts. Contrôles Safari réel tracés séparément ; build/lint
    verts. Signal SSE réel nécessaire
    à l'acceptation finale ; un signal simulé en test n'est pas une preuve d'intégration complète.

- [ ] T8: Brancher les remises au calcul BOM (R4, R8)
  - **Files**: `internal/bom/service.go`, `internal/bom/service_test.go`, points d'extension du
    moteur de totaux BOM et de sa sélection de contrat, `cmd/server/main.go` selon T1.
  - **Approach**: intégrer l'interface contrats approuvée au moteur livré par le propriétaire BOM :
    quantité par voiture × volume, remise sur toute la quantité, prix/devise d'origine puis pipeline
    de conversion existant, arrondi de sortie ADR 0004. Restituer contrat, palier, date et cotation.
    Définir le partage de fichiers avec l'équipe avant cette tâche ; pas de moteur parallèle.
  - **Done when**: test d'intégration du résultat BOM 9 500 EUR, changement de volume changeant le
    palier, expiration supprimant la remise et devise conservée ; aucun snapshot publié modifié.
    Tâche bloquée tant que le moteur et la sélection de contrat ne sont pas disponibles.

- [ ] T9: Générer et ouvrir le jeu Fiat Multipla déterministe (R7, R8, S2)
  - **Files**: nouveau `cmd/seed/main.go`, générateur de données dans un emplacement approuvé T1,
    `Makefile`, tests seed/intégration et `docs/workflow.md`.
  - **Approach**: commande avec graine/date/volume de données documentés, attribution stable des
    identités selon D4 ; catalogue et BOM réaliste jusqu'à ~30 000 nœuds, devises, offres et contrats.
    Faire initialiser l'historique par live-price-feed. Exécuter deux fois et comparer les contenus
    triés déterministes, pas seulement les comptes ; protéger les données hors périmètre de démo.
  - **Done when**: test automatique d'idempotence et de même contenu dans deux bases jetables ;
    collision et exécution interrompue contrôlées sans destruction ; test d'ouverture Multipla
    et totaux/provenance via le vrai éditeur. Bloqué par BOM, conversion et initialisation des prix.

- [ ] T10: Vérifier tous les critères et documenter la livraison (R1–R8)
  - **Files**: tests d'intégration transverses et UI, `.github/workflows/ci.yml` si nouveaux checks
    approuvés nécessaires, `docs/workflow.md`, `.specs/nlpf-67/{spec,state}.md`.
  - **Approach**: tracer les critères ci-dessous vers tests exécutés, relancer lint/tests/build,
    intégration prix/SSE/BOM/seed, migrations et sqlc. Livrer responsabilités des fichiers, une panne
    et son test, résultats et limites. Vérifier la CI du futur commit/PR sans pousser/ouvrir de PR
    sans accord ; préserver tous les jobs existants.
  - **Done when**: toutes les preuves présentes, dépendances livrées, CI verte sur la révision livrée ;
    aucune vérification obligatoire NOT RUN assimilée à PASS. Revue humaine requise avant merge.

## Verification

### Automated Checks

Commandes de livraison à exécuter après implémentation ; aucune annoncée PASS pendant ce plan :

- `make lint`
- `make test`
- `go test -race ./...` avec DATABASE_URL pointant vers une base de test migrée, puis contrôle des
  sorties pour s'assurer que les tests PostgreSQL n'ont pas été sautés.
- `npm --prefix web run build` avec la version Node requise par le frontend (CI : 24).
- `go tool sqlc generate` puis comparaison au code généré attendu, sans inclure des modifications
  manuelles de travail dans un verdict « génération instable ».
- Sur base PostgreSQL **jetable exclusivement** : `go tool goose -dir migrations postgres
  "$DATABASE_URL" up`, `reset`, `up` ; jamais reset sur une base de travail ou de snapshots publiés.
- `make seed` deux fois avec mêmes paramètres selon la future interface ; tests automatisés de
  comparaison des données, devises, liens et BOM. Pas d'exécution de seed pendant ce plan.
- Tests UI du harness approuvé D5 : `npm --prefix web run test:e2e`, configuré pour exécuter les
  trois moteurs Chromium/Firefox/WebKit, avec profils ordinateur/mobile (ADR 0010 révisée).
- `node ~/.agents/spec-drum/bin/validate-specs.js --all` pour la cohérence des specs.
- Jobs CI go/web/security/compose et futurs tests ciblés verts pour la révision livrée.

### Manual Checks

- Humain : choisir D1–D5, accepter les ADR proposées, approuver le plan et les interfaces partagées.
- Créer une pièce à plusieurs décimales et deux offres de devises différentes ; vérifier les réponses
  et l'absence de prix saisi dans le catalogue, puis l'archivage avec une BOM qui continue à la lire.
- Vérifier courbe et panneau fournisseur, clavier/survol, actualisation au vrai tick simulé ; comparer
  date de référence, variation et prix aux cotations utilisées.
- Ouvrir la Fiat Multipla seedée dans l'éditeur et vérifier totaux, devises, remises et provenance.
- Vérifier la non-régression /healthz et le démarrage de la stack après migration.
- Vérifier Safari réel sur macOS/iOS et consigner appareil, OS, version et résultat ; NOT RUN si
  indisponible. L'émulation mobile/WebKit seule ne clôt pas ces vérifications. Compléter par les
  navigateurs Chrome/Edge réels disponibles sans remplacer les trois moteurs obligatoires.

### Acceptance Criteria

| Critère | Exigences | Tâches | Preuve automatique attendue |
|---|---|---|---|
| A1 Pièce API relue avec CO2/% exacts | R1, R8 | T2–T4, T10 | HTTP + PostgreSQL, décimaux à forte précision et bornes |
| A2 Mini > maxi refusé explicitement | R2, R8 | T3–T4, T10 | Service, contrainte SQL et message HTTP JSON |
| A3 Une pièce, deux fournisseurs/offres | R2, R8 | T4, T10 | Lecture agrégée API sans doublon de pièce |
| A4 Filtre moteur et autres filtres | R2, R8 | T4, T10 | Table de cas nom/catégorie/fournisseur/zone, sans fuite |
| A5 Alternative réciproque | R3, R8 | T3–T4, T10 | A→B relu B→A, pas de doublon |
| A6 Archivage sans casser la BOM | R3, R8 | T3–T4, T10 | Pièce et offre référencées, course de référence, lecture BOM |
| A7 Remise 9 500 EUR et expiration | R4, R8 | T5, T8, T10 | Cas limites de paliers + résultat du vrai calcul BOM |
| A8 Page fournisseur/variation/sparkline | R5–R6, R8 | T6–T7, T10 | Statistiques exactes + test navigateur + vrai signal SSE |
| A9 Seed répétable, Multipla avec totaux | R7–R8 | T9–T10 | Contenu identique, absence doublons, ouverture éditeur et totaux |
| A10 Qualité, contrats, détails et erreurs | R1–R6, R8 | T3–T8, T10 | Notes, dates, multi-contrats, survol et données manquantes |
| A11 Livraison vérifiée | R8 | T1–T10 | ADR humaines, checks et CI verte sur la révision livrée |

- [ ] A1 à A11 couverts et réellement vérifiés ; aucune dépendance remplacée par un stub en validation finale.
- [ ] USD demeure USD ; pas de calcul monétaire/variation/pourcentage métier en float.
- [ ] Règles nouvelles acceptées par l'humain ; aucune modification d'ADR acceptée ni migration historique.

Traçabilité : R1→T1–T4 ; R2/R3→T1–T4 ; R4→T1–T5/T8 ; R5→T1/T6 ;
R6→T1/T7 ; R7→T1/T3/T9 ; R8→T1–T10 ; S1→T7 ; S2→T9.
T10 vérifie tous les résultats. Aucun Must Have sans tâche ni tâche hors périmètre identifié.

Cas de panne de référence : une offre n'a aucune cotation. Le service expose l'erreur nommée,
la page affiche « prix indisponible », le calcul BOM refuse le total incomplet ; tests services,
HTTP et navigateur doivent échouer si un prix zéro est introduit silencieusement.
