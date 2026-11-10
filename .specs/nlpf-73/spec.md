---
id: nlpf-73
title: Seed sample-dataset : données en double après deux lancements
source: jira
source_ref: https://3istors.atlassian.net/browse/NLPF-73
created: 2026-11-10
status: in-progress
priority: P2
depends_on: []
branch: nlpf-71
---

# Seed sample-dataset : données en double après deux lancements

## Context

Sur une base de dev, deux lancements du seed à 23 min d'écart (2026-11-10 13:30 et 13:53 UTC) ont laissé
54 fournisseurs pour 27 noms et les BOM Multipla / Valhalla en double (id 2-3 puis 45-46). Le README et les
ADR 0012/0021 annoncent un seed idempotent. Cause non établie : soit `SIM_*` a changé entre les deux
lancements (un second jeu est alors attendu), soit le seed ne reconnaît pas son propre jeu (bug).

## Research Findings

### Relevant Files
- `internal/sampledata/seedstore/store.go` — `Load` : verrou consultatif par `dataset_version`, lecture de
  `seed_loads` ; même version + mêmes paramètres → rien ; même version + autres paramètres →
  `ErrParametersConflict` ; **version nouvelle → jeu complet inséré** (conception, l. 1-3, 59-90).
- `internal/sampledata/seedstore/store_integration_test.go` — `TestLoadTwiceKeepsExactlyTheSameData`
  compte déjà 11 tables (fournisseurs, pièces, offres, BOM, nœuds…) avant/après ;
  `TestTwoDatasetVersionsCoexist` attend **4 BOM** pour deux versions : la coexistence est voulue.
- `migrations/00006_sample_dataset.sql` — `seed_loads(dataset_version PK, generator_version, seed, as_of,
  manifest_hash, loaded_at)` : de quoi diagnostiquer la base concernée.
- `cmd/server/seed.go`, `cmd/server/pricing.go:48-51` — `SIM_DATASET_VERSION`, `SIM_SEED`, `SIM_AS_OF` lus
  dans l'environnement ; défauts dans `Makefile:7-8` et `compose.yaml:42-44` (`demo-v3`, `42`).
- `README.md:73-78` — « Relancer ne change rien » ; ne dit pas qu'une autre version ajoute un second jeu.
- `docs/adr/0012-*.md`, `docs/adr/0021-contenu-du-jeu-sample-dataset.md` — ADR acceptées (ne pas réécrire).

### Existing Patterns
- Tests d'intégration Postgres avec `seedDB(t)` et `counts(t, pool)` ; table-driven ailleurs.
- Le seed est l'unique écrivain du jeu ; aucune `UPDATE`/`DELETE`.

### Constraints
- Indice fort pour l'hypothèse 1 : le seed arrive dans le commit `6377435` (2026-11-10 14:56 +01:00, soit
  13:56 UTC), qui fait passer la version par défaut de `demo-v1` à `demo-v3`. Les deux lancements ont eu
  lieu **avant** ce commit, pendant le développement : un changement de version entre les deux est
  probable. À confirmer sur la base concernée (T1), pas à supposer.
- AGENTS §11 : ne pas modifier une migration commitée ni une ADR acceptée ; toute nouvelle règle de
  coexistence = ADR au statut Proposé.

