.PHONY: up down migrate test build lint help bridge-bootstrap bridges-up bridges-down bridges-logs

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

up: ## Start local dev stack (postgres, redis, all services). Migrations run automatically.
	docker compose up -d
	@echo "Waiting for migrate service to finish..."
	@until docker compose ps migrate | grep -q 'Exited (0)'; do \
		sleep 2; \
	done
	@echo "Migrations applied. Stack is ready."

down: ## Stop local dev stack
	docker compose down

bridge-bootstrap: ## Generate one bridge config + Synapse registration (BRIDGE=telegram|messenger|instagram)
	@test -n "$(BRIDGE)" || (echo "Usage: make bridge-bootstrap BRIDGE=telegram|messenger|instagram [TELEGRAM_API_ID=... TELEGRAM_API_HASH=...]" >&2; exit 2)
	TELEGRAM_API_ID="$(TELEGRAM_API_ID)" TELEGRAM_API_HASH="$(TELEGRAM_API_HASH)" ./adapters/matrix-mautrix/bootstrap-bridge.sh "$(BRIDGE)"

bridges-up: ## Start Telegram, Instagram, and Messenger bridges after bootstrap
	docker compose -f docker-compose.yml -f docker-compose.bridges.yml up -d --force-recreate matrix-init synapse conversation-svc
	docker compose -f docker-compose.yml -f docker-compose.bridges.yml up -d mautrix-telegram mautrix-messenger mautrix-instagram

bridges-down: ## Stop the optional Telegram, Instagram, and Messenger bridge stack
	docker compose -f docker-compose.yml -f docker-compose.bridges.yml stop mautrix-telegram mautrix-messenger mautrix-instagram

bridges-logs: ## Tail logs for the optional bridge stack
	docker compose -f docker-compose.yml -f docker-compose.bridges.yml logs -f mautrix-telegram mautrix-messenger mautrix-instagram

logs: ## Tail logs from all services
	docker compose logs -f

# ---------------------------------------------------------------------------
# Production stack
# ---------------------------------------------------------------------------

prod-up: ## Start hardened production stack (requires .env configured)
	docker compose -f docker-compose.prod.yml up -d --build

prod-down: ## Stop hardened production stack
	docker compose -f docker-compose.prod.yml down

# ---------------------------------------------------------------------------
# Migrations
# ---------------------------------------------------------------------------

migrate: ## (Re-)run all pending goose migrations — normally automatic on `make up`
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

TEST_PACKAGES ?= ./packages/go-common/... ./services/identity-svc/... ./services/workspace-svc/... ./services/conversation-svc/... ./adapters/fake/... ./adapters/matrix-mautrix/... ./tests/integration/...

test: ## Run full test suite (unit + integration; requires `make up` first)
	@echo "Waiting for postgres..."
	@until docker compose exec postgres pg_isready -U whatfunnel -d whatfunnel > /dev/null 2>&1; do sleep 1; done
	@echo "Running migrations against test DB..."
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$(DATABASE_URL)" up
	@echo "Running tests..."
	go test $(TEST_PACKAGES) -count=1 -timeout 120s

test-short: ## Run unit tests only (no postgres required)
	go test $(TEST_PACKAGES) -short -count=1 -timeout 30s

test-verbose: ## Run full test suite with verbose output
	@echo "Waiting for postgres..."
	@until docker compose exec postgres pg_isready -U whatfunnel -d whatfunnel > /dev/null 2>&1; do sleep 1; done
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$(DATABASE_URL)" up
	go test $(TEST_PACKAGES) -v -count=1 -timeout 120s

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

pw: ## Run the Playwright E2E test suite (requires `make up` and `cd apps/web && npm install` first)
	cd apps/web && npx playwright test --reporter=list

pw-ui: ## Open Playwright interactive test runner
	cd apps/web && npx playwright test --ui
