# CONTINUE: handoff for the next agent

Read `PLAN.md` (architecture and phases) and `lazyagents-design/DESIGN.md` (visual source of truth) first. Nothing is committed yet: everything is untracked on `main`. Ask the user before committing, and branch first.

## Status

**Phase 0 (scaffold): done, apart from Docker.**
- `api/openapi.yaml` (OpenAPI **3.0.3**, since oapi-codegen's 3.1 support is incomplete). It contains only `GET /health`.
- `server/` holds the Go 1.26 module `github.com/liamphmurphy/lazyagents/server`:
  - `cmd/lazyagents`: `serve | migrate | healthcheck`. Migrations and bucket creation run on boot.
  - `internal/config`: env config.
  - `internal/db`: pgx pool with retry, goose migrations embedded from `migrations/`.
  - `internal/blob`: S3 client and `EnsureBuckets`.
  - `internal/httpapi`: strict-server implementation, request logging.
  - `internal/api`: generated code. Run `go generate` (oapi-codegen as a `go tool`).
  - `internal/testdb`: throwaway database per test. Tests skip unless `TEST_DATABASE_URL` is set.
  - `migrations/00001_init.sql`: `uuid_v7()` and `touch_updated_at()`.
- `web/` is Next **16.4** + React 19 + Tailwind v4 (turbopack loader), with `cacheComponents` and `partialPrefetching` on and `output: standalone`.
  - **Read `web/node_modules/next/dist/docs/` before writing Next code.** The APIs differ from older versions: Middleware is now `proxy`, uncached data needs `<Suspense>`, and so on.
  - `lib/api/client.ts` uses openapi-fetch with `/api/v1` in the browser and `API_INTERNAL_URL` on the server. `npm run gen` regenerates `lib/api/schema.d.ts`.
  - Dev rewrites send `/api`, `/mcp` and `/events` to the API. They're fixed at build time, so in production Caddy routes these paths instead.
  - The placeholder home page calls `/health` through TanStack Query.
- Infra and tooling files:
  - `docker-compose.yml` (postgres, seaweedfs 4.48, api, web, caddy on :8080) and `docker-compose.dev.yml` (infra only)
  - `deploy/Caddyfile`, `deploy/seaweedfs/s3.json`
  - `Makefile` (`make help`), `.env.example`, `.github/workflows/ci.yml`
- Verified locally: go vet and test (migration and UUIDv7 tests run against real Postgres), the health endpoint against real Postgres and SeaweedFS, and Next lint, typecheck, build and dev-mode proxying.
- **Not verified: `docker compose up`.** Docker isn't installed on this machine. The image tags were checked against Docker Hub. The first thing to do is run `docker compose up --build`, or ask the user to, and fix anything that breaks: the SeaweedFS healthcheck command, distroless file permissions, the Next standalone build in Docker.

**Phase 1 (design system and shell): in progress.**

Done:
- `web/styles/tokens.css` is a verbatim copy of `lazyagents-design/tokens.css`. **Edit the design copy first, then copy it over.** I added a "component tokens (v0.2)" block to both files (`--row-hover`, `--btn-secondary-bg`, `--priority-unlit`, `--scrim`, …) with light values, and documented it in DESIGN.md §2.
- `web/app/globals.css` maps the tokens into Tailwind `@theme inline`:
  - The default Tailwind palette is cleared.
  - Text colors use the `fg` prefix: `text-fg`, `text-fg-2`, `text-fg-secondary`, `text-fg-tertiary`.
  - Surfaces: `bg-surface`, `bg-surface-raised`, `bg-surface-sunken`, `bg-sidebar`, `bg-panel`.
  - State colors: `bg-state-*`, `bg-pill-*-bg` / `text-pill-*-fg`, `bg-doc-*` / `bg-doc-*-bg`.
  - Radii match the tokens: xs 4, sm 6, md 7, lg 10, xl 12, 2xl 14.
  - Type utilities: `type-display | title | heading | body | reading | small | micro | mono`.
  - Also defined there: `animate-skeleton`, which respects reduced motion; `.prose-code` for inline code; and an accent `:focus-visible` ring.
- Fonts: Inter and JetBrains Mono via `next/font`, exposed as `--font-inter` and `--font-jetbrains-mono` and wired into `--font-sans` / `--font-mono`.
- Written but not yet typechecked:
  - `web/lib/cx.ts`
  - `web/components/ui/icon.tsx`: the full icon set from the mockups, plus the placeholder `Mark`
  - `web/components/ui/button.tsx`: `Button`, `ButtonLink`, `IconButton` and `buttonClass`; variants are primary, secondary, outline, ghost, approve and approve-outline
  - `web/components/ui/primitives.tsx`: `Kbd`, `Card`, `FilterChip`, `AddFilterChip`, `Skeleton`, `EmptyState`, `Banner` (human or blocked), `Field`, `TextInput` (mono and prefix options) and `TextArea`
  - `web/components/ui/controls.tsx` (client): `Segmented`, `Toggle` (`role=switch`) and `Tabs` (links when given hrefs, otherwise a tablist), all with arrow-key focus movement

Remaining in Phase 1:
1. **`components/domain/`**, built from the specs in DESIGN.md §2/§4:
   - `StatusGlyph`: backlog dashed ring, todo ring, progress 50% pie, review 75% pie, done/human/blocked filled. The inset shadow takes the parent surface color.
   - `PriorityGlyph`: urgent "!" square, or 3 bars using `--fg-2` and `--priority-unlit`.
   - Avatars:
     - `AgentAvatar`: rounded square in the agent's hue, with sizes 20/6, 24/7, 38/10 and 22 stacked with a ring. Hue comes from `color_index` mapped to atlas, quill, sentry, juno, nova, relay, pike, echo, used as `var(--agent-<hue>)` / `var(--agent-<hue>-bg)`.
     - `HumanAvatar`: circle.
     - `UnassignedAvatar`: dashed `--unassigned`.
     - `AvatarStack`.
   - Labels and tags:
     - `Pill` (24px with a dot), `Flag` (radius 5), `DocTypeTag`, `LabelChip`
     - `MonoChip` (filled or outline)
     - `ScopeMode` and `EnforcementChip`
   - Data display: `ProgressBar`, `StackedBar`, `PropertyRow`, `KpiTile`.
2. **Shell** (`components/shell/`), following DESIGN.md §3 and the sidebar in `lazyagents-design/screens/tickets.html`:
   - **Sidebar.** A client component that uses `usePathname` for the active item and `aria-current`. It holds:
     - the workspace switcher with the `Mark` and a new-ticket button
     - a ⌘K search button
     - nav: Dashboard, Inbox (with badge), Tickets, Epics, Agents (with live dot), Artifacts
     - the projects list
     - the user row with a settings gear
   - **TopBar** with a breadcrumb.
   - Layout: a wrapping flex row, with the sidebar at `flex: 1 1 232px` and main at `flex: 999 1 560px`, so the sidebar stacks below about 800px.
   - Put it in an `app/(app)/layout.tsx` route group, with placeholder pages for `/`, `/inbox`, `/tickets`, `/epics`, `/agents`, `/artifacts` and `/settings`. Until Phase 2 adds the API, feed the sidebar static props.
3. **Theme:** an inline `<head>` script that reads `localStorage["la-theme"]` (`system | dark | light`) and sets `data-theme` before first paint. See `node_modules/next/dist/docs/01-app/02-guides/preventing-flash-before-hydration.md`. Add `suppressHydrationWarning` on `<html>`, and a theme switch for the settings page.
4. **`/dev/foundations`:** a route that mirrors `lazyagents-design/screens/foundations.html` using the real components. Compare it by eye in both themes against `lazyagents-design/shots/foundations.png`.
5. Run `npm run typecheck && npm run lint && npm run build`. Screenshot with Playwright at 1440 and 390 and check there's no horizontal overflow. Playwright and Chromium are installed in the previous session's scratchpad (`scratchpad/pw`, run with `PLAYWRIGHT_BROWSERS_PATH=$D/browsers node shoot.js <src> <out> names...`). If that's gone, install them again.

**Then follow PLAN.md §9:** Phase 2 (core tracker) → 3 (artifacts) → 4 (definitions and Agent API) → 5 (runner and adapters) → 6 (realtime, Inbox, ⌘K) → 7 (dashboard, epics, agents).

Phase 2 needs sqlc. sqlc needs cgo for its Postgres parser, so install it as a binary or run it through Docker rather than adding it as a `go tool` in the module. Keep `go.mod` lean, too: `air` was removed as a tool because it pulled in Hugo's whole dependency tree, and `make api` uses it only if it's installed.

## Local test setup without Docker

The previous session ran Postgres 17 and SeaweedFS from nix, inside the session scratchpad:
- Postgres: `nix build nixpkgs#postgresql_17`, then `initdb` and `pg_ctl -o "-p 54329 -k ''"`. TCP only, because the scratch path is too long for a unix socket.
- SeaweedFS: `nix build nixpkgs#seaweedfs`, then `weed server -s3 -s3.config=deploy/seaweedfs/s3.json -s3.port=18333 …`.
- Example: `TEST_DATABASE_URL=postgres://lazyagents@127.0.0.1:54329/lazyagents?sslmode=disable go test ./...`

Those processes may still be running from the old session. They're throwaway.

## Decisions to keep (from PLAN.md §11)

- Laptop-only deployment, single workspace.
- A human dispatches each ticket; assigning an agent doesn't start a run. Anyone can approve.
- One agent per ticket, and one project maps to one GitHub repo.
- GitHub access uses a PAT stored as a workspace secret, injected only when `repo:write` is allowed.
- A PR-polling job moves the ticket to Done when the PR merges.
- No spend caps. Notifications are pinned for later.
- Use a single `AGENTS.md` for every harness; Claude Code falls back to it when there's no `CLAUDE.md`.
- The logo is deferred. Keep the placeholder "L" mark.
