# Running and developing LazyAgents

[← Back to the project](../README.md)

## Run locally

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
  for Claude Code, `harness:openai` for Codex. Runs bill that key. OpenCode
  (opt-in, pinned CLI 2.2.1) uses the key of its model's provider; see
  [OpenCode harness](opencode.md);
- add `git:pat` (a GitHub token) and a repo URL for the project, if agents should
  clone, push and open PRs. Open PRs are polled every 60s: merged → Done.

> **Security:** the runner mounts the Docker socket, which is root-equivalent on
> the host. v1 is meant for a single developer laptop. Works with Docker Desktop
> and OrbStack.

### Runner tools and project libraries

Keep the running dependency list in Git:

- Shared tools: [`deploy/agent-base/Dockerfile`](../deploy/agent-base/Dockerfile)
  installs the toolchains and pinned harness CLIs, Go, sqlc, and Chromium system
  libraries once per image build. Rebuild with `docker compose build agent-base`.
- Local runners use host tools. Install missing tools with
  `brew bundle --file=deploy/agent-base/Brewfile` (or your existing package
  manager). Put Node 24, Go 1.26+, sqlc 1.31+, and the harness CLIs on the runner's
  PATH. The Brewfile does not replace your CLI logins.
- Project libraries: add `.lazyagents-deps.json` to the repository root. The
  manifest lists tool requirements and directories; exact library versions stay
  in `go.mod`/`go.sum` and `package-lock.json`. This repository includes an example:

```json
{
  "version": 1,
  "required_tools": ["git", "gh", "go", "node", "npm", "python3", "rg", "make", "sqlc"],
  "go_modules": ["server"],
  "npm_projects": ["web"],
  "playwright": ["web"]
}
```

The runner checks tools, downloads Go modules, installs locked npm packages
(including dev dependencies), and installs Chromium before starting model work.
It also prepares delegated worktrees, even for read-only agents. A continued
worktree reuses npm packages when its lockfile, package manifest, and Node/npm
versions match. A missing tool or failed install fails workspace setup with an
`Environment setup failed` message. Repositories without the manifest retain
existing setup behavior. Agent/project setup commands run after dependency setup.
Changes take effect after updating the API/runner binaries; repository manifests
must be on the branch that new runs clone. Existing workspaces are not rewritten.

All local runs share download/browser caches under `$RUNNER_DATA_DIR/cache`
(default `server/.lazyagents/runs/cache` when launched with `make runner`). Docker
runs share the named volume `lazyagents-dependency-cache` at `/cache`; workspace
cleanup preserves it. Each checkout owns its own `node_modules` and writable Go
build cache under `.lazyagents/cache/go-build`. Codex grants cache writes to
execution/repository writers; scoper keeps read-only tool permissions. Shared
caches are intended for projects trusted by the same user, not unrelated tenants.
Playwright browser versions follow the project lockfile; Linux OS library changes
require rebuilding the agent image. Database servers/credentials remain explicit
project configuration, not package dependencies.

The tool policy recognizes read-only shell sequences, pipelines, numeric `sed`
print ranges, and system-shell wrappers such as `/bin/zsh -lc 'rg ...'`. Dynamic
commands, file writes, and unknown programs still require `tests:run`. Ordinary
`git push` requires `repo:write`; combining it with test execution requires both.
Hooks are a best-effort check; the harness/OS sandbox remains the isolation boundary.

### View the runner environment

Open **Settings → Runner environment**, or follow **Environment** from a run log.
The page shows live runner capacity alongside the latest recorded setup per
runner and project: tool versions, direct Go/npm dependencies, Chromium setup,
and shared versus workspace cache availability. Select a project/runner to
inspect its report, or use a run's link for its historical snapshot. Reports are
stored in run events and survive workspace cleanup and runner shutdown.

These are observations from setup, not a live scan of the host. New reports
require the updated runner and a new workspace setup; older runs show a clear
empty state. Failed dependency installs retain their error and any inventory
collected before the failure. Refresh only reloads reports; it does not install
packages or start agent work.

