---
id: nlpf-67
status: blocked
branch: nlpf-67
current_task: T7
started: 2026-11-04
updated: 2026-11-05
---

# State: Catalogue et fournisseurs (NLPF-67)

## Progress

Joe a explicitement accepté les cinq ADR 0006–0010 et demandé de poursuivre. T1 terminée pour les
décisions/contrats documentés ; leurs dépendances techniques non livrées restent visibles.
T2 terminée : tests de caractérisation et baseline unitaires verts avant extension.
T3 terminée : migration, génération sqlc stable et tests PostgreSQL réels verts.
T4 terminée : services purs catalogue, stockage pgx isolé et API avec tests d'intégration verts.
T5 terminée : fournisseurs/contrats par API et calcul de remise isolé vérifiés.
T6 terminée : historique, statistiques et API fournisseur vérifiés.
T7 partiellement livrée : interface, formulaires et 54 tests navigateur verts sur trois moteurs ;
acceptation bloquée par le signal SSE réel. T8–T10 restent ouvertes. Rapport : delivery.md ;
conflits interbranches et conditions de reprise : integration-nlpf-69.md.

## Task Log

### Planification — 2026-11-04
- Source : NLPF-67, lecture Jira en consultation uniquement ; aucune modification distante.
- Inspection : socle catalogue/pricing/BOM, schéma, ADR acceptées, serveur, frontend, tests et CI.
- Confirmation humaine : périmètre complet, dépendances BOM/prix bloquantes ; notes 0–5 et
  recyclabilité 0–100 % en décimal.
- Contrôle de la nouvelle paire spec/state : PASS, deux fichiers validés ensemble dans une copie
  temporaire isolée avec le validateur global ; fichiers de travail conservés sans changement.

### T1 : Propositions d'architecture et contrats d'intégration — 2026-11-04
- **Status**: in-progress — livrables préparés ; acceptation humaine en attente, tâche non cochée.
- **Branch**: nlpf-67, créée depuis main (532b627) sans perte des documents de planification.
- **Files**: ADR 0006–0010 (modèle, remises, statistiques, seed, tests UI), integration.md,
  spec.md et state.md ; conventions .specs/AGENTS.md issues de la planification incluses.
- **Notes**: consultation Jira en lecture seule identifiant NLPF-68 (Joe) et NLPF-69 (Arthur Presle).
  Deux relectures indépendantes sans changement de code ; précision ajoutée sur les verrous
  d'archivage (FOR SHARE plutôt que FK seule) et la division exacte des variations.
- **Checks**: structure des cinq ADR contrôlée ; nouvelle paire de specs validée ensemble dans
  une copie temporaire. Contrôle global toujours en échec sur les deux anciens statuts incohérents.
- **Failure case**: ajout BOM concurrent d'un archivage ; protocole partagé de verrouillage et
  test PostgreSQL concurrent prévus T4. Aucun test de ce comportement nouveau encore implémenté.
- **Not done**: acceptation des ADR, confirmation des contrats d'équipe, T2–T10, dépendances,
  migrations et code applicatif. Aucun push ni changement distant.

### T1 : Préparation consignée — 2026-11-04
- **Status**: in-progress, en attente de décision humaine sur les propositions.
- **Commit**: 0bb6331 — `nlpf-67: T1 - proposer les decisions et contrats integration`.
- **Notes**: commit documentaire uniquement, qui inclut le plan initial non commité au début de
  l'exécution. T1 n'est pas terminée et sa case reste vide ; aucune acceptation d'ADR automatisée.
- **Checks**: `git diff --cached --check` avant commit : PASS ; nouvelle paire spec/state : PASS.

### T1 : Compatibilité entre navigateurs précisée — 2026-11-04
- **Request**: l'utilisateur veut que l'application soit utilisable quel que soit le navigateur.
- **Changes**: ADR 0010 révisée : Chromium/Firefox/WebKit obligatoires, profils ordinateur/mobile,
  standards web sans dépendance Chrome, versions testées consignées, contrôle Safari réel séparé.
  Spec et contrats d'intégration alignés ; accès aux détails sans survol obligatoire.
