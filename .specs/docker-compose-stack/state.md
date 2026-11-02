---
id: docker-compose-stack
status: blocked
branch: docker-compose-stack
current_task: T1
started: 2026-11-02
updated: 2026-11-02
---

# State: Lancer toute l'app avec un seul docker compose

## Progress

Bloqué sur T1 : ADR 0005 rédigée (Proposé), en attente d'acceptation humaine.
- Tasks: 0/7 completed (T1 brouillon commité, non coché)
- Branch: `docker-compose-stack`

## Task Log

### T1: Brouillon ADR 0005 — API de prix en processus séparé, stack compose complète
- **Status**: blocked (brouillon livré, acceptation humaine requise)
- **Time**: 2026-11-02
- **Commit**: 76a753b
- **Notes**: `docs/adr/0005-api-de-prix-processus-separe.md` au statut `Proposé`, 3 options (ne rien faire,
  simulateur dans le back, binaire `cmd/priceapi` + conteneur). Case T1 non cochée tant qu'un humain n'a pas
  passé l'ADR à `Accepté` avec son nom.

## Decisions

- 2026-11-02 — `.specs/CLAUDE.md` absent du dépôt (référencé par le skill) : exécution poursuivie avec
  AGENTS.md comme seule source de règles.
- 2026-11-02 — nlpf-70 (dépendance) déjà mergé dans `main` (PR #7) : branche créée depuis `main` à jour.
- 2026-11-02 — Choix de l'humain au cadrage : API de prix = binaire `cmd/priceapi` dans un conteneur
  dédié (squelette `/healthz`, simulateur reporté à live-price-feed) ; front = build statique + nginx ;
  migrations = service one-shot `migrate`. À formaliser dans l'ADR 0005 (T1).

## Blockers

### T1: Brouillon ADR 0005
- **Error**: aucune erreur technique ; T1 exige l'acceptation humaine de l'ADR avant tout code (AGENTS §6, §8).
- **Attempts**: 0 (blocage volontaire prévu par la spec)
- **Analysis**: l'agent ne peut pas passer une ADR à `Accepté` ni s'inscrire comme décideur.
- **Suggested fix**: relire l'ADR 0005, la passer à `Accepté` et remplir `Décideurs`, commiter, puis relancer
  `/spec-execute docker-compose-stack`.

## Verification Report(s)
