# Proposed opencode2 integration contract (LAZYAGENTS-20)

Status: proposed; identity/version and read-only-chat compatibility decisions required.
Evidence date: 2026-11-03. Baseline: [[Architecture/ADR - Project chat sessions]].
Scope: contract and inventory only. No adapter, public harness enum, credentials,
migration, deployment or model invocation is added by this change.

## Decision and alternatives

Keep the current two-harness runtime until the owner confirms whether “opencode2”
means official Anomaly OpenCode v2, and approves an exact distribution/version.
The installed Nix OpenCode 2.2.1 is the recommended evaluation candidate, not a
verified supported release. An executable name alone cannot settle product intent.

Alternatives: (1) alias opencode2 to whatever opencode is on PATH: cheap, but can
silently select another major version and incompatible permissions; rejected.
(2) pin official v2 after confirmation and implement one narrow adapter: additional
verification work, but reproducible invocation and replaceable boundary; proposed.
(3) retain current harnesses indefinitely: no implementation cost, but no new
provider choice. Revisit this decision when the owner supplies the canonical
package/repository, approved release and intended public harness identifier.
Rollback is to remove the future catalog entry and adapter registration, preserving
historical run/chat records. Never rewrite historical harness or session IDs.

## Verified identity and invocation evidence

`cli-help.txt` is a credential-free local capture using four isolated XDG roots in
`/tmp/lazyagents20-evidence`; no auth listing, models query or model turn was run.
The first unisolated help attempt failed opening the user log (EPERM); isolated
help succeeded. Installed symlink target ends in `opencode-2.2.1/bin/opencode`.
`opencode2` is absent from PATH. This establishes the installed candidate only.

Official sources, accessed 2026-11-03:
- https://opencode.ai/v2/docs/ identifies npm `@opencode/cli` and executable `opencode`.
- https://opencode.ai/v2/docs/cli/commands/ documents JSON runs, standalone CI
  authentication, explicit session continuation, provider login and model listing.
- https://opencode.ai/v2/docs/permissions/ distinguishes v2 permission rules from v1.
- https://opencode.ai/v2/docs/instructions/ and https://opencode.ai/v2/docs/skills/
  describe instruction and skill configuration; effective loading still needs a
  pinned-release test before capability advertisement.

Candidate argv (construct an argv array, never shell-concatenate user input):
`opencode run --standalone --format json --model <provider/model#variant> <prompt>`.
Continuation adds `--session <opaque-id>` in the same workspace and isolated state
roots. Never use `--continue`, which could select a different run. Local help says
`--session` creates a session if missing: verify existence and project ownership
before invoking; a vanished session must trigger the baseline handoff path rather
than silently lose context. `--fork` is not normal continuation. Model IDs retain
provider and optional variant; do not apply Claude aliases or Codex normalization.
The official example `anthropic/claude-sonnet-5-5` is an example, not a promised
account entitlement or a new default. Discover and validate eligible models with
bounded output/timeouts under the exact runtime credential context.

Authentication: proposed initial provider is Anthropic API-key mode, using existing
`harness:anthropic` -> `ANTHROPIC_API_KEY`, subject to owner confirmation. Standalone
mode keeps requests on a private process with injected credentials. Other providers,
OAuth/host login and a shared background service remain unsupported until separately
specified. Never infer a key from a provider-qualified string or inject every key.
Do not copy or inspect host auth stores. Reject a missing key before launch when
RequireHarnessKey is set. Pin package version and artifact integrity in the future
image; the host binary must pass the same version gate. Unsupported version or
provider yields a sanitized availability reason and no invocation.

## Normalization boundary

The fixtures in `normalized.json` are synthetic desired adapter outputs, NOT captured
OpenCode wire events. Wire envelopes, end markers, native cost semantics, streamed
usage and abort behavior remain unverified for an approved distribution. Do not
write a parser from these examples. Implementation is blocked until sanitized real
success, auth failure, tool/MCP failure, resume and cancellation captures exist.

Use existing harness.Event, Result and Usage in server/internal/harness/harness.go:
- Emit lifecycle `harness_started` with opaque session ID once known; persist it
  for this harness segment only. Enforce run/turn ownership; never reuse across
  harnesses. Same-harness model changes require a resume compatibility test.
- Accumulate assistant text in order; Result.Final is final assistant text only,
  excluding reasoning/tool output. Terminal success requires a verified completion
  marker and successful process exit, not merely EOF or one completed step.
- Map tool starts/results with stable call IDs; an MCP isError result is a failed
  tool result, not automatically a provider failure. Clip public output; redact
  tokens, auth headers, paths and provider response bodies before persistence.
- Set IsError and a safe Error for auth, provider, protocol, transport, missing
  completion, malformed/oversized JSON, unexpected EOF and nonzero exit. Only
  explicit provider quota/rate failures set UsageLimited; ordinary tool text cannot.
  Preserve unknown event compatibility by ignoring unknown nonterminal event types;
  never reinterpret them as successful completion. Bound line/buffer sizes.
- Usage is per turn, once only. Deduplicate repeated terminal summaries. For session
  totals subtract a stored baseline for that exact session; resets/new sessions do
  not inherit it. Preserve cache counters without double-counting cached input.
  Missing counters are unavailable, not evidence of zero consumption.
- Native finite nonnegative explicit cost -> cost_source=harness (including explicit
  zero). Otherwise use an exact provider/model/variant price only when accounting
  units are verified -> computed. Unknown price -> unknown, cost_known=false,
  omit event cost_usd. Internal numeric zero is a storage placeholder, never “free”.
  Mixed known/unknown run totals remain partial. Preserve existing REST numeric
  fields and cost_source; UI must display unknown/partial. Do not reuse another
  provider's matching model price. Unknown cost cannot prove a budget is unspent;
  surface unavailable cost and retain turn/runtime limits.

