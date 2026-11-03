# LazyAgents developer tasks. `make help` lists them.
SHELL := /bin/bash
COMPOSE_DEV := docker compose -f docker-compose.dev.yml

.PHONY: help up down stop-dev dev dev-infra api web runner gen gen-check test lint build reset

help: ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

up: ## Full stack in Docker → http://localhost:8080
	docker compose up --build

down: ## Stop the full stack
	docker compose down

dev-infra: ## Start postgres + seaweedfs only
	$(COMPOSE_DEV) up -d --wait

API_PORT ?= 8000
WEB_PORT ?= 3000

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
	cd server && export PUBLIC_URL=$${PUBLIC_URL:-http://localhost:8000}; \
	  if command -v air >/dev/null; then air; else go run ./cmd/lazyagents serve; fi

runner: ## Run dispatched tickets on this machine (local backend: real claude/codex CLIs and their logins, no isolation)
	cd server && RUNNER_BACKEND=local go run ./cmd/lazyagents runner

web: ## Run Next dev server
	cd web && npm run dev

gen: ## Regenerate API code (Go + TS) and sqlc queries. Needs sqlc 1.31+ on PATH.
	cd server && go generate ./...
	cd server && sqlc generate
	cd web && npm run gen

gen-check: gen ## Fail if generated code is out of date
	@git diff --exit-code -- server/internal/api server/internal/store web/lib/api || \
	  (echo "generated code is stale: run 'make gen' and commit" && exit 1)

test: ## Run Go and web tests
	cd server && go test ./...
	cd web && npm run typecheck

lint: ## Vet Go, lint web
	cd server && go vet ./... && test -z "$$(gofmt -l .)"
	cd web && npm run lint

build: ## Build both apps
	cd server && go build ./...
	cd web && npm run build

reset: ## Delete dev volumes (all local data)
	$(COMPOSE_DEV) down -v
	docker compose down -v
