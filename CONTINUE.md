# CONTINUE: handoff for the next agent

Read `PLAN.md` first for architecture and phases, then `lazyagents-design/DESIGN.md`, which is the visual source of truth.

Phases 0–1 were committed by the user as `072484f initial work` on `main`. The Phase 2 work is uncommitted in the working tree. Ask the user before committing.

## Status

| Phase | State |
|---|---|
| 0 Scaffold | ✅ Done, but **`docker compose up` has never run**: Docker isn't installed on this machine. Run it first (or ask the user to). Likely trouble spots: the SeaweedFS healthcheck command, distroless permissions, the Next standalone build inside Docker (it needs network for Google Fonts). |
| 1 Design system and shell | ✅ Done |
| 2 Core tracker | ✅ Done (deferred items are listed below) |
| 3 Artifacts | ✅ Done |
| 4 Agent definitions and Agent API | **Next** |
| 4.5 Wiki and agent memory | Planned (PLAN §12, added at the user's request) |
| 5–8 | Not started |

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

- Dashboard KPIs (7). Dispatch, the "Queued" flag and approval banners (4/5). The Inbox count is still 0 (4/6).
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
- **`/artifacts`:** search, type tabs with counts, a drop zone, the table, and a preview panel (thumbnail or real image, meta, outline with citations, Read by, tickets, New version; Generate tickets is disabled until Phase 8).
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
- "Generate tickets" (Phase 8).
- Wiki links (Phase 4.5).

## Next: Phase 4 (Agent definitions and Agent API)

Follow PLAN §5.2, §6.1–6.3 and Phase 4's exit criteria.
- **Definitions:** `agent_definition_versions` and `agent_project_overrides`, plus the merge logic where scopes can only tighten (heavily unit-tested).
- **Agent identity and secrets:** agent tokens (`la_…`, hashed) and encrypted secrets (AES-GCM, `SECRETS_KEY`).
- **Agent API:** an MCP server at `/mcp` via `modelcontextprotocol/go-sdk`, mirrored in REST.
  - `read_artifact` should call `artifacts.RecordRead`.
- **Runs:** runs/turns/plan_steps, approvals and the `run_gated` broker.
- **UI on the ticket:** the approval banner, run card and plan card.
- **Agent configuration:** the screen is mocked in `lazyagents-design/screens/agent-config.html`.
- **There's no login** (PLAN §5.4). Human actions are attributed to the local user, and agent tokens are a separate auth path at `/mcp`.

## Decisions to keep (PLAN.md §11)

- **Deployment:** laptop only, single workspace.
- **Dispatch:** a person dispatches each ticket; assigning doesn't start a run. Anyone can approve.
- **Scope:** one agent per ticket; one project maps to one GitHub repo.
- **GitHub access:** a PAT stored as a workspace secret, injected only when `repo:write` is allowed.
- **Completion:** a PR-polling job moves the ticket to Done when its PR merges.
- **Spend and notifications:** no spend caps; notifications are pinned for later.
- **Instructions file:** a single `AGENTS.md` for every harness.
- **Logo:** deferred.
