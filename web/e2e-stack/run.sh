#!/usr/bin/env bash
# Real-stack OpenCode chat e2e: Go API + runner (RUNNER_BACKEND=local) +
# Postgres + SeaweedFS (S3) + Next, with the deterministic fake-harness binary
# standing in for `opencode`, `claude` and `codex`. See README.md.
#
# Usage: web/e2e-stack/run.sh [playwright args...]
# Env:   E2E_DATABASE_URL  use this (disposable!) Postgres instead of starting one
#        E2E_S3_ENDPOINT   use this S3 instead of starting SeaweedFS
#        E2E_STACK_SETUP_ONLY=1  start the stack, print URLs, wait (no Playwright)
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WEB="$(cd "$HERE/.." && pwd)"
REPO="$(cd "$WEB/.." && pwd)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/lazyagents-e2e-stack.XXXXXX")"
PIDS=()
CONTAINERS=()

cleanup() {
  local code=$?
  set +e
  for pid in "${PIDS[@]:-}"; do [ -n "$pid" ] && kill "$pid" 2>/dev/null; done
  sleep 1
  for pid in "${PIDS[@]:-}"; do [ -n "$pid" ] && kill -9 "$pid" 2>/dev/null; done
  for c in "${CONTAINERS[@]:-}"; do [ -n "$c" ] && docker rm -f "$c" >/dev/null 2>&1; done
  if [ $code -ne 0 ] && [ -d "$TMP/logs" ]; then
    echo "--- stack failed (exit $code); log tails from $TMP/logs ---" >&2
    for f in "$TMP"/logs/*.log; do echo "== $f" >&2; tail -n 25 "$f" >&2; done
  fi
  [ -n "${E2E_KEEP_TMP:-}" ] || rm -rf "$TMP"
  exit $code
}
trap cleanup EXIT INT TERM
mkdir -p "$TMP/logs" "$TMP/bin" "$TMP/runs"

free_port() { node -e 'const s=require("net").createServer().listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'; }
wait_for() { # url, label, seconds
  local i
  for ((i = 0; i < $3; i++)); do curl -s -o /dev/null "$1" && return 0; sleep 1; done
  echo "timed out waiting for $2 ($1)" >&2
  return 1
}

API_PORT="$(free_port)"; WEB_PORT="$(free_port)"; STUB_PORT="$(free_port)"
API_URL="http://127.0.0.1:$API_PORT"
WEB_URL="http://127.0.0.1:$WEB_PORT"

# --- Fake CLIs and the PATH guard ------------------------------------------
echo ">> building fake-harness and lazyagents"
(cd "$REPO/server" && go build -o "$TMP/fake-harness" ./cmd/fake-harness && go build -o "$TMP/lazyagents" ./cmd/lazyagents)
for cli in claude codex opencode; do ln -s "$TMP/fake-harness" "$TMP/bin/$cli"; done
GIT_BIN="$(command -v git)" || { echo "git is required" >&2; exit 1; }
# The local runner scrubs OpenCode's environment, so the fake's script mode
# ("chat": answer without MCP; "slow" blocks until cancelled) comes from a file.
echo chat >"$TMP/bin/fake-harness.mode"
ln -s "$GIT_BIN" "$TMP/bin/git"
# The API and runner see ONLY this PATH: the fake CLIs, git, and system dirs.
# Real CLIs live in ~/.local/bin, ~/.nix-profile/bin, /opt/homebrew/bin: refuse
# to start if any of them could be found from here.
SAFE_PATH="$TMP/bin:/usr/bin:/bin"
for cli in claude codex opencode; do
  found="$(PATH="$SAFE_PATH" command -v "$cli" || true)"
  if [ "$found" != "$TMP/bin/$cli" ]; then
    echo "PATH GUARD: '$cli' resolves to '${found:-nothing}' instead of $TMP/bin/$cli; refusing to start (a real CLI could run)" >&2
    exit 1
  fi
done
[ "$(PATH="$SAFE_PATH" "$TMP/bin/opencode" --version)" = "opencode v2.2.1" ] || { echo "fake opencode --version mismatch" >&2; exit 1; }

# --- Postgres and S3 ---------------------------------------------------------
if [ -n "${E2E_DATABASE_URL:-}" ]; then
  DATABASE_URL="$E2E_DATABASE_URL"
else
  PG_PORT="$(free_port)"
  echo ">> starting disposable postgres on :$PG_PORT"
  CID="$(docker run -d --rm -e POSTGRES_USER=lazyagents -e POSTGRES_PASSWORD=lazyagents -e POSTGRES_DB=lazyagents \
    -p "127.0.0.1:$PG_PORT:5432" postgres:17-alpine)"
  CONTAINERS+=("$CID")
  for i in $(seq 1 60); do docker exec "$CID" pg_isready -U lazyagents -d lazyagents >/dev/null 2>&1 && break; sleep 1; done
  docker exec "$CID" pg_isready -U lazyagents -d lazyagents >/dev/null
  DATABASE_URL="postgres://lazyagents:lazyagents@127.0.0.1:$PG_PORT/lazyagents?sslmode=disable"
fi
if [ -n "${E2E_S3_ENDPOINT:-}" ]; then
  S3_ENDPOINT="$E2E_S3_ENDPOINT"
else
  S3_PORT="$(free_port)"
  echo ">> starting disposable seaweedfs on :$S3_PORT"
  CID="$(docker run -d --rm -p "127.0.0.1:$S3_PORT:8333" -v "$REPO/deploy/seaweedfs/s3.json:/etc/seaweedfs/s3.json:ro" \
    chrislusf/seaweedfs:4.48 server -s3 -s3.config=/etc/seaweedfs/s3.json -dir=/data \
    -master.volumeSizeLimitMB=1024 -volume.max=100 -ip=127.0.0.1 -ip.bind=0.0.0.0)"
  CONTAINERS+=("$CID")
  S3_ENDPOINT="http://127.0.0.1:$S3_PORT"
  wait_for "$S3_ENDPOINT" seaweedfs 90
fi

# --- Provider-discovery stub ---------------------------------------------------
node "$HERE/discovery-stub.mjs" "$STUB_PORT" >"$TMP/logs/stub.log" 2>&1 &
PIDS+=($!)
wait_for "http://127.0.0.1:$STUB_PORT/healthz" "discovery stub" 15

# --- API and runner -------------------------------------------------------------
# Same env for both. Every harness binary is the fake (script mode: see above).
mkdir -p "$TMP/home"
# One key for both processes (secrets are encrypted with it).
SECRETS_KEY="$(head -c 32 /dev/urandom | base64)"
stack_env() {
  exec env -i HOME="$TMP/home" PATH="$SAFE_PATH" TMPDIR="$TMP" \
    API_ADDR="127.0.0.1:$API_PORT" DATABASE_URL="$DATABASE_URL" \
    S3_ENDPOINT="$S3_ENDPOINT" SEED=false PR_SYNC_INTERVAL_S=0 \
    SECRETS_KEY="$SECRETS_KEY" PUBLIC_URL="$WEB_URL" \
    RUNNER_BACKEND=local RUNNER_DATA_DIR="$TMP/runs" RUNNER_MCP_URL="$API_URL/mcp" \
    RUNNER_CLAUDE_BIN="$TMP/bin/claude" RUNNER_CODEX_BIN="$TMP/bin/codex" RUNNER_OPENCODE_BIN="$TMP/bin/opencode" \
    RUNNER_OPENCODE_ISOLATED=true \
    LAZYAGENTS_TEST_DISCOVERY_URL="http://127.0.0.1:$STUB_PORT" "$@"
}

echo ">> starting api on :$API_PORT (runs migrations and first-boot seed)"
stack_env "$TMP/lazyagents" serve >"$TMP/logs/api.log" 2>&1 &
PIDS+=($!)
wait_for "$API_URL/api/v1/health" api 120
echo ">> starting runner"
stack_env "$TMP/lazyagents" runner >"$TMP/logs/runner.log" 2>&1 &
PIDS+=($!)

# A dummy workspace secret (through the real API) makes OpenCode's catalog list
# the stub provider's models. The value never leaves the stub.
curl -fsS -X PUT "$API_URL/api/v1/secrets" -H 'content-type: application/json' \
  -d '{"name":"harness:anthropic","value":"dummy-not-a-real-key"}' >/dev/null
curl -fsS -X PUT "$API_URL/api/v1/secrets" -H 'content-type: application/json' \
  -d '{"name":"harness:openai","value":"dummy-not-a-real-key"}' >/dev/null
# Chats are project-scoped and a fresh workspace has none.
curl -fsS -X POST "$API_URL/api/v1/projects" -H 'content-type: application/json' \
  -d '{"key":"E2E","name":"E2E stack","color":"#6366f1","repo_url":"git@github.com:example/e2e-stack.git","default_branch":"main"}' >/dev/null
# ...and a main agent to chat with (SEED=false creates no agents).
curl -fsS -X POST "$API_URL/api/v1/agents" -H 'content-type: application/json' \
  -d '{"handle":"e2e-main","name":"E2E main","kind":"main"}' >/dev/null
CATALOG="$(curl -fsS "$API_URL/api/v1/chat-models")"
for want in 'anthropic/claude-sonnet-5-5' 'openai/gpt-5' 'Claude Sonnet (fake)' 'GPT-5 Codex (fake)'; do
  grep -q "$want" <<<"$CATALOG" || { echo "chat-models catalog lacks '$want': $CATALOG" >&2; exit 1; }
done

# --- Web ---------------------------------------------------------------------------
echo ">> building and starting web on :$WEB_PORT"
(cd "$WEB" && API_INTERNAL_URL="$API_URL" npm run build >"$TMP/logs/web-build.log" 2>&1)
(cd "$WEB" && exec env API_INTERNAL_URL="$API_URL" npx next start -p "$WEB_PORT" -H 127.0.0.1) >"$TMP/logs/web.log" 2>&1 &
PIDS+=($!)
wait_for "$WEB_URL/chat" web 60

echo ">> stack up: web $WEB_URL api $API_URL"
if [ -n "${E2E_STACK_SETUP_ONLY:-}" ]; then
  echo "setup only: Ctrl-C to tear down"; wait; exit 0
fi
cd "$WEB"
E2E_STACK_BASE_URL="$WEB_URL" npx playwright test -c playwright.stack.config.ts "$@"