- **Status**: T1 toujours en cours ; ADR toujours Proposé, dépendance non installée et code inchangé.
- **Failure case**: formatage monétaire ou reconnexion SSE valide dans un moteur mais défaillant
  dans un autre ; mêmes scénarios prévus sur les trois moteurs. Tests navigateur non exécutés.
- **Checks**: diff sans erreur de whitespace ; copie isolée de spec/state validée ensemble,
  `Spec Drum: 2 file(s) OK`. Aucun test applicatif ajouté ou exécuté pour cette révision documentaire.

### T1 : Décisions acceptées — 2026-11-04
- **Status**: completed pour le cadrage ; les dépendances prix/BOM ne sont pas déclarées livrées.
- **Commit**: 2822c9a — `nlpf-67: T1 - consigner les decisions acceptees par Joe`.
- **Decision**: accord explicite « accepte toutes les adere et continue stp » enregistré dans les
  cinq ADR à la demande de l'humain. Les contrats d'intégration restent la cible des tâches concernées.
- **Next**: T2, tests de caractérisation et baseline avant modification du comportement.

### T2 : Protection du socle — 2026-11-04
- **Status**: completed.
- **Commit**: 39b63b8 — `nlpf-67: T2 - proteger precision et provenance du socle`.
- **Files**: catalog/stub_test.go, bom/service_test.go, money/money_test.go,
  httpapi/router_test.go : conservation devise/précision/provenance et contrat de santé.
- **Failure case**: conversion float d'un montant >2^53, arrondi prématuré de JPY fractionnaire,
  perte de source/date d'une cotation ou Content-Type de santé changé : tests en défaut.
- **Checks**: `make test DATABASE_URL=` PASS avec Go 1.27.2 et gofmt ; intégration DB volontairement
  désactivée pour cette baseline unitaire. `npm ci`, lint/build web PASS avec Node 22.23.4.
  `make lint` Go en cours ; PostgreSQL 18 jetable démarré pour les validations de T3.
- **Environment**: Go 1.27.2 téléchargé depuis source officielle sous /tmp et SHA256 vérifié,
  aucun changement go.mod ; caches /tmp isolés. Aucun Go/Node global installé ou configuration modifiée.

### T3 : Persistance du catalogue et contrats — 2026-11-04
- **Status**: completed.
- **Commit**: 2fb6042 — `nlpf-67: T3 - etendre catalogue et contrats en base`.
- **Files**: nouvelle migration 00002, queries catalog/supplier, sqlc.yaml et code généré ;
  migrations/catalogue_integration_test.go couvre les contraintes et la conservation des références.
- **Checks**: up/down/up sur PostgreSQL 18 jetable PASS ; pièces/offres insérées avant 00002
  gardent ID, attributs d'origine et bornes USD exactes, nouveaux attributs NULL. Génération sqlc
  stable contrôlée par SHA256 ; make lint et make test avec CI=true et vraie DATABASE_URL PASS.
- **Failure case**: mauvais fournisseur lié au contrat, décimal non fini, arrondi d'un NUMERIC
  >2^53, référence effacée : tests PostgreSQL bloquants, dont 39 cas NaN/±Infinity.
- **Not done**: CI distante non lancée, aucune migration sur base utilisateur, services/API en T4.

### T4 : Catalogue par API — 2026-11-04
- **Status**: completed.
- **Commit**: 4e98c26 — `nlpf-67: T4 - livrer le catalogue par API`.
- **Files**: types/service/validation catalog, adapter catalogstore et requête OfferHasQuotes,
  handlers httpapi/catalog, câblage cmd/server, tests métier/PostgreSQL/HTTP.
- **Checks**: make lint et make test avec CI=true sur PostgreSQL18 jetable PASS ; tests -race
  catalog/bom PASS, y compris absence de dépendance pgx/HTTP dans le domaine.
