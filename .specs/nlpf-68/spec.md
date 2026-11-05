---
id: nlpf-68
title: Prix et devises — simulateur, cotations, snapshots, conversion et budgets
source: jira
source_ref: https://3istors.atlassian.net/browse/NLPF-68
created: 2026-11-05
status: in-progress
priority: P2
depends_on: [nlpf-70, nlpf-69]
branch: nlpf-68
---

# Prix et devises (NLPF-68)

## Context

Le ticket Jira « Prix & devises (B) », assigné à Joe, couvre live-price-feed,
multi-currency-costing **et target-costing** : ses dix critères incluent les budgets et le volume
finançable. Les prix/taux doivent provenir du simulateur externe, être historisés sans modification,
pouvoir être figés et alimenter des résultats expliqués ainsi qu'une UI actualisée sans rechargement.
Cette livraison fournit aussi le flux et le générateur attendus par NLPF-67, sans reprendre son seed
Multipla ni réimplémenter l'éditeur de NLPF-69.

## Research Findings

### Relevant Files

- `docs/features.md:124–163` : API de prix/change, historique initial, horloge et snapshots ;
  `195–214` : conversion directe/inverse sans pivot ; `254–275` : budgets et volume ;
  `22` : seuil d'avertissement à 90 %, paramétrable ; `180–191` : paramètres figés à la publication.
- `docs/adr/0002-stockage-snapshot-de-prix.md`, `0003-canal-temps-reel.md`,
  `0004-arrondi-montants.md`, `0005-api-de-prix-processus-separe.md` : choix déjà acceptés,
  respectivement stockage pricing, SSE comme signal, half-even en sortie, processus externe dédié.
- `internal/pricing/{pricing,stub,history,statistics}.go` : interface minimale de prix/taux et
  lecture/statistiques annuelles déjà livrées. `History.CurrentAsOf` observe une date persistée ;
  ce n'est pas une horloge simulée. `internal/pricing/doc.go` inclut déjà les budgets dans pricing.
- `internal/pricing/pricingstore/history.go`, `internal/pricing/queries.sql`, `sqlc.yaml` : adapter
  pgx et requêtes typées existants, à étendre sans importer pgx ou HTTP dans le domaine.
- `migrations/00001_init.sql` : prix/taux, en-têtes/liens des snapshots, unicités et triggers
  refusant UPDATE/DELETE/TRUNCATE. Un ajout de lien à un snapshot existant n'est pas encore interdit.
  Les CHECK positifs de prix/taux nécessitent aussi une protection contre NUMERIC non finis.
- `internal/money/money.go` : Money exact en decimal, JSON texte, arrondi de sortie unique ;
  aucun mécanisme actuel ne représente une division périodique exacte.
- `cmd/priceapi/main.go`, `internal/priceapi/router.go` : processus externe existant, `/healthz`
  uniquement. `cmd/server/main.go` câble catalogue/fournisseurs/historique, sans ingestion ni SSE.
- `internal/httpapi/supplier_offers.go`, `web/src/App.tsx`, `web/e2e/` : projection fournisseur et
  écoute du futur `/api/v1/quotes/stream` ; tests actuels avec API et EventSource contrôlés.
- `internal/bom/service.go` : socle local sans arbre/totaux/paramètres budgets. La branche NLPF-69
  inspectée à `ff5cc69` porte les vrais totaux et snapshots ; ses extensions de Pricing ne sont pas
  présentes ici. `.specs/nlpf-67/integration-nlpf-69.md` décrit les incompatibilités.
- `compose.yaml`, `Dockerfile`, `web/nginx.conf`, `Makefile`, `.github/workflows/ci.yml` : stack
  distincte priceapi/back, proxy SSE non tamponné, Go 1.27.3, Node CI 24, PostgreSQL 18 et trois moteurs UI.

### Existing Patterns

- Domaine pur avec interfaces locales ; stockage dans `pricing/pricingstore`, transport dans
  `httpapi`/`priceapi` et adapter HTTP séparé. Câblage et horloge réelle aux frontières seulement.
- Montants/taux exacts, devises ISO prises en charge par money, chaînes JSON/TS, erreurs nommées
  enveloppées puis traduites par les handlers. Quantités et budgets restent en décimal.
- Dernière cotation déterminée par date puis ID ; résultat trié pour éviter l'ordre des maps.
  Préserver l'API, les sentinelles et les stubs utilisés par catalogue/BOM.
- Tests table-driven, httptest, PostgreSQL réel avec transactions annulées et contrôles -race.
  Playwright 1.64.1 est déjà autorisé et installé ; aucune nouvelle bibliothèque de graphique ou de SSE.

### Constraints

**Source et état observés.** Jira consulté le 2026-11-05 : Medium → priorité documentaire P2,
statut To Do, aucun lien de blocage formel. Les trois features fonctionnelles restent P1 dans
features.md. Le plan conserve les dix critères Jira, y compris ceux des budgets.

