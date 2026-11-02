---
id: docker-compose-stack
status: in-progress
branch: docker-compose-stack
current_task: T2
started: 2026-11-02
updated: 2026-11-02
---

# State: Lancer toute l'app avec un seul docker compose

## Progress

En cours : T1 terminée (ADR 0005 acceptée par Arthur.presle, commit 3e85deb).
- Tasks: 1/7 completed
- Branch: `docker-compose-stack`

## Task Log

### T1: Brouillon ADR 0005 — API de prix en processus séparé, stack compose complète
- **Status**: blocked (brouillon livré, acceptation humaine requise)
- **Time**: 2026-11-02
- **Commit**: 76a753b
- **Notes**: `docs/adr/0005-api-de-prix-processus-separe.md` au statut `Proposé`, 3 options (ne rien faire,
  simulateur dans le back, binaire `cmd/priceapi` + conteneur). Case T1 non cochée tant qu'un humain n'a pas
  passé l'ADR à `Accepté` avec son nom.

### T1: Brouillon ADR 0005 — API de prix en processus séparé, stack compose complète
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 3e85deb (acceptation humaine)
- **Notes**: ADR 0005 passée à `Accepté` par Arthur.presle ; blocage levé, reprise à T2.

## Decisions

- 2026-11-02 — `.specs/CLAUDE.md` absent du dépôt (référencé par le skill) : exécution poursuivie avec
  AGENTS.md comme seule source de règles.
- 2026-11-02 — nlpf-70 (dépendance) déjà mergé dans `main` (PR #7) : branche créée depuis `main` à jour.
- 2026-11-02 — Choix de l'humain au cadrage : API de prix = binaire `cmd/priceapi` dans un conteneur
  dédié (squelette `/healthz`, simulateur reporté à live-price-feed) ; front = build statique + nginx ;
  migrations = service one-shot `migrate`. À formaliser dans l'ADR 0005 (T1).

## Blockers

(aucun — blocage T1 levé le 2026-11-02 par l'acceptation de l'ADR 0005)

## Verification Report(s)
