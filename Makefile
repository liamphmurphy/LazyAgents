# Vesper developer tasks. `make help` lists them.
SHELL := /bin/bash
COMPOSE_DEV := docker compose -f docker-compose.dev.yml

.PHONY: help up down start setup login claude-login codex-login github-login stop-dev dev dev-infra api web web-deps runner local-dev local-runner gen gen-check test lint build reset

help: ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-13s %s\n", $$1, $$2}'

up: ## Full stack in Docker → http://localhost:8080
	docker compose up --build

down: ## Stop the full stack
	docker compose down

dev-infra: ## Start postgres + seaweedfs only
	$(COMPOSE_DEV) up -d --wait

API_PORT ?= 8000
WEB_PORT ?= 3000
# Agents run in hardened containers. `make local-dev` and `make local-runner` are the no-isolation fallback.
RUNNER_BACKEND ?= docker
# Turn containers reach the API on the host through this network and name.
RUNNER_NETWORK ?= bridge
RUNNER_MCP_URL ?= http://host.docker.internal:$(API_PORT)/mcp
# A key in the environment wins over .env (`make setup` creates that one).
SECRETS_KEY ?= $(shell sed -n 's/^SECRETS_KEY=//p' .env 2>/dev/null)
export SECRETS_KEY
# Processes the host runner runs at once; delegated runs and resumed turns share these slots.
RUNNER_MAX_CONCURRENT ?= 4
# Runs inherit this, so agents' DB tests use the dev Postgres (each test makes its own database) instead of starting one.
TEST_DATABASE_URL ?= postgres://lazyagents:lazyagents@127.0.0.1:5432/postgres?sslmode=disable

start: ## One command for everything: set up, sign in if needed, run the app and hardened runner → http://localhost:3000
	@API_PORT=$(API_PORT) scripts/vesper-setup.sh start

setup: ## Create .env with a SECRETS_KEY and build the agent image
	@scripts/vesper-setup.sh setup

login: claude-login codex-login github-login ## Sign Vesper in to Claude, Codex and GitHub (needs the API running)

claude-login: ## Store a Claude plan login for agent containers (needs the API running)
	@API_PORT=$(API_PORT) scripts/vesper-setup.sh login-claude

codex-login: ## Store a Codex plan login; CODEX_AUTH_FILE=~/.codex/auth.json copies an existing one (needs the API running)
	@API_PORT=$(API_PORT) scripts/vesper-setup.sh login-codex

github-login: ## Store your GitHub CLI token as git:pat so agents can push and open PRs (needs the API running)
	@API_PORT=$(API_PORT) scripts/vesper-setup.sh login-github

stop-dev: ## Kill whatever is listening on the API and web dev ports (left over from an earlier run)
	@for port in $(API_PORT) $(WEB_PORT); do \
	  pids=$$(lsof -nP -tiTCP:$$port -sTCP:LISTEN 2>/dev/null); \
	  [ -z "$$pids" ] && continue; \
	  for pid in $$pids; do echo "stopping existing process on :$$port — $$(ps -o command= -p $$pid)"; done; \
	  kill $$pids 2>/dev/null; \
	  for i in 1 2 3 4 5 6 7 8 9 10; do lsof -nP -tiTCP:$$port -sTCP:LISTEN >/dev/null 2>&1 || break; sleep 0.3; done; \
	  kill -9 $$(lsof -nP -tiTCP:$$port -sTCP:LISTEN 2>/dev/null) 2>/dev/null || true; \
	done

dev: dev-infra stop-dev ## Infra in Docker, API (hot reload) + Next dev on the host → http://localhost:3000 (replaces any API/web already on those ports)
	@trap 'kill 0' EXIT; \
	  $(MAKE) --no-print-directory api & \
	  $(MAKE) --no-print-directory web & \
	  wait

api: ## Run the API (hot reload if air is installed: go install github.com/air-verse/air@latest)
	cd server && export RUNNER_BACKEND=$(RUNNER_BACKEND) PUBLIC_URL=$${PUBLIC_URL:-http://localhost:8000}; \
	  if command -v air >/dev/null; then air; else go run ./cmd/vesper serve; fi

runner: ## Run dispatched tickets in hardened containers
	cd server && $(if $(filter docker,$(RUNNER_BACKEND)),RUNNER_NETWORK=$(RUNNER_NETWORK) RUNNER_MCP_URL=$(RUNNER_MCP_URL)) RUNNER_BACKEND=$(RUNNER_BACKEND) RUNNER_MAX_CONCURRENT=$(RUNNER_MAX_CONCURRENT) TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go run ./cmd/vesper runner

local-dev: ## Legacy: `make dev` for the no-isolation local runner
	@$(MAKE) --no-print-directory RUNNER_BACKEND=local dev

local-runner: ## Legacy: run agents directly on this machine with its claude/codex logins, no isolation
	@$(MAKE) --no-print-directory RUNNER_BACKEND=local runner

WEB_DEPS := web/node_modules/.package-lock.json

$(WEB_DEPS): web/package-lock.json
	cd web && npm ci
	@touch $@

web-deps: $(WEB_DEPS) ## Install web dependencies if the lockfile changed

web: web-deps ## Run Next dev server
	cd web && npm run dev

gen: web-deps ## Regenerate API code (Go + TS) and sqlc queries. Needs sqlc 1.31+ on PATH.
	cd server && go generate ./...
	cd server && sqlc generate
	cd web && npm run gen

gen-check: gen ## Fail if generated code is out of date
	@git diff --exit-code -- server/internal/api server/internal/store web/lib/api || \
	  (echo "generated code is stale: run 'make gen' and commit" && exit 1)

test: web-deps ## Run Go and web tests
	cd server && go test ./...
	cd web && npm run typecheck

lint: web-deps ## Vet Go, lint web
	cd server && go vet ./... && test -z "$$(gofmt -l .)"
	cd web && npm run lint

build: web-deps ## Build both apps
	cd server && go build ./...
	cd web && npm run build

reset: ## Delete dev volumes (all local data)
	$(COMPOSE_DEV) down -v
	docker compose down -v