**Base proposée pour l'exécution.** Créer ultérieurement `nlpf-68` depuis `nlpf-67@4dcdb48`, après
approbation : le catalogue et l'historique nécessaires sont déjà implémentés dans ce code. Ne pas
attendre la clôture de tout NLPF-67 pour commencer NLPF-68, puisque son flux est précisément un
prérequis de cette clôture. `depends_on` cite le socle et la dépendance BOM ; les capacités catalogue
déjà dans la base proposée sont détaillées ici pour éviter un cycle de blocage entre tickets complets.
Aucune branche, aucun commit ou changement applicatif n'est créé pendant cette planification.

| Capacité nécessaire | Disponibilité constatée | Effet sur le plan |
|---|---|---|
| Catalogue/offres et historique de NLPF-67 | Présents dans la base proposée ; ticket global bloqué | Permet T2–T7 sans attendre le seed ni la fin de NLPF-67 |
| Arbre/totaux/snapshots et UI BOM de NLPF-69 | Branche d'Arthur `ff5cc69`, non intégrée | Bloque les raccordements BOM de T6–T9 et l'acceptation globale T10 |
| Contrats/remises de NLPF-67 | Évaluateur disponible ; raccordement BOM encore ouvert | Coût au volume de référence à faire produire par le vrai moteur, pas par un moteur concurrent |
| Seed Fiat Multipla de NLPF-67 | Non livré | Consommer le générateur d'historique de T3/T4 via transaction ; le seed reste dans NLPF-67 |

**Arbitrages humains confirmés le 2026-11-05 :**

1. Budgets cibles global et par sous-système **par voiture** ; budget de production total distinct.
2. Déduire le volume avec un **coût unitaire calculé pour un volume de référence explicite**,
   conservé avec le résultat. Le volume déduit ne remplace pas ce volume de référence : pas de boucle
   remise → volume → remise. Recalculer le coût à ce même volume de référence quand les prix changent.
3. Au redémarrage, reprendre au **dernier jour simulé validé, sans rattrapage pendant l'arrêt**.

Ces choix sont acquis ; leur transcription ne doit pas rouvrir la question. Ils n'acceptent pas
automatiquement les autres décisions techniques ci-dessous.

**Décisions à documenter et soumettre avant code (T1, AGENTS §6–8).** Numéros ADR à revalider à
l'exécution, après les numéros actuels 0001–0010 ; ne pas renuméroter les ADR acceptées des deux branches.

| Décision | Proposition à discuter | Alternative / point restant à arbitrer |
|---|---|---|
| D1 Protocole simulateur et initialisation | Génération pure à partir de graine, version, définition d'offre/paire et jour UTC ; HTTP externe reçoit un contexte explicite, sans dépendance retour au back | Configuration du catalogue répliquée dans priceapi ; figer aussi précision des tirages, évolution des bornes, nouvelles offres et offres archivées encore utilisées |
| D2 Horloge, transactions et panne | État persistant version/graine/paramètres/dernier tick ; batch complet validé puis commit, publication après commit ; reprise sans rattrapage (choix confirmé) | Granularité de transaction et limites de batches ; reprise concurrente, paramètres divergents, sélection explicite du dernier snapshot valide en cas de timeout |
| D3 Fermeture des snapshots | Construction atomique puis interdiction de tout ajout de cotation/lien après création, en plus des refus existants | Mécanisme de fermeture à choisir et migration additive ; ne pas altérer les faits des snapshots déjà publiés ni affaiblir leur protection |
| D4 Inverse exact et frontière BOM | Garder une représentation rationnelle exacte jusqu'à la somme et l'arrondi final, avec la cotation du taux d'origine | Division décimale à précision finie exigeant une nouvelle règle d'arrondi intermédiaire, non autorisée actuellement ; valider le contrat de total avec Arthur |
| D5 Modèle des budgets | Paramètres de brouillon appartenant à BOM ; calculs purs dans pricing ; figer paramètres et contexte à la publication | Tables liées ou colonnes partagées ; validation des bornes, budget zéro, coût nul/manquant, volume déduit zéro ou dépassant la capacité de l'entier BOM |
| D6 Intégration des branches | Résoudre avec Arthur versions de migrations, références d'ADR et interfaces avant raccordement | Attendre une base commune livrée ; ne pas merger/reprendre sa branche sans accord |

La migration 00002 existe avec deux contenus sur NLPF-67 et NLPF-69 ; aucune nouvelle migration
ne prend un numéro présumé libre. La compatibilité doit être testée depuis les historiques existants
sur des bases jetables. L'autorisation antérieure des ADR 0006–0010 de NLPF-67 ne couvre pas D1–D6.

Propositions concrètes de T1 : D1→ADR 0011, D2→0012, D3→0013, D4→0014, D5→0015,
D6→0016, toutes au statut **Proposé**. Le dossier `.specs/nlpf-68/architecture-review.md` les
résume ; `docs/api-prix-et-devises.md` précise les ports et routes futurs. Ces documents ne valent
pas acceptation. L'invocation de spec-execute autorise le plan et sa branche, pas les nouveaux choix.

