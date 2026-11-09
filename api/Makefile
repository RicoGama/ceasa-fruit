# Carrega as variáveis do .env se existir (o "-" na frente ignora se não existir).
-include .env
export

.PHONY: help tools db-up db-down run build test test-integration sqlc migration promote-admin fmt lint

help: ## Mostra os comandos disponíveis
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-17s\033[0m %s\n", $$1, $$2}'

tools: ## Instala as CLIs de desenvolvimento (sqlc, goose)
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest

db-up: ## Sobe o PostgreSQL (docker compose)
	docker compose -f ../docker-compose.yml up -d

db-down: ## Para o PostgreSQL
	docker compose -f ../docker-compose.yml down

run: ## Roda a API (aplica as migrations ao subir)
	go run ./cmd/api

build: ## Compila o binário em bin/api
	go build -o bin/api ./cmd/api

test: ## Roda os testes unitários (não precisam de banco)
	go test ./...

TEST_DATABASE_URL ?= postgres://ceasa:ceasa@localhost:5432/ceasa_test?sslmode=disable
test-integration: ## Roda todos os testes, inclusive os de integração com o Postgres
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -count=1 ./...

sqlc: ## Regenera o código Go a partir de internal/db/queries/*.sql
	sqlc generate

migration: ## Cria uma migration nova: make migration name=create_customers
	goose -dir internal/postgres/migrations create $(name) sql

promote-admin: ## Torna um usuário admin da plataforma: make promote-admin email=voce@exemplo.com
	go run ./cmd/promote-admin -email $(email)

fmt: ## Formata o código
	gofmt -w .

lint: ## Verificações estáticas
	go vet ./...