### Chat

Select a project, open **Chat** in the sidebar, and choose **New chat**. Select
a harness/model, type a message, and send with Enter (Shift+Enter adds a newline).
Chats appear in the history panel so you can return to the conversation later.
Keep a runner online. Docker mode requires `harness:anthropic` for Claude Code
and `harness:openai` for Codex in **Settings → Connections & secrets**. Local
mode uses the runner account’s host CLI logins; no application provider keys are
needed. The API must use the same `RUNNER_BACKEND` mode as the runners.
Unavailable models are disabled in the picker. OpenCode is opt-in and off by
default; see [OpenCode harness](opencode.md).

Change the picker before sending to use a different model or harness for the
next reply. Model changes on the same harness resume its session. A harness
change starts a fresh session with a bounded transcript handoff; hidden reasoning,
tool results and older messages outside the budget do not transfer.

Use **Add context** or type `[[` to attach a wiki page, artifact or ticket.
**Start chat** on a context item's page opens a chat with that item attached.
The assistant can read project context through LazyAgents MCP; pages it reads
appear under **Sources**. Replies arrive when a turn finishes. **Stop** cancels
the current turn; **Save to wiki** and **Save as artifact** preserve a reply.
Chats use a scratch workspace and read-only context tools.

Verification status and known gaps are recorded in
[`verification/LAZYAGENTS-14-report.md`](../verification/LAZYAGENTS-14-report.md).
The real Claude Code → Codex smoke is still pending API/runner mode alignment and
a chat-capable runner; automated fake-harness coverage does not establish provider parity.

### Token-efficient execution

Ticket agents work directly in their persistent harness session by default.
Routine scoping, implementation, and shipping do not require separate agents.
Delegation remains available for substantial independent work, missing
capabilities, explicit requests, and independent review of risky or large changes.
Existing review requests for changes still block Done. Enable **Require
independent review before Done** in an agent's Runtime settings to enforce an
approval gate for every ticket that agent runs. Acceptance criteria, scopes, and
human approvals remain required.

Managed `claim_ticket` responses no longer repeat instructions and memory already
available in the workspace. Agents are instructed to read only relevant context
and reuse it on resume. New definitions default to **Selective** memory: at most
three relevant previews, with full pages fetched only when needed, within a
1,500-token budget. Existing explicit Index/Linked/Search settings are preserved;
choose Selective on an existing agent's Memory card to use the new selection.
Attached artifacts are still available locally, and duplicate memory/artifact
copies are skipped.

Run logs, usage events, chat replies, and agent run history distinguish cached
input, uncached input, output, and input with an unavailable cache breakdown.
Total tokens include cache reads; cache hits are not added again. Historical
breakdowns are recovered from saved turn usage where available. Unsupported or
self-reported usage stays unknown rather than being labeled uncached. Prices and
provider limits still depend on the harness/provider; cache counts are not a
spend or subscription-limit estimate.

### Skills

An agent definition can carry **skills**: reusable playbooks (a name, a one-line
"use when…" description, and markdown instructions). On Claude Code they're
written into the run workspace as native skills (`.claude/skills/<name>/SKILL.md`,
git-excluded), so the agent loads a skill's body only when a task matches its
description. On Codex they use native discovery under
`.agents/skills/<name>/SKILL.md`. Edit them in the agent's
**Skills** card.

`examples/skills/` holds a starter library, and
`python3 examples/attach-skills.py` attaches it to the starter agents
(`tech-lead`, `implementer`, `code-reviewer`, `scoper`). It's idempotent.

### Patterns

Reusable team conventions (architecture patterns, coding styles) live in a
curated, workspace-wide wiki area, separate from agents' run notes
(`agents/<handle>/`) and project decisions (`Architecture/ADR - …`).

- Published patterns are atomic notes at `Patterns/<Topic>/<Title>`. Topic is
  one of Go, React, Postgres, Testing, API, Architecture, Process; the title is
  short and imperative ("Wrap errors with context").
