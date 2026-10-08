# SocialOS backend. `make help` lists the targets.
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

GO        ?= go
BIN       ?= bin
CMDS      := api worker migrate telegram
IMAGE     ?= socialos-backend
GOLANGCI  ?= golangci-lint

# Local services used by `make test-integration` (see LOCAL_DEV.md). Override on the command line.
TEST_DATABASE_URL ?= postgres://socialos:socialos@127.0.0.1:5432/socialos_test?sslmode=disable
TEST_REDIS_URL    ?= redis://127.0.0.1:6379/15

# Runs a command with .env loaded (if present) so run-api/run-worker/migrate use your local config.
define with_env
set -a; if [ -f .env ]; then . ./.env; fi; set +a; $(1)
endef

.DEFAULT_GOAL := help
.PHONY: help build test test-integration cover lint fmt vet tidy migrate migrate-status migrate-down run-api run-worker telegram-set-webhook telegram-delete-webhook telegram-webhook-info docker env clean

help: ## Show this help
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z_-]+:.*## /{printf "  %-18s %s\n",$$1,$$2}' $(MAKEFILE_LIST)

build: ## Build api, worker, migrate and telegram into ./bin
	@mkdir -p $(BIN)
	@for c in $(CMDS); do CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o $(BIN)/$$c ./cmd/$$c && echo "built $(BIN)/$$c"; done

test: ## Unit tests only (no Postgres/Redis needed; integration tests are skipped)
	@env -u TEST_DATABASE_URL -u TEST_REDIS_URL $(GO) test ./... -count=1

test-integration: ## Everything, including e2e/HTTP tests against real Postgres + Redis
	@echo "TEST_DATABASE_URL=$(TEST_DATABASE_URL)"; echo "TEST_REDIS_URL=$(TEST_REDIS_URL)"
	@TEST_DATABASE_URL='$(TEST_DATABASE_URL)' TEST_REDIS_URL='$(TEST_REDIS_URL)' $(GO) test ./... -count=1 -p 1

cover: ## Coverage report (integration enabled) -> coverage.out
	@TEST_DATABASE_URL='$(TEST_DATABASE_URL)' TEST_REDIS_URL='$(TEST_REDIS_URL)' $(GO) test ./... -count=1 -p 1 -coverpkg=./internal/... -coverprofile=coverage.out
	@$(GO) tool cover -func=coverage.out | tail -1

lint: ## gofmt check, go vet, golangci-lint and architecture/size tests
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	@$(GO) vet ./...
	@$(GOLANGCI) run ./...
	@$(GO) test ./internal/archtest -count=1

fmt: ## Format all Go files
	@gofmt -w .

vet: ## go vet
	@$(GO) vet ./...

tidy: ## go mod tidy
	@$(GO) mod tidy

env: ## Create .env from .env.example with a fresh ENCRYPTION_KEY (never overwrites)
	@if [ -f .env ]; then echo ".env already exists"; else \
	  cp .env.example .env; key="$$(openssl rand -base64 32)"; \
	  sed -i.bak "s|^ENCRYPTION_KEY=.*|ENCRYPTION_KEY=$$key|" .env && rm -f .env.bak; \
	  echo "created .env (edit it, then make migrate && make run-api)"; fi

migrate: ## Apply all pending migrations to DATABASE_URL (from .env or the environment)
	@$(call with_env,$(GO) run ./cmd/migrate up)

migrate-status: ## Show migration status
	@$(call with_env,$(GO) run ./cmd/migrate status)

migrate-down: ## Roll back the last migration
	@$(call with_env,$(GO) run ./cmd/migrate down)

run-api: ## Run the API on HTTP_ADDR (default :8080); needs .env (run `make env` first)
	@$(call with_env,$(GO) run ./cmd/api)

run-worker: ## Run the publish worker + reconciler; needs .env
	@$(call with_env,$(GO) run ./cmd/worker)

telegram-set-webhook: ## Register the Telegram webhook (TELEGRAM_UPDATES_MODE=webhook); URL=https://... overrides API_PUBLIC_URL
	@$(call with_env,$(GO) run ./cmd/telegram set-webhook $(if $(URL),-url '$(URL)',))

telegram-delete-webhook: ## Remove the Telegram webhook (needed before polling can be used)
	@$(call with_env,$(GO) run ./cmd/telegram delete-webhook)

telegram-webhook-info: ## Show the webhook Telegram has registered
	@$(call with_env,$(GO) run ./cmd/telegram webhook-info)

docker: ## Build the container image ($(IMAGE))
	docker build -t $(IMAGE) .

clean: ## Remove build output
	@rm -rf $(BIN) coverage.out
