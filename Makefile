DATABASE_URL ?= postgres://bom:bom@localhost:5432/bomstudio?sslmode=disable
export DATABASE_URL

.PHONY: run test cover sec lint migrate db dev web sqlc

run: ## API sur :8080
	go run ./cmd/server

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
	$(MAKE) -j2 run web

web/node_modules: web/package-lock.json
	cd web && npm ci
	touch $@
