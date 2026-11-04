# OpenCode integration contract (LAZYAGENTS-20)

Status: finalized contract for review; runtime implementation remains gated.
Evidence date: 2026-11-03. Baseline: [[Architecture/ADR - Project chat sessions]],
with the chat policy amendment below. Historical directory/ticket name: opencode2.
Scope: contract only; no adapter, enum, credentials, migration or model invocation.

## Decision and alternatives

Use official `@opencode/cli`, executable `opencode`, public harness ID `opencode`,
and verified installed release **2.2.1** as the supported contract baseline.
The owner's explicit reply confirms this product and Claude/Codex feature parity.
Pin that release and artifact integrity in the implementation image; do not accept
an arbitrary PATH version or register `opencode2` as an alias. New versions require
compatibility verification. The inspected local distribution is the Nix package;
help/version evidence does not claim npm artifact provenance or integrity testing.

Alternatives: arbitrary PATH alias is simpler but silently changes behavior;
rejected. Keeping two harnesses avoids integration cost but misses the owner's
request; rejected. A pinned adapter costs verification work but preserves a narrow,
replaceable boundary; selected. Rollback removes catalog/dispatch registration,
while preserving historical harness/model/session values. Revisit the pin when
upstream changes the wire or policy interface.

## Verified identity and invocation evidence

`cli-help.txt` is a credential-free local capture using four isolated XDG roots in
a fresh `/tmp/lazyagents20-verified-*` directory; no auth listing, models query or model turn was run.
The prior unisolated help attempt failed opening the user log (EPERM); fresh
isolated help succeeded. Installed symlink target ends in `opencode-2.2.1/bin/opencode`.
`opencode2` is absent from PATH. Fresh isolated reverification again reports v2.2.1; this is the selected baseline.

Official sources, accessed 2026-11-03:
- https://opencode.ai/v2/docs/ identifies npm `@opencode/cli` and executable `opencode`.
- https://opencode.ai/v2/docs/cli/commands/ documents JSON runs, standalone CI
  authentication, explicit session continuation, provider login and model listing.
- https://opencode.ai/v2/docs/permissions/ distinguishes v2 permission rules from v1.
- https://opencode.ai/v2/docs/instructions/ and https://opencode.ai/v2/docs/skills/
  describe instruction and skill configuration; effective loading still needs a
  pinned-release test before capability advertisement.

Contract argv (construct an argv array, never shell-concatenate user input):
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

Authentication/provider contract:

| Model provider | Existing LazyAgents secret | Injected credential | Mode |
| --- | --- | --- | --- |
| `anthropic/<model>#<variant>` | `harness:anthropic` | `ANTHROPIC_API_KEY` | API key, equivalent to Claude harness |
| `openai/<model>#<variant>` | `harness:openai` | `OPENAI_API_KEY` | API key, equivalent to Codex harness |

