# Verification and handoff

## Review correction after 98d58b6

Resumed a clean checkout at 98d58b6; read ticket, review blocker, supplied context
and backend wiki ADR before editing. No plan-read tool or persisted plan artifact
was exposed; posted a replacement three-step continuation plan. Preserved prior work.

Retain 2.2.1 with source-verified `integration.use` / `mcp:*` deny and
`mcp:lazyagents` allow, plus `skill:*` and `plugin:*` denies. New action names
are rejected before launch. See policy-evidence.md for immutable tagged source,
exact schema/evaluation references and limits. cli-help.txt remains installed 2.2.1
evidence; it is not newer-release or effective-denial evidence.

Follow-up checks passed: `GOCACHE=/tmp/lazyagents20-go-cache make gen-check`
(no generated diff), `git diff --check`, Python JSON parsing of ADR template and
normalized.json, policy statement shapes against the inspected 2.2.1 action enum,
eight ordered allow/deny cases (selected/unapproved MCP, skills, plugins, provider
allow/deny and shell/MCP-tool deny), and blanket permission denial. These static
checks do not execute upstream or establish runtime conformance. No runtime source
changed, so prior Go/typecheck evidence is preserved without claiming a new run.

Main must update [[Architecture/ADR-020 OpenCode integration baseline]] to specify
the retained 2.2.1 prefixed integration policy, obtain independent reviewer approval,
and reassess criterion 4. No parent criteria, push or PR mutation performed.

## Preserved continuation on 2026-11-03

Inspected clean working tree, current branch and history before editing. Preserved
5cd2b80 and its preceding work. Read LAZYAGENTS-20 description, criteria and owner
reply plus all four supplied backend/tech-lead/chat ADR context pages. whoami
confirms delegated Backend Engineer; no claim_ticket or parent criteria changes.
The available tools expose plan mutation but no plan-read operation; existing plan
was not readable. Posted a three-step continuation plan before editing.

Owner decisions are resolved: official @opencode/cli, executable/public ID opencode,
Claude/Codex parity including current nine project-scoped chat tools. Reverified
installed v2.2.1 in four fresh temporary XDG roots, with credential-like environment
variables removed for help capture. Captured version, run, auth login, session list
and models help in cli-help.txt, all exit 0. No auth listing/login, host auth store
inspection, models query or model request. Isolated help verifies syntax, not runtime
policy isolation. Nix distribution inspection from the preserved evidence remains;
no npm package integrity claim is made.

ADR.md links authoritative V2 invocation, provider/auth, permissions, policies,
MCP, instructions, skills and config docs accessed on 2026-11-03. The baseline is
2.2.1; current web documentation may evolve and must be checked against the pinned
release during implementation. Explicit env provider mapping and config templates
are implementation requirements, not wire-tested fixtures.

Checks on the follow-up (all exit 0):

- `GOCACHE=/tmp/lazyagents20-go-cache make gen-check` at repo root: regenerated
  Go/sqlc/TypeScript with no generated diff.
- From server: `GOCACHE=/tmp/lazyagents20-go-cache go test ./internal/harness
  ./internal/agentdef ./internal/chatmodels ./internal/runs ./internal/runner
  ./internal/agentapi ./internal/httpapi`: pass; agentapi has no tests. Database
  coverage remains skipped without TEST_DATABASE_URL; no DB service/migration run.
- From web: `npm run typecheck`: pass.
- Synthetic normalized.json remains documentation-only; no parser conformance claim.

Contract evidence for main review: criterion 1 has exact verified release/help,
headless/model/auth/resume documentation; criterion 4 has concrete configuration,
current purpose policy and unsupported-capability rejection. Neither requires a paid
model call for this contract ticket. Main must independently review and obtain the
requested delegated reviewer approval before checking parent criteria/completion.

## Actual implementation gaps and rollout

LAZYAGENTS-21/22 must capture sanitized real wire success/error/resume/cancel events,
implement parser and provider-aware secret/model resolution, validate the corrected configuration
against 2.2.1, and enforce effective deny policy despite ambient configs/plugins.
There is no verified authenticated pre-tool bridge or filesystem sandbox for this
adapter today. Reject modes needing those guarantees until implemented; never mark
capabilities true from documentation alone. LAZYAGENTS-23/25 must verify nine-tool
chat project isolation, allowed tracker actions, denied memory/repo execution,
session ownership/missing-session handoff, same-harness model change, usage/cost
normalization and descendant cancellation. OAuth/subscription auth needs explicit
isolated provisioning/refresh support; custom providers need declared mappings.
These are technical gates, not unresolved product choices.