Cancellation: route through runner Process.Interrupt, grace, then Kill; standalone
must terminate its server and descendants too. Cancellation wins against late final
text under existing session-then-run lock ordering. Persist observed usage once,
never append a successful reply after Stop; chat returns idle and its next turn can
resume. Ticket cancellation ends the run; pause retains the session. Verify no
orphan process and no late DB write on cancel-before-launch and cancel-during-turn.
A lost chat session retries once fresh with existing 24,000-character/40-message
handoff and a system notice, not an unbounded automatic retry. Cross-harness switches
always create a new segment with no native session ID. Keep message-level SSE.

## Policy, instructions, MCP and skills

All behavior below is a proposed capability gate, not an implemented guarantee.
The existing server remains the authority for MCP token/project authorization.
The adapter must fail closed before launch if it cannot enforce requested scopes;
never mark ToolGating or PermissionPrompt true merely because a CLI has permissions.

| Purpose | Workspace/config | Tools and policy |
| --- | --- | --- |
| Ticket | Repo worktree, resolved overrides, generated marked AGENTS.md and context | Inject only allowed credentials; denied tools denied; approval-required operations use authenticated tool-policy/run_gated; delegated run does not claim ticket |
| Planning | Scratch workspace, PlanningSpec, no repo/setup/external MCP | Read supplied context; only draft proposal workflow; no project shell execution, writes or ticket claims; readonly fallback options retained |
| Read-only chat | Scratch workspace per segment, ChatSpec, no setup/repo/external MCP/fallbacks | Baseline six context tools only: whoami, search_knowledge, read_page, read_artifact, get_backlinks, get_ticket; deny memory/tracker writes, shell, delegation and complete_run |

Render agent instructions/persona and attached context with existing purpose-specific
renderers; preserve repo instructions outside the generated section for tickets.
Keep resolved skills as inert instruction files with a generated index; validate
paths and never execute skill setup scripts. Native discovery is a separate tested
capability. Restrict MCP to the reserved lazyagents server plus resolved external
servers for tickets only, with per-run bearer token via protected configuration,
not command-line arguments or logs. Clear inherited global servers/plugins/config
and bind effective rules to this run so project instructions cannot weaken policy.

OpenCode v2 permission rules are not a security boundary for arbitrary shell access.
A CLI approval prompt is not LazyAgents approval. Without a verified authenticated
pre-tool integration, reject approval-required nonbrokered scopes; brokered actions
must stay server-side with no bypass credential. Reject unknown harness options,
unmapped tools and unsupported sandbox/read-only guarantees before a process starts.
Use 422 field errors for user selection/spec validation; queued frozen specs fail
with a safe lifecycle/error diagnostic before credentials or setup are provided.
Missing MCP, inability to load policy, or unsupported enforced scope must never
silently degrade to prompt-only enforcement. Keep the catalog unavailable until
ticket, planning and read-only chat gates are independently verified.

Compatibility blocker: current `agentapi/chat.go`, `runs/chat.go` and
`harness.ChatAgentsMD` permit create_ticket/create_epic/list_epics. That exceeds the
baseline read-only ADR. Do not remove existing behavior in this contract ticket.
Owner must explicitly choose baseline read-only for the new harness (requiring a
server-side purpose/capability gate) or update the ADR to accept tracker mutations.
CLI permissions alone cannot enforce a six-tool server token allowlist. Neither
choice is represented as already implemented here.

## API, persistence and rollout

No OpenAPI or generated shape change is necessary while identity/capabilities are
unresolved: advertising an enum would claim unavailable runtime support. Keep
Harness=[claude-code,codex]. Once the public ID is approved, update
api/openapi.yaml first, regenerate Go and TS with make gen-check, then implement
validation, capabilities, discovery, runner and consumer handling together.
Unknown selections remain existing 422 field errors. No new endpoint is required.
Existing optional cost_source and free-form usage/event payloads carry normalized
cost availability; retain existing chat 404/409/422 behavior and SSE invalidation.

No migration is required for these documents or for a future string harness value:
run/chat harness/model/session columns are text and usage is JSON. Inspect checks
again at implementation time. New nullable usage availability columns or a new
persistent credential/provider model, if chosen later, require a separate additive
migration proposal and db:migrate approval; no migration file or DB command here.

## Risk review and exit gates

| Risk | Likelihood/impact | Mitigation and owner |
| --- | --- | --- |
| Wrong product/version | High/high | Main agent obtains canonical distribution, pinned release and public ID |
| Chat policy drift | Certain/high | Owner resolves baseline vs current tracker writes; backend verifies server allowlist |
| CLI/global config bypass | Medium/high | Backend isolates config/state and tests policy failure before launch; reject unsupported modes |
| Session silently recreated | High/high | Backend checks existence/ownership and bounded handoff with regression capture |
| Double usage/unknown price | Medium/high | Backend tests duplicate totals, resume deltas, missing prices and UI unknown/partial display |
| Orphan server/cancel race | Medium/high | Backend tests standalone descendants, cancellation locks and late terminal events |
| Generated enum rollout | Medium/medium | Contract first; regenerate and consumer tests before advertising support |

Verification for implementation: fresh/resumed/same-harness-model-switch turn,
missing session one-retry handoff, harness switch transcript bounds, missing/wrong
key, unknown provider/version/options, malformed/truncated stream, tool error vs
quota, explicit zero vs absent cost, resumed cumulative usage, denied/approval
policy and cross-project MCP reads, chat memory/tracker write rejection, skills
loading and cancellation before/during/after final. No live model call is claimed.
See inventory.md and verification.md for repository evidence and checks.
