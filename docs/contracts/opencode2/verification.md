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

## LAZYAGENTS-25 requirements matrix

Every inventory surface and accepted ADR capability is mapped to its implementation,
named tests and an honest status (fake-CLI vs real) in
[`verification/LAZYAGENTS-25-requirements-matrix.md`](../../../verification/LAZYAGENTS-25-requirements-matrix.md).
Items under "Not verified" above remain UNVERIFIED there.