The target enum addition is specified in ADR.md for coordinated LAZYAGENTS-22
contract-first rollout; current two-value OpenAPI remains intentional while runtime
cannot honor opencode. No migration is required for the existing text/JSON storage.
No runtime support advertised; no push, PR, parent completion, migration, deployment
or secret access performed.

## Preserved prior checks at 5cd2b80

The preceding contract commit recorded the same focused Go/type checks, parsed eight
synthetic normalized examples, and regenerated pre-existing /chat-models description
drift in Go/TS. Its identity and policy questions are superseded by the owner's reply
and this follow-up; those earlier blockers are no longer current requirements.

## LAZYAGENTS-22 runtime wiring and wire capture (2026-11-03)

Public ID `opencode` is in the API enum (`api/openapi.yaml`, regenerated Go and
`web/lib/api/schema.d.ts`), `agentdef.Harnesses`, `harness.For`, runner dispatch,
the local and Docker backends and `deploy/agent-base/Dockerfile`
(`@opencode/cli@2.2.1`; npm `dist.integrity` sha512-U8UIdEOh…G3Vug==). Chat:
LAZYAGENTS-23 lists OpenCode in `chatmodels.chatHarnesses`, available only with
RUNNER_OPENCODE_ISOLATED and a revealable provider key (fake-CLI coverage in
`runner/chat_opencode_test.go`); the picker UI is LAZYAGENTS-24.

Fail-closed gates in the runner:

- `RUNNER_OPENCODE_ISOLATED=true` is an operator attestation that no system,
  ancestor or repository OpenCode config exists on the backend. Without it every
  OpenCode run (primary or fallback target) fails before setup and credentials.
  The runner cannot verify this itself: effective-denial probes need a model.
  The local backend is not isolation, so it stays off by default.
- Provider-scoped credentials: `anthropic/…` → `harness:anthropic` →
  `ANTHROPIC_API_KEY`, `openai/…` → `harness:openai` → `OPENAI_API_KEY`, nothing
  else; an unmapped provider or harness is an error, never another key. OpenCode
  always requires its key (it has no host login under its isolated HOME).
- The local backend gives OpenCode an allowlisted host environment (PATH, locale,
  TLS roots) plus the run's injected variables: no inherited provider keys, HOME
  or XDG roots.
- A preflight `opencode --version` runs before the first turn of a segment:
  missing executable or any version other than 2.2.1 fails the run with a bounded
  message (no environment or secret values).
- `Parser.Exited` is wired through the optional `harness.ExitObserver`; Claude
  and Codex parsers do not implement it and are unchanged.
- Session ownership for `--session` is the run's own recorded session id. It does
  not prove the session still exists in the workspace state (see gaps).

### Wire capture without a paid call

Against the installed 2.2.1 (`~/.nix-profile/bin/opencode`), with four fresh
temporary XDG roots and `env -i`:

1. No credentials, `run --standalone --format json --model anthropic/claude-sonnet-5-5 "say hi"`
   exit 1, one stdout line (stderr empty):
   `{"type":"error","timestamp":…,"sessionID":"ses_…","error":{"type":"provider.no-route","message":"Model unavailable: anthropic/claude-sonnet-5-5"}}`
2. Same with `ANTHROPIC_API_KEY=sk-ant-invalid-fake-key-000000` (one 401 from the
   provider, no model usage): exit 1,
   `{"type":"error",…,"error":{"type":"provider.auth","message":"invalid x-api-key","status":401,"response":{"body":"…"}}}`
3. `--session ses_doesnotexist1` on a missing session was accepted and created it
   (reported `sessionID":"ses_doesnotexist1"`), confirming `--session` silently
   creates sessions.
4. Garbage `opencode.json` in the cwd, parent, XDG config or `.opencode/`, and
   `OPENCODE_CONFIG`, changed nothing observable before model routing, so config
   layering could not be probed without a model.

Both error captures are stored as `server/internal/harness/testdata/opencode-real-*.jsonl`
and the parser now reads this envelope (`sessionID`, nested `error.type/message`).
It differs from the synthetic fixtures (flat `message`, snake_case `session_id`),
which are kept for the unverified success/tool/quota/resume shapes.

