# BOM Studio

Conception et pilotage de la nomenclature (BOM) d'une voiture : coût, délais, CO2, recyclabilité.
Spec : [docs/features.md](docs/features.md) · Règles du projet (humains et agents IA) : [AGENTS.md](AGENTS.md).

## Prérequis (dans WSL / Linux)

- Go ≥ 1.27.4 (voir `go.mod`), make, Docker ; Node 20 ≥ 20.19 ou Node ≥ 22.12 (CI : 24).

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
make seed     # jeu de démo : Fiat Multipla (~30 000 nœuds) et Aston Martin Valhalla (~15 000), idempotent
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

Le simulateur et l'ingestion de NLPF-68 sont disponibles. `make dev` démarre aussi priceapi sur
8090 ; `make priceapi` le lance seul. L'écran consulte les cotations
persistées et s’actualise par SSE après chaque tick validé, avec repli daté en cas de panne.

Le Makefile et Compose fixent des entrées **synthétiques** reproductibles : dataset `demo-v3`,
graine `42`, date initiale `2026-01-01T00:00:00Z`, 12 paires directes EUR/USD/GBP/JPY dans ±5 %, 40 minutes
par année de 365 jours. Ces valeurs ne sont pas des cours réels. Surcharger `SIM_DATASET_VERSION`,
`SIM_SEED`, `SIM_AS_OF`, `SIM_FX_PAIRS`, `SIM_YEAR_DURATION` pour une autre simulation ;
les bandes/références FX sont des chaînes JSON, voir [le contrat](docs/api-prix-et-devises.md).
Une reprise avec graine/date/version/références incompatibles refuse de réécrire l'historique.
La cadence et le timeout peuvent changer au redémarrage, sans régénération ni rattrapage de l'arrêt.
Le worker initialise 365 cotations quotidiennes par série, puis valide chaque nouveau jour par
transaction. Un prix aberrant ou une panne ne valide pas le tick ; les anciennes données restent intactes.

`make seed` (ou `docker compose run --rm back seed` avec la stack Docker) charge le jeu sample-dataset
avec ces mêmes paramètres `SIM_*` : 27 fournisseurs fictifs, ~420 pièces, ~1 000 offres en 4 devises,
contrats à paliers, alternatives, et les BOM Fiat Multipla (EUR) et Aston Martin Valhalla (GBP), avec
leurs 365 jours de cotations initiales. Noms et valeurs sont **fictifs** (ADR 0021). Relancer ne change
rien ; si une donnée du jeu a été modifiée (une BOM éditée, par exemple), le seed refuse et n'écrit rien :
changer `SIM_DATASET_VERSION` ou repartir d'une base neuve (`docker compose down -v`).

Une **autre** `SIM_DATASET_VERSION` n'écrase pas le jeu déjà chargé : elle ajoute un jeu complet à côté
(ADR 0012, 0021). Les fournisseurs et les BOM apparaissent alors en double, avec les mêmes noms. Pour voir
les jeux chargés :

```bash
docker compose exec db psql -U bom -d bomstudio \
  -c "SELECT dataset_version, seed, as_of, loaded_at FROM seed_loads ORDER BY loaded_at"
```

Plus d'une ligne = plusieurs jeux. Le seed ne supprime rien ; pour revenir à un seul jeu (avant une
démo, par exemple), repartir d'une base neuve :

```bash
docker compose down -v               # efface la base (et toute saisie manuelle)
make up                              # recrée la stack et applique les migrations
docker compose run --rm back seed    # un seul jeu : 27 fournisseurs, 2 BOM
```

Tests de l'interface (Node compatible avec Vite ; CI : Node 24) :

```bash
cd web
npm ci
npx --no-install playwright install --with-deps chromium firefox webkit
npm run test:e2e
```

La suite couvre les trois moteurs sur ordinateur et petit écran (60 parcours contrôlés).
La source HTTP, l’ingestion, PostgreSQL, SSE et l’UI réels sont aussi vérifiés sur les six profils :

```bash
# Depuis la racine, avec une base jetable dédiée et les navigateurs installés ci-dessus.
NLPF68_BROWSER_MODE=native DATABASE_URL='<base_de_test>' \
  go test -race -count=1 -timeout=4m -run '^TestPricingLiveBrowsers$' ./internal/pricing/pricingstore
```

Ce test crée et supprime son schéma privé ; ses routes de pilotage n’existent pas dans le serveur
livré. Il conserve les contrôles UI existants et ajoute panne/reprise, reconnexion, snapshot stable
et actualisation sans navigation. La CI contient un job `web-live` dédié. Les contrôles Safari
macOS/iOS réels restent distincts de WebKit (ADR 0013).

Le gel des prix (offre/fournisseur/BOM/catalogue) et la conversion exacte sont disponibles par
l’API ; voir [le contrat et les exemples](docs/api-prix-et-devises.md). Les évaluateurs de budgets
et de volume sont prêts pour le vrai moteur BOM ; leur persistance/publication et leurs écrans
attendent la base coordonnée NLPF-69. [État détaillé de NLPF-68](.specs/nlpf-68/delivery-report.md).
