---
id: nlpf-73
status: in-progress
branch: nlpf-71
current_task: T2
started: 2026-11-10
updated: 2026-11-10
---

# State: Seed sample-dataset : données en double après deux lancements

## Progress

T1 terminé (hypothèse 1). En cours : T2.

## Task Log

### T1: Diagnostiquer la base concernée
- **Status**: completed
- **Time**: 2026-11-10
- **Commit**: (aucun fichier de code, constat ci-dessous)
- **Notes**: requête sur la base compose (`stp-db-1`, `bomstudio`). Deux lignes dans `seed_loads` →
  hypothèse 1 (changement de `SIM_DATASET_VERSION` entre les deux lancements). T3 sans objet.

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

## Blockers

None.

## Verification Report(s)