### Not verified (needs a paid model call or a real deployment)

Superseded in part on 2026-11-05: success, tool, resume, cancel and policy/isolation behaviour were later captured with the real CLI against a localhost stub provider (next sections and the "Real CLI against a localhost stub" section below). The list is kept as written; read the stub section for what changed. Real-provider behaviour stays unverified.

- Success, tool/MCP, resume, quota and cancellation event shapes, the completion
  marker, and whether usage is per turn or cumulative: fixtures stay SYNTHETIC.
- Whether the generated `opencode.json` policy is effective (denials, MCP
  allowlist) and config-layer isolation; native cost semantics.
- Session existence on resume (a lost session is silently recreated by the CLI).
- Docker backend runs against a real image; only the local backend with the fake
  CLI was exercised.
- Computed cost: operator prices from `RUNNER_MODEL_PRICES` keyed by the exact
  model id (e.g. `openai/gpt-5#high`) are applied to the assumed token fields; the
  accounting units are unverified.

## Real CLI against a localhost stub (2026-11-05)

Evidence level: **real CLI + stub provider**. The real installed 2.2.1
(`~/.nix-profile/bin/opencode`, `opencode --version` prints `opencode v2.2.1`) ran
with the exact `opencode.json` that `harness.OpenCode.Invocation` generates, plus
one added key that routes the model to `127.0.0.1`. The owner authorized running
the real CLI locally with no paid run. No real provider, key, `claude` or `codex`
was involved. Not evidence for a real provider: token counts, cost and model
behaviour come from a scripted stub.

Safety setup, every run: `env -i` with a fresh `HOME` and four `XDG_*` roots and a
fresh temp cwd (no `~/.config/opencode`, `~/.local/share/opencode` or host auth
store touched), `ANTHROPIC_API_KEY=sk-fake-000`, and
`HTTP(S)_PROXY=http://127.0.0.1:9` (dead) with `NO_PROXY=127.0.0.1,localhost`.
The stub was the only reachable endpoint: its log shows only the model and MCP
traffic and the dead proxy made the CLI's own `models.opencode.ai` catalog fetch
fail (`Failed to fetch models.dev`; it falls back to its bundled catalog, which is
why `claude-sonnet-5-5` resolved). A first attempt with only `ANTHROPIC_BASE_URL`
did **not** route (the CLI ignores it and tried `api.anthropic.com`; the dead proxy
stopped it before any bytes left the machine). That is the reason the proxy
guard stays.

Routing that worked: the legacy-but-honored key
`provider.anthropic.options.baseURL = http://127.0.0.1:<port>/v1` in `opencode.json`
(the v2 `providers.<id>.settings` object has no `baseURL` field). The CLI then POSTs
`/v1/messages?beta=true` (Anthropic messages SSE), a second request to
`claude-haiku-5-5` for the session title, and talks MCP (streamable HTTP, JSON
responses accepted) to the `lazyagents` server URL.

Invocation, exactly as the adapter: `opencode run --standalone --format json --model
anthropic/claude-sonnet-5-5 [--session ses_…] <prompt>` with **stdin closed**
(`/dev/null`). Throwaway tooling lives outside the repo (`/tmp/la25-capture/`:
`stub.js`, `run.sh`, `cancel.sh`, `amb.sh`). Sanitized captures are
`server/internal/harness/testdata/opencode-real-stub-*.jsonl`.

### Wire shape (replaces the synthetic assumptions)