Resolve by validated provider ID, not a single `HarnessKeys[opencode]` entry.
Only the selected provider key is injected. This mapping is the integration contract;
existing runner maps must be extended in LAZYAGENTS-22, including discovery/fallbacks.
[Official CI instructions](https://opencode.ai/v2/docs/cli/commands/) verify Anthropic
environment authentication. [Provider configuration](https://opencode.ai/v2/docs/providers/)
supports credential environment names: explicitly configure the OpenAI provider's
`env: ["OPENAI_API_KEY"]` rather than assume `CODEX_API_KEY` is recognized.
This is a proposed explicit configuration, not a tested provider request.
[Provider login docs](https://opencode.ai/v2/docs/cli/providers/) document API-key
login and provider-specific methods; `opencode auth login openai` selects an OpenAI
method. [ChatGPT provider docs](https://opencode.ai/v2/docs/providers/#chatgpt)
also document ChatGPT subscription authentication. Such login is an operator action,
never an interactive runner turn. Subscription/OAuth support requires an explicitly
provisioned isolated account store and refresh/lifecycle tests; reject selection
with an unavailable reason until that mechanism is implemented. Never copy or inspect
host credentials. Other providers/custom endpoints require a declared credential,
endpoint, discovery and pricing mapping; reject unmapped provider/auth modes before
launch, without blocking the two API-key equivalents. Missing required key, wrong
version or unavailable model yields sanitized diagnostics before launch. No secret
is passed through argv/logs. Do not equate subscription zero price with zero usage.

## Normalization boundary

The fixtures in `normalized.json` are synthetic desired adapter outputs, NOT captured
OpenCode wire events. Wire envelopes, end markers, native cost semantics, streamed
usage and abort behavior remain unverified for the pinned distribution. Do not
write a parser from these examples. Runtime enablement is gated until sanitized real
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

All behavior below is the required capability gate, not an implemented guarantee.
The existing server remains the authority for MCP token/project authorization.
The adapter must fail closed before launch if it cannot enforce requested scopes;
never mark ToolGating or PermissionPrompt true merely because a CLI has permissions.

| Purpose | Workspace/config | Tools and policy |
| --- | --- | --- |
| Ticket | Repo worktree, resolved overrides, generated marked AGENTS.md and context | Inject only allowed credentials; denied tools denied; approval-required operations use authenticated tool-policy/run_gated; delegated run does not claim ticket |
| Planning | Scratch workspace, PlanningSpec, no repo/setup/external MCP | Read supplied context; only draft proposal workflow; no project shell execution, writes or ticket claims; readonly fallback options retained |
| Chat (filesystem read-only) | Scratch workspace per segment, ChatSpec, no setup/repo/external MCP/fallbacks | Nine current tools: whoami, search_knowledge, read_page, read_artifact, get_backlinks, get_ticket, create_ticket, create_epic, list_epics; deny memory writes, shell, delegation and complete_run |

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
ticket, planning and chat gates are independently verified.

Chat ADR amendment: the owner's parity decision adopts current
`agentapi/chat.go`, `runs.ChatSpec` and `harness.ChatAgentsMD` behavior, superseding
the older six-tool-only/no-tracker-write clauses for this integration. Ticket/epic
creation remains project-scoped and explicitly requested; cross-project access,
memory writes, repository execution and ticket claiming remain denied. No current
behavior is removed. Planning retains its draft-only `propose_tickets` workflow.

### Concrete configuration contract

Retain **2.2.1** using its versioned `integration.use` contract. See
[policy-evidence.md](policy-evidence.md) for immutable upstream schema, implementation
and documentation links. The newer `mcp.use`, `skill.use`, `plugin.use` and
`integration.connect` actions are unsupported on this baseline: reject them before
launch, never pass them through or count ignored statements as enforcement.
Do not use the changed 2.2.3 integration semantics on 2.2.1. An upgrade requires
separate compatibility verification; installed help remains 2.2.1 evidence only.

Generate a protected run-specific `opencode.json` and isolated HOME plus XDG roots.
Keep state stable for a segment and isolate it from other runs. A sanitized chat
configuration shape follows; placeholders are runner substitutions, never literals
or user-controlled URLs. Validate against the pinned-release schema before launch.

```json
{
  "providers": {"openai": {"env": ["OPENAI_API_KEY"]}},
  "mcp": {"servers": {"lazyagents": {
    "type": "remote", "url": "<runner MCP endpoint>", "oauth": false,
    "codemode": false,
    "headers": {"Authorization": "Bearer {env:LAZYAGENTS_RUN_TOKEN}"}
  }}},
  "permissions": [{"action": "*", "resource": "*", "effect": "deny"}],
  "policies": [
    {"action": "tool.use", "resource": "*", "effect": "deny"},
    {"action": "provider.use", "resource": "*", "effect": "deny"},
    {"action": "provider.use", "resource": "openai", "effect": "allow"},
    {"action": "integration.use", "resource": "mcp:*", "effect": "deny"},
    {"action": "integration.use", "resource": "mcp:lazyagents", "effect": "allow"},
    {"action": "integration.use", "resource": "skill:*", "effect": "deny"},
    {"action": "integration.use", "resource": "plugin:*", "effect": "deny"}
  ]
}
```

The renderer adds explicit permission and policy allows for each purpose's allowed
MCP action (`lazyagents_<tool>` / `lazyagents_<tool>:*`) and reads of generated
context/instruction/skill files, after blanket denies. For native resolved skill discovery, add only exact
`integration.use` / `skill:<resolved-id>` allows after the skill deny, once discovery
is verified; otherwise keep it denied and use inert instruction files. Keep optional
plugins denied; required built-in policy machinery must remain active.
This template intentionally
permits no tool until that expansion. Ticket policy adds only resolved scope actions;
planning permits context reads and proposal tools, with no writes or project execution.
No `--auto`. Required approval actions use the authenticated LazyAgents policy bridge;
if no bridge can enforce a nonbrokered scope, reject the spec. Native subagents cannot
replace LazyAgents delegation. Brokered operations stay server-side.

[MCP configuration](https://opencode.ai/v2/docs/mcp-servers/) supplies the remote
header and direct-tool shape. [Permissions](https://opencode.ai/v2/docs/permissions/)
use ordered last-match rules. The [2.2.1 policy contract](policy-evidence.md)
also uses last-match rules and hard-denies tools after approvals. Current unversioned
policy documentation describes newer actions and is not the 2.2.1 schema. Invalid policy statements may be dropped, so schema
validation and effective-denial probes are mandatory. [Managed config precedence](https://opencode.ai/v2/docs/config/) must protect policy from repository
config and plugins with managed configuration plus OS/container isolation; XDG roots
alone do not suppress system, ancestor, compatibility-skill or project config.
If the backend cannot isolate these inputs or enforce read-only filesystem bounds,
reject the requested mode; prompt instructions and shell patterns are insufficient.

Render marked `AGENTS.md` using existing purpose renderers. [Instructions docs](https://opencode.ai/v2/docs/instructions/)
confirm V2 loads AGENTS.md, not CLAUDE.md. Stage only resolved skill directories under
`.opencode/skills/<id>/SKILL.md` with their bounded supporting files and descriptions;
[skills docs](https://opencode.ai/v2/docs/skills/) describe native discovery and
compatibility directories. Disable unapproved skill/plugin sources via isolation and
policy. Test effective loading; do not execute setup scripts or fetch remote skills.
Unknown options/tools, inherited arbitrary MCP/plugins, unsupported auth or scope
promises are rejected with 422 at selection/validation or sanitized frozen-run failure
before setup/credentials. Advertise capabilities only after enforcement tests.

## API, persistence and rollout

The confirmed target API enum is `Harness=[claude-code,codex,opencode]`.
Do not add it in this documentation release: current validation, dispatch, secret
selection and parser branches cannot execute it safely. An enum addition now would
make generated clients accept a value that runtime cannot honor.

Coordinated rollout: LAZYAGENTS-21 implements the gated adapter/wire boundary;
LAZYAGENTS-22 updates api/openapi.yaml first and regenerates Go/TS, then lands
validation, provider-aware credentials, binary routing, frozen specs, capabilities
and discovery together. LAZYAGENTS-23 verifies chat segment behavior;
LAZYAGENTS-24 updates consumers to the exact `opencode` enum/label and qualified
model IDs. LAZYAGENTS-25 verifies enforcement/parity before runtime advertisement.
Keep adapter/capability/catalog availability disabled until those gates pass.
Amendment (LAZYAGENTS-23): the chat catalog lists OpenCode, but it is available
only where the operator attests isolation (RUNNER_OPENCODE_ISOLATED, default
false) and a provider key is revealable, the same gate the runner enforces.
Default deployments therefore still report it unavailable. Capability
advertisement and real-CLI parity (including cross-provider session resume,
so far proven only against the fake CLI) remain LAZYAGENTS-25 gates.
No alias, new endpoint, default harness change or historical value rewrite.
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
| Version drift | Medium/high | Backend pins verified 2.2.1 and rejects unverified versions |
| Chat policy drift | Medium/high | Backend tests nine-tool project-scoped parity and filesystem denial |
| CLI/global config bypass | Medium/high | Backend isolates config/state and tests policy failure before launch; reject unsupported modes |
| Session silently recreated | High/high | Backend checks existence/ownership and bounded handoff with regression capture |
| Double usage/unknown price | Medium/high | Backend tests duplicate totals, resume deltas, missing prices and UI unknown/partial display |
| Orphan server/cancel race | Medium/high | Backend tests standalone descendants, cancellation locks and late terminal events |
| Generated enum rollout | Medium/medium | Contract first; regenerate and consumer tests before advertising support |

Verification for implementation: fresh/resumed/same-harness-model-switch turn,
missing session one-retry handoff, harness switch transcript bounds, missing/wrong
key, unknown provider/version/options, malformed/truncated stream, tool error vs
quota, explicit zero vs absent cost, resumed cumulative usage, denied/approval
policy and cross-project MCP reads, chat memory write rejection and permitted tracker actions/cross-project rejection, skills
loading and cancellation before/during/after final. No live model call is claimed.
See inventory.md and verification.md for repository evidence and checks.