- **Failure case**: devise/fournisseur modifié après cotation → 409 ; prix/CO2 >2^53 exacts ;
  archivage conserve lecture BOM. Verrou d'ajout FOR SHARE testé contre UPDATE via NOWAIT.
- **Not done**: UI fournisseur, contrats/remises, statistiques, intégration prix/BOM et CI distante.
- **Resume**: reprise après interruption de quota ; arbre Git propre et commit T4 présents.

### T5 : Fournisseurs, contrats et remises — 2026-11-05
- **Status**: completed.
- **Commit**: a1811fc — `nlpf-67: T5 - livrer fournisseurs contrats et remises`.
- **Files**: domaine supplier et adapter supplierstore, requêtes sqlc/lectures composites atomiques,
  handlers fournisseurs/contrats et câblage ; tests métier, PostgreSQL et HTTP.
- **Checks**: make lint et make test avec CI=true/PostgreSQL18 PASS ; -race supplier/store PASS.
  L'évaluateur a 49 cas, dont 1 000 × 10 EUR à 5 % = 9 500 EUR, seuils, dates, sélection et précision.
- **Failure case**: une modification concurrente ne doit pas mélanger anciennes dates/nouveaux paliers ;
  les lectures contrat sont atomiques par statement. Nouvelles associations vers archives refusées ;
  anciennes conservées et PATCH informatif possible. Expiration datée visible par API.
- **Not done**: application aux vrais totaux BOM (T8/NLPF-69), historique/UI (T6/T7), CI distante.

### T6 : Historique et statistiques — 2026-11-05
- **Status**: completed pour lecture/calcul ; le flux entrant et son signal restent NLPF-68.
- **Commit**: 881c14f — `nlpf-67: T6 - exposer historique et statistiques fournisseurs`.
- **Files**: pricing history/statistics, adapter pricingstore et sqlc, projection HTTP fournisseur,
  câblage server et tests Go/PostgreSQL/HTTP.
- **Checks**: make lint et make test PostgreSQL18/CI=true PASS. Revue indépendante des ratios,
  année bissextile et lectures atomiques ; génération sqlc stable.
- **Failure case**: prix futur exclu, référence hors fenêtre exclue des extrema, division périodique
  arrondie seulement à la sortie ; prix courant conservé quand fenêtre/référence manquent.
- **Provenance**: chaque résultat donne prix, devise, source, dates et IDs. À défaut de paramètre,
  as_of est la date maximale persistée, explicitement observée ; aucune horloge réelle simulée.
- **Not done**: simulateur/ingestion/SSE NLPF-68, UI T7, intégration BOM/seed T8/T9 et CI distante.

### Reprise après interruption de quota — 2026-11-05
- **Status**: tests terminés sur le périmètre disponible ; dépendances métier toujours bloquantes.
- **Time**: 2026-11-05T12:24:18+01:00
- **Commit**: 659fe73 — correctif GO-2026-6809, Go 1.27.3 dans go.mod/Dockerfile.
- **Checks**: le scan initial Go 1.27.2 échoue (code 3, crypto/x509 atteint via pgx) ; make sec
  passe après correctif, sans nouvelle dépendance. Go/Node restaurés sous /tmp, SHA256 vérifiés.

### T7 : Interface et tests livrés, acceptation d'intégration encore ouverte — 2026-11-05
- **Status**: partial ; case T7 laissée vide, aucune preuve de SSE réel.
- **Time**: 2026-11-05T12:24:18+01:00
- **Commit**: 848025e — interface fournisseurs, formulaires, courbes, historique, Playwright et CI.
- **Checks**: 54/54 tests, aucun skip/retry, Chromium 156.0.8078.80 / Firefox 157.0 / WebKit 27.2,
  ordinateur et petit écran ; lint/build PASS. Signal EventSource contrôlé dans ces tests uniquement.
  make lint, make test PostgreSQL/CI=true, go test -race -count=1 ./..., make sec PASS.
  goose up/reset/up et génération sqlc stable PASS ; go mod tidy -diff PASS.