- The body starts with `Topic: <topic>` and `Tags: tag1, tag2` (2-5 lowercase
  tags), then the sections When to use, Rule, Example, When not to use, Related.
- Agents look conventions up with `find_pattern({q, topic?, limit?})` before
  implementing. Search tolerates typos and partial words (`eror wrap`, `pgx`);
  an empty `q` lists everything. The web API is `GET /api/v1/wiki/patterns`.
- Agents propose with `propose_pattern({title, topic, tags, body,
  source_ticket?})`, which writes `Patterns/Proposed/<Title>` (always allowed,
  whatever `wiki:write` says). Proposals are not searchable.
- An agent whose `wiki:write` scope is `allowed` (for example a librarian or
  `tech-lead`) reviews them: `list_proposed_patterns`, then `publish_pattern`
  (optionally editing title, topic or body) or `reject_pattern` with a reason.
  Both remove the proposal. If the pattern already exists, merge into it with
  `write_note` (mode replace) and reject the duplicate.
- Run notes under `agents/<handle>/` are only for durable lessons that help a
  future run on this codebase, not a log of what was done. General lessons go
  through `propose_pattern`; project-specific decisions stay in ADRs.

### Planning work

Open **Plans**, choose a project and agent, describe a goal, then **Approve & start planning**.
You can also start from **Plan with an agent** on an epic or **Generate tickets** on a
markdown PRD/SPEC. The planner uses the runner and harness key described above,
with read-only capabilities, no repo cloning/setup, and no write credentials.

Review the draft in Plans or Inbox. Edit, drop or reassign proposals, and select
which proposals or existing tickets must finish first, then
**Accept & create tickets**. Tickets remain in Backlog until you separately
**Review & dispatch** them. Planning can incur model costs; it waits in the queue
if no runner is online.

### Projects and dependencies

Open **Settings → Projects & repos** (or **Manage** beside Projects in the sidebar)
to create a project, edit its GitHub repository/default branch, or archive and restore it.
Project keys stay fixed. Archiving preserves history and requires queued/active runs
to be finished or cancelled first. Approved runs retain their repository settings.

Tickets in an epic ship together. The epic gets one shared branch,
`epic/<project key>-<epic id prefix>`, and one draft pull request from it into
the default branch. Each ticket's branch is cut from the epic branch, its PR is
stacked on the epic branch, and the agent merges it there once checks pass. You
review the epic as one GitHub stack and merge the epic's PR once. Tickets outside
an epic still get their own PR into the default branch.

On a ticket, use **Dependencies → Edit** to add blockers. Every blocker must be
**Done** before dispatch; In Review is insufficient. Cycles are rejected. The ticket
list's **Ready to dispatch** filter and an accepted plan's dispatch action show ready
work. Later waves become available as prerequisites finish, with human approval for
every dispatch.

### Kanban board

Open **Board** to see project tickets by status, edit a ticket in its modal,
or filter by epic to manage final acceptance criteria and the done gate.
See [the board guide](board.md) for estimates, statuses and the agent MCP tools.

## Develop

Requires Go 1.26+, Node 24+ and Docker for Postgres and SeaweedFS.

```sh
make dev     # infra in Docker; API on :8000 and Next on :3000 run on the host
make runner  # optional: run dispatched tickets with this machine's own claude/codex logins
make gen     # after editing api/openapi.yaml or server/internal/store/queries (needs sqlc)
make test
make help    # everything else
```

`make api`, `make dev` and `make runner` default to `RUNNER_BACKEND=local`.
The API inherits this setting through hot reload too. Local execution has no
isolation and uses the runner account's installed CLIs and logins. To develop
with Docker runners, use `RUNNER_BACKEND=docker make api` (or `make dev`) and
`RUNNER_BACKEND=docker make runner` in their respective terminals. Direct
`lazyagents serve` / `runner` commands and the full Docker stack still default
to Docker. Set `RUNNER_BACKEND=local` explicitly for direct local commands.

