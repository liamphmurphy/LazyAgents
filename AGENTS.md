# Vesper: notes for agents

## Trying Vesper in a playground

To let the person click through a change (UI or API), start a playground with
`start_playground` instead of describing how to run it. Run Vesper in
**no sign-in mode** (`VESPER_NO_SIGNIN=1`). A playground can't do the normal
sign-in: `make signin` needs a shell the person doesn't have, and the
playground gateway keeps the `vesper_session` cookie for the outer Vesper.

```json
{
  "services": [
    { "name": "postgres", "image": "postgres:17-alpine",
      "env": { "POSTGRES_USER": "lazyagents", "POSTGRES_PASSWORD": "lazyagents", "POSTGRES_DB": "lazyagents" },
      "healthcheck": ["CMD-SHELL", "pg_isready -U lazyagents -d lazyagents"] },
    { "name": "s3", "image": "chrislusf/seaweedfs:4.48",
      "command": ["server", "-s3", "-dir=/data", "-ip.bind=0.0.0.0", "-master.volumeSizeLimitMB=64"],
      "tmpfs": ["/data"], "healthcheck": ["CMD-SHELL", "nc -z 127.0.0.1 8333"] }
  ],
  "setup": [
    "(cd web && { [ -d node_modules ] || npm ci; })",
    "(mkdir -p .lazyagents/pgbin && cd server && go build -o ../.lazyagents/pgbin/vesper ./cmd/vesper && go build -o ../.lazyagents/pgbin/fake-harness ./cmd/fake-harness)",
    "(cd .lazyagents/pgbin && ln -sf fake-harness claude && ln -sf fake-harness codex)"
  ],
  "processes": [
    { "name": "api", "dir": "server", "port": 8000,
      "command": "(until curl -sf http://localhost:8000/api/v1/health >/dev/null; do sleep 1; done; exec /workspace/.lazyagents/pgbin/vesper runner) & exec /workspace/.lazyagents/pgbin/vesper serve",
      "env": { "DATABASE_URL": "postgres://lazyagents:lazyagents@postgres:5432/lazyagents?sslmode=disable",
               "S3_ENDPOINT": "http://s3:8333", "API_ADDR": "0.0.0.0:8000", "VESPER_NO_SIGNIN": "1",
               "RUNNER_BACKEND": "fake", "RUNNER_MCP_URL": "http://localhost:8000/mcp", "RUNNER_DATA_DIR": "/tmp/runs",
               "RUNNER_CLAUDE_BIN": "/workspace/.lazyagents/pgbin/claude", "RUNNER_CODEX_BIN": "/workspace/.lazyagents/pgbin/codex",
               "FAKE_HARNESS_MODE": "chat-read" } },
    { "name": "web", "dir": "web", "port": 3000, "command": "npm run dev -- -H 0.0.0.0 -p 3000",
      "env": { "API_INTERNAL_URL": "http://api:8000" } }
  ],
  "routes": [
    { "path": "/api", "process": "api" },
    { "path": "/events", "process": "api" },
    { "path": "/mcp", "process": "api" },
    { "path": "/", "process": "web" }
  ],
  "openapi": "api/openapi.yaml"
}
```

- Keep the `/` route: a path no route matches goes to the first process,
  which is `api`, so without it the pages come from the Go API.
- `VESPER_NO_SIGNIN=1` serves the API as the seeded local user with no
  session. The API refuses to start with it unless `PUBLIC_URL` is a
  loopback address, which the default (`http://localhost:8080`) is. Never set
  it anywhere but a disposable local instance.
- Leave `SECRETS_KEY` unset: secrets are then off, which a throwaway instance
  doesn't need.
- The inner Vesper has a runner (`vesper runner`, fake backend), started in
  the api container once the API is healthy: a process needs a port, and the
  runner listens on none. Its `claude` and `codex` are the repository's
  `fake-harness`, so chats get canned replies through the real MCP path and
  no credentials enter the playground. Change `FAKE_HARNESS_MODE` for other
  scripts (see `server/cmd/fake-harness/main.go`).
- Wrap each setup command in `( … )` so a `cd` stays inside it, and keep each
  process's `dir` one that already exists: Docker creates a missing one as
  root, and nothing can write there afterwards.
- The first start compiles the API and starts `next dev`, which takes a few
  minutes. Call `playground_status` until it's running, and read `log` if it
  failed.
