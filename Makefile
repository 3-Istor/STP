DATABASE_URL ?= postgres://bom:bom@localhost:5432/bomstudio?sslmode=disable
export DATABASE_URL

# Entrées synthétiques explicites et reproductibles, jamais des taux réels téléchargés.
PRICE_API_PORT ?= 8090
PRICE_API_URL ?= http://127.0.0.1:$(PRICE_API_PORT)
SIM_DATASET_VERSION ?= demo-v4
SIM_SEED ?= 42
SIM_AS_OF ?= 2026-01-01T00:00:00Z
SIM_YEAR_DURATION ?= 40m
# 12 paires directes EUR/USD/GBP/JPY : le moteur BOM n'inverse pas les taux (ADR 0020, 0021).
SIM_FX_PAIRS ?= [{"series_key":"fx:EUR:GBP","base_currency":"EUR","quote_currency":"GBP","reference":"0.855","band_percent":"5"},{"series_key":"fx:EUR:JPY","base_currency":"EUR","quote_currency":"JPY","reference":"161","band_percent":"5"},{"series_key":"fx:EUR:USD","base_currency":"EUR","quote_currency":"USD","reference":"1.087","band_percent":"5"},{"series_key":"fx:GBP:EUR","base_currency":"GBP","quote_currency":"EUR","reference":"1.17","band_percent":"5"},{"series_key":"fx:GBP:JPY","base_currency":"GBP","quote_currency":"JPY","reference":"190","band_percent":"5"},{"series_key":"fx:GBP:USD","base_currency":"GBP","quote_currency":"USD","reference":"1.27","band_percent":"5"},{"series_key":"fx:JPY:EUR","base_currency":"JPY","quote_currency":"EUR","reference":"0.0062","band_percent":"5"},{"series_key":"fx:JPY:GBP","base_currency":"JPY","quote_currency":"GBP","reference":"0.0053","band_percent":"5"},{"series_key":"fx:JPY:USD","base_currency":"JPY","quote_currency":"USD","reference":"0.0067","band_percent":"5"},{"series_key":"fx:USD:EUR","base_currency":"USD","quote_currency":"EUR","reference":"0.92","band_percent":"5"},{"series_key":"fx:USD:GBP","base_currency":"USD","quote_currency":"GBP","reference":"0.787","band_percent":"5"},{"series_key":"fx:USD:JPY","base_currency":"USD","quote_currency":"JPY","reference":"150","band_percent":"5"}]
export PRICE_API_URL SIM_DATASET_VERSION SIM_SEED SIM_AS_OF SIM_YEAR_DURATION SIM_FX_PAIRS

.PHONY: run priceapi seed test cover sec lint migrate db dev web sqlc up down models verify risk hooks tools

run: ## API sur :8080
	go run ./cmd/server

priceapi: ## source de prix externe sur :8090 (ADR 0005/0014)
	PORT=$(PRICE_API_PORT) go run ./cmd/priceapi

seed: migrate ## jeu sample-dataset (Multipla, Valhalla), idempotent (ADR 0012, 0021)
	go run ./cmd/server seed

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

# Modèle réaliste de la vue 3D (ADR 0035) : « Car Concept », © 2024 Darmstadt Graphics Group GmbH,
# Eric Chadwick, CC-BY 4.0. Téléchargé, jamais commité (.gitignore). Compressé (meshopt) si gltfpack est
# installé, sinon copié tel quel ; le décodeur meshopt est fourni par three, aucune dépendance ajoutée.
MODEL_URL ?= https://raw.githubusercontent.com/KhronosGroup/glTF-Sample-Assets/main/Models/CarConcept/glTF-Binary/CarConcept.glb
MODEL_DIR := web/public/models

models: ## modèle 3D réaliste (téléchargement, ~12 Mo)
	@mkdir -p $(MODEL_DIR)
	@if [ -s $(MODEL_DIR)/CarConcept.glb ]; then echo "models: $(MODEL_DIR)/CarConcept.glb déjà présent (supprimer pour retélécharger)"; exit 0; fi; \
	set -e; tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	curl -fsSL --retry 3 -o "$$tmp/src.glb" "$(MODEL_URL)"; \
	[ "$$(head -c 4 "$$tmp/src.glb")" = "glTF" ] || { echo "models: le fichier téléchargé n'est pas un .glb" >&2; exit 1; }; \
	if command -v gltfpack >/dev/null 2>&1 && gltfpack -i "$$tmp/src.glb" -o "$$tmp/packed.glb" -cc -kn -km -ke; then \
		mv "$$tmp/packed.glb" $(MODEL_DIR)/CarConcept.glb; echo "models: compressé par gltfpack (meshopt)"; \
	else \
		mv "$$tmp/src.glb" $(MODEL_DIR)/CarConcept.glb; echo "models: gltfpack absent, modèle copié sans compression"; \
	fi; \
	ls -l $(MODEL_DIR)/CarConcept.glb

# --- Harnais (ADR 0039, docs/harnais.md) ------------------------------------------------
LEVEL ?= fast
verify: ## moulinette : make verify LEVEL=fast|pr|nightly, résumé dans .verify/verify-summary.md
	./scripts/verify.sh $(LEVEL)

risk: ## niveau de risque (0–3) du changement courant et point de contrôle humain
	go run ./cmd/risk

hooks: ## active les hooks git du dépôt (pre-commit = moulinette fast, commit-msg = trailer IA)
	git config core.hooksPath .githooks
	@echo "hooks actifs : .githooks/"

tools: ## outils du harnais épinglés (gitleaks, osv-scanner, actionlint, gremlins, SBOM) dans .tools/bin
	./scripts/install-tools.sh
