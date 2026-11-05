---
id: nlpf-68
status: in-progress
branch: nlpf-68
current_task: T1
started: 2026-11-05
updated: 2026-11-05
---

# State: Prix et devises (NLPF-68)

## Progress

Exécution du plan autorisée par les invocations de spec-execute. Branche nlpf-68 créée depuis
4dcdb48, avec les documents de planification conservés. T1 en cours : propositions d'ADR et contrats
préparés avant code ; acceptation des nouvelles décisions techniques encore nécessaire.
Les trois arbitrages métier répondus par Joe sont intégrés et ne sont pas à redemander.

## Task Log

### Planification — 2026-11-05

- Source : lecture Jira NLPF-68 via le connecteur Atlassian, statut To Do, assigné Joe,
  priorité Medium (P2 documentaire). Aucune modification de Jira ni message à l'équipe.
- Recherche : règles AGENTS, features, ADR acceptées 0002–0005 et contrats NLPF-67,
  interfaces/services/schema pricing, source priceapi, Money, UI et harness existants.
- Contexte Git : branche courante nlpf-67, HEAD 4dcdb48, propre avant création des deux documents.
  Base proposée pour l'exécution nlpf-68 depuis cette révision, pas de branche créée ni de commit.
- Dépendance : NLPF-69 inspectée par les objets déjà présents de ff5cc69, sans reprise de branche.
  Collisions migration 00002 et ADR acceptées 0006–0008 connues, pas corrigées pendant ce plan.
- Questions métier posées avec références à docs/features.md et réponses humaines intégrées.
- Documents produits : spec.md et state.md seulement ; aucune dépendance ou code applicatif modifié.

### Démarrage de l’exécution — 2026-11-05

- **Status**: in-progress, T1.
- **Branch**: nlpf-68 depuis 4dcdb48 ; seulement les nouveaux documents de ce ticket dans l’arbre.
- **Authorization**: invocations explicites « $spec-execute nlpf-68 » puis « $spec-execute » ;
  approbation du plan et conservation de ses fichiers sur sa branche. Aucun accord nouveau déduit
  sur D1–D6 ; les trois arbitrages métier déjà confirmés sont conservés.
- **Not done**: code, dépendances, migrations, reprise de NLPF-69, push/PR et tests applicatifs.

### T1 : Propositions et contrats préparés — 2026-11-05

- **Status**: in-progress ; six ADR Proposé, aucune acceptée, case T1 laissée vide.
- **Time**: 2026-11-05T13:19:15+01:00
- **Files**: ADR 0011–0016, docs/api-prix-et-devises.md, architecture-review.md, spec/state.
- **Notes**: propositions D1–D6 concrètes : source déterministe, progression/repli/SSE,
  fermeture transactionnelle des snapshots, inverse rationnel exact, paramètres budgets et
  intégration différée de la vraie BOM. Aucun nouveau choix déduit de l'invocation du skill.
- **Responsibility**: chaque ADR expose options/décision proposée/conséquences ; le contrat API
  fixe les formats et ports futurs ; architecture-review résume les arbitrages et conditions de reprise.
- **Failure case**: prix de mauvaise devise suivi de timeout ; lot et tick non validés,
  repli complet expliqué ou erreur sans zéro. Tests Source/DB/navigateur à écrire après accord.
- **Not done**: code métier, dépendances, migrations, données, reprise de NLPF-69, CI distante,
  tests applicatifs nouveaux. Les trois réponses de Joe restent acquises, pas soumises à nouveau.

## Decisions

- 2026-11-05 — Joe : budgets cibles global/sous-systèmes par voiture ; budget de production total distinct.
- 2026-11-05 — Joe : volume déduit à partir du coût unitaire calculé pour un volume de référence
  explicite, conservé avec le résultat. Ce volume de référence ne devient pas le volume déduit.
- 2026-11-05 — Joe : reprise au dernier jour simulé validé, sans rattrapage du temps d'arrêt.
- Périmètre : tous les critères du ticket, dont target-costing ; ce plan ne remplace pas NLPF-67/69.
- ADR 0002–0005 restent les décisions existantes ; D1–D6 sont des propositions techniques à faire
  valider en T1. Les accords antérieurs des ADR NLPF-67 ne constituent pas un accord sur ces nouveautés.

## Blockers

- Plan et base autorisés par l'invocation de spec-execute ; ADR 0011–0016 et contrat associé
  proposés, acceptation humaine requise avant code (AGENTS §6–8). T1 reste ouverte.
- Raccordements BOM/budgets/snapshots : vraie livraison NLPF-69 et accord de son propriétaire,
  compatibilité des migrations/ADR/interfaces à résoudre sans changer les historiques existants.
- Le catalogue présent dans la base proposée suffit pour démarrer le flux ; attendre la clôture
  de tout NLPF-67 créerait une dépendance circulaire inutile. Son seed attendra ce générateur.
- Deux incohérences de statuts de specs anciennes sont connues (nlpf-70 et docker-compose-stack) ;
  elles ne sont pas modifiées par la planification de NLPF-68.

## Verification Report(s)

### Revue de traçabilité — 2026-11-05

- Dix tâches ordonnées, R1–R10 et S1/S2 reliés à leurs tâches et critères J1–J10/C1–C4.
- Les dix critères Jira sont présents ; budgets, inverse exact, immuabilité par ajout et dépendance
  BOM explicites. Aucune intégration finale annoncée réalisable avec de simples stubs.
- Aucun test applicatif, build, migration ou CI exécuté pendant ce plan ; les commandes de la spec
  sont des vérifications futures, pas des PASS de planification.
- Validation de la nouvelle paire spec/state à consigner après exécution du validateur.

### Validation des documents — 2026-11-05

- PASS : nouvelle paire validée ensemble dans /tmp/nlpf-68-plan-validation-g0hs7cb0 ;
  `Spec Drum: 2 file(s) OK`.
- PASS : contrôles de structure : dix tâches consécutives avec fichiers/approche/critère,
  dix exigences Must Have et les dix critères Jira tracés. Relecture des frontières et dépendances.
- FAIL préexistant : validateur global, code 2, uniquement les incohérences
  `.specs/nlpf-70` et `.specs/docker-compose-stack` (spec draft/state done). Fichiers laissés intacts.
- NOT RUN : tests, lint, build, migrations et CI applicatifs ; aucun code produit pendant le plan.

### Vérification de la préparation T1 — 2026-11-05

- PASS : six nouvelles ADR 0011–0016 au statut Proposé, quatre sections et au moins deux
  options chacune ; format court et liens du dossier de revue contrôlés.
- PASS : paire spec/state in-progress/T1 validée ensemble dans
  /tmp/nlpf-68-t1-validation-azdawsw2 ; `Spec Drum: 2 file(s) OK`.
- PASS : code, dépendances, migrations, features.md et ADR acceptées 0001–0010 inchangés.
- FAIL préexistant : contrôle global (code 2), uniquement mismatch des statuts
  nlpf-70 et docker-compose-stack. Aucune correction hors périmètre.
- NOT RUN : tests/lint/build applicatifs et CI distante ; lot documentaire, aucune
  implémentation nouvelle à valider avant décision humaine. T1 non terminée, T2–T10 non commencées.
