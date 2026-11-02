---
id: docker-compose-stack
title: Lancer toute l'app avec un seul docker compose (db, migrations, back, front, API de prix)
source: manual
source_ref: manual
created: 2026-11-02
status: draft
priority: P2
depends_on: [nlpf-70]
branch: docker-compose-stack
---

# Lancer toute l'app avec un seul docker compose

## Context

Aujourd'hui `compose.yaml` ne lance que Postgres ; le back et le front tournent sur le poste via `make dev`
(Go + Node installés dans WSL). On veut qu'un seul `docker compose up` démarre toute l'application :
la base, les migrations, le back, le front et l'API de prix, pour une démo / soutenance ou un nouveau membre
sans toolchain locale. L'API de prix (simulateur, feature live-price-feed de `docs/features.md` l. 124-161)
n'existe pas encore : ce ticket pose son **binaire et son conteneur** (squelette `/healthz`), pas le simulateur.

Choix validés par l'humain (2026-11-02) : API de prix = nouveau binaire `cmd/priceapi` dans un conteneur
dédié ; front = build statique servi par nginx ; migrations = service one-shot `migrate`.

## Research Findings

### Relevant Files
- `compose.yaml` — seul service `db` (postgres:18-alpine, healthcheck `pg_isready`, volume `pgdata`). À étendre.
- `Makefile` — `make db` = `docker compose up -d --wait db` ; `make dev` = db + migrate + `run` et `web`
  en local. Doit continuer à marcher tel quel.
- `cmd/server/main.go` — câblage : `PORT` (défaut 8080), arrêt propre sur SIGTERM ; ne lit pas encore
  `DATABASE_URL`. Modèle pour `cmd/priceapi/main.go`.
- `internal/httpapi/router.go` (+ `router_test.go`) — `GET /healthz` ; modèle du routeur de l'API de prix.
- `web/vite.config.ts` — proxy dev `/api` et `/healthz` → `:8080` ; nginx doit reproduire ce proxy.
- `go.mod` — Go 1.27.2, goose et sqlc en `tool` (`go tool goose`) : le service `migrate` s'en sert, sans
  nouvelle dépendance Go.
- `migrations/00001_init.sql` — unique migration (goose).
- `internal/pricing/stub.go` — seule implémentation actuelle de `Pricing` (en mémoire) ; inchangée.
- `.github/workflows/ci.yml` — jobs `go`, `web`, `security` ; on ajoute un job `compose` (smoke test).
- `docs/adr/0003-canal-temps-reel.md` — SSE retenu : impose `proxy_buffering off` côté nginx sur `/api`.
- `README.md` — section « Lancer » à compléter.

### Existing Patterns
- Config uniquement par variables d'env lues dans `cmd/*` (`envOr`), logs `log/slog`, arrêt propre.
- Tests table-driven avec `httptest` (`router_test.go`).
- ADR au format §8 d'AGENTS.md, prochaine = `0005`, statut `Proposé` jusqu'à acceptation humaine.
- Commentaires et docs en français.

### Constraints
- Ajouter un service et une frontière (binaire `priceapi`) = décision §7 → **ADR 0005 acceptée avant le code**.
- Images de base Docker (golang, alpine, node, nginx) : versions **épinglées** (tag précis), pas `latest`.
- Pas de nouvelle dépendance Go ni npm.
- Port 8080 et 5173 sont pris par `make dev` : la stack compose expose le front sur un autre port
  (`8000` par défaut) ; le back et l'API de prix ne sont pas obligatoirement exposés sur l'hôte.
- Healthchecks sans shell impossibles en image distroless → images d'exécution `alpine` avec `wget`.
- SSE (ADR 0003) : nginx ne doit pas bufferiser `/api`.

```
docker compose up --build --wait
  db (postgres, healthy) ──► migrate (goose up, exit 0) ──► back :8080 (healthy)
                                                              ▲
  priceapi :8090 (healthy) ◄── PRICE_API_URL ─────────────────┘ (passée, pas encore lue)
  web :80 → hôte :8000  ── /api, /healthz ──► back
```

