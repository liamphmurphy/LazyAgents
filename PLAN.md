# LazyAgents — Implementation Plan

LazyAgents is an issue tracker for autonomous coding agents. It holds projects, epics, tickets, agent assignments, approvals and the context documents (PRDs, specs, ADRs, reports) that agents read and write.

The visual source of truth is `lazyagents-design/`. `DESIGN.md` there wins over the HTML mockups. This file covers everything else: architecture, data, APIs, infrastructure and the order we build in.

---

## 1. Key decision: what LazyAgents is to an agent

The design shows run time, tool calls, tokens, spend, "Pause agent", "View run log", plans with live step state, approval gates, and "comments mentioning @atlas resume the run". So LazyAgents is more than a place where agents look up tickets. It has to talk to agents while they run.

**Decision: LazyAgents is the control plane. Agents are defined in LazyAgents and run on a harness of your choice.**

- An **agent definition** (identity, role, instructions, harness, model, scopes, budget, MCP servers) is a core entity. It doesn't depend on any harness, and each project can override parts of it (§6).
- The **runner** is a Go worker that resolves the definition for a ticket's project, prepares an isolated workspace, and launches the agent headless through a **harness adapter**. v1 ships adapters for **Claude Code** and **Codex**, so the abstraction is tested against two real harnesses from the start.
- Every agent, managed or not, talks back through the same **Agent API**, offered as an MCP server and as plain REST. Through it agents claim tickets, post plans, update steps, attach artifacts and request approval.
- **Unmanaged ("bring your own") sessions** are still supported. Someone running Claude Code locally can attach to an agent definition with its token, and it gets that identity, scopes and instructions. The UI labels these sessions *unmanaged* because we can't enforce budgets or isolation on them.

---

## 2. Stack

| Layer | Choice | Notes |
|---|---|---|
| Frontend | **Next.js (App Router) + TypeScript + Tailwind v4** | A pure UI layer. All business logic stays in Go, with no Server Actions and no DB access from Next. |
| UI primitives | Radix UI (popover, dialog, dropdown, tooltip), `cmdk` for ⌘K | Unstyled and accessible. We style them with tokens. |
| Data fetching | TanStack Query + generated typed client (`openapi-typescript` + `openapi-fetch`) | SSE events invalidate queries. |
| Backend | **Go 1.25**, `net/http` (1.22+ pattern routing) or `chi` | One binary that serves the REST API, MCP, SSE and background workers. |
| API contract | **OpenAPI 3.0.3, spec-first** (`api/openapi.yaml`) → `oapi-codegen` (Go strict server) and `openapi-typescript` (web) | One source of truth, so the two sides can't drift. 3.0 rather than 3.1 because oapi-codegen's 3.1 support is incomplete. |
| DB access | `pgx/v5` + **sqlc** | Typed SQL, no ORM. |
| Migrations | **goose**, embedded in the binary, run on startup (or via `api migrate`) | |
| Background jobs | **River** (Postgres-backed queue) | Handles outline extraction, stats rollups, notifications and later the runner. No Redis. |
| Realtime | Postgres `LISTEN/NOTIFY` → Go SSE hub → browser | Enough for a single node. Swap in NATS or Redis only if we scale out. |
| Database | **Postgres 17** | Also covers full-text search (`tsvector`) for ⌘K and artifact search. |
| Object storage | **SeaweedFS** (S3 gateway) | Accessed only through the S3 API (`aws-sdk-go-v2`), so it can be swapped for MinIO, Garage or AWS S3 by changing config. |
| MCP | `github.com/modelcontextprotocol/go-sdk`, streamable HTTP at `/mcp` | Agents point their MCP config at it with a bearer token. |
| Runner | Go (same binary, `lazyagents runner`) + Docker Engine API | One container per run. Harness CLIs are baked into a `lazyagents/agent-base` image. |
| Edge | **Caddy** | One origin: `/` → web, `/api`, `/mcp`, `/events` → api. Gives cookies with no CORS and no SSE buffering problems. |

### Feedback on the proposed stack

