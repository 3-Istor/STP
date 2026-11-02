---
id: docker-compose-stack
status: review
branch: docker-compose-stack
current_task: null
started: 2026-11-02
updated: 2026-11-02
---

# State: Lancer toute l'app avec un seul docker compose

## Progress

All tasks completed. Ready for verification.
- Tasks: 7/7 completed
- Branch: `docker-compose-stack`
- Commits: 17
- Vérifié en local : make lint, make test, go test -race, go mod tidy -diff, docker compose config -q,
  up --build --wait + /healthz via nginx + priceapi, migration en échec qui bloque le back.
- Non vérifié : job CI `compose` (branche non poussée).

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
### T2: Binaire `cmd/priceapi` (squelette `/healthz`)
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: aad50f9
- **Notes**: Patron de cmd/server (PORT défaut 8090, slog, arrêt propre) ; routeur net/http GET /healthz ; test table-driven 200/405/404. go run répond sur :8090/healthz, make lint vert.
### T3: Image Go multi-stage (back, API de prix, migrate)
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: f71586f
- **Notes**: Dockerfile racine : stage build golang:1.27.2-alpine3.23 ; cibles server/priceapi/migrate sur alpine:3.23.2, utilisateur non root. Écart : migrate lance un binaire goose précompilé (même version que le tool de go.mod) au lieu de go tool goose sur l'image golang (voir Decisions). Trois docker build --target OK.
### T4: Image front (build Vite + nginx)
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: a067bb9
- **Notes**: node:24.11.1-alpine3.23 (npm ci + build) puis nginx:1.29.4-alpine ; proxy /api/ et /healthz vers back:8080, proxy_buffering off, proxy_read_timeout 1h ; SPA fallback. Image sert index.html sur / et sur une route inconnue.
### T5: `compose.yaml` complet
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 9e6f239
- **Notes**: up --build --wait rend la main, tous healthy, migrate Exited (0) ; curl :8000/healthz 200 via nginx ; priceapi /healthz 200 dans son conteneur ; PRICE_API_URL présent dans back. Migration forcée en échec (override local) : back reste Created. Écart : back ne dépend pas de priceapi (voir Decisions).
### T6: `make up` / `make down` + README
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 8c100db
- **Notes**: make up = docker compose up -d --build --wait ; make down = docker compose down (sans -v, volume pgdata conservé). README : section Docker (ports, services, down -v) et section Développer. make up puis make down vérifiés ; make db relancé ensuite (ne démarre que db).
### T7: Job CI `compose` (smoke test de la stack)
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: b6de80c
- **Notes**: Job compose : up --build --wait, curl :8000/healthz, exec -T priceapi wget /healthz, ExitCode migrate = 0, logs si échec, down -v en always(). Étapes rejouées en local : OK. NON vérifié en CI : la branche n'est pas poussée (push = accord humain, AGENTS §11).

## Decisions

- 2026-11-02 — `.specs/CLAUDE.md` absent du dépôt (référencé par le skill) : exécution poursuivie avec
  AGENTS.md comme seule source de règles.
- 2026-11-02 — nlpf-70 (dépendance) déjà mergé dans `main` (PR #7) : branche créée depuis `main` à jour.
- 2026-11-02 — Choix de l'humain au cadrage : API de prix = binaire `cmd/priceapi` dans un conteneur
  dédié (squelette `/healthz`, simulateur reporté à live-price-feed) ; front = build statique + nginx ;
  migrations = service one-shot `migrate`. À formaliser dans l'ADR 0005 (T1).
- 2026-11-02 — T3 : la cible `migrate` exécute un binaire `goose` compilé au build (`go build
  github.com/pressly/goose/v3/cmd/goose`, version figée par go.mod) sur alpine, au lieu de `go tool goose`
  sur l'image golang prévue par la spec : image 72 Mo au lieu d'environ 1 Go, pas de compilation au démarrage,
  même version d'outil et aucune dépendance ajoutée.
- 2026-11-02 — T5 : `back` ne déclare pas `depends_on: priceapi` (la spec ne l'exigeait pas) : le back doit
  démarrer même si l'API de prix est en panne (fallback dernier snapshot, AGENTS §2).

## Blockers

(aucun — blocage T1 levé le 2026-11-02 par l'acceptation de l'ADR 0005)

## Verification Report(s)
