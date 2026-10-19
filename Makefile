DATABASE_URL ?= postgres://bom:bom@localhost:5432/bomstudio?sslmode=disable
export DATABASE_URL

.PHONY: run test lint migrate db dev web

run: ## API sur :8080
	go run ./cmd/server

test:
	go test ./...

lint: web/node_modules
	go vet ./...
	go tool golangci-lint run ./...
	cd web && npm run lint

db: ## Postgres local
	docker compose up -d --wait db

migrate: db
	@if ls migrations/*.sql >/dev/null 2>&1; then go tool goose -dir migrations postgres "$(DATABASE_URL)" up; \
	else echo "migrate: aucune migration"; fi

web: web/node_modules
	cd web && npm run dev

dev: migrate ## Postgres + migrations, puis API et front en parallèle
	$(MAKE) -j2 run web

web/node_modules: web/package-lock.json
	cd web && npm ci
	touch $@
