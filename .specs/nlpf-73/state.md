---
id: nlpf-73
status: done
branch: nlpf-71
current_task: null
started: 2026-11-10
updated: 2026-11-10
---

# State: Seed sample-dataset : données en double après deux lancements

## Progress

All tasks completed. Ready for verification.
- Tasks: 4/4 completed (T3 sans objet)
- Branch: `nlpf-71`
- Commits: 5

## Task Log

### T1: Diagnostiquer la base concernée
- **Status**: completed
- **Time**: 2026-11-10
- **Commit**: (aucun fichier de code, constat ci-dessous)
- **Notes**: requête sur la base compose (`stp-db-1`, `bomstudio`). Deux lignes dans `seed_loads` →
  hypothèse 1 (changement de `SIM_DATASET_VERSION` entre les deux lancements). T3 sans objet.

### T2: Test de bout en bout de l'idempotence via le point d'entrée `seed`
- **Status**: completed
- **Time**: 2026-11-10
- **Commit**: d88b7b8
- **Notes**: `cmd/server/seed_integration_test.go` appelle `runSeed` deux fois (mêmes `SIM_*`, une paire FX)
  dans un schéma privé ; compare suppliers, parts, offers, boms, bom_nodes, price_quotes, fx_quotes,
  pricing_runs, seed_loads. Vert du premier coup (hypothèse 1). Coût : ~70 s (prix initiaux réels).

### T3: Correctif du seed
- **Status**: skipped (sans objet)
- **Time**: 2026-11-10
- **Commit**: aucun
- **Notes**: T1 conclut à l'hypothèse 1, pas de bug.

### T4: README : coexistence des versions et remise à zéro de la démo
- **Status**: completed
- **Time**: 2026-11-10
- **Commit**: 06b57b2
- **Notes**: paragraphe après le § seed : second jeu par nouvelle version, requête `seed_loads`,
  procédure `down -v` → `make up` → `seed`. Procédure **non rejouée** : elle effacerait la base de dev
  (diagnostic T1 et BOM `test1`).

## Decisions

- 2026-11-10 — Distinguer deux jeux : option 1, documentation + remise à zéro, rien dans l'UI
  (choix humain : Arthur Presle).
- 2026-11-10 — Exécution sur la branche `nlpf-71` (demande de l'utilisateur : « sur la même branche »),
  pas de branche `nlpf-73` dédiée.
- 2026-11-10 — **Diagnostic T1 : hypothèse 1 confirmée, pas de bug du seed.**
  `SELECT dataset_version, generator_version, seed, as_of, manifest_hash, loaded_at FROM seed_loads ORDER BY loaded_at`

  ```
   dataset_version | generator_version | seed |         as_of          | manifest_hash | loaded_at
  -----------------+-------------------+------+------------------------+---------------+-------------------------------
   demo-v2         | sampledata.v1     | 42   | 2026-01-01 00:00:00+00 | 07746837…235c | 2026-11-10 13:30:28.719437+00
   demo-v3         | sampledata.v1     | 42   | 2026-01-01 00:00:00+00 | 9e44864f…bb05 | 2026-11-10 13:53:04.803071+00
  ```

  Fournisseurs : 54 lignes pour 27 noms. BOM : `2-3` (créées 13:30:28, = `loaded_at` demo-v2) et
  `45-46` (13:53:04, = `loaded_at` demo-v3) ; BOM `1` (`test1`) est une saisie manuelle antérieure.
  Les deux `manifest_hash` diffèrent : le contenu généré a changé entre v2 et v3 pendant le développement
  (le défaut est passé à `demo-v3` dans `6377435`). Chaque version a donc ajouté son jeu complet, comme
  le prévoit `seedstore.Load` et `TestTwoDatasetVersionsCoexist`. T3 (correctif) non exécuté.

- 2026-11-10 — S1 (journaliser les versions déjà chargées) non fait : aucune tâche ne le portait.

## Blockers

None.

## Verification Report(s)

- 2026-11-10 — `make lint` : 0 issues (go vet, golangci-lint, oxlint) ; `make test` : tous les paquets ok
  (DATABASE_URL du Makefile, intégration comprise). CI non vérifiée (rien poussé).

## Verification Report — 2026-11-10

### Overall: PASS

### Task Completion: 4/4 (T3 sans objet, hypothèse 1)

### Commit Verification
T2 `d88b7b8`, T4 `06b57b2` présents ; T1 (constat) et T3 (sans objet) sans commit de code, attendu.

### Automated Checks
| Check | Result | Notes |
|---|---|---|
| `make lint` | PASS | 0 issues |
| `make test` | PASS | `cmd/server` (intégration seed) et `seedstore` ok (résultat en cache) |

### Requirements Validation
| Requirement | Status | Evidence |
|---|---|---|
| R1 cause établie | MET | sortie `seed_loads` (demo-v2 / demo-v3) dans state.md |
| R2 relance identique = rien ajouté | MET | `cmd/server/seed_integration_test.go` appelle `runSeed` deux fois, compare 9 tables |
| R3 README dit la coexistence | MET | paragraphe ajouté après le § seed |
| R4 repérer deux jeux, revenir à un | MET | requête `seed_loads` + procédure |
| R5 remise à zéro | MET | `down -v` / `make up` / `seed` |
| R6 | N/A | hypothèse 2 écartée |
| S1 journaliser les versions chargées | NOT MET (non bloquant) | noté dans state.md, aucune tâche ne le portait |

### Acceptance Criteria
| Criterion | Result | Evidence |
|---|---|---|
| AC1 cause + sortie de la requête | PASS | state.md Decisions |
| AC2 test de relance vert | PASS | `make test` |
| AC3 README | PASS | diff README |

### Code Quality (/simplify)
Non exécuté (diff limité à un test et au README).

### Summary
Pas de bug : les doublons viennent de deux versions de jeu différentes, désormais documentées. Réserves : procédure du README non rejouée (elle effacerait la base de dev), S1 non fait, CI non vérifiée.