1. **Next.js, Tailwind, Go and Postgres are all good choices.** The main risk is letting Next.js become a second backend, so we make it a pure client of the Go API from day one.
2. **SeaweedFS is fine, but most artifacts aren't blobs.** PRDs, specs, ADRs and reports are markdown that we need to version, diff, search, split into an outline (with per-section citation counts) and feed to agents. **Store markdown bodies in Postgres** (`artifact_versions.body`). Use SeaweedFS for binary uploads (design files, images, PDFs), run logs and large agent outputs. This removes a whole class of consistency problems.
3. **Write the OpenAPI spec first.** With a Go backend and a TS frontend, a hand-maintained client will drift within a week.
4. **No Redis or Kafka for now.** Postgres can do the queue (River), pub/sub (NOTIFY) and search, so compose stays at six containers (the five services plus the runner).
5. **Make MCP a top-level surface, not an afterthought.** For a tracker built for agents, the Agent API is the product. The UI is how humans supervise it.
6. **Add Caddy as a reverse proxy** so the browser sees a single origin. It's tiny and saves a lot of trouble with CORS, cookies and SSE.

---

## 3. Repository layout

```
LazyAgents/
├── PLAN.md
├── docker-compose.yml          # full stack: docker compose up → http://localhost:8080
├── docker-compose.dev.yml      # infra only (postgres, seaweedfs) for local hot reload
├── Makefile                    # make dev / gen / test / seed / reset
├── .env.example
├── api/
│   └── openapi.yaml            # REST contract (source of truth)
├── server/                     # Go module
│   ├── cmd/lazyagents/         # main: serve | migrate | seed | runner
│   ├── internal/
│   │   ├── http/               # handlers (generated interfaces + impl), middleware, SSE hub
│   │   ├── mcp/                # MCP tools → domain services
│   │   ├── domain/             # services: tickets, epics, agents, artifacts, approvals, activity
│   │   ├── store/              # sqlc queries + generated code
│   │   ├── blob/               # S3 client wrapper (SeaweedFS)
│   │   ├── jobs/               # River workers
│   │   ├── agentdef/           # definition schema, project-override merge, snapshotting
│   │   ├── runner/             # run lifecycle, workspace prep, container mgmt, credential injection
│   │   │   └── harness/        # Harness interface + adapters: claudecode/, codex/
│   │   └── auth/               # sessions (humans), API tokens (agents)
│   ├── migrations/             # goose SQL
│   └── Dockerfile
├── web/                        # Next.js app
│   ├── app/                    # routes: /, /tickets, /tickets/[key], /epics, /agents, /artifacts, /inbox
│   ├── components/             # ui/ (primitives), domain/ (TicketRow, AgentAvatar, StatusGlyph…)
│   ├── lib/api/                # generated types + client
│   ├── styles/tokens.css       # copied from lazyagents-design, mapped into Tailwind @theme
│   └── Dockerfile
├── deploy/
│   ├── Caddyfile
│   ├── agent-base/Dockerfile   # run image: git, toolchains, claude + codex CLIs
│   └── seaweedfs/s3.json       # S3 identities/keys for local
└── lazyagents-design/          # design handoff (reference only, not imported)
```

---

## 4. Domain model (initial schema)

All primary keys are UUIDv7. Every table has `created_at` and `updated_at`. Every table has a `workspace_id`, so multi-workspace support is possible later, but v1 seeds exactly one workspace.

**Core**
- `workspaces`: id, name, slug
- `users`: id, workspace_id, name, email, initials, role_title, password_hash
- `projects`: id, key (`PAY`), name, color, next_ticket_seq
- `epics`: id, project_id, name, objective, owner_user_id, source_artifact_id, start_date, target_date, health (`on_track|at_risk|off_track`), icon
- `tickets`: id, project_id, number, `key` (generated: `PAY-214`), title, description_md, status (`backlog|todo|in_progress|in_review|done`), priority (`none|low|medium|high|urgent`), epic_id, assignee_agent_id, reviewer_user_id, estimate, due_date, branch, pr_number, pr_url, pr_additions, pr_deletions, pr_state, sort_order, search tsvector
- `acceptance_criteria`: id, ticket_id, position, text, done
- `labels`, `ticket_labels`