### Décision à trancher par un humain
Comment distinguer deux jeux chargés (aujourd'hui deux BOM portent le même nom) :
1. **Documenter seulement** (README + procédure de remise à zéro) — rien dans l'UI.
2. **Le nom des BOM du jeu porte la version** (`Fiat Multipla 1.6 16V · demo-v4`) — change le contenu
   généré, donc exige une nouvelle `SIM_DATASET_VERSION`.
3. **Le seed refuse un second jeu** si une autre version est déjà chargée, sauf option explicite — change la
   règle de l'ADR 0012/0021 → nouvelle ADR.

**Retenu : option 1** (décision humaine du 2026-11-10) : la distinction passe par la documentation et la
remise à zéro, pas par l'UI. Si T1 conclut à l'hypothèse 2, la spec est révisée avant T3.

## Requirements

### Must Have
- R1 : cause établie et notée dans state.md (requête sur `seed_loads` de la base concernée).
- R2 : relancer `make seed` avec les mêmes paramètres n'ajoute aucune ligne — test d'intégration qui compte
  fournisseurs, pièces, offres et BOM avant/après (existant : à vérifier qu'il couvre bien le chemin réel
  de `server seed`, sinon le compléter).
- R3 : si un changement de version crée volontairement un second jeu, le README le dit.
- R4 : (option 1 retenue) le README explique comment reconnaître deux jeux chargés (`seed_loads`) et
  comment revenir à un seul.
- R5 : procédure de remise à zéro pour la démo dans le README (`docker compose down -v`, `make up`, seed).
- R6 (si l'hypothèse 2 est confirmée) : test qui reproduit le bug, puis correctif.

### Should Have
- S1 : `server seed` journalise (slog) les versions déjà chargées avant d'en ajouter une nouvelle.

### Won't Do
- Commande de purge d'un jeu (`DELETE`) : contraire au seed sans `UPDATE`/`DELETE`.
- Modifier la migration 00006 ou réécrire l'ADR 0012/0021.

## Tasks

- [x] T1: Diagnostiquer la base concernée
  - **Files**: aucun (constat dans `.specs/nlpf-73/state.md`)
  - **Approach**: `SELECT dataset_version, generator_version, seed, as_of, loaded_at FROM seed_loads ORDER BY
    loaded_at` sur la base qui montre les doublons. Deux lignes → hypothèse 1 ; une seule → hypothèse 2.
  - **Done when**: résultat de la requête et conclusion écrits dans state.md (Decisions).

- [ ] T2: Test de bout en bout de l'idempotence via le point d'entrée `seed`
  - **Files**: `cmd/server/seed_test.go` (nouveau, intégration) ou `store_integration_test.go`
  - **Approach**: lancer deux fois la fonction de `cmd/server/seed.go` avec les mêmes `SIM_*`, comparer les
    comptes de `suppliers`, `parts`, `offers`, `boms` ; si hypothèse 2, ce test doit d'abord échouer.
  - **Done when**: le test passe (hypothèse 1) ou échoue puis passe après correctif (hypothèse 2, R6).

- [ ] T3: Correctif du seed — seulement si T1 conclut à l'hypothèse 2
  - **Files**: `internal/sampledata/seedstore/store.go`
  - **Approach**: à définir d'après le diagnostic ; spec révisée avant de coder.
  - **Done when**: T2 passe.

- [ ] T4: README : coexistence des versions et remise à zéro de la démo
  - **Files**: `README.md` (§ seed, l. 73-78)
  - **Approach**: dire qu'une nouvelle `SIM_DATASET_VERSION` ajoute un jeu complet à côté de l'ancien, et
    comment le voir (`SELECT dataset_version, loaded_at FROM seed_loads`) ; procédure
    `docker compose down -v` → `make up` → `docker compose run --rm back seed`.
  - **Done when**: la procédure, suivie à la lettre sur une machine propre, donne 27 fournisseurs et 2 BOM.

## Verification

### Automated Checks
- `make lint && make test` (tests d'intégration Postgres compris)

### Manual Checks
- Remise à zéro selon le README, puis `make seed` deux fois : `SELECT count(*) FROM suppliers` = 27,
  `SELECT count(*) FROM boms` = 2.

### Acceptance Criteria
- [ ] AC1 (R1) : cause écrite dans state.md, avec la sortie de la requête.
- [ ] AC2 (R2, R6) : test d'intégration de relance à paramètres identiques vert.
- [ ] AC3 (R3, R4, R5) : README décrit la coexistence des versions, comment la repérer, et la remise à zéro.
