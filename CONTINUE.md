# CONTINUE: handoff for the next agent

Read `PLAN.md` first for architecture and phases, then `lazyagents-design/DESIGN.md`, which is the visual source of truth.

Phases 0–7 are committed on `main` (latest: `cb8c419 dashboards, epics, kpis`). The approved Phase 8 planning workflow and the project/dependency follow-up are implemented in the working tree. The user waived browser QA for this work. Ask the user before committing.

> ⚠️ **Never exec a real harness CLI by accident.** This laptop has logged-in `claude` and `codex` in `~/.local/bin` and `~/.nix-profile/bin`. A runner test once ran the real `claude` (5 Opus sessions on the user's account) because binary substitution missed the `sh -c` wrapper. Runner tests now use `Local{OnlyBins: true}` and a PATH with only the fake harness and a `git` symlink, and fail if a real CLI is reachable. Keep both guards. Ask the user before any real-key run.

## Status

| Phase | State |
|---|---|
| 0 Scaffold | ✅ Done. `docker compose up --build` verified on 2026-11-02: all five services healthy, demo seeded, pages served through Caddy on :8080. |
| 1 Design system and shell | ✅ Done |
| 2 Core tracker | ✅ Done (deferred items are listed below) |
| 3 Artifacts | ✅ Done |
| 4 Agent definitions and Agent API | ✅ Done (see "Phase 4 progress") |
| 4.5 Wiki and agent memory | ✅ Done (the user chose it before Phase 5; see "Phase 4.5: what shipped") |
| 5 Runner and harness adapters | ✅ Done (see "Phase 5: what shipped") |
| 6 Realtime and attention surfaces | Implemented; integration exit passes; browser QA waived by user |
| 7 Dashboard, Epics, Agents | Implemented; API, metrics, queue, build and regression checks pass |
| 8 Planning workflow | Implemented, including the project/dependency follow-up; remaining stretch items are deferred. See "Phase 8: planning workflow" below. |

## What exists

### API: Go, `server/`

- **Contract:** `api/openapi.yaml` (OpenAPI 3.0.3).
  - Endpoints: `health`, `bootstrap` (workspace, me, users, projects, agents, epics, labels, counts), `tickets` (list with `view`/`project`/`status`/`assignee`/`label`/`q`), create, get, patch, `tickets/bulk`, `activity`, `comments`, criteria CRUD.
  - `TicketPatch`: an omitted field is left unchanged; `null` clears it. Go represents this as `nullable.Nullable`.
- **Codegen:** `make gen` runs:
  - `go generate` (oapi-codegen strict server, config in `internal/api/oapi.yaml`)
  - `sqlc generate` (config in `server/sqlc.yaml`; queries in `internal/store/queries/*.sql`)
  - `npm run gen` (TS types)
  - sqlc isn't a `go tool` because it needs cgo; install the binary (v1.31.1).
- **Schema:** `migrations/00002_tracker.sql`.
  - Tables: workspaces, users, projects, agents (plain records for now), epics, labels, tickets, ticket_labels, acceptance_criteria, comments, activity_events.
  - Enumerations are `text` + `CHECK`.
  - The ticket `key` is fixed at creation.
  - `tickets.search` is a generated tsvector.
- **`internal/tracker`** is the domain service:
  - Every mutation writes its activity events in the same transaction.
  - Verbs and payload shapes are documented in `activity.go`. Payloads carry display names, so the timeline needs no lookups.
  - Event `created_at` uses `clock_timestamp()` so events from one transaction keep their order.
  - Bulk updates are all-or-nothing.
  - Comment replies are one level deep: a reply to a reply attaches to the root comment.
- **No login (user's call, for now; see PLAN §5.4).** Every API request acts as the workspace's first user (`GetLocalUser`; Maya in the demo). `internal/auth` keeps only argon2id hashing (seeded users get a random, unknown password) and the context helpers. Migration `00003` drops the `sessions` table; bring both back together when sign-in returns.
- **`internal/httpapi`:**
  - `localUser` middleware attaches the acting user; with no user yet the API answers 503 `no_user`.
  - Errors map to the shared `Error` shape: 401, 404, and 422 with `field`.
- **`internal/seed`:** runs on first boot only.
  - `SEED=true`: the Northwind Labs demo built from the mockups.
    - Contents: 3 users, 8 agents, 4 projects, 7 epics, 15 labels and 14 PAY tickets with their mockup numbers.
    - PAY-214 also has its description, criteria, PR #491 and Dev's comment thread with Atlas's reply.
  - `SEED=false`: an empty workspace with one user from `LOCAL_USER_NAME`/`LOCAL_USER_EMAIL`.
- **Tests:**
  - `internal/tracker/tracker_test.go`: seeding, concurrent ticket numbering, patch event order, validation, bulk atomicity, comment threading.
  - Plus password hashing, migrations, the health handler and the local-user middleware (`httpapi/localuser_test.go`).
  - Integration tests need `TEST_DATABASE_URL`; each test creates and drops its own database.

### Web: Next 16.4, `web/`

**Read `web/node_modules/next/dist/docs/` before writing Next code.** Middleware is now `proxy.ts`. Cache Components are on, so `useSearchParams` and `params` need `<Suspense>`. `PageProps<"/route">` / `LayoutProps` are global types generated at build time; run `npm run build` if `tsc` complains about routes.

**Data**
- `lib/api/client.ts`: openapi-fetch plus `unwrap()`, which throws `ApiError`.
- `lib/api/hooks.ts`: TanStack Query hooks and query keys. Patches update the detail cache optimistically.
- Data loads client-side. PLAN §7 was updated to explain why.

**Design system**
- `globals.css` maps `tokens.css` into Tailwind:
  - Text uses `fg`: `text-fg`, `text-fg-tertiary`.
  - Surfaces: `bg-surface`, `bg-surface-raised`, `bg-panel`, `bg-sidebar`.
  - State and pills: `bg-state-*`, `bg-pill-*-bg text-pill-*-fg`.
  - Component tokens: `bg-row-hover`, `border-border-float`, `bg-scrim`, …
  - Type utilities: `type-display | title | heading | body | reading | small | micro | mono`.
  - The default Tailwind palette is cleared.
- `web/styles/tokens.css` is a copy of `lazyagents-design/tokens.css`. Edit the design copy, then copy it over.
- `components/ui`: icons, buttons, primitives (Card, Banner, EmptyState, Skeleton, Field, TextInput, …) and controls (Segmented, Toggle, Tabs).
- `components/domain`: glyphs, avatars, labels, data, markdown, meta, and `pickers` (Radix: Status/Priority/User/Epic/Label menus, plus AgentPicker as a combobox popover).
- `components/shell`: sidebar, top bar, app shell (bootstrap-driven, global `C` shortcut), theme (an inline head script plus a System/Dark/Light switch stored in `localStorage["la-theme"]`).

**Routes**
- `/`: greeting and system status
- `/tickets`
  - view tabs as links, search, agent filter
  - grouped list
  - shift-click selection, a floating bulk bar (status, priority, assign), Esc to clear
- `/tickets/[key]`
  - title and description editing (markdown)
  - criteria
  - properties panel (every field editable)
  - timeline with threaded comments, composer (⌘↵)
  - Code section
  - pager with J/K
- `/tickets/new`: the composer dialog over the list
- `/settings`: theme
- `/dev/foundations`: every component, live
- `/inbox`, `/epics`, `/agents`, `/artifacts`: placeholders naming their phase

**Verified:** typecheck, lint and build pass. A Playwright run in a real browser covered ticking a criterion, a status change, a markdown comment and threaded reply, the agent picker, J/K, a shift-click bulk priority update, search, view tabs, creating via `C` with criteria. After login was removed, a second run confirmed pages open directly and comments post as Maya. No console errors, and no horizontal overflow at 390px.

### Deferred from Phase 2, to do in the phase noted

- Dashboard KPIs (7), dispatch/Queued/approval banners (4/5), and Inbox counts (6) are now implemented.
- `needs_human` currently means `in_review`. Add open approvals in Phase 4.
- A "Display" menu and grouping options, and Status/Epic filter chips on the list. Only the agent filter and search exist.
- Drag to reorder (`sort_order` exists but nothing sets it).
- Editing and deleting comments, and @mentions (mentions resume runs in Phase 4).
- Web component tests (PLAN §10: glyph ↔ shape mapping). Add vitest.

## Local test setup without Docker

The previous sessions ran Postgres 17, SeaweedFS and sqlc from nix inside the session scratchpad:
- **Postgres:** `nix build nixpkgs#postgresql_17`, `initdb`, then `pg_ctl -o "-p 54329 -k ''"`. TCP only, because the scratch path is too long for a unix socket.
- **SeaweedFS:** `nix build nixpkgs#seaweedfs`, then `weed server -s3 -s3.config=deploy/seaweedfs/s3.json -s3.port=18333 -master.port=19333 -volume.port=18080 -filer.port=18888 -master.volumeSizeLimitMB=1024 -volume.max=100` (without the last two flags it runs out of volumes). Run it as a background job, and don't `pkill -f "weed server"` from a shell whose own command line contains that text.
- **sqlc:** `nix build nixpkgs#sqlc`.
- **API:** `DATABASE_URL=postgres://lazyagents@127.0.0.1:54329/lazyagents?sslmode=disable S3_ENDPOINT=http://127.0.0.1:18333 API_ADDR=:18000 SEED=true lazyagents serve`
- **Web:** for a standalone build pointing at it, run `API_INTERNAL_URL=http://127.0.0.1:18000 npm run build`, copy `public` and `.next/static` into `.next/standalone`, then `PORT=13000 node server.js`. Rewrites are fixed at build time, so `API_INTERNAL_URL` must be set when building.
- **Playwright:** browsers are installed in the old scratchpad under `pw/`. The e2e script there is `e2e.js`, worth porting into `web/e2e/` along with `@playwright/test`.
- **Leftover processes:** check with `lsof -iTCP:13000` before restarting. A stale `next-server` held that port twice.

## Phase 3: what shipped

**Server**
- **Schema:** `migrations/00004_artifacts.sql` adds `artifacts`, `artifact_versions`, `artifact_sections`, `artifact_reads` and `ticket_artifacts`.
  - Markdown lives in `artifact_versions.body_md`; files live in SeaweedFS under `blob_key`. A CHECK enforces exactly one.
  - `artifacts.search` is a generated tsvector over title, summary and `search_text` (the plain text of the current version).
  - ADR numbers are unique per workspace.
- **Activity order:** `00005` adds `activity_events.seq`, because seeded events share timestamps and UUIDv7 is random within a millisecond.
- **`internal/artifacts`:**
  - `outline.go`: ATX headings, skipping code fences, with dedup'd slug anchors. It's mirrored in `web/lib/slug.ts`; keep the two identical.
  - `service.go`: Create, AddVersion, UpdateMeta, RecordRead and DisplayID.
    - `.md` uploads are stored as text.
    - Other files are spooled to a temp file, then uploaded.
    - A blob is deleted if the DB transaction fails.
    - ADRs take the next number, or keep an explicit one on import.
  - **Citation counts** are per section **anchor** across versions, so they survive new versions as long as the heading stays.
- **Shared errors:** `internal/apperr` now holds `ErrNotFound` and `ValidationError`; `tracker` aliases them.
- **Tracker:** gains `AttachArtifact` / `DetachArtifact` (activity verbs `artifact_attached` / `artifact_detached`), and `Bulk` takes an optional artifact to attach.
- **API:**
  - Endpoints: `GET/POST /artifacts` (multipart), `GET/PATCH /artifacts/{id}`, `POST /artifacts/{id}/versions` (multipart), `GET …/versions/{v}/content`, `POST/DELETE /tickets/{key}/artifacts…`. `Ticket.artifacts` returns doc cards.
  - **Content safety:** only inert types (PDF, PNG, JPEG, GIF, WebP, text) are served inline. Everything else gets `attachment` + `Content-Security-Policy: sandbox` + `nosniff`, so an uploaded SVG or HTML file can't run script on the app origin.
- **Seed:** 9 markdown documents plus an SVG design export (needs S3), mirroring `artifacts.html`.
  - ADR-012.
  - Demo reads give the "Read by" chips and citation counts. In a real workspace those come from agents in Phase 4.
  - PAY-214's context and generated report are attached.
- **SeaweedFS fix (both compose files):** `-master.volumeSizeLimitMB=1024 -volume.max=100`. The defaults ran out of volumes after one bucket, and uploads failed with a 500.
- **Tests:** `internal/artifacts` covers outline/slug/summary, ADR numbering, citations across versions, files vs markdown, blob cleanup on failure, validation, and attach/bulk/detach. `internal/blob` round-trips against real S3 when `TEST_S3_ENDPOINT` is set.

**Web**
- **`/artifacts`:** search, type tabs with counts, a drop zone, the table, and a preview panel (thumbnail or real image, meta, outline with citations, Read by, tickets, New version; Generate tickets opens the Phase 8 planning flow for PRDs and SPECs).
- **`/artifacts/[id]`:** the full document with heading anchors, PDF/image preview, the outline, readers, tickets, and version history (older versions viewable read-only), plus inline summary editing and download.
- **Dialogs** (`components/domain/artifact-dialogs.tsx`):
  - Upload: several files make several artifacts, or write markdown directly.
  - New version: edit the markdown in place, or replace the file.
- **Ticket detail:** a "Context artifacts" section with doc cards ("Read by Atlas", "Generated by Atlas", "Not read by Atlas yet") and an Attach picker. The bulk bar's "Attach artifact" works now.
- **Avatars:** human avatar tints are blended into the theme token, so initials stay readable in light mode.
- **Verified in a browser:** list and preview, ADR tab, search, upload, the anchor ids, new version via the editor, viewing an old version, ticket cards, attach and detach, bulk attach. No overflow at 390px.

**Deferred**
- Archiving or deleting artifacts.
- Changing an artifact's project.
- Choosing a version label on upload.
- Embedding a PDF's own outline.
- "Generate tickets" shipped in Phase 8 below.
- Wiki links (Phase 4.5).

## Phase 4 progress

**4a: Definitions ✅**
- `internal/agentdef` holds the harness-independent definition (no DB code):
  - `Spec`, `Override`, and `Resolve`. Scalars replace; `instructions_append` and `mcp_servers_add` append; budget takes the lower of each limit.
  - Scopes only tighten. A scope the agent doesn't grant stays denied whatever the project says. Wildcards like `prod:*` cover matching scopes, including scopes added later.
  - `EnforcementFor` (credential / brokered / harness / none), harness `Capabilities`, and the scope `Catalog`.
  - Tested in `resolve_test.go`, including the loosening cases. A loosening bug was caught and fixed while writing it.
- **Schema:** `00006` adds `agent_definition_versions` (immutable, numbered per agent), `agents.current_definition_version_id` and `agent_project_overrides`.
- **`internal/agents` service:**
  - Create (default spec), SaveDefinition (writes a version and syncs the agent row's model and concurrency), SetOverride (empty deletes), CurrentSpec and Resolve.
  - `WriteVersion` is exported for seeding.
- **API:** `GET/POST /agents`, `GET /agents/{handle}`, `PUT …/definition`, `PUT …/overrides/{project}`, `GET …/resolved?project=`.
- **Seed:** all 8 agents have definitions. Atlas has v1–v7 with the notes from `agent-config.html`, plus overrides for PAY (instructions and postgres-staging MCP) and BIL (Codex, `prod:*` denied).
- **Web:**
  - `/agents`: a minimal roster plus New agent. The full screen is Phase 7.
  - `/agents/[handle]`: the configuration screen.
    - Identity, Runtime (harness capabilities), Instructions, a Permissions table with live enforcement chips (`lib/enforcement.ts` mirrors the Go rules), MCP servers and harness options.
    - "Save as vN" with ⌘S. The version note is generated from the diff (`describeChanges`).
    - Right panel: overrides list, override dialog (only offers tighter modes), a "Resolved for [project]" panel, and versions.
  - `TextInput`/`TextArea` are full width unless given a width class.
  - `lib/format.ts` gains `ago()`.
- **Now implemented:** Tokens (4b), Overview and paginated Runs (7).

**4b: Tokens and secrets ✅**
- **Schema:** `00007` adds `agent_tokens` (sha256 hash, a display prefix `la_xxxx`, kind `run|unmanaged`, expiry, revocation) and `secrets`.
  - Secrets hold AES-256-GCM nonce and ciphertext plus a last-4 preview (empty under 12 characters).
  - Secret names are unique per workspace and per project.
- **`internal/agents/tokens.go`:** IssueToken (plaintext returned once), ResolveToken (unknown, revoked and expired all give `ErrBadToken`; last-used is touched at most once a minute) and RevokeToken.
- **`internal/secrets`:**
  - The key comes from `SECRETS_KEY` (base64, 32 bytes). Without it, `ErrNoKey` → 409 with instructions.
  - The ciphertext is bound by AAD to workspace + project + **name**, not the row id. Upserts keep the old id, so binding to the id would make replaced values undecryptable.
  - `Reveal` is server-side only (broker, runner). A project secret beats the workspace one.
  - `DefaultGrants` maps `git:pat` → `repo:write`, `db:staging` → `db:migrate`, and so on.
- **API:** `GET/POST /agents/{handle}/tokens`, `DELETE …/tokens/{id}`, `GET/PUT /secrets`, `DELETE /secrets/{id}`. Values are never returned.
- **Config:** `PUBLIC_URL` (the MCP URL shown with tokens; `make api` defaults it to `:8000`) and `SECRETS_KEY` (now in compose and `.env.example`).
- **Web:**
  - Agent **Tokens** tab (`?tab=tokens`): create → shows the token once, with copy-ready `claude mcp add --transport http … --header "Authorization: Bearer …"` and `codex mcp add … --bearer-token-env-var` commands (checked against the installed CLIs' `--help`), plus revoke.
  - **Settings → Connections & secrets:** Anthropic, OpenAI and GitHub status; a write-only form (name with known-name suggestions, value, workspace or project scope); a table with grants and previews; a banner when the key is missing.
  - `components/ui/copy-button.tsx`.
- **Tests:** `secrets_test.go` (wrong key, values moved between names or projects, short key, replace-then-reveal, project precedence) and `agents/tokens_test.go`.

**4c: Runs, approvals and the broker ✅**
- **Schema:** `00008` adds `runs` (a frozen `resolved_spec`, state, wait_reason, usage; one live run per ticket via a partial unique index), `run_turns`, `run_events`, `plan_steps` and `approval_requests` (with `executed_at` and `result`).
  - FKs from `agent_tokens.run_id` and `artifact_reads.run_id`.
- **`internal/broker`:** `Registry` maps scope → action. The first action is `db:migrate` → `SQLMigration`.
  - It runs `detail.sql` against the `db:staging` secret in one transaction, with a 60s statement timeout and a 5s lock timeout.
  - The secret is scrubbed from error text.
- **`internal/runs` (`service.go`, `approvals.go`):**
  - Claim: assignee only. Re-claiming returns the live run. It freezes the resolved definition, moves the ticket to In Progress and sets agent status.
  - Plan and steps (one current at a time), LogEvent (`test_run` → timeline), ReportUsage (unmanaged only), SetBranch, SetPR (GitHub URLs only), CheckCriterion, CreateReport (report + generated attachment), Comment, Complete.
  - **RequestApproval** checks the run's *frozen* scopes:
    - allowed → `not_required`;
    - denied → error;
    - requires_approval → pending, and the run waits.
  - **RunGated** executes exactly the approved `action_detail`, once.
    - A missing secret releases the claim, since nothing ran.
    - A failed run uses the approval up.
  - WaitApproval and AwaitSignal are polling long-polls (≤50s).
  - **Decide** resumes the run with a new turn carrying the decision text.
  - AwaitHuman; `OnHumanComment` resumes only on an `@handle` mention (`Mentions`).
  - SetRunControl handles pause, resume and cancel.
  - `RecomputeAgentStatus` derives agent status from live runs.
- **Human REST:** `POST /approvals/{id}/{approve|deny}`, `GET /runs/{id}` (turns, approvals, events after `?after=seq`), `POST /runs/{id}/{pause|resume|cancel}`.
  - `Ticket.run` (latest run, with plan and scopes) and `Ticket.approvals` (pending).
  - Bootstrap's inbox count = pending approvals.
  - Human comments call `OnHumanComment`.
- **Shared:** `apperr.Conflict` → 409; `tracker.Record` and `tracker.AgentActor` are exported for other domains.
- **Tests:** `internal/runs/runs_test.go` covers the whole lifecycle, including migrating a throwaway "staging" DB exactly once. `broker_test.go` covers rollback.

**4d: Agent API ✅**
- **`internal/agentapi`:** one tool table (`Tools`, built with generic `define`) serves MCP at `/mcp` (go-sdk v1.8.0, streamable HTTP) and REST at `POST /api/v1/agent/tools/{name}` (`GET /api/v1/agent/tools` lists them).
- **Auth:** `auth.RequireBearerToken` with a verifier that resolves the agent token on *every* request, so revocation is immediate even mid-session. The caller travels in `TokenInfo.Extra`. Agent paths bypass the local-user middleware.
- **Tools:** whoami, next_ticket, claim_ticket, get_ticket, read_artifact (by id or `ADR-012`; optional `section` anchor via `artifacts.SectionText`; reads recorded against the run), post_plan, update_step, log_event, report_usage, set_branch, set_pr, check_criterion, create_artifact (reports only), comment, create_ticket, request_approval, wait_for_approval, run_gated, await_human, await_signal, complete_run.
- **Errors:** domain errors reach the agent verbatim; anything else is "internal error" and logged (`PublicError`).
- **Tests:** `internal/httpapi/agentapi_test.go` drives PAY-214 end to end through a real MCP client, including the person approving over REST, the broker migrating staging, the ticket reflecting the run, and revoked tokens failing on both transports.

**4e: Ticket UI ✅** (`app/(app)/tickets/[key]/run-parts.tsx`)
- `ApprovalBanner`: one per pending approval. Shows the scope, action and risk. "View action" shows the exact SQL with "LazyAgents runs exactly this…". An optional note goes with Deny or Approve & continue.
- `RunCard` in the properties panel: state pill, model · harness · unmanaged, run time, tool calls, tokens, spend, permissions (approval scopes amber), outcome, Pause/Resume and Cancel.
- `PlanCard` renders the live `ticket.run.plan` under the latest `plan_posted` event of that run.
- Timeline sentences for every run verb (`timeline.tsx`).
- **Live updates until SSE (Phase 6):** a ticket with a live run polls the ticket and activity every 5s (`hasLiveRun`); bootstrap refetches every 20s for the inbox badge.
- **Helpers:** `lib/format.ts` gains `duration`, `compact`.

**4f: Exit demo ✅**
- `server/cmd/fake-agent` acts as Atlas over real MCP. `examples/README.md` covers Claude Code, Codex and the scripted agent.
- **Verified in a browser:**
  - The fake agent claimed PAY-214, read the 3 context documents, posted the plan and ran "tests", then requested `db:migrate`.
  - The banner appeared by polling, showing the SQL. Approving with a note resumed the agent with that note.
  - The broker created the partitioned table in a separate `staging_demo` database.
  - The ticket moved to In Review with the run card filled in.
- **To reproduce locally:**
  1. Set the `db:staging` secret to a scratch DB.
  2. Mint a token on Atlas's Tokens tab.
  3. Run `LAZYAGENTS_TOKEN=… go run ./cmd/fake-agent -url http://localhost:8000/mcp`.

**Known gaps (later phases):**
- **Phase 5:** the run log viewer (`GET /runs/{id}` exists; no UI yet); managed runs, dispatch and the runner; turn-based harness resume.
- **Phase 6:** Inbox and SSE are implemented.
- **Phase 7:** the agent Overview/Runs tabs are implemented.

## Phase 4.5: what shipped

**Server**
- **Schema (`00009_wiki.sql`):**
  - `artifacts.type` gains `note`. Notes have `path` (unique per workspace, case-insensitive), and `project_id` is now nullable for notes only.
  - `artifact_aliases` keeps old paths of moved pages.
  - `artifact_versions.state` (`published|suggested|rejected`) plus `based_on_version_id` and `decided_by/at`.
  - `knowledge_links`: one row per `[[link]]` in current content. Source kinds: artifact, ticket, comment. Targets: artifact, section, ticket, epic, agent, or unresolved.
- **`internal/links`** (below artifacts and tracker, so both can index):
  - `Parse` skips code fences and spans and reads `\|` in tables. `web/lib/wikilinks.ts` mirrors it.
  - `Resolve` order: `@handle` → `ADR-NNN` → ticket key → note path → note title (shortest path wins) → other artifact title → epic name → alias. A note that's only a suggestion doesn't resolve.
  - `Index` replaces a source's links in the same transaction as the save. `ResolvePending` re-resolves every ghost after anything nameable is created.
  - `Slug` moved here; `artifacts.Slug` delegates.
- **Where links are indexed:**
  - artifact versions, but only published ones (`artifacts.writeVersion`/`makeCurrent`);
  - ticket create and description edits;
  - comments.
- **`internal/wiki`:**
  - `notes.go`: `SaveAsUser` publishes silently. `AgentWrite` applies §12.5 through `Decide`:
    - the agent's own `agents/<handle>/` is always direct;
    - another agent's namespace is refused;
    - other pages follow `wiki:write`, and **unset means suggest**.
  - Agent writes to its own namespace are listed on `agents/<handle>/Memory`, which is created on first write.
  - `DecideSuggestion`: accepting relabels the version `vN` and publishes it. Rejecting the only suggestion of a new page deletes the page.
  - `Rename` adds an alias. Saving to an old path follows the alias; `mode: create` takes the path over instead.
  - `backlinks.go`: `Backlinks` gives one entry per source with snippets and cited anchors. `UnlinkedMentions` uses FTS or ILIKE candidates, then a boundary-checked match outside code and links. `LinkMention` rewrites the first mention (a new version, or a description patch). `LookupRef` handles refs typed by agents.
  - `graph.go`: `LocalGraph` (BFS, ≤60 nodes; comment links count as their ticket's), `Search` (`ts_rank` × (1 + ln(1 + backlinks))), `Autocomplete`, `LinkText`.
  - `memory.go`: `RunContext` returns, in order:
    1. the Memory index;
    2. notes linked from the ticket, its comments and its attached artifacts;
    3. notes linking to the ticket or its artifacts, the agent's own first;
    4. with inject `search`, note hits for the ticket title.

    It's trimmed to `max_tokens` (about 4 bytes a token). `RecordContextReads` counts each delivered page as one read.
  - `vault.go`: export and import as a zip with frontmatter. Import is one transaction; `project` frontmatter applies to new pages.
- **Agent definitions:** `spec.memory {enabled, inject: index|linked|search, max_tokens}` (pointer; nil means the default linked/4000), carried into `Resolved.Memory`. `wiki:write` is `APIOnly` in the catalog, so it's always "brokered" (mirrored in `web/lib/enforcement.ts`).
- **Agent API:**
  - New tools: `search_knowledge`, `read_page` (records reads and section citations; lists outgoing links), `write_note`, `get_backlinks`.
  - `claim_ticket` returns `memory` pages and a hint to write notes.
- **REST:**
  - `/wiki/pages` (GET tree, POST create), `/wiki/page?path=`, `PUT /wiki/pages/{id}` (body and/or path, `base_version_id` → 409 when stale), `/wiki/suggestions` (list, and `/{versionId}/{accept|reject}`), `/wiki/export`, `/wiki/import`.
  - `/knowledge/resolve|suggest|backlinks|link-mention|graph|search`.
  - Bootstrap `counts.wiki_suggestions`, which also adds to `inbox`.
  - `ListArtifacts` "All" excludes notes; there's a `note` count.
- **Seed:** three team pages, Atlas's memory note and index, ADR-012 attached to PAY-224, and a pending Sentry suggestion on the runbook.
- **Fix found along the way:** `await_signal` built its cursor from host time but compared it to Postgres timestamps. On Docker Desktop the DB clock lags by milliseconds, so replies were skipped (`TestRunLifecycle` failed at HEAD too). Cursors now advance only to DB timestamps the agent was given. A comment from a transaction that started before the last seen timestamp but committed after it can still be missed (rare; fix with a sequence column like `activity_events.seq` if it matters).
- **Tests:**
  - `links/parse_test.go`;
  - `wiki/wiki_test.go`: ghosts, rename and aliases, ticket and comment links, unlinked mentions and link-it, the permission matrix and suggestions, `RunContext` (including the exit scenario, budget, index-only, disabled, search), graph, search, autocomplete, vault round trip;
  - `httpapi/agentapi_test.go` drives the four tools over real MCP.

**Web**
- **Wiki links:** `lib/wikilinks.ts` rewrites `[[…]]` into `wikilink:` links before react-markdown, and `Markdown` resolves them in one `/knowledge/resolve` call per body, so links render in descriptions, comments, artifacts and pages. `Markdown` is now a client component.
- **`components/domain`:**
  - `wikilink.tsx`: `WikiLink`, ghost links that create the page, and `TargetMark`;
  - `link-editor.tsx`: textarea with `[[` autocomplete;
  - `backlinks.tsx`;
  - `local-graph.tsx`.
- **`/wiki` and `/wiki/[...path]`** (`app/(app)/wiki/_components`): page tree with Agent memory, page view (E to edit, ⌘S to save, path rename), suggestion cards with a folded line diff (`lib/diff.ts`), right panel (graph, backlinks, outline, read by, history), and home (search, suggestions, recent pages, memory, import/export). New-page dialog.
- **Elsewhere:**
  - Backlinks on artifact detail and the ticket panel.
  - The Artifacts "Wiki pages" tab.
  - `/artifacts/{note id}` redirects to the wiki unless `?versions=1`.
  - A Memory card on the agent definition.
  - Wiki nav item with an amber suggestions count.
  - Icons `wiki` and `graph`; token `--doc-note`.
- **Verified in a browser (Playwright, 1440 and 390px, no console errors, no overflow):**
  - tree navigation and link rendering;
  - accepting a suggestion;
  - a ghost click creating a page, then `[[PAY-2` autocomplete and ⌘S;
  - the ghost resolving on the source page;
  - rename and old-path redirect;
  - ADR-012 backlinks and unlinked mentions;
  - comment links and ghosts;
  - the Notes tab and the Memory card.
- **Exit demo, run live:** the fake agent on PAY-214 writes memory; claiming PAY-224 returns `agents/atlas/Partition rollback gotchas` ("Links to ADR-012"), recorded as reads.

**Deferred (wiki)**
- `#tags`, rendering embeds (`![[…]]` currently renders as a link), rewriting link text when a page moves (aliases cover resolution), and the workspace graph view.
- Deleting pages. Suggestions show on the page and the wiki home; the Inbox (Phase 6) should list them too.
- ⌘K search now ships in Phase 6; `/knowledge/search` remains the wiki-specific search API.
- The runner (Phase 5) should write `RunContext` pages to `.lazyagents/context/` and list them in `AGENTS.md` (§12.3). The function is ready; `claim_ticket` already uses it.
- Open questions §11 (2)–(4) still stand. Implemented defaults: suggestions, no embeddings, zip import/export.

## Phase 5: what shipped

**Server**
- **Schema (`00010_runner.sql`):**
  - `run_turns.status` (`pending|running|done`; managed turns start pending, unmanaged start done), `process_started_at`, `log_key` and `usage`.
  - `runs.dispatched_by_user_id`, `runner_id`, `workspace_volume` and `idle_turns`.
  - A `runners` heartbeat table.
  - `tickets.pr_synced_at` and `pr_closed_unmerged`.
- **Dispatch (`runs.Dispatch`):** all or nothing. It creates a `queued` managed run with a pending `start` turn (`runs.StartPrompt`). Activity `dispatched`.
  - Unmanaged `claim_ticket` on a dispatched ticket is refused.
  - Queued runs don't count toward agent status.
- **`internal/harness`:**
  - **Interface:** `Invocation(ws, turn)` returns argv, env and files; `NewParser(model)` turns a line stream into normalized events plus a `Result` (session, usage, cost, error).
  - **Claude Code** (`claude -p … --output-format stream-json --verbose --mcp-config .lazyagents/mcp.json --strict-mcp-config --permission-mode <opt|acceptEdits> --permission-prompts none --allowedTools/--disallowedTools [--resume]`):
    - tool rules come from scopes;
    - the run token goes into the git-excluded MCP config;
    - `AnthropicModel` maps `opus-5.5` to `claude-opus-5-5`;
    - harness-reported cost.
  - **Codex** (`codex exec [resume] --json --skip-git-repo-check -m … -c approval_policy="never" -c sandbox_mode=… -c mcp_servers.lazyagents.url/bearer_token_env_var …`):
    - network access is on only when `repo:write` is allowed;
    - cost is computed from `harness.Prices` (unknown models leave cost unset).
  - **Flags** were checked against Claude Code 2.1.316 and codex-cli 0.166.0. Codex headless auth is `CODEX_API_KEY`, which I verified.
  - **Fixtures:** the `*-auth-error.jsonl` envelopes were recorded from the real CLIs with invalid keys (no spend). The tool-call fixtures are composed in the same shapes (`testdata/README.md`).
  - **`AgentsMD`:** our section sits between markers above the repo's own `AGENTS.md`.
- **`internal/runner`:**
  - **Backends:**
    - `Docker`: Engine API over the socket (no SDK). One named volume per run at `/workspace`; files go in by tar upload; attach before start; `Init` so SIGINT reaches the harness; cleanup by label.
    - `Local`: host processes. `Bins`/`OnlyBins` control which binaries run. It uses its own `os.Pipe`s so `Wait` can't drop the stream's last lines.
  - **`Runner.Tick`** claims queued runs, then pending turns, up to `MaxConcurrent`. Per-agent `max_concurrency` is enforced in SQL. One turn per run at a time, so a reply that arrives while a turn exits waits.
  - **`start`:** secret plan → setup script → `running`, branch set, ticket moves to In Progress.
    - The setup script clones or `git init`s, uses repo-local git config only, creates the branch, excludes `.lazyagents/`, and runs project `setup`.
    - A tracked `AGENTS.md` is marked skip-worktree.
  - **`turn`:**
    - checks guards (`max_turns`, and `max_runtime` measured on harness time, not waits);
    - issues a per-turn token (revoked after);
    - writes context files (ticket artifacts and wiki `RunContext`), the MCP config and the AGENTS section;
    - runs `TurnWrapper` + harness argv;
    - streams events (secrets and token redacted, nothing else), stores the session id as soon as it's seen, and uploads the raw log to `run-logs/runs/<id>/turn-NNN.jsonl`;
    - a watchdog handles pause (SIGINT, then SIGKILL after `Grace`), cancel and the runtime deadline.
  - **`afterTurn`:**
    - an ended run removes its workspace; waiting or paused does nothing;
    - an error or non-zero exit fails the run (`run_failed`);
    - an idle end gets one nudge (`IdleNudge`); a second idle end goes to `waiting/human_reply` (resumable by @mention).
  - **`Recover`** on start requeues `starting` runs and `running` turns, and Docker cleanup removes stray turn containers. `Sweep` removes workspaces of runs that ended between turns.
  - **Secrets (`PlanSecrets`/`SecretEnv`):**
    - the harness key is always injected (`ANTHROPIC_API_KEY`, or `CODEX_API_KEY`+`OPENAI_API_KEY`);
    - a scope-granting secret is injected only when that scope is `allowed`, as `DB_STAGING` and so on; `git:pat` also becomes `GH_TOKEN`/`GITHUB_TOKEN`;
    - setup also gets the PAT when `repo:read` is allowed.

    With no `SECRETS_KEY`, runs get no secrets. A Docker run without its harness key fails at start with a clear message (`RequireHarnessKey`).
- **`internal/prsync`:** polls GitHub (`PR_SYNC_INTERVAL_S`, default 60) in the API process.
  - merged → `pr_merged` event + ticket Done (system actor);
  - closed unmerged → `pr_closed` + `pr_closed_unmerged`;
  - diff stats replace self-reported ones; the PAT is optional for public repos.
- **REST:**
  - `GET/POST /tickets/{key}/dispatch` (preview with blockers, scopes and enforcement, secrets injected or withheld, context files, runner slots / dispatch);
  - `POST /tickets/dispatch` (bulk);
  - `GET /runner`;
  - `GET /runs/{id}/turns/{seq}/log`.

  Also: `Run.session_id`/`dispatched_by`, `RunTurn.status/has_log/usage`, and `PullRequest.closed_unmerged/synced_at`.
- **Binary and deployment:**
  - `lazyagents runner` (config: `RUNNER_*`, `DOCKER_SOCKET`).
  - `deploy/agent-base` (node 24, git, gh, pinned CLIs, uid 1000, `/workspace` owned by it).
  - compose adds `agent-base` (build-only) and `runner` (root, Docker socket, `RUNNER_IMAGE` overridable).
  - `make runner` (local backend).
- **Fake harness (`cmd/fake-harness`):** impersonates `claude` *and* `codex exec` with their real stream formats and does real MCP calls. Modes: `FAKE_HARNESS_MODE=approval|complete|idle|crash|slow`, `FAKE_HARNESS_RESUME=idle`.
- **Tests:**
  - `harness` (fixtures, invocations, `AGENTS.md`);
  - `runner` (local backend + fake): approval round trip incl. workspace git state, `AGENTS.md`, reads, token revocation and over-redaction; idle nudge then @mention; crash then re-dispatch; pause/resume (exit 130) and the turn guard; Codex via a real project override (deny → resumed → handed back);
  - `docker_test.go` (opt-in `RUNNER_DOCKER_TEST=1`, see below);
  - `prsync` against a fake GitHub.

**Verified end to end in compose** (with `RUNNER_IMAGE=lazyagents/agent-base:runner-test`, the fake baked in as `claude`, and a dummy key):
- preview → dispatch → setup container → turn container → approval → approve → turn 2 resumed the session → completed;
- raw logs downloadable;
- volume and containers removed;
- ticket In Review.

The same flow ran in the browser (Playwright: banner, dialog, run log, approve; no console errors; 390px OK).

**Web**
- **Dispatch:**
  - `components/domain/dispatch-dialog.tsx` (single or bulk, per dispatch.html) and `DispatchBanner`/`needsDispatch` in `run-parts.tsx`;
  - a "Not dispatched"/"Queued" flag on the Agent property;
  - "Dispatch…" in the bulk bar.
- **Run log:** `/runs/[id]` (run-log.html): summary strip, turns rail (with raw links and the waiting block), event stream with tabs, filter and Follow, tool cards paired with results, "denied by harness" and error rails, Pause/Resume/Cancel, Raw log. SSE now refreshes it; a 2s polling fallback remains while live, appending events via `?after=`.
- **Elsewhere:** the run card has "View run log" and queued text; the Code section shows closed-unmerged and the last sync; timeline sentences for `dispatched`, `run_failed`, `pr_merged`, `pr_closed`, and cancel reasons.

**Decided: MCP DNS-rebinding check is off** (`agentapi/transport.go`, `DisableLocalhostProtection: true`, the user's call on 2026-11-02).
- The SDK rejected requests that arrive on a loopback connection with a non-localhost `Host`.
- Docker Desktop and OrbStack deliver `host.docker.internal` over loopback, so an API on the host (`make dev`) plus the Docker runner got 403.
- Every `/mcp` request needs an agent bearer token, checked first, so rebinding has nothing to ride on.
- `TestDockerBackend` (opt-in, `RUNNER_DOCKER_TEST=1`) passes: real containers against a host API.

**Deferred (Phase 5)**
- A real-key nightly smoke test (needs the user's go-ahead and keys).
- Claude's `--permission-prompt-tool` (prompts are denied with `--permission-prompts none`; agents use `request_approval`).
- Immediate PR sync on `set_pr` (the 60s poll covers it).
- A `model_prices` table (it's `harness.Prices` for now).
- Per-project images.
- Network policy beyond Codex's sandbox.
- Settings → Runner page.
- Dashboard and sidebar runner-capacity displays are implemented in Phase 7.

## Original proposal: plan and start agent work from the UI

Raised by the user on 2026-11-02. The user subsequently approved a focused first version: any existing agent in restricted planning mode, separate editable drafts, human acceptance, and manual dispatch. See the implementation below; the larger sketch here is historical.

**The gap today:** work starts one ticket at a time: a person writes a ticket, assigns an agent, then dispatches. Nothing helps turn a goal ("ship Ledger v2 reconciliation") into tickets, and the only way to have an agent plan is an unmanaged session in a terminal.

**Sketch:**
1. **Start a plan.** A "Plan with an agent" action on an epic, a PRD or SPEC, or a blank goal. It dispatches a *planning run*: a managed run whose agent has read-only scopes and a planner prompt. It reads the source, the wiki and the linked tickets.
2. **The agent proposes, a person decides.** The planning run calls a new tool such as `propose_tickets` (titles, descriptions, criteria, a suggested agent, dependencies, context docs), not `create_ticket`. The proposals appear as a reviewable draft:
   - edit, merge, drop or reassign each item;
   - see the dependency order;
   - see a rough cost or turn estimate.

   This works like wiki suggestions.
3. **Accept and dispatch.** Accepting creates the tickets, attaches the context and assigns the agents. "Accept & dispatch" then opens the existing bulk dispatch dialog for the tickets with no unmet dependencies. Dependents wait and are offered for dispatch when their blockers reach In Review or Done.
4. **Shortcut for small things.** "Create & dispatch" in the new-ticket composer for a one-off task. It still goes through the dispatch dialog.

**Keep:** a person approves every run (§6.6). Agents never dispatch other agents.

**Open questions:**
- Is the planner a dedicated agent definition ("Planner") or any agent in a planning mode?
- Do dependencies need a schema (`ticket_dependencies`) and auto-dispatch of the next wave, or is that manual at first?
- Where do draft plans live: a new `plans` table, or an artifact of a new type `plan` that reuses versions and suggestions?
- Should a plan have a spend estimate or cap before dispatch? (v1 has no spend caps.)
- Does Codex or Claude Code planning need different prompts or harness options?

## Local test setup with Docker (this session)

- `docker compose -f docker-compose.dev.yml up -d --wait` gives Postgres on 5432 and S3 on 8333.
- Tests: `TEST_DATABASE_URL=postgres://lazyagents:lazyagents@127.0.0.1:5432/lazyagents?sslmode=disable go test ./...`.
- sqlc came from `nix build nixpkgs#sqlc`.
- The dev `lazyagents` database already held a workspace from an earlier session, so the demo used scratch databases `la_wiki_demo` and `la_staging_demo`. Drop them when you're done.
- `RUNNER_DOCKER_TEST=1 go test ./internal/runner -run Docker` needs `lazyagents/agent-base:latest` built (`docker compose build agent-base`).
- The compose stack was last left **down**. Its demo DB had PAY's repo URL cleared and a dummy `harness:anthropic` key added during testing.
- Playwright still lives in the old scratchpad (`…/e3bd4d0c-…/scratchpad/pw`, `PLAYWRIGHT_BROWSERS_PATH=$P/browsers`). `wiki.js` there is this phase's walkthrough.

## Decisions to keep (PLAN.md §11)

- **Deployment:** laptop only, single workspace.
- **Dispatch:** a person dispatches each ticket; assigning doesn't start a run. Anyone can approve.
- **Scope:** one agent per ticket; one project maps to one GitHub repo.
- **GitHub access:** a PAT stored as a workspace secret, injected only when `repo:write` is allowed.
- **Completion:** a PR-polling job moves the ticket to Done when its PR merges.
- **Spend and notifications:** no spend caps; notifications are pinned for later.
- **Instructions file:** a single `AGENTS.md` for every harness.
- **Logo:** deferred.


## Phase 6: implemented (2026-11-02)

**Server**
- `00011_attention.sql`: per-user `inbox_reads` and statement-level commit notifications on tracker, run, approval, artifact, wiki and read-state tables. Payloads contain only `{type, ids: []}` invalidation hints. Existing installations do not require service mutations to be rewritten; runner writes notify too.
- `internal/realtime`: one dedicated pooled LISTEN connection, bounded fan-out queues, slow-client disconnection, listener retry, SSE heartbeats and write deadlines. `/events` sends a reset on connection; listener reconnection resets all connected clients. Missed events are recovered by refetch, not replay.
- `/api/v1/inbox`: pending and historical approvals, dispatchable assignments, human mentions, personal reviews, blocked/failed/human-reply runs, and wiki suggestions. Project filter; unresolved unread items sort first, maximum 500 rows. Bootstrap's unread count is computed over the whole result set before that limit.
- Human mentions recognize `@<email-local-part>` (e.g. `@maya`) or `@<full name>`; self-comments are excluded. Agent mention/resume behavior is unchanged.
- `/api/v1/inbox/read`: persistent read state per local user. Ticket-based items become unread again after ticket updates; immutable comment/approval items keep their state. This is attention state, not external notifications.
- `/api/v1/search`: reuses full-text ticket/artifact/wiki ranking and adds epic/agent name matching. Optional project scope includes workspace-wide wiki pages and agents assigned within that project.
- The ticket list's `needs_human` view includes pending approvals regardless of status.

**Web**
- One `EventSource` in the shell invalidates TanStack Query prefixes, coalesces bursts, and refetches on reconnect. Run event hints refresh run logs; run state updates refresh attention/tickets too. Existing polling remains as a fallback. Reconnecting toast follows the design.
- Inbox replaces the placeholder: kind tabs/counts, project filter, Needs you / Waiting on others / Earlier groups, detail panel, exact approval action and optional note, existing dispatch dialog, run card, wiki suggestion links, and persistent read state. J/K navigates; A/D decides pending approvals, guarded against typing and dialogs.
- Shared Radix command dialog opens with ⌘K/Ctrl-K or sidebar Search. Grouped results, highlighted title matches, keyboard navigation, current-project scope with Tab toggle, `>` actions, and create-ticket-with-query-title. Composer accepts `?title=`. The existing C shortcut now ignores open dialogs.
- `api/openapi.yaml`, Go API code, sqlc code, and TS client types are regenerated.

**Verification**
- Full `go test ./...` with real throwaway Postgres databases passed, including the guarded fake-harness runner tests. No real harness CLI or real API key was used.
- `go test -race ./internal/httpapi ./internal/realtime` passed. New tests cover commit-only/rollback notification delivery, slow subscribers, read persistence/isolation, human mentions, project scope, needs-human filtering, and two SSE subscribers receiving an approval decision within one second while a managed resume turn is queued with the reviewer's note.
- Typecheck, ESLint, go vet and `git diff --check` passed.
- Production API/web images built successfully and the compose stack is healthy. Host `npm run build` hit an environmental Turbopack helper port-binding restriction; the same production build passed inside Docker.
- Through Caddy: health, Inbox, search, Inbox HTML and SSE reset all returned 200.
- **Browser visual/interaction QA was waived by the user on 2026-11-02.** The browser access host was unavailable, and native Safari control timed out. Do not describe Phase 6 as browser-verified or block future work on that QA.

**Environment left running**
- Full compose API/web/Caddy/Postgres/S3 at `http://localhost:8080`, plus dev infra from `docker-compose.dev.yml`. Runner was explicitly excluded and is off.
- Migration 00011 was applied to the local compose DB during this session. The demo run-events trigger was aligned with its final `run.events` hint after the optimization.
- An unrelated pre-existing host server still listens on :13000; it was left alone.
- No commit was made. The planning-workflow proposal above is still unstarted and still requires discussion.


## Phase 7: implemented (2026-11-02)

**Server / contract**
- `GET /dashboard?range=24h|7d|30d`: recorded performance, live workflow counts, attention items, project progress, live agent work, recent documents and runner capacity.
- `GET /epics`: objective, dates, health, owner, latest linked PRD as source, counts, agent avatars, cumulative observed turn time, and up to eight ticket previews per epic.
- `GET /agent-overviews`: live/queued assignments with actual run harness/model and plan steps, undispatched queue (top ten plus full count), base definition scopes with server enforcement explanations, concurrency and seven-day stats. This is top-level so a valid handle like `overview` is not reserved.
- `GET /agents/{handle}/runs?offset=0`: stable newest-first pagination, 50 rows plus `has_more`, run costs, tokens, tools and timestamps.
- Ticket lists accept `?epic=<uuid>`; the UI shows a removable chip. The featured epic's View all link uses this filter.
- Bootstrap and roster agent status now derive from active runs, matching the existing run-status priority. Seeded `running` labels without runs no longer count as live agents. Queued runs remain idle until they start.

**Rollups / definitions**
- River 0.51.0 and its pgx driver are pinned in `server/go.mod`. River schema migrations run after Goose in `db.Migrate`, inside an advisory-locked transaction; API, runner and `migrate` share this setup.
- The API process runs a single River statistics worker and a unique periodic job, once on startup and every minute. This worker does not execute harnesses. It shuts down before the database pool closes.
- Migration `00012_metrics.sql` adds `metric_snapshots` and an activity index. All three ranges publish atomically per workspace and emit `metrics.updated` SSE invalidations. Endpoints fall back to calculating directly when a snapshot is missing or older than 90 seconds.
- `internal/metrics` uses recorded `status_changed` events. Completed = distinct tickets whose latest observed Done transition falls in the rolling window. Re-completion counts once. Cycle = first observed In Progress before that completion, falling back to ticket creation. Missing cycle observations render `—`, not a guessed duration.
- PR merge percentage = merged / all terminal PRs with recorded `pr_merged` or `pr_closed` events in the window; no terminal PRs renders `—`. Attribution uses the ticket's current assignee.
- Spend = reported/computed run cost for **runs started in the window**, including their accumulated cost. It is not a precise billing-by-day ledger; the card help text identifies this definition. Unknown harness pricing is not estimated.
- Daily completion bars use UTC dates, with partial first/last days for rolling windows. Seven rolling days therefore span up to eight calendar bars. Chart totals match the KPI.
- Epic agent hours sum observed turn durations (managed turns begin at `process_started_at`), excluding managed queue/wait gaps. No synthetic demo statistics were added.

**Web**
- Dashboard: greeting, range control, four KPI tiles and small charts; attention actions (including dispatch preview), project table, live agent plan blocks, ticket flow, recent artifacts and runner capacity. Sidebar capacity appears on Dashboard only. Performance timestamps are visible.
- Epics: selected/featured objective with owner/source/hours and ticket preview, project filter, selectable cards, progress/health, and navigable quarter timeline with clipped bars and Today marker. The quarter/year follow the current date, rather than hard-coded Q4 dates.
- Agents: counted status filters, responsive cards, current assignments, waiting/queued work, seven-day stats, expandable permission enforcement and the existing new-agent flow. Agent detail adds Overview and paginated Runs tabs while preserving Definition, overrides and Tokens.
- Empty, loading and failure states are present; controls use the shared design system. SSE invalidates the new query prefixes; runner heartbeat freshness also has polling fallback.

**Verified**
- Full `TEST_DATABASE_URL=... go test ./...` passed, including guarded fake-harness tests. No real harnesses or real keys were used.
- `go test -race ./internal/metrics ./internal/httpapi` passed. New tests cover empty data, range boundaries, latest completion after reopening, duplicate PR events, cycle fallback, immutable metrics after unrelated edits, per-agent/workspace attribution, cached rollups, actual River startup execution, epic metadata/filtering, live vs queued assignments and plan steps, bootstrap counts, and run-history pagination.
- Typecheck, ESLint, go vet, generated API/sqlc clients, and `git diff --check` pass. Docker production builds pass; the host Turbopack limitation from Phase 6 remains environmental.
- Through Caddy, all three dashboard ranges, Epics, roster summaries, run history and their HTML routes return 200. The running demo has 14 tickets, 4 projects, 7 epics and 8 agents. River jobs completed and all three snapshots were populated.
- The browser access host is still unavailable, so these screens are not claimed as visually browser-verified. This was recorded rather than used to block the requested implementation.

**State for the next session**
- Full API/web/Caddy/Postgres/S3 stack remains running at `http://localhost:8080`; dev infra is also running. Agent runner is off. An unrelated existing host server on :13000 was left alone.
- Phases 6 and 7 were subsequently committed in `cb8c419`; Phase 8 changes remain uncommitted.
- The user approved the Phase 8 planning workflow; see below. Project management was subsequently authorized below. Epic creation/editing, other settings expansions, external notifications and other stretch work remain outside this scope.


## Phase 8: planning workflow (2026-11-02)

**Approved scope**
- Start from a blank goal, an epic, or a markdown PRD/SPEC, using any existing agent's resolved harness/model and run guards.
- A person approves starting the planning run. Proposals are editable drafts; accepting creates tickets. Dispatch stays manual through the existing dispatch dialog.
- Dependencies and project/repository management were subsequently authorized and implemented below. Automatic dispatch waves, estimates/spend caps, additional harnesses, GitHub Apps, login and other stretch work remain deferred.

**Server / runner**
- Migration `00013_planning.sql`: `planning_sessions` stores the goal, project/epic/source, pinned source version, run, proposals, revision and accepted ticket keys. `runs.ticket_id` is nullable so planners do not create coordination tickets or pollute tracker statistics.
- `/planning-sessions`: list/start; `/planning-sessions/{id}`: get/review (`save`, `accept`, `dismiss`). OpenAPI, Go and TS clients and sqlc queries are regenerated.
- Planning snapshots replace execution instructions with a planner prompt, remove project setup and external MCP servers, deny credential scopes, force Codex's read-only sandbox and Claude Code's plan mode, and deny Claude Edit/Write/Bash/Agent tools. The runner starts an empty workspace without cloning. Only the selected harness key is injected; no Git or application secrets are delivered.
- The shared Agent API tool wrapper allows planners only `whoami`, `get_ticket`, `read_artifact`, `search_knowledge`, `read_page`, `get_backlinks` and `propose_tickets`. This applies to MCP and REST; future tools are denied by default. `propose_tickets` requires the caller's own running managed planning token and atomically submits the draft and completes its run. Suggested agent IDs use strings at the MCP boundary (UUID arrays would infer the wrong schema).
- The source is pinned when planning starts, delivered as `.lazyagents/planning-source.md`, and attributed as a read of that version. Wiki memory is delivered too. Epic prompts include existing ticket keys/statuses so the planner can inspect related work.
- Acceptance locks the draft, checks its revision and creates all selected tickets, criteria, assignments, epic links, source context and activity in one transaction. Duplicate/stale acceptance returns 409; any failure rolls everything back. Accepted tickets start in Backlog and are never auto-dispatched.
- Shared run logs/control, runner concurrency, usage/cost, cancellation, token revocation and cleanup work for standalone planners. One planning-specific nudge is allowed for an idle planner; a second idle turn fails with a retry explanation.

**Web**
- `/plans`: recent sessions, approval form, live run state/log link, pinned source download, editable proposal titles/descriptions/criteria/assignments, drop/add, save, dismiss and accept. Merge by combining text/criteria and dropping the redundant proposal. Conflicting remote edits preserve the current local draft and ask for reload.
- Accepted tickets link to their detail pages; assigned tickets can open the existing bulk Review & dispatch dialog.
- Epics and PRD/SPEC artifact previews/details link into the approval form. Draft review and failed/cancelled planning work appear in Inbox/Dashboard attention. SSE invalidates planning queries; polling remains a fallback.
- Active planners appear on agent cards, agent Overview and the Dashboard. Status/counts include standalone planning runs; ticket-run history stays separate and links to Plans for planning history.

**Verification / environment**
- Browser QA was explicitly waived. No real harness CLI or real provider key was used. Both fake-harness safety guards remain intact.
- Full Postgres-backed Go regression suite passed; race checks passed for HTTP API, run services and runner. Planning tests cover REST/MCP mutation denial, both fake harness adapters, skipped repo/setup, withheld credentials, source pinning, no tickets before acceptance, stale/concurrent acceptance, rollback after a forced failure on the second ticket, source/criteria preservation, dismissal, workspace isolation, cancellation and token/workspace cleanup.
- Typecheck, ESLint, go vet and diff checks passed. Production API/web Docker builds passed. Host Turbopack still hits its known helper port-binding restriction; the Docker build is the verified production build.
- No commit was made. Do not start a real-key run or the runner without the user's instruction.
- Local preview refreshed at `http://localhost:8080/plans`; API/web are healthy and migration 00013 is applied. Through Caddy, health, planning list, Inbox, agent overviews, Dashboard, Plans HTML and Epics HTML returned 200. The planning list is empty because only throwaway test databases ran fake planners. Runner remains off; dev infra and the unrelated :13000 server were left alone.


## Follow-up: project management and ticket dependencies (2026-11-02)

The user explicitly authorized the first two recommended additions: project/repository management and ticket dependencies, including dependency suggestions in planning. Dispatch remains human-approved. Browser QA remains waived.

**Projects**
- `/settings/projects` (linked from Settings and the sidebar's Manage action): create/edit name, color, HTTPS GitHub repository and default branch; archive and restore. Keys are normalized to 2–10 uppercase letters/digits and remain immutable so ticket IDs do not change.
- `GET/POST /projects`, `PUT /projects/{key}`; updates require the current `revision`, returning 409 for stale edits. Editors preserve their local baseline revision across live refreshes.
- Migration `00014_projects_dependencies.sql` adds `projects.archived/revision`. Archiving preserves tickets/artifacts/history, hides the project from the active sidebar, default ticket lists, workflow rollups, agent queues and epic overview, and blocks new tickets, artifacts, planning and execution. Explicit project ticket filters and direct detail links retain history. Restore is reversible.
- Active/queued/waiting/paused execution or planning runs must finish or be cancelled before archiving. Project-row locks serialize archiving with new ticket/run creation.
- New dispatched runs snapshot repository URL (including no repository) and default branch in `resolved_spec.repository`. Settings updates first capture the current repository for legacy active ticket runs lacking this metadata. The runner uses the frozen repository and clones the approved branch; the dispatch preview shows the base branch. This prevents later settings edits from changing queued work.

**Dependencies**
- `ticket_dependencies` stores directed blocker relationships; ticket `dependency_revision` protects edits. `PUT /tickets/{key}/dependencies` replaces blockers with workspace validation, deduplication, self/cycle rejection, transactional activity and SSE invalidation.
- Graph edits and planning acceptance serialize via a per-workspace advisory lock, so concurrent inverse edges cannot create a cycle. Dependencies cannot be changed while the dependent has an active/queued run.
- A blocker must be **Done** (In Review is insufficient). Reopening restores the block. Dispatch previews, atomic bulk dispatch, unmanaged claims and the runner's final queued-start check all enforce readiness. If a prerequisite reopens while work is queued, the run fails before launching a harness. Already running work is not automatically cancelled.
- Ticket detail shows editable Blocked by and reverse Blocks lists with statuses; list rows flag unmet blockers. `GET /tickets?view=all&ready=true` and the Ready to dispatch filter show assigned, unfinished, undispatched work with no unmet dependencies. The Agent API's `next_ticket` also returns ready work.
- Inbox dispatch attention excludes blocked/archived work and becomes unread again when prerequisites finish. Dependency changes and completions refresh planning readiness through SSE.

**Planning**
- Proposals support stable `ref` identifiers, `depends_on` proposal references, and `blocked_by` existing ticket keys in MCP/REST/UI. References remain correct when proposals are reordered. Removing a referenced proposal requires removing its dependencies first; server validation rejects dangling references and cycles.
- Acceptance creates all tickets first, then resolves and persists all dependency edges inside the same transaction. No partial ticket/graph changes can commit.
- Accepted plans expose current `ready_keys`; bulk Review & dispatch includes only that ready subset. Later waves become available when blockers reach Done, with no automatic dispatch.

**Verification**
- Full Postgres-backed Go regressions, race checks for HTTP API/runner/tracker/run services, go vet, TypeScript, ESLint and generated-binding consistency checks pass. Production API/web Docker builds pass.
- New integration tests cover project validation/uniqueness/stale revisions, archive/restore/history, execution and planning archive guards, workspace isolation, dependency and proposal cycles (including concurrent inverse edits), duplicate blockers, ready filtering/Inbox unread state, atomic bulk rejection, unmanaged-claim enforcement, stable proposal refs, next-wave readiness without auto-dispatch, and legacy/new repository snapshots. Guarded runner tests verify reopened queued blockers launch no harness and a local Git checkout uses the approved repository/base branch despite later settings changes.
- No real harness or provider key was used. No commit was made. Agent runner remains off. Browser QA was skipped as requested.
- Local preview refreshed with migration 00014 applied: `http://localhost:8080/settings/projects` and `http://localhost:8080/tickets?view=all&ready=true`. API/web are healthy. Caddy returned 200 for project list, ready tickets, ticket dependency fields, dispatch preview, planning list, Dashboard and the new HTML routes. The persistent demo still has its original four projects; all mutation/runner checks used throwaway databases. Only API/Caddy/Postgres/S3/web are running; runner remains off.