**Agents and definitions** (see §6)
- `agents`: id, name, handle (`atlas`), color_index, role, specialty, status (`running|waiting|blocked|idle`, derived from active runs plus open approvals), max_concurrency, current_definition_version_id
- `agent_definition_versions`: id, agent_id, version, spec (jsonb: harness, model, instructions, scopes, budget, mcp_servers, setup, harness_options), created_by, created_at. Immutable. Editing a definition creates a new version.
- `agent_project_overrides`: id, agent_id, project_id, spec (jsonb partial, same shape), version, updated_by
- `agent_tokens`: id, agent_id, token_hash, kind (`run|unmanaged`), run_id (nullable), expires_at, last_used_at, revoked_at. Managed runs get a short-lived token scoped to that run.
- `agent_queue`: agent_id, ticket_id, position (tickets assigned to the agent but not yet started)
- `projects` gains `repo_url`, `default_branch`, `setup` (jsonb)

**Secrets**
- `secrets`: id, scope (`workspace|project`), project_id, name, ciphertext (AES-GCM, key from env), grants_scope (for example, `db:migrate` or `harness:anthropic`). The runner injects a secret only when the resolved scopes allow it.

**Runs** (one ticket can have many runs over its life)
- `runs`: id, ticket_id, agent_id, mode (`managed|unmanaged`), resolved_spec (jsonb snapshot), definition_version_id, override_version, harness, model, harness_session_id, container_id, state (`queued|starting|running|waiting|paused|blocked|completed|failed|cancelled`), wait_reason (`approval|human_reply|pause`), started_at, ended_at, tool_calls, input_tokens, output_tokens, cost_usd_micros, cost_source (`harness|computed`), log_blob_key
- `run_events`: id, run_id, seq, type (normalized: `message|tool_call|tool_result|usage|error|lifecycle`), payload (jsonb), at. These are the normalized harness events, with the raw stream archived to SeaweedFS.
- `run_turns`: id, run_id, seq, trigger (`start|approval|comment|resume`), input_message, started_at, ended_at, exit_code
- `plan_steps`: id, run_id, position, text, state (`pending|current|done|needs_human|failed`)
- `model_prices`: harness, model, input_per_mtok, output_per_mtok, cached_per_mtok (used when a harness doesn't report cost)

**Human-in-the-loop**
- `approval_requests`: id, run_id, ticket_id, agent_id, scope, action_summary, action_detail (jsonb), risk_summary, state (`pending|approved|denied|expired`), decided_by_user_id, decided_at, decision_note
- `comments`: id, ticket_id, parent_comment_id, author (user or agent), body_md, mentions (agent ids). A mention of an assigned agent queues a resume signal.

**Artifacts**
- `artifacts`: id, project_id, epic_id, type (`prd|spec|adr|report|design`), title, number (ADR numbering), current_version_id, author (user or agent)
- `artifact_versions`: id, artifact_id, version_label (`v3`, `v1.2`), body_md *(text artifacts)* **or** blob_key, content_type and size *(binary artifacts)*, summary, created_by
- `artifact_sections`: id, version_id, anchor, heading, level, position (extracted from markdown headings by a background job)
- `artifact_reads`: artifact_version_id, section_id (nullable), agent_id, run_id, read_at. This table feeds the "Read by" chips and the outline citation counts.
- `ticket_artifacts`: ticket_id, artifact_id, relation (`context|generated`)

**Activity** (append-only)
- `activity_events`: id, ticket_id (nullable), epic_id, actor (user or agent), verb, objects (jsonb refs), payload (jsonb, used for plan, test_run, comment, artifact_generated and approval_requested), created_at

The activity timeline, the dashboard's "Needs your attention" list and the Inbox are all **queries over this table** plus open approvals. The event is written in the same transaction as the mutation that caused it, and a `NOTIFY` fires on commit.

Actors are polymorphic: we use `actor_type` (`user|agent`) plus `actor_id` columns instead of a shared parent table. This is simpler, and the UI needs the type anyway (circles for people, rounded squares for agents).

---

## 5. Interfaces

### 5.1 REST (humans and UI) at `/api/v1`
Resource CRUD for projects, epics, tickets, labels, agents, artifacts and comments, plus:
- `GET /tickets?view=active|backlog|agent_owned|needs_human&status=…&group_by=status`
- `POST /tickets/bulk` (status, priority, assign agent, attach artifact)
- `POST /tickets/{key}/dispatch` (a human approves the ticket for its assigned agent, see §6.6)
- `POST /approvals/{id}/approve|deny` (any human in v1, with no per-scope approver roles)
- `POST /runs/{id}/pause|resume|cancel`
- `GET /dashboard?range=24h|7d|30d` (KPIs, attention list, project table, live agents, flow)
- `GET /search?q=` (⌘K across tickets, epics, artifacts and agents)
- `POST /artifacts` (markdown body or multipart upload), `POST /artifacts/{id}/versions`, `GET /artifacts/{id}/versions/{v}/content`
- `POST /artifacts/{id}/generate-tickets` (Phase 8: an agent drafts tickets from a PRD)

### 5.2 Agent API: MCP at `/mcp` (mirrored in REST at `/api/v1/agent/*`)
Requests authenticate with a per-agent bearer token. Every tool is checked against the agent's permission scopes.

| Tool | Purpose |
|---|---|
| `whoami` | Returns the agent's identity, permissions and queue |
| `next_ticket` / `claim_ticket` | Takes the next queued ticket and opens a run |
| `get_ticket` | Ticket, criteria, comments and the list of attached artifacts |
| `read_artifact(id, section?)` | Returns the markdown and records a read or citation |
| `post_plan(steps[])` / `update_step(n, state)` | The plan card shown in the timeline |
| `log_event(kind, payload)` | Test runs and other rich timeline events |
| `report_usage(tool_calls, tokens, cost)` | Run card counters. Unmanaged sessions only, since managed runs get usage from the harness stream. |
| `set_branch` / `set_pr` | The Code section |
| `check_criterion(n)` | Ticks an acceptance criterion |
| `create_artifact(type, title, body)` | Agent reports (always `REPORT`, "Generated by") |
| `comment(body)` / `create_ticket(...)` | Agents can reply and open follow-up tickets |
| `request_approval(scope, action, detail, risk)` | Records the request and returns `pending`, telling the agent to end its turn. When a human approves or denies, the runner resumes the session with the decision (§6.4). |
| `run_gated(scope, action_id, params)` | Brokered execution of an approval-gated action (for example, `db:migrate`), using a secret the agent never sees. Only works after an approval. |
| `await_human(reason)` | Ends the turn so the run waits for a reply. A comment that mentions the agent resumes it. |
| `set_status(status)` / `complete_run(outcome)` | Moves the ticket to In Review or Done, or marks it blocked |

Unmanaged sessions can't be resumed by us, so for them `request_approval` takes `wait: true`, which long-polls (bounded by the harness's MCP tool timeout and repeated on timeout). Unmanaged sessions also use `await_signal(timeout)` to pick up pause, cancel and comments.

