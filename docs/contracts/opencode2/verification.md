# Verification and handoff

## Continuation on 2026-11-03

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
implement parser and provider-aware secret/model resolution, validate the configuration
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
