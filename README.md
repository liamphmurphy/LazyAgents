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

## Develop

Requires Go 1.26+, Node 24+ and Docker for Postgres and SeaweedFS.

```sh
make dev     # infra in Docker; API on :8000 and Next on :3000 run on the host
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
| `server/` | Go module: the `lazyagents` binary (API, workers, later the runner) |
| `server/migrations/` | goose SQL, embedded and applied on boot |
| `server/internal/store/queries/` | SQL for sqlc. `internal/store` is generated. |
| `server/internal/tracker/` | Ticket domain service; every change writes its activity event in the same transaction |
| `web/` | Next.js app: a pure UI client of the API |
| `deploy/` | Caddy and SeaweedFS config |
