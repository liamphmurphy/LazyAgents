# Real-stack browser e2e

Browser → Next → **real Go API** → **real runner** (`RUNNER_BACKEND=local`) →
**real Postgres**. Unlike `web/e2e/*.spec.ts`, nothing is mocked: the specs
contain no `page.route` and the chat REST/SSE traffic is the real thing.

> **Fake CLIs, not real providers.** `opencode`, `claude` and `codex` are all
> the deterministic `server/cmd/fake-harness` binary. A pass proves the
> product plumbing (catalog, model/harness selection, runner dispatch, segment
> switching, cancel, persistence, history reload) with the *synthetic* OpenCode
> envelope. It is **not** evidence that the real pinned OpenCode 2.2.1 CLI, its
> output format, or any provider works. That needs a separate, credentialed
> verification.

## What it covers (`opencode-chat.spec.ts`)

1. Pick OpenCode + a provider-qualified model (`anthropic/claude-sonnet-5-5`)
   in the picker, send, see the fake reply (and the persisted message carries
   the qualified model id).
2. Change model inside OpenCode (`openai/gpt-5`), then switch harness to Claude
   Code (fake `claude`); each message gets a reply and its own model tag.
3. Reload: all three messages and their per-segment harness/model tags persist.
4. Cancel a running turn (`slow` blocks the fake until stopped), then repost
   and get a reply; history survives a reload.

## The disposable fixture (`run.sh`)

`board-flow.spec.ts` covers an agent claiming a ticket and estimating a sibling
through the real MCP endpoint, live SSE updates on the open board, a person's
modal edit to Done, and the epic gate refusing closure until all final criteria
are checked. It then verifies the saved ticket and epic through REST. This flow
does not invoke a model. The fixture inserts one epic directly into the disposable
database because epic creation has no standalone REST endpoint; criteria,
tickets, estimates and status changes use the real API/MCP. The script provides
both `E2E_STACK_BASE_URL` (web) and `E2E_STACK_API_URL` (API) to Playwright.

Everything lives under a fresh temp dir and is torn down on exit (success,
failure or Ctrl-C):

- **Postgres** `postgres:17-alpine` via `docker run --rm` on a free port, or
  `E2E_DATABASE_URL` (CI service container; must be disposable, it is
  migrated and written to). `lazyagents serve` runs migrations and first-boot
  seed (`SEED=false`).
- **SeaweedFS** (S3, required by the API) via `docker run --rm` on a free port,
  or `E2E_S3_ENDPOINT`.
- **Discovery stub** (`discovery-stub.mjs`): OpenCode's catalog lists models
  from provider discovery. `LAZYAGENTS_TEST_DISCOVERY_URL` (a test-only,
  off-by-default seam in `server/internal/chatmodels/discovery.go`) points it at
  this local stub, so no request reaches a provider. A dummy
  `harness:anthropic` / `harness:openai` secret is created through the real
  `PUT /api/v1/secrets`, plus a project and a main agent (chats need both).
- **API + runner**: built binaries, `RUNNER_OPENCODE_ISOLATED=true`,
  `RUNNER_MCP_URL` and `PUBLIC_URL` on the free ports.
- **Next**: `npm run build` then `next start` with `API_INTERNAL_URL` set.

### Fake harness and the PATH guard

`RUNNER_CLAUDE_BIN`, `RUNNER_CODEX_BIN` and `RUNNER_OPENCODE_BIN` point at
symlinks to the built fake in `$TMP/bin`. The API and runner run under
`env -i` with `PATH=$TMP/bin:/usr/bin:/bin` (`git` is symlinked in). Before
anything starts, the script requires `command -v claude|codex|opencode` to
resolve inside `$TMP/bin` under that PATH and refuses to start otherwise, so a
real, logged-in CLI (`~/.local/bin`, `~/.nix-profile/bin`, `/opt/homebrew/bin`)
cannot be reached. Claude/Codex model lists come from the fake answering the
same discovery protocol the local catalog uses.

The local runner gives OpenCode a scrubbed environment, so the fake's script
mode (`chat`: answer without MCP) is read from `$TMP/bin/fake-harness.mode`
instead of `FAKE_HARNESS_MODE`.

## Run locally

Needs Docker, Go, Node 24 and Chromium (`npx playwright install chromium`).

```sh
cd web
npm ci
npm run e2e:stack                       # whole suite
npm run e2e:stack -- --headed           # extra args go to playwright
E2E_STACK_SETUP_ONLY=1 npm run e2e:stack  # just start the stack and print URLs
E2E_KEEP_TMP=1 npm run e2e:stack        # keep $TMP (logs/) for debugging
```

On failure the script prints the tail of each process log. CI runs the same
command in the `e2e-stack` job and uploads `playwright-report/` and
`test-results/` (traces) on failure.