## Requirements

### Must Have
- M1 : `docker compose up --build` (une seule commande) démarre `db`, `migrate`, `back`, `priceapi` et `web`.
- M2 : ordre garanti : `migrate` attend `db` healthy ; `back` attend que `migrate` se termine avec succès ;
  un échec de migration empêche le back de démarrer.
- M3 : le front est servi sur `http://localhost:8000` et `/api` + `/healthz` y sont proxifiés vers le back.
- M4 : l'API de prix tourne dans son propre conteneur (binaire `cmd/priceapi`), répond `GET /healthz`, et le
  back reçoit son adresse par `PRICE_API_URL`.
- M5 : chaque service long a un healthcheck, de sorte que `docker compose up --wait` rende la main quand tout
  est prêt (ou échoue).
- M6 : ADR 0005 (API de prix en processus séparé + stack compose) rédigée et acceptée par un humain avant le code.
- M7 : l'existant ne casse pas : `make db`, `make migrate`, `make dev`, `make test`, `make lint` fonctionnent
  comme avant.

### Should Have
- S1 : cibles `make up` / `make down` et section README.
- S2 : job CI `compose` qui monte la stack et vérifie les `/healthz` (via nginx et API de prix).
- S3 : `.dockerignore` pour garder des contextes de build légers (`web/node_modules`, `.git`…).

### Won't Do
- Le simulateur de prix, ses routes de cotations, l'adapter HTTP du back vers l'API de prix
  (timeout, validation, fallback snapshot) : feature live-price-feed.