### 5.3 Realtime at `/events` (SSE)
The browser subscribes once. The server pushes `{type, ids}` invalidation messages such as `ticket.updated`, `activity.created`, `approval.created` and `run.updated`. The client invalidates the matching TanStack Query keys. Payloads stay small and the client refetches what it needs.

### 5.4 Auth
- **Humans:** email and password (argon2id) with an HTTP-only session cookie. A first admin is seeded from env. OIDC comes later.
- **Agents:** opaque bearer tokens (`la_…`), stored hashed. Managed runs get a per-run token that expires when the run ends. Unmanaged sessions use a long-lived token, shown once at creation.

---

## 6. Agent definitions and the runner

### 6.1 Definitions

A definition describes *what the agent is*, never *how a particular CLI is invoked*. It's resolved in three layers, each overriding the one before:

```yaml
# 1. Agent definition (workspace level, versioned)
name: Atlas
handle: atlas
role: Backend engineer
harness: claude-code            # claude-code | codex
model: opus-5.5
instructions: |
  You own backend changes. Post a plan before editing. Never touch prod.
scopes:
  repo:write: allowed
  tests:run: allowed
  db:migrate: requires_approval
budget: { max_runtime: 2h, max_turns: 20 }   # optional runaway guard; no spend caps in v1
mcp_servers: []                 # extra servers; the lazyagents server is always injected
harness_options:                # escape hatch, passed only to the matching adapter
  claude-code: { permission_mode: acceptEdits }
  codex: { sandbox: workspace-write }

# 2. Project override: Payments Platform (partial; merged over the agent)
harness: codex                  # optional harness swap per project
instructions_append: "Follow ADR-012 for partitioning."
scopes: { "prod:*": denied }
mcp_servers_add: [{ name: postgres-staging, url: … }]
setup: ["make deps"]

# 3. Run snapshot: the fully resolved spec, frozen on runs.resolved_spec at start
```

