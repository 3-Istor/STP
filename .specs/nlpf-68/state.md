---
id: nlpf-68
status: in-progress
branch: nlpf-68
current_task: T9
started: 2026-11-05
updated: 2026-11-05
---

# State: Prix et devises (NLPF-68)

## Progress

T1–T8 terminées sur les capacités décrites dans le journal. T9 en cours.
ADR 0011–0016 acceptées explicitement par Joe ; dépendances BOM toujours visibles.

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

### T1 : Préparation enregistrée pour revue — 2026-11-05

- **Status**: in-progress ; attente de validation des propositions, T1 non cochée.
- **Time**: 2026-11-05T13:21:47+01:00
- **Commit**: 725ed4d — nlpf-68: T1 - proposer architecture et contrats de prix.
- **Checks**: six ADR Proposé et paire de suivi validées ; aucun code/migration/dépendance modifié.
- **Next**: acceptation humaine des choix recommandés dans ADR 0011–0016 et du contrat associé,
  ou amendements demandés. Les trois choix métier déjà confirmés ne sont pas à redemander.

### T1 : Faire valider les contrats et décisions restant ouverts (R1–R10) — 2026-11-05

- **Status**: completed.
- **Time**: 2026-11-05T13:35:46+01:00
- **Commit**: 603421c.
- **Notes**: Joe a explicitement accepté les ADR 0011–0016 et le contrat associé. Cadrage terminé, raccordements BOM différés selon ADR 0016. Les trois choix métier antérieurs restent acquis.

### T2 : Protéger les interfaces et comportements existants (R1, R5–R7, R10) — 2026-11-05

- **Status**: completed.
- **Time**: 2026-11-05T13:39:18+01:00
- **Commit**: c57c31c.
- **Notes**: Baseline make lint, make test avec CI=true/PostgreSQL18 et build frontend verts. Tests de protection du taux exact, de sa paire/source/date et du Content-Type priceapi ajoutés avant extension.

### T3 : Livrer le simulateur pur derrière l'API externe (R1–R2, R10, S2) — 2026-11-05

- **Status**: completed.
- **Time**: 2026-11-05T13:51:38+01:00
- **Commit**: 2200c98.
- **Notes**: Simulateur pur, cinq routes externes et client HTTP livrés (ADR 0011). make lint/test PostgreSQL et -race sources verts. Tests graines/ordre/batches/365 jours, >2^53, JSON/devise/date/bornes et timeout ; aucune dépendance ajoutée.

### T4 : Persister l'initialisation et les ticks avec reprise (R2–R4, R10, S1–S2) — 2026-11-05

- **Status**: completed.
- **Time**: 2026-11-05T14:25:08+01:00
- **Commit**: 994f68a.
- **Notes**: Historique et curseur atomiques, bail de worker et port seed transactionnel livrés (ADR 0011–0012/0016). make lint/test PostgreSQL et -race verts ; horloge virtuelle 365 jours/40 min, deux schémas/IDs différents, concurrence et rollback vérifiés. up/reset/up, sqlc stable et vrais binaires HTTP/reprise verts. Cadence ajustable sans régénération ; correctif de précision 48f9063 consigné.

### T5 : Exposer les cotations, historiques et le repli expliqué (R1, R4–R5, R10) — 2026-11-05

- **Status**: completed.
- **Time**: 2026-11-05T14:51:48+01:00
- **Commit**: f8b59a4.
- **Notes**: Lectures réelles prix/FX/historiques et repli complet expliqué livrés. make lint/test PostgreSQL, build, race, sqlc stable et make sec verts ; 60 scénarios navigateur contrôlés sur trois moteurs verts (33,5 s), aucune preuve de SSE réel encore. Statistiques épinglées à la cotation retenue et horloge validée.

### Précision du séquençage T6–T10 — 2026-11-05

- **Status**: in-progress, T6.
- **Authorization**: ADR 0016 acceptée : capacités pricing indépendantes autorisées sans reprise de NLPF-69.
- **Notes**: spec amendée avant code : capacités autonomes T6–T9 puis raccordements réels BOM obligatoires en T10. Tous les critères d’acceptation sont conservés ; le ticket ne sera pas déclaré terminé sur des stubs.
- **Migration**: lecture distante main=532b627 et NLPF-69=ff5cc69 inchangés ; 00004 est le prochain numéro de cette base locale, sans réservation globale.

### T6 : Construire et consommer des snapshots de prix complets (R4, R6, R10) — 2026-11-05

- **Status**: completed.
- **Time**: 2026-11-05T14:58:48+01:00
- **Commit**: 4bafdb5.
- **Notes**: Snapshots autonomes quatre périmètres, couverture complète et fermeture transactionnelle livrés (ADR 0013). make test PostgreSQL18, make lint, race ciblé et up/reset/up verts ; migration conserve les faits et IDs legacy, refuse ajouts tardifs et XID forgé. Raccordements de publication/attachement et devises budgets obligatoires en T10, non validés.

### T7 : Intégrer la conversion directe et inverse exacte (R7, R10) — 2026-11-05

- **Status**: completed.
- **Time**: 2026-11-05T15:02:42+01:00
- **Commit**: a135b19.
- **Notes**: Conversion autonome exacte et API livrées (ADR 0014), direct prioritaire/inverse sans pivot ; port rationnel pour agrégation et floor. make test PostgreSQL18, make lint et race ciblé verts. Tests 92 EUR, inverse périodique total 10 USD, JPY/KWD, >2^53, taux figés inchangés et entrée exposant excessive. Vrai moteur BOM obligatoire en T10.

### T8 : Livrer les évaluateurs de budgets et de volume pour la vraie BOM (R7–R8, R10) — 2026-11-05

- **Status**: completed.
- **Time**: 2026-11-05T15:07:07+01:00
- **Commit**: 46d3d30.
- **Notes**: Évaluateurs purs budgets/allocation/production livrés (ADR 0015), sans second moteur ni route BOM factice. make test PostgreSQL18, make lint et race ciblé verts. Tests seuils exacts, devise manquante, allocation imbriquée/inverse avant somme, 66 voitures, référence/remise conservées, zéro et overflow. Persistance, publications et écrans BOM restent obligatoires en T10.

## Decisions

- 2026-11-05 — Joe : budgets cibles global/sous-systèmes par voiture ; budget de production total distinct.
- 2026-11-05 — Joe : volume déduit à partir du coût unitaire calculé pour un volume de référence
  explicite, conservé avec le résultat. Ce volume de référence ne devient pas le volume déduit.
- 2026-11-05 — Joe : reprise au dernier jour simulé validé, sans rattrapage du temps d'arrêt.
- Périmètre : tous les critères du ticket, dont target-costing ; ce plan ne remplace pas NLPF-67/69.
- ADR 0002–0005 restent les décisions existantes ; D1–D6 sont des propositions techniques à faire
  valider en T1. Les accords antérieurs des ADR NLPF-67 ne constituent pas un accord sur ces nouveautés.

- 2026-11-05 — Joe : « accepte toutes les adere et continue stp ». Acceptation explicite des
  ADR 0011–0016 et du contrat API présenté ; enregistrement des en-têtes à sa demande.

## Blockers

- ADR 0011–0016 acceptées : aucun blocage sur ces décisions. Raccordements BOM toujours
  soumis à la base et au partage de fichiers convenus avec Arthur, selon ADR 0016.
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