- **Failure case**: un clic sur le fournisseur courant masquait une erreur 503 ; corrigé sans
  affaiblir le scénario. Fermeture/réouverture clavier du dialogue sur Chromium mobile corrigée.
  Contrats/paliers en chaînes et erreurs des formulaires vérifiés sur six profils.
- **Not done**: ingestion/SSE/horloge réels, remises dans le vrai moteur BOM, seed Multipla,
  Safari réel et CI distante ; aucun push/merge/PR. Responsabilités détaillées dans delivery.md.

### Dépendances confirmées et état bloqué — 2026-11-05
- **Status**: blocked sur T7, T8, T9 et acceptation globale T10.
- **Evidence**: main distant toujours 532b627 ; nlpf-69 disponible à ff5cc69 (Arthur), aucune
  branche nlpf-68 observée par ls-remote au contrôle final. La disponibilité ailleurs n'est pas présumée.
- **Analysis**: inspection en lecture seule de NLPF-69 : collisions de migration 00002 et ADR
  acceptées 0006–0008, attributs catalogue divergents, sélection/gel des contrats absents.
  Aucun travail d'Arthur repris et aucune base de l'équipe touchée. Voir integration-nlpf-69.md.
- **Next**: accord du propriétaire et décisions de compatibilité avant intégration ; livrer
  NLPF-68 et ses contrats réels avant de valider SSE, remises BOM et seed complet.

## Decisions

- 2026-11-04 — Utilisateur : conserver l'intégralité de NLPF-67, y compris seed et Multipla.
- 2026-11-04 — Utilisateur : notes qualité/confiance de 0 à 5 ; recyclabilité de 0 à 100 %.
- Priorité du document P2 = Jira Medium ; les features sources restent P1 dans docs/features.md.
- D1–D5 du plan sont des propositions, pas des décisions d'architecture acceptées.
- 2026-11-04 — Invocation de spec-execute : autorisation de démarrer le plan et de conserver ses
  fichiers sur la branche nlpf-67 ; aucune acceptation des choix d'architecture n'en est déduite.
- 2026-11-04 — Utilisateur : application utilisable quel que soit le navigateur ; Chromium seul
  ne constitue pas une couverture suffisante. La cible vérifiable proposée est celle des navigateurs
  modernes et l'ADR 0010 n'est pas automatiquement passée à Accepté.
- 2026-11-04 — Joe : « accepte toutes les adere et continue stp ». Acceptation explicite des ADR
  0006–0010 et autorisation de leur application, dont @playwright/test et les trois moteurs navigateur.
  Enregistrement des en-têtes à sa demande ; les ADR 0001–0005 déjà acceptées restent inchangées.
- 2026-11-04 — Utilisateur : pour changer devise/fournisseur d'une offre déjà cotée, créer une
  nouvelle offre et archiver l'ancienne ; refuser le changement sur l'offre cotée. Règle enregistrée
  dans spec/integration sans réécrire les ADR acceptées ni modifier le modèle des cotations.

## Blockers

- T7 : signal SSE, horloge et ingestion de NLPF-68 indisponibles dans cette branche ; les tests
  contrôlés ne remplacent pas cette intégration. Les cotations déjà persistées sont consultables.
- T8/T9 : moteur/éditeur NLPF-69 sur branche d'Arthur, non intégré. Accord du propriétaire et
  décisions sur les migrations/ADR et le gel des remises nécessaires ; impact dans integration-nlpf-69.md.
- T9 : générateur transactionnel d'historique NLPF-68 et éditeur raccordé manquants. Ni seed
  concurrent ni réinitialisation des données pour contourner ces dépendances.
- T10 : CI distante et Safari réel non vérifiés ; paire NLPF-67 validée séparément, deux anciennes
  paires spec/state incohérentes laissées intactes (docker-compose-stack, nlpf-70).


## Verification Report(s)