Merge rules: scalar fields replace. `instructions_append` and `mcp_servers_add` append. `scopes` merge key by key, and **a project can only tighten a scope** (`allowed` → `requires_approval` → `denied`), never loosen it. `budget` takes the lower of the two values. The UI shows the resolved definition for each project next to the agent, with overridden fields marked.

### 6.2 Harness adapters

```go
type Harness interface {
    Name() string
    Capabilities() Caps // fine-grained tool gating? native cost? permission-prompt routing? resume?
    Prepare(spec ResolvedSpec, ws Workspace) (Invocation, error)   // argv, env, files to write
    Resume(spec ResolvedSpec, ws Workspace, sessionID, msg string) (Invocation, error)
    Parse(r io.Reader, emit func(Event)) error                     // native stream → normalized events
}
```

| Concern | Claude Code adapter | Codex adapter |
|---|---|---|
| Invocation | `claude -p --output-format stream-json` | `codex exec --json` |
| Instructions | Write `AGENTS.md` into the workspace (Claude Code falls back to it when there's no `CLAUDE.md`) | `AGENTS.md` |
| MCP | `--mcp-config` file with the lazyagents server and run token | `-c mcp_servers.…` overrides |
| Tool gating | `--allowedTools` / `--disallowedTools` from scopes. `--permission-prompt-tool` routes permission prompts to LazyAgents. | `--sandbox` mode and approval policy (coarser) |
| Resume | `--resume <session_id>` | `codex exec resume <session_id>` |
| Cost | Read from the result event | Computed from tokens × `model_prices` |

Use a single `AGENTS.md` for instructions across every harness. If the repo already has one, the resolved instructions go in a clearly delimited section so the repo's own content is kept.

The exact CLI flags will be checked against current harness docs while building each adapter. Each adapter also gets fixture tests (§10).

### 6.3 Scopes and enforcement

Harnesses understand tools, not our scopes. A `requires_approval` scope only holds if the agent *can't* perform the action without us, so enforcement happens in three layers:

1. **Credentials:** the runner injects a secret into the container only when its `grants_scope` is `allowed`. An agent with no staging DB credentials can't migrate staging, whatever its shell access.
2. **Brokered actions:** `requires_approval` scopes are carried out through LazyAgents MCP tools (`run_gated`), which hold the credential on the server side and run only after approval.
3. **Harness controls:** sandbox mode, allowed tools and network policy, mapped from scopes on a best-effort basis.

Each adapter declares its `Capabilities()`, and the agent page shows how closely each scope is enforced on that harness (for example, "db:migrate: brokered" or "repo:write: harness-gated"). Unmanaged sessions get only the brokered layer.

### 6.4 Run lifecycle (turn-based)

We never freeze a process partway through a turn. A run is a series of **turns**, each a single headless invocation, joined by the harness's session resume:

```
queued → starting (workspace + container) → running (turn 1)
  ├─ agent calls request_approval / await_human → turn ends → waiting
  │     human approves / denies / comments @atlas → running (turn n+1, resumed with that message)
  ├─ human clicks Pause → runner interrupts at the next safe point, turn ends → paused → Resume → next turn
  ├─ budget exceeded / Cancel → container stopped → cancelled
  └─ complete_run → completed (ticket → In Review), container removed, workspace archived
```

- **No long-lived blocking calls,** so approvals can take hours without running into tool timeouts, and nothing runs while a run waits.
- **It survives restarts:** run state is in Postgres and the session ID is stored, so the runner just resumes.
- **The workspace persists between turns** (a named volume per run). It's deleted after completion once the branch has been pushed.
- **Pause** sends SIGINT and lets the harness close the turn cleanly. If the turn doesn't end within a grace period, the runner escalates to killing the process. A paused run resumes with a "You were paused; continue." message.

### 6.5 Workspace and isolation

- One container per run from `lazyagents/agent-base`, which contains git, common toolchains and both CLIs. Projects can name their own image.
- Workspace preparation clones `projects.repo_url` (**one project = one GitHub repo** in v1), creates the branch `<handle>/<ticket-key>-<slug>`, runs `setup`, and writes `AGENTS.md` plus the MCP config.
- **Context delivery:** the ticket's attached artifacts are written to `.lazyagents/context/<type>-<slug>.md` (git-excluded) and listed in `AGENTS.md`, so any harness can read them as files. They are also available through MCP `read_artifact`. Only MCP reads count toward the "Read by" chips and section citation counts. Reading the files counts as one read per file when the run starts.
- **Git credentials (v1): a personal access token** stored as a workspace secret (`git:pat`, granting `repo:write`). The runner sets it up as a git credential helper plus `GH_TOKEN` (or the host's equivalent) so the agent can clone, push and open PRs with `gh`. **v1 supports only GitHub as a Git host.** Other hosts would come later through a per-project provider setting. It's injected only when `repo:write` is allowed. A project can override it with its own token. Known limitation: every agent shares one identity and the token's full repo access. A GitHub App with short-lived tokens limited to one repo is the planned upgrade (Phase 8).
- The container receives the per-run LazyAgents token, the harness API key (a workspace secret such as `harness:anthropic` or `harness:openai`) and any scope-granted secrets. Nothing else. Headless runs use API-key authentication, not personal logins.
- The runner manages containers through the Docker socket (Docker Desktop on a laptop). **v1 targets a single developer laptop**, where that's acceptable. It's still effectively root on the host, so it's documented as such. No TLS, no backups beyond the Docker volumes. A shared server, or a rootless or remote runner (Kubernetes job, Firecracker), can come later behind the same `runner` interface.
- Normalized events go to `run_events` (and the UI over SSE). The raw stream is archived to SeaweedFS for the run-log viewer.

### 6.6 Dispatch and scheduling

- **A ticket has one agent in v1.** No handoffs, no reviewer agents, no parallel agents on the same ticket. Agents can still create follow-up tickets.
- **Assigning doesn't start work.** Assigning a ticket puts it in the agent's queue as *assigned*. A human then **dispatches** it (the "Approve & dispatch" action on the ticket, or in bulk from the list), which moves it to *queued*. Any human can dispatch. This puts a human check in front of every run, along with the approval gates inside it.
- A River job picks up queued tickets for each agent, up to `max_concurrency`. The global limit comes from `RUNNER_MAX_CONCURRENT`, and the dashboard's "agent capacity" segments are these slots.
- An agent's queue length in the UI counts both assigned and queued tickets, with queued ones listed first.

### 6.7 GitHub sync

A River periodic job (about every 60s, plus right away when an agent calls `set_pr`) polls each open PR through the GitHub REST API using the PAT:
- PR state (draft, open, merged or closed) and +/− diff stats go into the ticket's Code section. This replaces the agent's own reported numbers.
- **When the PR merges, the ticket moves to Done** (logged as an activity event by the system). When it's closed without merging, the ticket gets a *Blocked* flag for a human to look at.
- No webhooks, which suits a laptop with no public URL. We'd switch to webhooks along with the GitHub App.

---

## 7. Frontend approach

- **Tokens → Tailwind v4:** import `tokens.css` and map its custom properties in `@theme`, for example `--color-bg: var(--bg)` and `--color-state-progress: var(--state-progress)`. Utilities then read like `bg-surface border-border text-text-tertiary`. Dark/light switching keeps working through the existing `data-theme` attribute and the OS media query.
- **Fonts:** load Inter and JetBrains Mono through `next/font`, and apply `font-feature-settings: "cv11","ss01","ss03"` globally.
- **Build domain components first,** before any screen, and check them against `foundations.html`: `StatusGlyph`, `PriorityGlyph`, `AgentAvatar`, `HumanAvatar`, `AvatarStack`, `Pill`, `Flag`, `DocTypeTag`, `LabelChip`, `FilterChip`, `Segmented`, `Button` variants, `Card`, `KpiTile`, `ProgressBar` and `StackedBar`, `PropertyRow`, `Kbd`.
- **Then the shell:** sidebar, top bar with breadcrumb, content container and optional right panel. Screens are composed from these pieces.
- **Rendering:** server components fetch the first load from the API over the internal network, so pages paint with data. Client components take over for interactive lists, popovers and live updates.
- **Accessibility rules from DESIGN.md §6 are requirements,** not nice-to-haves: real links and buttons, `aria-label` on icon buttons, and glyph shapes that carry state independently of color.
- **Visual regression:** Playwright screenshots of each seeded screen at 1440px, compared by eye against `lazyagents-design/shots/*.png` during build-out.

---

## 8. docker-compose

Running `docker compose up` gives you a working, seeded app at `http://localhost:8080`.

| Service | Image | Purpose |
|---|---|---|
| `postgres` | `postgres:17-alpine` | Volume `pgdata`. Healthcheck with `pg_isready`. |
| `seaweedfs` | `chrislusf/seaweedfs` | `weed server -s3 -s3.config=/etc/seaweedfs/s3.json -dir=/data`. Volume `seaweed`. |
| `api` | built from `server/` | Waits for healthy postgres and seaweedfs, runs migrations, makes sure buckets exist (`artifacts`, `run-logs`), seeds when `SEED=true`, then serves on `:8000` with workers in-process. |
| `runner` | same image as `api`, `lazyagents runner` | Mounts `/var/run/docker.sock`. Starts run containers on the `lazyagents` network so they can reach `api:8000/mcp`. Builds or pulls `agent-base` on first start. |
| `web` | built from `web/` | Next.js standalone output on `:3000`. `API_INTERNAL_URL=http://api:8000`. |
| `caddy` | `caddy:2-alpine` | Exposes `:8080`. Routes `/api/*`, `/mcp*` and `/events` → api, everything else → web. `flush_interval -1` for SSE. |

**Local development:** `docker compose -f docker-compose.dev.yml up` starts only postgres and seaweedfs. Then `make dev` runs the API with `air` (hot reload) and `next dev`, behind the same Caddy routing, or behind Next rewrites in dev only.

**Seed data:** the fictional workspace from the mockups (Northwind Labs, the Payments Platform, Mobile App v3, Developer Docs and Billing Migration projects, agents Atlas through Echo, ticket PAY-214 and so on). This lets us compare screens against the shots directly. It also gives us a demo script that plays the part of an agent and walks a ticket through plan → steps → approval → PR.

---

Harness API keys and `SECRETS_KEY` are set in `.env`. The app works without harness keys, but managed runs stay disabled with a clear banner until a key is added.

## 9. Build phases

Each phase ends with something you can demo.

**Phase 0: Scaffold** (1–2 days)
- Monorepo layout, Go module, Next.js app, Makefile, `.env.example`
- docker-compose with postgres, seaweedfs, api, web and caddy up and healthy (runner added in Phase 5), Caddy routing, migrations running on boot
- OpenAPI codegen wired into both Go and TS (`make gen`), plus CI running lint, test and build
- *Exit:* `docker compose up` serves a placeholder page that calls `/api/v1/health`

**Phase 1: Design system and shell**
- `tokens.css` mapped into Tailwind, fonts, every domain component in a `/dev/foundations` route that mirrors `foundations.html`
- App shell (sidebar, top bar), dark and light themes
- *Exit:* the foundations page and the empty shell match the mockups

**Phase 2: Core tracker (people only)**
- Schema and migrations for workspaces, users, projects, epics, tickets, criteria, labels and activity, plus session auth and seed data
- Tickets list: grouping, view tabs, filters, selection, bulk bar, assign popover (agents as plain records for now)
- Ticket detail: description, criteria, properties panel, activity timeline, comments, pager
- *Exit:* a person can create, triage and edit tickets end to end, and the timeline records it all

**Phase 3: Artifacts**
- Markdown artifacts with versions stored in Postgres, binary uploads to SeaweedFS, outline extraction job
- Artifacts screen (tabs, table, drop zone, preview panel), attaching artifacts to tickets, doc cards
- *Exit:* upload a PRD, attach it to a ticket, preview its outline

**Phase 4: Agent definitions and Agent API**
- Agents, versioned definitions, project overrides, the merge logic (with tightening-only scopes) and the resolved-definition view
- Tokens, encrypted secrets, MCP server and REST mirror
- Runs and turns, plan steps, approvals and the `run_gated` broker, comments that mention an agent, read tracking and citation counts, agent-generated reports
- Ticket detail shows the approval banner, run card, plan card and Code section
- Unmanaged sessions are working: `examples/fake-agent` (a script that drives the full lifecycle) plus a sample MCP config for a local Claude Code session
- *Exit:* a local, unmanaged Claude Code session picks up PAY-214, posts a plan, requests `db:migrate` approval, a human approves it, and the migration runs through the broker

**Phase 5: Runner and harness adapters**
- `agent-base` image, workspace preparation, container lifecycle, credential injection, scheduler and concurrency limits
- The `Harness` interface with **Claude Code and Codex adapters**, normalized events, cost computation, turn-based approval / comment / pause resume, budget enforcement
- Run-log viewer (normalized events live, raw log from SeaweedFS), Pause / Resume / Cancel in the UI
- Dispatch flow (single and bulk), context files in the workspace, GitHub PR sync job (merged → Done)
- *Exit:* assigning PAY-214 to Atlas runs it on Claude Code. A project override switches it to Codex, and the same approval flow works on both.

**Phase 6: Realtime and attention surfaces**
- SSE hub over LISTEN/NOTIFY with client-side invalidation
- Inbox (approvals, mentions, reviews), the ⌘K command menu backed by `/search`, and the `C` new-ticket shortcut
- *Exit:* approving in one tab resumes the run and updates another open tab within a second

**Phase 7: Dashboard, Epics, Agents screens**
- Dashboard KPIs and rollups (River periodic jobs for 7-day stats and median cycle time), agent capacity from runner slots
- Epics: featured card, Q4 timeline, card grid, health
- Agents: status filters, cards with current assignment, stats, enforcement levels for each scope, queue, plus an agent configuration editor (extending the design system, since it isn't designed yet)
- *Exit:* every designed screen is live on real data

**Phase 8: Later / stretch**
- More harnesses (Gemini CLI, a Claude Agent SDK native adapter) and a remote or rootless runner backend
- "Generate tickets" from a PRD
- GitHub App integration: short-lived tokens limited to one repo instead of the PAT, PRs authored by the bot, and webhooks for PR state and diff stats instead of agent self-reporting
- OIDC login, multiple workspaces, settings, empty and loading states (listed as "not designed yet")
- 📌 **Notifications (pinned):** approvals and dispatch requests stall work, so they'll eventually need to reach people outside the app (Slack, email or desktop push). We'll design this once the core loop works. Until then, the Inbox and the sidebar badge are the only signal.
- Spend caps per workspace or project, approver roles per scope, multiple agents per ticket (handoffs or reviewer agents), projects that span several repos, other Git hosts

---

## 10. Testing

- **Go:** unit tests for domain services and definition merging (especially the rule that scopes can only tighten). Integration tests against real Postgres and SeaweedFS (testcontainers or the dev compose), covering MCP tool contracts and the approval and signal flows.
- **Harness adapters:** recorded stream fixtures from real Claude Code and Codex runs, kept in `testdata/`, feed the `Parse` tests. A fake harness binary emits scripted streams so runner lifecycle tests (approval → resume, pause, budget kill, restart recovery) run without API keys. A nightly opt-in smoke test uses real keys.
- **Web:** component tests for glyphs and avatars (state ↔ shape mapping). Playwright e2e for the main flows: triage, assign, approve.
- **Contract:** CI fails if `make gen` produces a diff, so the generated clients always match the spec.

---

## 11. Open questions

**Decided (v1)**
- **Deployment:** a single developer laptop running Docker Desktop (§6.5).
- **Tenancy:** a single workspace. `workspace_id` columns are kept for later.
- **Approvals:** any human can approve any request, or dispatch any ticket.
- **Project ↔ repo:** one project maps to one GitHub repo.
- **Git:** GitHub only, with a PAT for clone, push and PRs (§6.5). A GitHub App comes later.
- **Run triggering:** a human dispatches each ticket. Assigning doesn't start a run (§6.6).
- **Completion:** a PR-polling job moves the ticket to Done when its PR merges (§6.7).
- **Context delivery:** both MCP and workspace files (§6.5).
- **One agent per ticket.**
- **Spend:** no caps. The runtime and turn limits on each run are optional runaway guards.
- 📌 **Notifications:** pinned for later (Phase 8).
- **Undesigned screens:** mocked in `lazyagents-design/` v0.2 (Inbox, ⌘K, new ticket, dispatch, agent config, run log, settings, states) and specified in DESIGN.md §4–5.

**Still open**
1. **Logo:** deferred. The placeholder mark stays for now.
