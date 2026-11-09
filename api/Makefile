# Load variables from .env if it exists (the leading "-" ignores a missing file).
-include .env
export

.PHONY: help tools db-up db-down run build test sqlc migration fmt lint

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

tools: ## Install dev CLIs (sqlc, goose)
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest

db-up: ## Start PostgreSQL (docker compose)
	docker compose -f ../docker-compose.yml up -d

db-down: ## Stop PostgreSQL
	docker compose -f ../docker-compose.yml down

run: ## Run the API (applies migrations on startup)
	go run ./cmd/api

build: ## Build the binary into bin/api
	go build -o bin/api ./cmd/api

test: ## Run all tests
	go test ./...

sqlc: ## Regenerate Go code from internal/db/queries/*.sql
	sqlc generate

migration: ## Create a new migration: make migration name=create_customers
	goose -dir internal/postgres/migrations create $(name) sql

fmt: ## Format code
	gofmt -w .

lint: ## Static checks
	go vet ./...