The API catalog uses its own configured backend; it does not discover runner
capabilities. Mixed local/Docker pools have no backend-aware chat routing, so
catalog availability cannot guarantee which runner claims a turn. Use a
coordinated, consistent pool for chat verification. See the
[runtime refresh procedure](../verification/LAZYAGENTS-14-runtime-evidence.md#coordinated-refresh-procedure)
before replacing existing runners with active work. `make dev` replaces API/web
listeners; use it only during a coordinated maintenance window.

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

Agents can continue automatically when a provider's usage or rate limit is
reached. Add an ordered list under **Usage fallbacks** in the agent definition
editor, or set `fallbacks` in the definition API:

```json
{
  "harness": "claude-code",
  "model": "your-primary-model",
  "fallbacks": [
    {"harness": "codex", "model": "your-codex-model"}
  ]
}
```

Each target requires a harness and model; optional `harness_options` apply only
to that target. Configure credentials for every harness (local runners may use
CLI logins). Targets are tried once, in order, only for provider quota/rate-limit
errors. Other errors still fail the run. The same checkout, plan and accumulated
usage are retained; the next harness starts a fresh session with the original
task, recent inputs and last assistant message. Full conversation state is not
transferred between harnesses. Existing turn/runtime guards still apply. When
all targets are exhausted the run fails, and each switch appears as a
`harness_fallback` lifecycle event. Definition changes apply to new runs.

Both harness adapters now use the same run-scoped tool policy: native tool hooks
check the frozen scopes before execution. Denied actions are blocked. Actions
requiring approval pause the run and appear in the existing approval UI; a yes
authorizes that exact tool input once, and the resumed turn retries it. Planning
runs permit context reads (including simple read-only shell commands) and block
project command execution and edits. The authenticated
policy endpoint is `POST /api/v1/agent/tool-policy` and accepts `{tool, input}`;
it requires a token bound to the running agent's run and returns `allow` or
`deny`. A policy outage blocks the tool rather than permitting it.

Codex performs an offline hook preflight before each new or resumed turn. It
checks that the generated service hook has an absolute path, can execute from
the workspace, and can reach the authenticated run policy before loading a model.
It also checks that the hook is loaded and refuses to run if another
active hook is unreviewed. Only then does it use Codex's automation hook-trust
flag. It does not disable the sandbox or shell approval policy. Like the CLI's
own tool rules, hooks provide best-effort enforcement, not isolation from a
malicious program; Docker remains the isolation boundary. Local runners inherit
the host's environment and logins. `python3` is required by both adapters and is
included in the runner image.

Repository-authorized Codex turns use a workspace permission profile that also
allows writes to `.git`, so staging and committing work headlessly. Planning
and runs without repository write authorization retain their sandbox settings.
Local macOS runners append Homebrew's binary directories to PATH so installed
tools such as `gh` are available even when launched from a desktop application.

Skills use native discovery on both CLIs: `.claude/skills/<name>/SKILL.md` for
Claude Code and `.agents/skills/<name>/SKILL.md` for Codex. Generated skill files
are excluded from git. Cross-harness fallbacks include a bounded transcript of
recent messages, tool calls and outcomes, and approval decisions; CLI-private
session files and internal reasoning are not portable.

Cost displays distinguish known, unknown and partially priced totals instead of
showing unpriced usage as free. Native reported costs take precedence when
available. Codex prices tokens using the built-in model table or the optional
`RUNNER_MODEL_PRICES` JSON object, keyed by exact model id. Each entry must include
nonnegative `input_per_million`, `cached_input_per_million`, and
`output_per_million` USD rates. Supply your provider's applicable rates; no price
is inferred for an unknown model. Aggregates identify windows containing
unpriced runs. The migration also marks older unpriced turns as unknown/partial.

Codex session-total token counters are normalized to per-turn deltas on resume,
so resumed turns do not bill earlier usage twice. The migration repairs older
turns where recorded lifecycle events identify the Codex session; original usage
JSON is retained on those turns for audit.
