DATABASE_URL ?= postgres://bom:bom@localhost:5432/bomstudio?sslmode=disable
export DATABASE_URL

# Entrées synthétiques explicites et reproductibles, jamais des taux réels téléchargés.
PRICE_API_PORT ?= 8090
PRICE_API_URL ?= http://127.0.0.1:$(PRICE_API_PORT)
SIM_DATASET_VERSION ?= demo-v1
SIM_SEED ?= 42
SIM_AS_OF ?= 2026-01-01T00:00:00Z
SIM_YEAR_DURATION ?= 40m
SIM_FX_PAIRS ?= [{"series_key":"fx:USD:EUR","base_currency":"USD","quote_currency":"EUR","reference":"0.92","band_percent":"5"}]
export PRICE_API_URL SIM_DATASET_VERSION SIM_SEED SIM_AS_OF SIM_YEAR_DURATION SIM_FX_PAIRS

.PHONY: run priceapi test cover sec lint migrate db dev web sqlc up down

run: ## API sur :8080
	go run ./cmd/server

priceapi: ## source de prix externe sur :8090 (ADR 0005/0014)
	PORT=$(PRICE_API_PORT) go run ./cmd/priceapi

test:
	go test ./...

cover: ## tests + couverture (indicateur, voir docs/workflow.md §5)
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

sec: ## failles connues dans les dépendances Go et npm (voir ADR 0001)
	go tool govulncheck ./...
	cd web && npm audit --omit=dev --audit-level=high

lint: web/node_modules
	go vet ./...
	go tool golangci-lint run ./...
	cd web && npm run lint

db: ## Postgres local
	docker compose up -d --wait db

migrate: db
	@if ls migrations/*.sql >/dev/null 2>&1; then go tool goose -dir migrations postgres "$(DATABASE_URL)" up; \
	else echo "migrate: aucune migration"; fi

sqlc: ## régénère le code des requêtes SQL typées (voir sqlc.yaml)
	go tool sqlc generate

web: web/node_modules
	cd web && npm run dev

dev: migrate ## Postgres + migrations, puis API et front en parallèle
	$(MAKE) -j3 run web priceapi

up: ## toute l'app en Docker (front :8000), voir ADR 0005
	docker compose up -d --build --wait

down: ## arrête la stack Docker (garde les données ; `docker compose down -v` pour repartir de zéro)
	docker compose down

web/node_modules: web/package-lock.json
	cd web && npm ci
	touch $@
