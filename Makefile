SHELL := /bin/bash
COMPOSE := docker compose

.PHONY: help env up down logs ps migrate test test-backend test-integration test-mcp test-frontend dev acceptance lint

help: ## List targets
	@grep -hE '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-18s %s\n", $$1, $$2}'

env: ## Create .env from .env.example with a fresh ENCRYPTION_KEY (never overwrites)
	@test -f .env && echo ".env already exists" || \
	  (sed "s#^ENCRYPTION_KEY=.*#ENCRYPTION_KEY=$$(openssl rand -base64 32)#" .env.example > .env && echo "wrote .env")

up: env ## Build and start the whole stack (postgres redis minio backend worker mcp frontend)
	$(COMPOSE) up -d --build

down: ## Stop the stack (data volumes are kept)
	$(COMPOSE) down

logs: ## Follow logs of all services
	$(COMPOSE) logs -f --tail=100

ps: ## Show service status
	$(COMPOSE) ps

migrate: ## Apply database migrations
	$(COMPOSE) run --rm migrate

test: test-backend test-mcp test-frontend ## Unit tests of every component (no services needed)

test-backend: ## Go unit tests
	$(MAKE) -C backend test

test-integration: ## Go integration + e2e tests (needs TEST_DATABASE_URL and TEST_REDIS_URL)
	$(MAKE) -C backend test-integration

test-mcp: ## MCP server tests
	cd mcp && npm ci --no-audit --no-fund && npm test

test-frontend: ## Frontend lint, typecheck and tests
	cd frontend && npm ci --no-audit --no-fund && npm run lint && npm run typecheck && npm test

acceptance: ## Full MVP flow (register → connect → MCP draft → schedule → published) against a running stack
	cd mcp && node scripts/acceptance.mjs

dev: env ## Infra in Docker, apps on the host with hot reload
	$(COMPOSE) up -d postgres redis minio
	$(COMPOSE) run --rm migrate
	@echo "Run in separate terminals:"
	@echo "  make -C backend run-api"
	@echo "  make -C backend run-worker"
	@echo "  cd mcp && SOCIALOS_API_URL=http://localhost:8080 npm run dev"
	@echo "  cd frontend && npm run dev"

lint: ## Lint everything
	$(MAKE) -C backend lint
	cd mcp && npm run typecheck
	cd frontend && npm run lint