- Faire lire `DATABASE_URL` ou `PRICE_API_URL` par le back (rien ne s'en sert encore).
- Hot reload dans Docker (on garde `make dev` pour développer).
- Déploiement prod, TLS, gestion de secrets (mots de passe de dev en clair, comme aujourd'hui), données de démo.

## Tasks

- [x] T1: Brouillon ADR 0005 — API de prix en processus séparé, stack compose complète
  - **Files**: `docs/adr/0005-api-de-prix-processus-separe.md`
  - **Approach**: Options : (1) ne rien faire, (2) simulateur dans le process du back, (3) binaire
    `cmd/priceapi` + conteneur dédié, appelé en HTTP via un adapter (futur). Conséquences : frontière
    « externe » réelle (timeouts testables, pivot multi-source = un autre conteneur), coût : un binaire et un
    contrat HTTP de plus ; images de base épinglées ; front nginx ; migrate one-shot. Statut `Proposé`.
    **Bloquer** jusqu'à acceptation humaine.
  - **Done when**: le fichier existe au format §8 et un humain l'a passé à `Accepté` avec son nom.

- [x] T2: Binaire `cmd/priceapi` (squelette `/healthz`)
  - **Files**: `cmd/priceapi/main.go`, `internal/priceapi/router.go`, `internal/priceapi/router_test.go`,
    `internal/priceapi/doc.go`
  - **Approach**: Copier le patron de `cmd/server` (PORT défaut `8090`, slog, arrêt propre) ; routeur
    `net/http` avec `GET /healthz` → `{"status":"ok"}`. Test table-driven (`GET` 200, `POST` 405, route
    inconnue 404). Commentaire `voir ADR 0005`.
  - **Done when**: `go test ./internal/priceapi/...` passe, `go run ./cmd/priceapi` répond sur `:8090/healthz`,
    `make lint` vert.

- [x] T3: Image Go multi-stage (back, API de prix, migrate)
  - **Files**: `Dockerfile`, `.dockerignore`
  - **Approach**: Stage `build` (`golang:1.27.2-alpine`, cache `go mod download`, `CGO_ENABLED=0 go build`
    des deux binaires) ; cibles `server` et `priceapi` sur `alpine` épinglé (utilisateur non root, `wget`
    dispo pour le healthcheck) ; cible `migrate` sur l'image golang qui lance
    `go tool goose -dir migrations postgres "$DATABASE_URL" up`.
  - **Done when**: `docker build --target server .`, `--target priceapi` et `--target migrate` réussissent.

- [x] T4: Image front (build Vite + nginx)
  - **Files**: `web/Dockerfile`, `web/nginx.conf`, `web/.dockerignore`
  - **Approach**: Stage `node` épinglé : `npm ci` + `npm run build` ; stage `nginx` (alpine épinglé) qui sert
    `dist/` avec fallback `index.html`, proxifie `/api` et `/healthz` vers `http://back:8080`
    (`proxy_buffering off`, `proxy_read_timeout` long pour SSE, cf. ADR 0003).
  - **Done when**: `docker build web` réussit et l'image sert `index.html` sur `/`.

- [x] T5: `compose.yaml` complet
  - **Files**: `compose.yaml`
  - **Approach**: Garder `db` tel quel (5432 exposé, volume) ; ajouter `migrate` (`depends_on: db:
    service_healthy`), `back` (`depends_on: migrate: service_completed_successfully`, env `DATABASE_URL`
    vers `db`, `PRICE_API_URL=http://priceapi:8090`, healthcheck `wget /healthz`), `priceapi` (healthcheck),
    `web` (`depends_on: back: service_healthy`, port `${WEB_PORT:-8000}:80`, healthcheck). Back et
    priceapi non exposés sur l'hôte (pas de conflit avec `make dev`).
  - **Done when**: `docker compose up --build --wait` rend la main avec tous les services healthy et
    `migrate` en `exited (0)` ; `curl localhost:8000/healthz` renvoie 200 ; `make db` démarre toujours
    seulement `db`.

- [x] T6: `make up` / `make down` + README
  - **Files**: `Makefile`, `README.md`
  - **Approach**: `up: ## toute l'app en Docker (front :8000)` → `docker compose up -d --build --wait` ;
    `down` → `docker compose down` (sans `-v`, on garde les données). README : commande unique, ports,
    différence avec `make dev`, `docker compose down -v` pour repartir de zéro.
  - **Done when**: `make up` puis `make down` fonctionnent ; README à jour.

- [x] T7: Job CI `compose` (smoke test de la stack)
  - **Files**: `.github/workflows/ci.yml`
  - **Approach**: Nouveau job : `docker compose up -d --build --wait`, puis `curl -fsS localhost:8000/healthz`
    et `docker compose exec priceapi wget -qO- localhost:8090/healthz` ; en cas d'échec,
    `docker compose logs` ; `docker compose down -v` en `always()`. Aucun job existant modifié.
  - **Done when**: le job passe en CI sur la branche.

## Verification

### Automated Checks
- `make lint`
- `make test` (et `go test -race ./...` comme la CI)
- `docker compose config -q`
- `docker compose up -d --build --wait && curl -fsS localhost:8000/healthz && docker compose down -v`
- Job CI `compose` vert

### Manual Checks
- Ouvrir `http://localhost:8000` : la page React s'affiche, l'appel `/healthz` passe par nginx.
- `docker compose ps -a` : `migrate` en `Exited (0)`, les autres `healthy`.
- Casser volontairement une migration (copie locale, non commitée) : `back` ne démarre pas.
- `make dev` fonctionne toujours en parallèle d'une stack arrêtée (ports 8080/5173 libres).

### Acceptance Criteria
- [x] QUAND je lance `docker compose up --build` ALORS db, migrate, back, priceapi et web DOIVENT démarrer
  sans autre commande (M1)
- [x] SI une migration échoue ALORS le back NE DOIT PAS démarrer (M2)
- [x] QUAND j'ouvre `http://localhost:8000/healthz` ALORS la réponse DOIT venir du back via nginx (M3)
- [x] QUAND j'appelle `/healthz` de l'API de prix ALORS elle DOIT répondre 200 depuis son propre conteneur,
  et le back DOIT avoir `PRICE_API_URL` dans son environnement (M4)
- [x] QUAND je lance `docker compose up --wait` ALORS la commande DOIT rendre la main seulement quand tous les
  services longs sont healthy (M5)
- [x] ADR 0005 au statut `Accepté` avec un décideur (M6)
- [x] `make db`, `make dev`, `make test`, `make lint` se comportent comme avant (M7)