### Vérification du plan — 2026-11-04

- PASS : `node ~/.agents/spec-drum/bin/validate-specs.js --all --project
  /tmp/nlpf-67-validation-xmksajgk` → `Spec Drum: 2 file(s) OK`, copie isolée de NLPF-67.
- FAIL préexistant : `node ~/.agents/spec-drum/bin/validate-specs.js --all` → code 2 :
  `.specs/docker-compose-stack: spec/state status mismatch` et
  `.specs/nlpf-70: spec/state status mismatch`.
  Confirmation par lecture de HEAD : spec draft / state done pour chacun. Aucun de ces fichiers modifié.
- Inspection : dix tâches, exigences R1–R8 reliées aux tâches et critères A1–A11 ; propositions
  d'architecture non présentées comme acceptées. Aucun fichier de code, branche ou commit créé.
- NOT RUN : make lint, make test, build frontend, intégration et CI ; aucune implémentation dans
  cette livraison documentaire. Les tâches T1–T10 ne sont pas commencées.

### Vérification des propositions T1 — 2026-11-04

- PASS : contrôle de structure des ADR 0006–0010, toutes Proposé, quatre sections et au moins deux
  options ; aucune ADR existante modifiée. Les recommandations ne sont pas présentées comme acceptées.
- PASS : `node ~/.agents/spec-drum/bin/validate-specs.js --all --project
  /tmp/nlpf-67-t1-check-kjre3ptl` → `Spec Drum: 2 file(s) OK`, copie de NLPF-67 à l'état in-progress/T1.
- FAIL préexistant : validation globale, code 2, mêmes status mismatch nlpf-70/docker-compose-stack.
- FAIL environnement : `make test` → `go test ./...` puis `make: go: No such file or directory`,
  `make: *** [Makefile:10: test] Error 127` (commande make : code 2).
- NOT RUN : make lint (Go absent et node_modules absent), build web (Node 18.19.1 incompatible avec
  engines du lockfile Vite ^20.19.0 || >=22.12.0), tests DB/navigateur/CI. Aucun outil installé.

### Vérification locale de la reprise — 2026-11-05

- PASS : make lint ; make test (CI=true et PostgreSQL 18 dédié) ; go test -race -count=1 ./... ;
  make sec ; build frontend ; go mod tidy -diff ; génération sqlc stable (9 empreintes identiques).
- PASS : migrations up/reset/up dans la base jetable stp-nlpf67-resume-test uniquement.
- PASS : 54 scénarios Playwright (9 × 3 moteurs × 2 formats), zéro skip/flaky/échec, 31,0 s.
  Captures ordinateur/mobile inspectées ; dates, valeurs exactes, couleurs et absence de débordement.
- PASS : binaire serveur et stack Docker Compose construite depuis le code de cette reprise,
  projet distinct stp-nlpf67-check, base neuve ; back/web/priceapi/db healthy, migrate Exited (0).
  Via nginx : /, /healthz et /api/v1/suppliers répondent 200 ; healthz priceapi répond également.
- FAIL préexistant : validateur global, mêmes incohérences nlpf-70 et docker-compose-stack.
  La validation isolée de la paire NLPF-67 est consignée après le contrôle final.
- NOT RUN : CI distante, Safari réel et intégration réelle SSE/prix/BOM/seed. T7–T10 restent
  décochées et statuts spec/state blocked. Aucun test obligatoire manquant présenté comme réussi.
- Rapport : delivery.md (fichiers/responsabilités, pannes/tests/limites) ; integration-nlpf-69.md
  (conflits vérifiables et conditions de reprise). Aucun snapshot publié ni donnée utilisateur modifié.

- Contrôle final : PASS, `Spec Drum: 2 file(s) OK`, paire NLPF-67 validée ensemble dans
  /tmp/nlpf-67-final-validation-xeeywsfw. Global : code 2, uniquement les deux incohérences
  préexistantes ci-dessus. Les statuts blocked de NLPF-67 sont cohérents.
