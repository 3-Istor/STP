# BOM Studio

Conception et pilotage de la nomenclature (BOM) d'une voiture : coût, délais, CO2, recyclabilité.
Spec : [docs/features.md](docs/features.md) · Règles du projet (humains et agents IA) : [AGENTS.md](AGENTS.md).

## Prérequis (dans WSL / Linux)

- Go (dernière version stable), make, Docker, Node ≥ 20.19

## Lancer toute l'app en Docker (démo, sans Go ni Node)

Seul Docker est nécessaire :

```bash
make up       # = docker compose up -d --build --wait
```

Front sur http://localhost:8000 (`WEB_PORT` pour changer de port) ; nginx y proxifie `/api` et `/healthz`
vers le back. La commande rend la main quand tout est prêt : `db`, `migrate` (applique les migrations puis
s'arrête ; s'il échoue, le back ne démarre pas), `back`, `priceapi` (API de prix simulée, propre conteneur)
et `web`. Back et API de prix ne sont pas exposés sur l'hôte. Voir [ADR 0005](docs/adr/0005-api-de-prix-processus-separe.md).

```bash
docker compose ps -a     # état des services (migrate : Exited (0))
docker compose logs back # journaux d'un service
make down                # arrête la stack, garde les données
docker compose down -v   # arrête et efface la base pour repartir de zéro
```

Pas de hot reload dans Docker : pour développer, arrêter la stack (`make down`) et utiliser `make dev`.

## Développer

```bash
make dev      # Postgres (Docker) + migrations, puis API :8080 et front :5173
```

Ou séparément :

```bash
make db       # Postgres local
make migrate  # migrations goose
make run      # API sur http://localhost:8080 (GET /healthz)
make web      # front sur http://localhost:5173
make test     # tests Go
make lint     # go vet + golangci-lint + oxlint
```

`DATABASE_URL` et `PORT` se surchargent par variable d'environnement.