| Event | Shape |
| --- | --- |
| all | `{"type":…,"timestamp":ms,"sessionID":"ses_…","part":{…}}` (errors carry `error` instead of `part`) |
| `step_start` | one per model step; `part.type = "step-start"` |
| `text` | `part.text` (one per text part) |
| `tool_use` | call **and** result in one line: `part.id` (the model's call id), `part.tool`, `part.state.{status: completed\|error, input, output\|error, metadata}` |
| `step_finish` | `part.reason` (`tool-calls`), `part.cost`, `part.tokens.{input,output,reasoning,cache.{read,write}}` |
| `error` | `error.{type,message,status?,response?}` |

Findings (a to g):

- **a. Success, completion, usage.** A plain turn is `step_start`, `text`, exit 0.
  There is **no completion marker and no `completed`/`session` event**. The
  **final step never gets a `step_finish`** (reproduced 6/6, with 20ms to 300ms
  stub latency): a one-step turn has no usage on stdout; a tool turn has
  `step_finish` only for the `tool-calls` steps. `tokens` are per model step (each
  equals that request's own usage, exactly what the stub sent), not cumulative
  across steps or turns; `tokens.input` excludes cache reads/writes. The CLI
  prices steps itself from its bundled catalog (`cost` 0.002488 for 1090 in /
  29 out / 5 cache read / 7 cache write on `claude-sonnet-5-5`, i.e. $2 and $10
  per million), so even native cost is a catalog estimate. Complete numbers exist
  only in the session store: `opencode session export <id>` returns session
  cumulative `info.tokens`/`info.cost` (includes the title request), usable as a
  follow-up for exact accounting.
- **b. Resume.** `--session <id>` on a second invocation sends the full prior
  history (the stub saw the first user message, the first assistant reply, then
  the new user message). Each invocation spawns a private `serve --stdio` child.
  Tokens in the resumed turn are its own steps only.
- **c. Tools.** Advertised tool list with the generated policy:
  `edit glob grep opencode_worktree_remove read skill write` (baseline without
  policy: `edit glob grep question read shell skill subagent webfetch websearch
  write execute`). Findings:
  1. **The generated tool allowlist blocks the tools it allows.** `read`, `glob`,
     `grep`, `edit`, `write` all return `Blocked by configuration policy` (and
     `opencode_worktree_remove` likewise); only the MCP `lazyagents_*` call
     executed. In 2.2.1 a `tool.use` policy resource is matched against
     `<permission-action>:<resource>` (for example `write:/path/file`,
     `shell:git push origin x`), so the exact names `read`, `write`, … never match
     and the preceding `tool.use * deny` wins. Verified fix (stub run): resources
     `read:*`, `glob:*`, `grep:*`, `edit:*`, `write:*`, `shell:*` make those tools
     complete.
  2. **`bash` is not a 2.2.1 tool name; the shell tool is `shell`.** The adapter's
     `bash` allow rules (permissions and policies) match nothing, so ticket runs
     with `tests:run`/`repo:write` never get a shell (calls return `No tool named
     "bash"/"shell" is currently available`). With `shell` in both lists a shell
     call executes, and the adapter's `git push*` / `gh pr create*` deny rules do
     work (`Permission denied: shell`; `echo hi` ran).
  3. **MCP tools are not advertised on the first model request of a turn**
     (reproduced in every run, also with no policy at all, also on resumed turns,
     even when the MCP `tools/list` completed before the request). They appear
     from step 2. A model cannot call `lazyagents_claim_ticket` as its first
     action; a first step that answers in text ends the turn (the runner's idle
     nudge covers it).
  4. A tool name that is not advertised (`bash`, `evil_probe`, `ambient_ping`,
     `execute`) returns `No tool named … is currently available` and nothing runs;
     no process or file was created for any denied or unknown call (proof files
     checked).
  5. `opencode_worktree_remove` is gated by the edit permission; allowing `edit`
     allows it (it validated its own arguments and refused a non-project path in
     the stub run).
- **d. Cancellation.** Mid-stream SIGINT and SIGTERM (to the CLI pid): exit **130**,
  stdout `step_start` then `error {"type":"unknown","message":"Transport: The socket
  connection was closed unexpectedly…"}`; SIGKILL: exit 137, no further event. In
  all three cases the `serve --stdio` child exited and the stub saw the connection
  close: **no lingering descendants** (the pre-existing `serve --service` background
  process on this machine is not ours and was left alone). The runner's
  process-group SIGINT therefore works.
- **e. Config isolation: ambient config can override the Vesper policy.**
  An ambient config that allows everything, adds an MCP server (`ambient`) and an
  instructions file was tried in each layer, with the generated `opencode.json` in
  the cwd:

  | Layer | Policy/MCP loosened? | Instructions injected? |
  | --- | --- | --- |
  | `$XDG_CONFIG_HOME/opencode/opencode.json` | **yes** (write executed, ambient MCP server connected) | yes |
  | ancestor directory `opencode.json` | **yes** (same) | yes |
  | `OPENCODE_CONFIG=<file>` | **yes** (same) | yes |
  | `$HOME/.config/opencode/opencode.json` (with `XDG_CONFIG_HOME` set) | no (not read) | no |
  | `<cwd>/.opencode/opencode.json` | no (Vesper deny still wins, MCP server dropped) | **yes** |
  | `OPENCODE_CONFIG_CONTENT` | no | **yes** |

  `--model` on the command line always won over an ambient `model`. Because the
  adapter's own `XDG_CONFIG_HOME` is `<workspace>/.lazyagents/opencode/config`,
  **an agent that is allowed to `write` can drop `opencode/opencode.json` there and
  the next (resumed) turn loads it**: with the corrected allowlist from c.1, a stub
  turn wrote that file and the next turn connected the extra MCP server.
  Denying `edit`/`write` on `*/.lazyagents/*` and `*/opencode.json` with `tool.use`
  policy resources (`write:*/.lazyagents/*`, after the allows) blocked it while
  normal writes still worked. Managed/system config (`/Library/Application
  Support/opencode`) was not tested. So `RUNNER_OPENCODE_ISOLATED=true` remains a
  claim the runner cannot verify, and the per-run state directory is writable by
  the agent.
- **f. Provider errors.** A 400 `invalid_request_error` "credit balance is too
  low" surfaces as `error {"type":"provider.invalid-request","status":400,…}`, exit
  1 (the parser marks it usage-limited through its existing message markers).
  **A 429 (with or without `retry-after`) and a 529 never produce an error
  event**: the CLI retries with growing backoff (8 attempts in 60s observed, one
  `step_start` each) until killed. A rate-limited run therefore hangs until the
  runtime limit or cancel, and falls back only on a run-level timeout, not on a
  quota envelope.
- **g. Instructions.** `AGENTS.md` in the cwd is injected into the system prompt
  as `Instructions from: <abs path>/AGENTS.md` followed by its content (the
  adapter's file is therefore read). An ancestor `AGENTS.md` was not.
- **Other quirks.** `opencode run` hangs after database bootstrap (no output, no
  model request) when stdin is an open pipe that never closes; the runner starts it
  with no stdin so it is unaffected, but any wrapper must close stdin. A
  multi-word prompt argument reaches the model wrapped in literal double quotes
  (`"go do it"`). `--session` on a missing id still creates it (earlier finding).

Parser: the real events were not read by the old assumptions. `opencode_parser.go`
now parses `step_start`/`text`/`tool_use`/`step_finish` and the nested `error`;
success on the real wire is exit 0 after real events without an error event;
usage is summed from `step_finish` and always reported as a lower bound
(`usage_partial`, cost `unknown`), never as a bill. The synthetic envelope is still
accepted. Tests: `opencode_real_stub_test.go` (`TestOpenCodeRealStub*`).

### Local runner end to end (opt-in)

`server/internal/runner/opencode_real_test.go` drives the real runner (local
backend, `OnlyBins`, `OpenCodeIsolated`) with the real CLI and an in-test stub
provider. Without a production base-URL knob, the test builds a tiny launcher and
registers it as the `opencode` binary; it merges the `baseURL` into the adapter's
`opencode.json`, points the proxy variables at a dead port and execs the real
binary. Skipped unless `LAZYAGENTS_REAL_OPENCODE=1`:

```
cd server && LAZYAGENTS_REAL_OPENCODE=1 TEST_DATABASE_URL=… \
  go test ./internal/runner -run TestRealOpenCodeAgainstStub -count=1 -v
```

Result on 2026-11-05 (real 2.2.1, Go 1.26, darwin/arm64): all three pass:
a ticket turn (policy-blocked `read`, real MCP `claim_ticket`, `post_plan`,
`complete_run status=todo`, session id, lower-bound usage with unknown cost, only
the run's own fake provider key seen by the stub, AGENTS.md in the prompt, MCP tool
list absent on step 1 and present on step 2), a two-turn chat resuming one session
with first-turn history reaching the model, and a SIGINT cancel that ends the
provider connection, exits non-zero and leaves the run `cancelled`.

### Still not verified

Everything about a real provider: real token units and billing, model tool-call
behaviour, real quota/rate-limit envelopes beyond the stub's, OAuth/subscription
auth, Docker image runs, managed/system config layers, the `session export`
accounting path, Windows, and any CLI version other than 2.2.1.

## LAZYAGENTS-25 requirements matrix

Every inventory surface and accepted ADR capability is mapped to its implementation,
named tests and an honest status (fake-CLI vs real) in
[`verification/LAZYAGENTS-25-requirements-matrix.md`](../../../verification/LAZYAGENTS-25-requirements-matrix.md).
Items under "Not verified" above remain UNVERIFIED there.