**Précision et référence temporelle.** L'historique initial contient exactement 365 cotations
quotidiennes par offre et par paire. Proposition T1 : terminer au `as_of` initial explicite, inclure
ce jour et les 364 précédents ; 365 jours n'est pas toujours une année calendaire. Ne pas ajouter une
366e cotation pour contourner la référence strictement antérieure de l'ADR 0008 : période réelle ou
référence annuelle indisponible doivent rester explicites, y compris au 29 février.

Un taux inverse comme 1/3 n'est pas un décimal fini : ne pas utiliser une division à précision globale
implicite, ni arrondir chaque ligne avant la somme. La représentation exacte proposée peut utiliser
la stdlib [math/big.Rat](https://pkg.go.dev/math/big#Rat), sous réserve de D4. Le flux SSE doit suivre
le [standard EventSource](https://html.spec.whatwg.org/multipage/server-sent-events.html), notamment
les événements nommés, le champ id et Last-Event-ID ; la politique de rattrapage applicative est à
définir en T1 car le flux transporte un signal, pas les prix.

## Requirements

### Must Have

- **R1 — Source externe et contrats exacts.** Derrière une interface Go définie par pricing,
  obtenir une offre, les offres d'un fournisseur, toutes les offres, une paire FX et toutes les
  paires. Chaque valeur porte identifiant/paires, montant ou taux textuel, devise, source et date UTC.
  Un adapter HTTP avec timeout valide schéma, identité, devise, bornes, valeurs finies et instant ;
  rejeter/journaliser une donnée aberrante avant persistance. Pas de SDK, réseau ou HTTP dans le domaine.
- **R2 — Simulateur déterministe.** Même graine, version et entrées explicites donnent les mêmes
  valeurs, indépendamment de l'ordre des offres, du découpage des requêtes et d'un redémarrage.
  Prix dans [mini,maxi] ; FX dans la bande décimale paramétrée autour d'une référence explicite.
  Bornes égales, forte précision et montants >2^53 couverts. Aucune utilisation de float pour les tirages
  de valeurs économiques ; ne pas inventer des taux réels ou une devise pivot.
- **R3 — Historique et continuité.** Initialiser exactement 365 cotations quotidiennes par offre
  et paire sans doublon ; rendre le générateur réutilisable par la transaction du seed NLPF-67.
  Persister chaque prix/taux récupéré, garder l'unicité source/entité/date et refuser un conflit de
  contenu au lieu de l'ignorer. Rendre l'initialisation et les ticks reprenables sans écrasement,
  même après interruption ou exécution concurrente ; ne pas régénérer les données existantes.
- **R4 — Horloge et robustesse.** À vitesse par défaut, 40 minutes réelles correspondent à 365
  jours simulés supplémentaires après l'initialisation ; vitesse réglable et dates de cotation simulées. Tester le seuil exact sans attendre
  40 minutes ni accumuler la troncature de 6,6 secondes. Persister la progression et reprendre au
  dernier jour validé sans rattrapage d'arrêt. Un tick ne devient visible/annoncé qu'après commit.
  Timeout → dernier snapshot valide identifiable, avec motif et dates ; absence de repli exploitable
  → erreur nommée. Ne pas présenter un repli ancien comme un nouveau prix live.
- **R5 — Lectures et provenance.** API de dernier prix, prix fournisseur isolés, prix catalogue,
  dernière paire FX et ensemble FX ; historique d'offre **et de paire** borné par période.
  Utiliser un instant de lecture commun, exclure le futur, ordre stable, préserver `History` et
  les statistiques NLPF-67. Une pièce/offre/taux manquant est signalé explicitement, jamais zéro.
- **R6 — Snapshots de prix complets et immuables.** Créer et relire les quatre périmètres offre,
  fournisseur, BOM, catalogue à un instant fixé, avec les prix et les taux directs/inverses nécessaires
  aux devises cibles explicites. Stocker les références aux cotations immuables conformément à ADR 0002.
  Si des prix/taux manquent, lister toutes les références manquantes et ne rien créer partiellement.
  Refuser une utilisation BOM insuffisamment couverte. Une nouvelle cotation ne change pas le snapshot ;
  aucun UPDATE/DELETE/TRUNCATE ni ajout tardif de lien ne doit le modifier. Ne pas modifier les snapshots
  BOM déjà publiés ; brancher les lectures/gel réels attendus par NLPF-69 après accord d'intégration.
- **R7 — Conversion expliquée et exacte.** 100 USD × 0,92 = 92,00 EUR, avec paire/source/date et
  identifiant de cotation ; taux direct prioritaire, sinon inverse de la seule paire opposée, jamais
  de chemin via pivot. Conserver la valeur/devise d'origine. Indiquer calcul direct, inverse ou identité,
  contexte live/snapshot et taux source sans fabriquer une cotation inverse. Refuser un taux absent
  ou invalide ; garder l'inverse exact pendant les calculs et arrondir half-even seulement en sortie,
  en accord avec ADR 0004, y compris dans les totaux BOM et le calcul de volume.
- **R8 — Budgets et volume.** Saisir budget cible global et budgets de sous-systèmes par voiture
  dans le contexte de devise BOM ; afficher l'état vert/orange/rouge avec `--ok/--warn/--over` et du
  texte. Respecter le seuil de proximité paramétrable, 90 % par défaut ; avertir si la somme des budgets
  des sous-systèmes dépasse le global. En mode budget de production, calculer exactement
  `floor(budget_total / coût_unitaire_au_volume_de_référence)` ; 1 000 000 / 15 000 = 66.
  Conserver volume de référence, coût unitaire, cotations, taux, contrats et contexte utilisés avec
  le résultat ; recalculer quand le coût de référence change, sans itération sur le volume déduit.
  Coût manquant/nul, division invalide ou volume non représentable doivent être explicites ; aucune
  saturation silencieuse à 1 et aucune modification automatique d'un snapshot publié.
- **R9 — Actualisation réelle et compatibilité.** `GET /api/v1/quotes/stream` publie après commit
  un numéro de tick et l'instant simulé, sans transporter les prix ; l'UI relit REST. Reconnexion,
  Last-Event-ID, client lent, déconnexion et arrêt du serveur n'entraînent ni blocage de l'ingestion,
  ni fuite de goroutine, ni donnée plus ancienne écrasant une réponse récente. Page fournisseur puis
  totaux/budgets d'un brouillon live se mettent à jour sans navigation ; vues figées restent stables.
  Tester les trois moteurs sur ordinateur/mobile et un parcours avec vraie ingestion/REST/SSE.
- **R10 — Preuves et livraison.** Dix points Jira reliés à des tests exécutés. Vérifier domaines,
  adapter réseau, PostgreSQL, migrations/sqlc, API, navigateur et stack ; tests de concurrence et cas
  de panne inclus. lint/tests/build/sécurité et CI doivent être verts avant clôture. Livrer les
  responsabilités des fichiers, la panne de référence, son test et les contrôles non exécutés.

### Should Have

- **S1** : mesurer taille/durée de l'historique initial et coût d'un tick sur le catalogue correspondant
  à une BOM de ~30 000 nœuds, en distinguant offres uniques et nœuds ; mesurer recalcul BOM <1 s.
  Ajouter batch/COPY et index seulement pour un besoin mesuré, sans cache transverse anticipé.
- **S2** : configuration et logs structurés rendent visibles graine/version, dernier tick validé,
  nombre de cotations, cause de rejet et repli, sans fuite de configuration sensible.

### Won't Do

- Vraie API fournisseur/change, prévision de prix/taux, authentification, services supplémentaires.
- Conversion par pivot, float économique, arrondi intermédiaire implicite, choix automatique de
  contrat ou optimisation du volume pour rechercher le meilleur palier/remplir exactement le budget.
- Réimplémenter le catalogue, les statistiques annuelles, le seed Multipla ou un éditeur/moteur BOM
  parallèle. Les intégrations avec les services existants sont incluses ; leur duplication ne l'est pas.
- Proposition automatique de répartition des budgets, import/export, alertes automatiques de prix.
- Modifier `docs/features.md`, une migration commitée ou une ADR acceptée ; purger une base,
  écraser des cotations ou toucher aux faits de snapshots publiés.
- Ajouter une dépendance sans accord ; pousser, merger, ouvrir une PR ou reprendre la branche
  d'Arthur sans les accords requis. Cette phase produit seulement les documents de planification.

## Tasks

**Séquençage précisé après acceptation de l’ADR 0016 (2026-11-05).** T6–T9 livrent et
vérifient les capacités pricing autonomes. Les raccordements au vrai moteur/éditeur BOM et aux
publications décrits dans ces tâches restent obligatoires ; leur exécution et leur preuve sont
regroupées en T10 après disponibilité d’une base coordonnée. Aucun critère R/J/C n’est supprimé,
aucun test à interface contrôlée ne vaut preuve de ces raccordements. T8 ne crée pas de second
moteur BOM ; seuls ses évaluateurs purs peuvent précéder l’intégration. Un achèvement de T6–T9
ne signifie donc pas que J4/J8/J9 ni le ticket complet sont acceptés.

- [x] T1: Faire valider les contrats et décisions restant ouverts (R1–R10)
  - **Files**: cette spec, state.md et architecture-review.md ;
    `docs/adr/0011-protocole-du-simulateur-deterministe.md`,
    `0012-horloge-ingestion-et-repli-des-prix.md`,
    `0013-fermeture-transactionnelle-des-snapshots-de-prix.md`,
    `0014-conversion-exacte-par-taux-direct-ou-inverse.md`,
    `0015-budgets-cibles-et-volume-de-reference.md`,
    `0016-integration-progressive-des-prix-et-de-la-bom.md` dans docs/adr ;
    `docs/api-prix-et-devises.md` pour les contrats proposés puis convenus.
  - **Approach**: consigner les trois choix déjà confirmés sans les rouvrir ; présenter D1–D6
    avec alternatives et faire accepter les nouveaux choix avant code. Finaliser les cinq requêtes
    de source, leurs paramètres/version et les routes REST additives ; état persistant, finitude,
    fermeture de snapshot, repli, inverse exact et modèle budgets. Coordonner avec Arthur les interfaces
    FreezePrices/SnapshotOfferPrice/SnapshotRate et leurs garanties de date/couverture. Valider une
    base d'intégration sans collision de versions de migration et sans altérer les ADR acceptées.
  - **Done when**: contrats et nouvelles ADR validés explicitement, choix humains attribués ;
    capacités et fichiers partagés convenus. Un raccordement indisponible reste bloqué, pas remplacé
    par une seconde implémentation. Aucun choix technique nouveau déduit de l'accord des ADR NLPF-67.

- [x] T2: Protéger les interfaces et comportements existants (R1, R5–R7, R10)
  - **Files**: `internal/pricing/{stub,statistics}_test.go`,
    `internal/pricing/immutability_integration_test.go`, `internal/money/money_test.go`,
    `internal/bom/service_test.go`, tests `internal/{priceapi,httpapi}/`, `web/e2e/suppliers.spec.ts`.
  - **Approach**: exécuter la baseline ; compléter les seules protections nécessaires avant de
    modifier Pricing/History/Money : précision >2^53, provenance, ordre temporel, erreur sans cotation,
    sémantique de CurrentAsOf observée et santé des deux serveurs. Préserver les tests de NLPF-67.
  - **Done when**: baseline et nouveaux tests de caractérisation verts avec PostgreSQL réel ;
    contrôles sautés/faits antérieurs distingués. Un changement de devise, date ou précision les met en défaut.

- [x] T3: Livrer le simulateur pur derrière l'API externe (R1–R2, R10, S2)
  - **Files**: nouveaux `internal/pricing/source.go`,
    `internal/priceapi/{quotes,simulation}.go`, `internal/pricesim/` (adapter pur partagé),
    `internal/pricing/priceclient/`, tests associés, `cmd/priceapi/main.go`.
  - **Approach**: interface métier dans pricing, générateur sans réseau/horloge/DB, adapter HTTP
    typé et cinq requêtes du contrat T1 dans priceapi. Identités/graine/version/jour explicites,
    échantillonnage économique entier/décimal et validation stricte côté serveur et client.
    Réutiliser le même générateur pour historique, ticks et seed ; définir un ordre stable qui ne
    change pas les valeurs d'une offre quand une autre est ajoutée ou qu'un batch est découpé.
  - **Done when**: tests comparant mêmes/différentes graines, requêtes groupées/séparées, ordre
    permuté et redémarrage ; prix/FX dans leurs bornes, borne égale, grande précision. httptest
    prouve les cinq requêtes et rejette timeout, JSON invalide, mauvais ID/devise, NaN/Infinity,
    date et prix/taux aberrants ; les deux /healthz restent compatibles.

- [x] T4: Persister l'initialisation et les ticks avec reprise (R2–R4, R10, S1–S2)
  - **Files**: nouvelle `migrations/NNNNN_pricing_runtime.sql` au numéro validé,
    `internal/pricing/{runtime,ingestion}.go`, `internal/pricing/queries.sql`,
    `internal/pricing/pricingstore/`, sqlc généré, tests d'intégration, `cmd/server/main.go`.
  - **Approach**: métadonnées approuvées, contraintes de finitude, verrou d'initialisation et
    comparaison de contenu en cas de clé déjà présente. Contrat d'insertion dans une transaction
    fourni au seed. Générer 365 jours explicites par offre/paire puis avancer par ticks complets ;
    n'exposer qu'un état validé. Horloge réelle injectée, progression entière sans dérive, curseur
    persistant, pas de rattrapage d'arrêt. Récupération réseau hors transaction longue ; valider le
    contexte catalogue avant insertion. Ne pas corriger silencieusement de vieilles données invalides.
  - **Done when**: premier lancement = 365 lignes/entité ; deux lancements et deux processus
    concurrents donnent le même contenu, sans doublon. Interruption/reprise retrouve la suite exacte ;
    conflit de paramètres/contenu expliqué et rollback. Faux temps à 40 min = 365 jours, tests avant
    et après la borne, autre vitesse ; panne avant commit = aucune progression annoncée. up/down/up
    sur base jetable et génération sqlc stable, aucune migration historique modifiée.

- [x] T5: Exposer les cotations, historiques et le repli expliqué (R1, R4–R5, R10)
  - **Files**: nouveaux `internal/pricing/{service,history_fx}.go`,
    extension `pricingstore/`, `queries.sql`, nouveau `internal/httpapi/pricing.go`,
    `internal/httpapi/{router,supplier_offers}.go`, `cmd/server/main.go`, tests associés.
  - **Approach**: implémentation réelle de Pricing et lectures groupées à un contexte temporel
    cohérent ; prix offre/fournisseur/catalogue, paire/toutes les paires et historiques par période.
    Conserver History et ses statistiques ; raccorder distinctement l'horloge validée aux compositions
    qui en ont besoin sans transformer silencieusement une date observée en horloge. Appliquer le
    contrat de repli T1 avec snapshot/source/date/motif explicites ; erreurs d'absence par pièce.
  - **Done when**: HTTP + PostgreSQL vérifient uniquement les offres du fournisseur, chaque devise
    d'origine, dernière cotation <= as_of, bornes d'historique offre/FX, source/date/ID et aucune
    valeur zéro fabriquée. Test d'API indisponible avec/sans snapshot valide, nouvel état jamais
    étiqueté live à tort ; non-régression des statistiques et de /healthz.

- [x] T6: Construire et consommer des snapshots de prix complets (R4, R6, R10)
  - **Files**: nouveaux `internal/pricing/{snapshot,coverage}.go`, extension `queries.sql` et
    `pricingstore/`, nouvelle migration de fermeture si nécessaire et approuvée,
    `internal/httpapi/pricing_snapshots.go`, tests PostgreSQL et tests avec services BOM/snapshot.
  - **Approach**: résolution des périmètres, déduplication des offres et devises cibles, sélection
    cohérente au même instant puis insertion atomique de l'en-tête et des liens. Capturer la paire
    réellement utilisée, même en sens inverse, pas une cotation fabriquée. Vérifier couverture avant
    attachement/publication BOM. Protection service + DB contre toute modification et ajout tardif,
    y compris course d'ajout pendant fermeture. Adapter les méthodes déjà attendues par NLPF-69
    seulement après le contrat/partage validé ; ne pas copier sa démo comme source réelle.
  - **Done when**: chaque périmètre testé, création concurrente cohérente, défauts prix/FX tous
    listés sans snapshot partiel ; après nouvelle cotation, ancien prix/taux toujours relus.
    Mutations et ajout tardif refusés, ancien snapshot publié conservé. Test du vrai brouillon
    figé insuffisamment couvert ; cette preuve est bloquée si NLPF-69 n'est pas disponible.

- [x] T7: Intégrer la conversion directe et inverse exacte (R7, R10)
  - **Files**: nouveau `internal/pricing/conversion.go`, extension ciblée `internal/money/`
    selon D4, tests métier, points de conversion dans `internal/bom/{service,totals}.go`
    après intégration autorisée, sérialisation HTTP de provenance.
  - **Approach**: une stratégie de conversion choisie en un point, direct prioritaire puis
    inverse, identité explicite pour même devise ; jamais pivot. Maintenir l'exactitude rationnelle
    jusqu'à l'agrégation et l'arrondi de sortie. Garder cotation originale, paire, sens, source/date,
    contexte live/snapshot ; pas de taux réciproque arrondi inventé. Remplacer le chemin de conversion
    du vrai moteur, sans un calcul concurrent dans l'UI ni mutation de snapshot publié.
  - **Done when**: 100 USD à 0,92 → 92,00 EUR ; EUR→USD par inverse et préférence direct
    vérifiées. Sans USD→JPY ni inverse, le seul chemin via EUR échoue explicitement. Cas inverse
    périodique agrégé (trois conversions de 1 EUR avec USD→EUR=0,3 → total 10 USD), JPY/KWD,
    half-even et bornes >2^53 testés ; résultats figés ne changent pas avec le live. Intégration BOM
    bloquée tant que son vrai moteur et le contrat exact ne sont pas livrés.

- [x] T8: Livrer les évaluateurs de budgets et de volume pour la vraie BOM (R7–R8, R10)
  - **Files**: nouveaux `internal/pricing/{budget,production_volume}.go`, tests associés ;
    extensions `internal/bom/`, `internal/snapshot/`, leurs stores/requêtes/API après accord,
    nouvelle migration budgets au numéro validé et code sqlc correspondant.
  - **Approach**: calculs purs à partir de coûts expliqués fournis par le moteur BOM, paramètres
    possédés par le brouillon et figés à la publication. Budget par voiture global/sous-systèmes,
    comparaison exacte et avertissement de répartition. Mode production distinct du volume manuel :
    demander un volume de référence explicite, produire le coût unitaire à ce volume avec ses contrats,
    puis prendre floor de la division exacte. Restituer ces entrées avec le résultat ; ne pas réinjecter
    le volume déduit en référence. Traiter notamment résultat zéro face au CHECK actuel volume >0,
    coût nul/manquant et dépassement d'entier selon D5, sans clamp ni division arrondie avant floor.
  - **Done when**: tests des états juste sous/à/au-dessus des seuils, seuil paramétré, somme budgets
    au-dessus du global, devise cohérente et 1 000 000/15 000=66. Prix changé → nouveau volume et
    contexte ; remises évaluées au volume de référence conservé, sans boucle de recalcul. Budget
    inférieur au coût, coût indisponible, zéro, grand entier explicitement couverts. Modification d'un
    brouillon n'altère aucun budget/résultat publié ; intégration bloquée si BOM/snapshots indisponibles.

- [x] T9: Raccorder SSE et les écrans au flux réel (R5–R9, R10)
  - **Files**: nouveau adapter signal/flux `internal/httpapi/pricing_stream.go`,
    interface de notification dans pricing et câblage `cmd/server/main.go`, `web/src/App.tsx`,
    composants budgets/BOM existants ou ciblés après intégration, nouveaux tests `web/e2e/`.
  - **Approach**: signal après commit avec id de tick et as_of, files clients bornées,
    déconnexion/reconnexion conforme au contrat, annulation à l'arrêt et aucune dépendance HTTP dans
    pricing. Réutiliser EventSource, proxy Vite/nginx et REST. Brancher les budgets et le contexte
    de prix sur le vrai écran BOM ; maintenir la sélection de fournisseur et le détail courant,
    afficher repli/erreur/valeurs manquantes. Montants toujours formatés depuis des chaînes.
  - **Done when**: test d'un vrai tick inséré → SSE → REST → prix/variation et budgets mis à jour
    sans navigation sur Chromium/Firefox/WebKit, ordinateur/mobile. Client lent n'arrête pas les
    autres ; reconnexion récupère l'état validé et Last-Event-ID est traité sans faux événement.
    Snapshot figé stable ; erreurs d'ingestion n'émettent pas de succès ; zéro fuite/courses vérifié.

- [ ] T10: Vérifier les dix critères Jira et documenter la livraison (R1–R10, S1–S2)
  - **Files**: tests transverses Go/PostgreSQL/API/navigateur, `.github/workflows/ci.yml`
    si nécessaire, `compose.yaml`/Makefile pour configuration convenue, README,
    `docs/{workflow,api-prix-et-devises}.md`, `.specs/nlpf-68/{spec,state}.md`.
  - **Approach**: matrice ci-dessous avec preuves, tests de panne/reprise et mesures du catalogue
    volumineux. Configurer les valeurs de référence FX, graine/as_of/version, vitesse et timeout
    explicitement ; aucun taux réel implicitement téléchargé. Vérifier stack séparée et transfert
    des cotations au back, préserver les jobs existants. Livrer responsabilités/panne/test/limites.
    Réaliser tous les raccordements BOM différés de T6–T9 (couverture/gel, conversion agrégée,
    persistance et publication des budgets, API/éditeur et SSE), avec leurs tests réels conservés.
    La CI distante n'est déclarée verte que sur la révision effectivement contrôlée ; aucun push/PR
    sans accord. Ne pas clore si une dépendance/validation obligatoire manque.
  - **Done when**: chaque point Jira a une preuve réellement exécutée, checks et CI verts,
    aucune intégration remplacée par un stub en acceptation. Les ADR requises sont acceptées,
    l'application conserve la précision et les snapshots publiés, rapport compréhensible fourni.

## Verification

### Automated Checks

Commandes **à exécuter pendant l'implémentation**, pas des résultats de cette planification :

- `make lint`, `make test`, `make sec` avec les versions Go/Node requises par go.mod et le lockfile.
- `CI=true DATABASE_URL=<base_jetable_migrée> go test -race -count=1 ./...` : contrôler que tous les
  tests PostgreSQL requis se sont exécutés, pas seulement un code retour vert avec skips.
- `npm --prefix web ci`, `npm --prefix web run build`, `npm --prefix web run test:e2e` : réutiliser
  les six projets du harness, compléter par le mode de test intégrant vraie API/ingestion/SSE.
- `go mod tidy -diff` ; `go tool sqlc generate`, puis comparaison aux fichiers attendus.
- goose `up`, `reset`, `up` **uniquement dans des bases jetables dédiées** ; tester aussi les
  historiques de migration approuvés par T1 sans reset de base de développement.
- Tests temporels avec horloge contrôlée à 0, juste avant/après 40 min et au seuil 365 jours,
  autre vitesse, redémarrage sans rattrapage et reprise après échec de commit.
- Initialisation deux fois et concurrente, comparaison des contenus ordonnés par identités
  logiques dans deux bases neuves, interruption et conflit de paramètres/contenu ; aucune purge.
- HTTP httptest : délai dépassé, réponse manquante/malformée, monnaie/date incohérente, bornes,
  repli snapshot et toutes les offres/paire de chaque périmètre.
- Tests DB d'immuabilité, fermeture et courses ; test inverse périodique/somme avant arrondi ;
  tests budget/volume et API de la vraie BOM, avec contrats/prix/taux de provenance.
- Smoke Docker Compose dans un projet et des ports dédiés : services sains, migrations réussies,
  routes via nginx, vrai signal SSE reçu, redémarrage sans nouveau bootstrap. Couper priceapi et
  vérifier repli/diagnostic puis reprise, sans toucher aux autres conteneurs.
- Mesures de charge S1 et de fuite/goroutine après connexions SSE répétées ; borner les données
  et le temps du test plutôt que maintenir une connexion sur 40 minutes réelles.
- `node ~/.agents/spec-drum/bin/validate-specs.js --all` ; distinguer les erreurs préexistantes
  de nlpf-70/docker-compose-stack de toute erreur de cette nouvelle paire.

### Manual Checks

- Humain : approuver le plan/base proposée et les nouvelles décisions T1 ; les trois réponses
  du 2026-11-05 sont conservées. Accord d'Arthur avant reprise/intégration de ses travaux.
- Ouvrir deux fournisseurs ; prix d'origine/source/date visibles, aucune fuite d'offres. Laisser
  avancer un tick et vérifier courbe/prix/variation sans rechargement ; couper/reprendre le flux.
- Dans un vrai brouillon BOM, choisir direct puis snapshot ; nouvelle cotation → seul le direct
  change. Publier puis vérifier que taux/prix et réglages/budgets publiés restent identiques.
- Configurer budget voiture et sous-système, dépasser une cible puis le global de répartition ;
  vérifier couleur et texte. Budget production 1 000 000, coût référence 15 000 → 66 ; détail
  montrant volume de référence, contrats, cotations et taux utilisés. Changer le prix puis vérifier
  le recalcul sans remplacement implicite du volume de référence.
- Redémarrer back/priceapi : reprise du dernier tick validé, aucun rattrapage du temps d'arrêt,
  365 lignes initiales toujours uniques ; données antérieures/snapshots conservés.
- Safari réel macOS/iOS séparé des tests WebKit, autres navigateurs réels disponibles ; versions
  et résultat consignés, NOT RUN si indisponibles. Pas de promesse pour un navigateur ancien non testé.

### Acceptance Criteria

| Critère Jira / complément fonctionnel | Exigences | Tâches | Preuve automatique attendue |
|---|---|---|---|
| J1 Même graine, valeurs dans leurs bornes | R1–R2 | T1–T3/T4/T10 | Générateur pur, ordre/batch/redémarrage, bornes prix et FX |
| J2 Cotation non modifiable | R3/R6 | T2/T4/T6/T10 | SQLSTATE de refus UPDATE/DELETE/TRUNCATE prix/FX, exactitude NUMERIC |
| J3 Endpoints offre/fournisseur/toutes et FX, devise/date | R1/R5 | T3/T5/T10 | HTTP + DB, cinq requêtes, filtrage fournisseur et provenance |
| J4 Snapshot garde ancien prix et reste immuable | R6 | T6/T9/T10 | Nouveau prix/taux sans effet, mutations/ajouts tardifs refusés, vrai brouillon figé |
| J5 92 EUR, inverse et absence de pivot | R7 | T7/T10 | Exact 100×0,92, inverse périodique avant somme, USD→JPY impossible via EUR |
| J6 365 cotations, pas de doublon au redémarrage | R3–R4 | T4/T10 | Deux démarrages, interruption, concurrence et comparaison du contenu |
| J7 40 minutes = 365 jours, vitesse réglable | R4 | T4/T10 | Faux temps aux bornes, autre vitesse et reprise sans rattrapage |
| J8 Nouveau prix UI sans rechargement | R9 | T5/T9/T10 | Vraie ingestion/DB/SSE/REST/navigateur, aucune navigation, reconnexion |
| J9 Dépassement sous-système rouge, avertissement budgets | R8–R9 | T8–T10 | Service/API et UI BOM, frontières du seuil et somme des budgets |
| J10 Budget production donne 66 voitures | R7–R8 | T7–T10 | Division entière exacte et contexte volume de référence conservé |
| C1 Historique de paire et d'offre, signalement des absences | R3/R5 | T4–T5/T10 | Périodes/as_of, aucun futur, donnée absente non remplacée par zéro |
| C2 Tous périmètres et couverture complète du snapshot | R6 | T6/T10 | Offre/fournisseur/BOM/catalogue, manques listés, rollback total |
| C3 Panne et repli explicable | R1/R4–R5 | T3–T5/T10 | Timeout, aberration rejetée, dernier snapshot valide et erreur sans repli |
| C4 Définition de terminé et compatibilité | R10 | T1–T10 | ADR acceptées, checks/CI verts, trois moteurs, rapport des limites |

- [ ] J1–J10 et C1–C4 réellement couverts ; pas de stub en acceptation de l'intégration finale.
- [ ] Aucun montant, taux, calcul de budget ou de volume en float ; provenance conservée.
- [ ] Aucune mutation de cotation/snapshot publié, aucune migration/ADR historique réécrite.
- [ ] Décisions T1 acceptées et dépendances réelles disponibles avant statut review/done.

Traçabilité : R1→T1–T3/T5 ; R2→T1/T3/T4 ; R3/R4→T1/T4/T5/T6 ; R5→T2/T5/T9 ;
R6→T1/T2/T6 ; R7→T1/T2/T7/T8 ; R8→T1/T8/T9 ; R9→T1/T9 ; R10→T1–T10 ;
S1→T4/T10 ; S2→T3/T4/T10. Aucun Must Have sans tâche ni tâche hors des limites Won't Do.

Panne de référence : l'API externe renvoie à un tick un prix dans la mauvaise devise puis devient
injoignable. Le lot invalide n'est pas enregistré/annoncé, l'horloge ne valide pas ce tick, le dernier
snapshot valide reste lisible avec ses vraies dates et le motif de repli ; sans snapshot exploitable,
le coût reste indisponible. Tests adapter, transaction et navigateur doivent détecter toute réussite
silencieuse, prix zéro ou perte de provenance.
