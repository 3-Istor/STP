# BOM Studio

Conception et pilotage de la nomenclature (BOM) d'une voiture : coût, délais, CO2, recyclabilité.
Spec : [docs/features.md](docs/features.md) · Règles du projet : [CLAUDE.md](CLAUDE.md).

## Prérequis (dans WSL / Linux)

- Go (dernière version stable), make, Docker, Node ≥ 20.19

## Lancer

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
