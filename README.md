# LazyAgents

An issue tracker for autonomous coding agents: projects, epics, tickets,
approvals and the context documents agents read and write.

- **Plan:** [`PLAN.md`](PLAN.md): architecture, data model, build phases
- **Design:** [`lazyagents-design/`](lazyagents-design/): `DESIGN.md` is the source of truth

## Run it

Requires Docker (Docker Desktop on a laptop).

```sh
cp .env.example .env
docker compose up --build      # → http://localhost:8080
```

The first boot seeds the Northwind Labs demo from the design mockups. There's no
login for now: you act as the workspace's first user (Maya Chen in the demo).
Set `SEED=false` and `LOCAL_USER_NAME` in `.env` to start with an empty
workspace instead.

### Running agents

Assign a ticket to an agent, then **Review & dispatch** it. The `runner`
service runs each turn in a container from `lazyagents/agent-base` (built by
compose, with Claude Code and Codex installed). Before dispatching:

- set `SECRETS_KEY` in `.env` (`openssl rand -base64 32`);
- add the harness key in **Settings → Connections & secrets**: `harness:anthropic`
  for Claude Code, `harness:openai` for Codex. Runs bill that key;
- add `git:pat` (a GitHub token) and a repo URL for the project, if agents should
  clone, push and open PRs. Open PRs are polled every 60s: merged → Done.

> **Security:** the runner mounts the Docker socket, which is root-equivalent on
> the host. v1 is meant for a single developer laptop. Works with Docker Desktop
> and OrbStack.

## Develop

Requires Go 1.26+, Node 24+ and Docker for Postgres and SeaweedFS.

```sh
make dev     # infra in Docker; API on :8000 and Next on :3000 run on the host
make runner  # optional: run dispatched tickets with this machine's own claude/codex logins
make gen     # after editing api/openapi.yaml or server/internal/store/queries (needs sqlc)
make test
make help    # everything else
```

Go integration tests need a Postgres they can create databases on:

```sh
TEST_DATABASE_URL=postgres://lazyagents:lazyagents@localhost:5432/lazyagents?sslmode=disable make test
```

## Layout

| Path | What |
|---|---|
| `api/openapi.yaml` | REST contract. Go server interfaces and the TS client are generated from it. |
| `server/` | Go module: the `lazyagents` binary (`serve`, `runner`, `migrate`) |
| `server/internal/harness/` | Claude Code and Codex adapters: invocation and stream parsing |
| `server/internal/runner/` | Queue, workspaces (Docker or local), turns, guards |
| `deploy/agent-base/` | The image managed runs execute in |
| `server/migrations/` | goose SQL, embedded and applied on boot |
| `server/internal/store/queries/` | SQL for sqlc. `internal/store` is generated. |
| `server/internal/tracker/` | Ticket domain service; every change writes its activity event in the same transaction |
| `web/` | Next.js app: a pure UI client of the API |
| `deploy/` | Caddy and SeaweedFS config |
