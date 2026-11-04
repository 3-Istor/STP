---
id: nlpf-67
status: in-progress
branch: nlpf-67
current_task: T1
started: 2026-11-04
updated: 2026-11-04
---

# State: Catalogue et fournisseurs (NLPF-67)

## Progress

Exécution demandée par invocation explicite de spec-execute ; branche locale nlpf-67 créée depuis main
avec les fichiers de planification conservés. T1 en cours : cinq ADR Proposé et un contrat d'intégration
concrets sont préparés. Aucun code ni migration modifié. T1 reste décochée jusqu'à acceptation humaine
et confirmation des contrats d'intégration ; T2–T10 non commencées.

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

## Blockers

- Acceptation humaine des ADR proposées 0006–0010 et du contrat integration.md avant le code concerné.
- Contrats à confirmer avec NLPF-68 (Joe : prix/historique/horloge/SSE/conversion) et NLPF-69
  (Arthur Presle : BOM/éditeur/totaux) ; ces fonctions sont absentes du code inspecté.
- Go absent de PATH et des emplacements usuels inspectés : make test échoue. Node 18.19.1 ne
  satisfait pas engines du Vite verrouillé (^20.19.0 || >=22.12.0) ; CI configurée en Node 24.
- Aucun package installé pour ces propositions ; ajout @playwright/test soumis à accord ADR 0010.
- CI courante non consultée ; lint/tests/build métier non exécutés pendant cette planification.

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
