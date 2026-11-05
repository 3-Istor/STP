# BOM Studio

Conception et pilotage de la nomenclature (BOM) d'une voiture : coût, délais, CO2, recyclabilité.
Spec : [docs/features.md](docs/features.md) · Règles du projet (humains et agents IA) : [AGENTS.md](AGENTS.md).

## Prérequis (dans WSL / Linux)

- Go ≥ 1.27.3 (voir `go.mod`), make, Docker ; Node 20 ≥ 20.19 ou Node ≥ 22.12 (CI : 24).

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

## Catalogue et fournisseurs

L'API gère les pièces, offres, alternatives, fournisseurs et contrats. L'onglet **Fournisseurs**
présente les offres, les prix enregistrés, les courbes et les contrats. Montants et pourcentages
transitent en chaînes décimales ; un prix manquant reste explicitement indisponible.
Voir [les routes et exemples](docs/api-catalogue-fournisseurs.md).

Le simulateur/ingestion et son signal SSE dépendent de NLPF-68 ; l'éditeur, les totaux de BOM et
l'ouverture de la Multipla dépendent de NLPF-69. Tant que ces composants ne sont pas livrés,
l'écran consulte les cotations déjà en base et permet une actualisation manuelle ; il signale
l'indisponibilité de l'actualisation automatique. Le seed complet n'est pas encore disponible.

Tests de l'interface (Node compatible avec Vite ; CI : Node 24) :

```bash
cd web
npm ci
npx --no-install playwright install --with-deps chromium firefox webkit
npm run test:e2e
```

La suite couvre les trois moteurs sur ordinateur et petit écran. Les contrôles Safari/macOS/iOS
réels et du vrai flux SSE restent distincts des tests contrôlés ; voir ADR 0010 et le suivi NLPF-67.
