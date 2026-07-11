.PHONY: up down migrate test build lint help

# ===========================================================================
# WhatFunnel Makefile
# ===========================================================================

DATABASE_URL ?= postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable
MIGRATIONS_DIR ?= packages/go-common/migrations
GOOSE_CMD ?= goose
GOOSE_DRIVER ?= postgres

help: ## Show this help message
	@echo "Usage: make [target]"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# Dev stack
# ---------------------------------------------------------------------------

up: ## Start local dev stack (postgres, redis, all services)
	docker compose up -d
	@echo "Waiting for postgres to be healthy..."
	@until docker compose exec postgres pg_isready -U whatfunnel -d whatfunnel > /dev/null 2>&1; do sleep 1; done
	@echo "Postgres is ready."

down: ## Stop local dev stack
	docker compose down

logs: ## Tail logs from all services
	docker compose logs -f

# ---------------------------------------------------------------------------
# Migrations
# ---------------------------------------------------------------------------

migrate: ## Run all pending goose migrations (up)
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$(DATABASE_URL)" up

migrate-down: ## Roll back the last migration
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$(DATABASE_URL)" down

migrate-status: ## Show migration status
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$(DATABASE_URL)" status

migrate-reset: ## Roll back ALL migrations (destructive!)
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$(DATABASE_URL)" reset

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

test: ## Run full test suite (unit + integration; requires `make up` first)
	@echo "Waiting for postgres..."
	@until pg_isready -h localhost -U whatfunnel -d whatfunnel > /dev/null 2>&1; do sleep 1; done
	@echo "Running migrations against test DB..."
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$(DATABASE_URL)" up
	@echo "Running tests..."
	go test ./... -count=1 -timeout 120s

test-short: ## Run unit tests only (no postgres required)
	go test ./... -short -count=1 -timeout 30s

test-verbose: ## Run full test suite with verbose output
	@until pg_isready -h localhost -U whatfunnel -d whatfunnel > /dev/null 2>&1; do sleep 1; done
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$(DATABASE_URL)" up
	go test ./... -v -count=1 -timeout 120s

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

build: ## Build all Go services
	go build ./services/api-gateway/...
	go build ./services/identity-svc/...
	go build ./services/workspace-svc/...

# ---------------------------------------------------------------------------
# Lint
# ---------------------------------------------------------------------------

lint: ## Run golangci-lint
	golangci-lint run ./...

# ---------------------------------------------------------------------------
# Tools
# ---------------------------------------------------------------------------

tools: ## Install required Go tools
	go install github.com/pressly/goose/v3/cmd/goose@latest
