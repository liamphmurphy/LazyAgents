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
  "setup": ["cd web && { [ -d node_modules ] || npm ci; }"],
  "processes": [
    { "name": "api", "dir": "server", "port": 8000, "command": "go run ./cmd/vesper serve",
      "env": { "DATABASE_URL": "postgres://lazyagents:lazyagents@postgres:5432/lazyagents?sslmode=disable",
               "S3_ENDPOINT": "http://s3:8333", "API_ADDR": "0.0.0.0:8000",
               "RUNNER_BACKEND": "local", "VESPER_NO_SIGNIN": "1" } },
    { "name": "web", "dir": "web", "port": 3000, "command": "npm run dev -- -H 0.0.0.0 -p 3000",
      "env": { "API_INTERNAL_URL": "http://api:8000" } }
  ],
  "routes": [
    { "path": "/api", "process": "api" },
    { "path": "/events", "process": "api" },
    { "path": "/mcp", "process": "api" }
  ],
  "openapi": "api/openapi.yaml"
}
```

- `VESPER_NO_SIGNIN=1` serves the API as the seeded local user with no
  session. The API refuses to start with it unless `PUBLIC_URL` is a
  loopback address, which the default (`http://localhost:8080`) is. Never set
  it anywhere but a disposable local instance.
- Leave `SECRETS_KEY` unset: secrets are then off, which a throwaway instance
  doesn't need.
- The inner Vesper has no runner, so its chats and runs never get answered.
  It's for clicking through the UI and trying the API, not for running
  agents.
- The first start compiles the API and starts `next dev`, which takes a few
  minutes. Call `playground_status` until it's running, and read `log` if it
  failed.
