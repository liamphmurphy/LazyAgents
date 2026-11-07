# OpenCode harness

OpenCode is a third harness next to Claude Code and Codex: public ID `opencode`,
official CLI `@opencode/cli`, executable `opencode`. It is **opt-in and off by
default** (see [Isolation](#isolation-attestation)).

> **Status:** three evidence levels, kept apart. (1) **Fake CLI**: the product
> plumbing, against a scripted binary with a synthetic envelope. (2) **Real CLI +
> stub provider** (2026-11-05): the real `opencode` 2.2.1 against a localhost stub,
> no paid call: real wire shapes, resume, cancel and policy/isolation behaviour,
> with **open policy defects** listed in [Limitations](#limitations). (3) **Real
> provider**: still **unverified** (owner directed no paid runs).
>
> ⚠️ **Do not set `RUNNER_OPENCODE_ISOLATED=true` on a shared runner yet.** The
> generated config now lives outside the checkout and launch guards reject
> ambient config, but the operator must still provide an OS boundary that stops
> shell and MCP tools from modifying protected state or racing those guards.
> Ticket file-tool policy remains blocked on LAZYAGENTS-26 (see Limitations).
> Requirement-by-requirement evidence:
> [`verification/LAZYAGENTS-25-requirements-matrix.md`](../verification/LAZYAGENTS-25-requirements-matrix.md).

Design and evidence: [`docs/contracts/opencode2/`](contracts/opencode2/ADR.md)
(`ADR.md`, `verification.md`, `inventory.md`, `policy-evidence.md`).

## Supported CLI version

Exactly **2.2.1**. Before the first turn of a segment the runner runs
`opencode --version` and fails the run on anything else (missing binary, hang
over 15s, or another version). There is no alias (`opencode2`) and no PATH
fallback to a different release. Upgrading needs a new compatibility check.

## Installation

| Backend | How |
| --- | --- |
| Docker (default) | `deploy/agent-base/Dockerfile` installs `@opencode/cli@2.2.1` (`OPENCODE_VERSION` build arg). Rebuild the image to pick it up: `docker compose build`. |
| Local (`RUNNER_BACKEND=local`) | Install `@opencode/cli@2.2.1` on the host. Put `opencode` on `PATH` or set `RUNNER_OPENCODE_BIN=/path/to/opencode`. |

The local backend runs on your machine with whatever OpenCode config exists
there, so it is not isolation and OpenCode stays off by default on it. Only set
the attestation where an OS sandbox prevents all granted tools (including shell
and MCP tools) from changing protected config, external state and workspace
ancestors. An ordinary local same-user process does not meet this requirement.

## Authentication

API keys only, from workspace secrets (**Settings → Connections & secrets**,
needs `SECRETS_KEY`). The model's provider chooses the key; nothing else is
injected:

| Model | Secret | Env var in the run |
| --- | --- | --- |
| `anthropic/<id>` | `harness:anthropic` | `ANTHROPIC_API_KEY` |
| `openai/<id>` | `harness:openai` | `OPENAI_API_KEY` |

The key is required on every backend, including local: OpenCode runs with an
isolated HOME and never uses host logins, ChatGPT/OAuth or `opencode auth`.
Any other provider is rejected (`opencode provider "x" is not supported
(supported: anthropic, openai)`); there is no fallback to another harness's key.
On local, OpenCode inherits only an allowlisted host environment (PATH, locale,
TLS roots), not your provider keys.

## Models

OpenCode models are provider-qualified, `provider/model[#variant]`, and are
passed through unchanged (no Claude/Codex aliasing):
`anthropic/claude-sonnet-5-5`, `openai/gpt-5`.

- **Agents, usage fallbacks and ticket overrides** validate the same way. A
  harness override to `opencode` needs a qualified model too.
- Editor suggestions: `anthropic/claude-sonnet-5-5`, `anthropic/claude-opus-5-5`,
  `openai/gpt-5`. They are examples, not entitlements.
- **Chat picker:** the catalog lists models by calling provider discovery with
  each revealable key (`harness:anthropic`, `harness:openai`) and prefixing the
  provider. A provider with no key or failed discovery is skipped. With no
  models at all, OpenCode is unavailable.
- Harness options: only `readonly` (`true`/`false`) is accepted; others are
  rejected.

## Isolation attestation

`RUNNER_OPENCODE_ISOLATED=true` is an operator statement that the backend has no
system, ancestor or repository OpenCode config that could override the policy
Vesper generates for a run. The runner cannot check this itself. Without it:

- every run that uses OpenCode (as primary or usage-fallback target) fails before
  setup and credentials;
- the chat catalog reports OpenCode unavailable.

Set it on **both the API and the runner process**: the API uses it for the chat
catalog, the runner for execution. `docker-compose.yml` shares it through
`x-api-env`, so set it once in `.env`. Running them separately with different
values makes the picker and the runner disagree. The Docker image is the
natural place for the claim to hold; on local, only if you have verified it.

## Resume and switching

- Each harness has its own session per chat/run segment; a session ID is never
  reused across harnesses. The runner only resumes (`--session <id>`) a session
  recorded by that run.
- The CLI **silently creates** a session it doesn't know. The runner does not
  check the session still exists in the workspace state, so a vanished
  session would restart without context (known gap).
- Changing the model **within OpenCode**, even across providers
  (`anthropic/…` → `openai/…`), keeps the session. Only the new model's provider
  key is injected. Cross-provider resume is proven only against the fake CLI.
- Switching **harness** (e.g. OpenCode → Claude Code) starts a new segment with
  no native session and a bounded transcript handoff; reasoning, tool results
  and older messages outside the budget don't transfer.

## Limitations

- **Current usability (until LAZYAGENTS-26 lands):** OpenCode works only for
  read-only chat and runs that use nothing but MCP tools. Ticket runs can't read
  or edit files. `RUNNER_OPENCODE_ISOLATED=true` remains an operator claim
  about OS isolation; setting the flag alone does not create a sandbox.

- **Real-provider parity is not verified (UNVERIFIED, no paid runs).** Nothing
  here has run against a real provider: real token units, billing, model
  behaviour and real quota envelopes are unknown. What *has* been captured with
  the real CLI and a localhost stub provider
  ([verification.md](contracts/opencode2/verification.md#real-cli-against-a-localhost-stub-2026-11-05)):
  the success, tool, resume, cancel and quota-error wire shapes (the parser reads
  them), session resume with history, SIGINT cancellation without lingering
  children, and the policy/config behaviour below.
- **Policy defects found by the stub capture (not fixed here):** (1) the generated
  `tool.use` allowlist uses bare tool names, but 2.2.1 matches
  `<action>:<resource>`, so `read`/`glob`/`grep`/`edit`/`write` are all
  **blocked** (only `lazyagents_*` MCP calls work); (2) the shell tool is named
  `shell`, not `bash`, so shell is never granted; (3) MCP tools are not
  advertised on the first model step of a turn. Ticket runs cannot read or edit
  files through OpenCode until LAZYAGENTS-26 lands.
- **Usage is a lower bound:** the real stream has no completion marker and omits
  the final step's `step_finish`, so tokens are partial and cost is `unknown`
  (success is a zero exit with no error event). A provider 429 or 529 is retried
  by the CLI indefinitely instead of producing an error.
- **Cost:** on the synthetic envelope a native cost is used (`cost_source=harness`,
  including zero); the real wire has none that is complete (see above). Otherwise cost is computed only from an operator price in
  `RUNNER_MODEL_PRICES` for the exact model id (e.g. `openai/gpt-5#high`), with
  unverified token units. With neither, it is `unknown` (`cost_known=false`)
  and shown as unknown/partial, never as free. Unknown cost can't show a budget
  is unspent; runtime and turn limits still apply.
- **Policy:** the generated `opencode.json` targets the 2.2.1 `integration.use`
  contract. Denials (unknown tools, non-`lazyagents` MCP, skills) were observed to
  hold against the stub; the allowlist and config-layer isolation are defective
  as listed above. Newer `mcp.use`,
  `skill.use`, `plugin.use`, `integration.connect` options are rejected.
  OpenCode permission rules are not a security boundary for shell access.
- **No approvals:** with no authenticated pre-tool bridge, scopes set to
  require approval are rejected, except brokered/API-only ones. A CLI prompt is
  not Vesper approval. No sandbox guarantees; no `permission_mode`.
- **No unmanaged mode:** OpenCode never runs without the attestation and a key.
- **Chat is read-only** (scratch workspace, project-scoped context tools).
- Docker backend runs against a real image are unexercised; only the local backend
  (fake CLI, and the real CLI against a stub) was.

## Configuration and state isolation

The backend owns persistent OpenCode state outside the checkout:
local uses `<runner-dir>/runs/<workspace-id>/.lazyagents/opencode/<namespace>`;
Docker mounts a separate `lazyagents-run-<workspace-id>-opencode` volume at
`/.lazyagents/opencode`. Each delegated worktree has a separate namespace.
Generated config is selected by a runner-owned `OPENCODE_CONFIG`; HOME and all
XDG roots live alongside it. State survives resumed turns and runner recovery
and is deleted with the workspace.

Every launch, including version preflight and resumed turns, rejects ancestor
and repository `opencode.json`/`opencode.jsonc`, known HOME/XDG configuration
layers, and legacy checkout-local state. Ambient host/image `OPENCODE_*`
switches are discarded. Staging refuses symlink destinations. Ordered tool
denies cover both absolute and relative `.lazyagents` paths.

This is defense in depth within the required OS boundary, not a same-user
sandbox. Docker's ordinary writable state mount also needs that boundary for
shell/MCP tools. Keep the isolation flag off on a shared runner without it.

Existing checkout-local sessions are deliberately not imported: after upgrading,
start a new run. Missing external resume state fails instead of silently
starting a conversation with no history.

## Troubleshooting

| Message (starts with) | Cause | Fix |
| --- | --- | --- |
| *(no message: the run never finishes while the provider is rate-limiting)* | On a 429/529 the CLI retries forever and emits no error. | Cancel the run and check the provider quota. Runner detection is tracked in LAZYAGENTS-28. |
| `OpenCode needs isolated config (managed config, no ambient ~/.config/opencode), which this backend does not attest. Set RUNNER_OPENCODE_ISOLATED=true only where it holds, or pick another harness.` | Attestation unset (chat reason or run failure). | Set `RUNNER_OPENCODE_ISOLATED=true` on API **and** runner if true, else use another harness. |
| `OpenCode is not available: install @opencode/cli@2.2.1 (the agent-base image pins it) or set RUNNER_OPENCODE_BIN.` | Executable missing or `--version` exits non-zero. | Rebuild the image, or install on the host / set `RUNNER_OPENCODE_BIN`. |
| `OpenCode 2.2.1 is required; this runner has <output>.` | Another version installed. | Install exactly 2.2.1. |
| `OpenCode did not answer \`opencode --version\` within 15s: check RUNNER_OPENCODE_BIN points at a working OpenCode 2.2.1.` | Hung or wrong binary. | Point `RUNNER_OPENCODE_BIN` at a working 2.2.1. |
| `No usable harness:anthropic secret for <model>: add the API key in Settings → Connections & secrets (and set SECRETS_KEY).` (or `harness:openai`) | Key missing, unrevealable, or `SECRETS_KEY` unset. | Add the secret for the model's provider. |
| `opencode provider "x" is not supported (supported: anthropic, openai)` / `opencode model "x" must be provider-qualified: provider/model[#variant], …` | Unmapped provider or unqualified model. | Use `anthropic/…` or `openai/…`. |
| `OpenCode needs a usable harness:anthropic or harness:openai secret (and a reachable provider) to list models` | Chat catalog found no models. | Add a valid key; check provider reachability. |
| `provider.no-route`: `Model unavailable: <model>` | CLI error: model not routable, e.g. no or wrong key for it. | Check the key and that the account can use the model. |
| `provider.auth` (`invalid x-api-key`, status 401) | Provider rejected the key. | Replace the secret. |

Quoted strings come from `runner.go`, `chatmodels.go` and `agentdef/opencode.go`;
the last two rows are the captured CLI error envelopes
(`server/internal/harness/testdata/opencode-real-*.jsonl`).

## Testing

`LAZYAGENTS_REAL_OPENCODE=1 go test ./internal/runner -run
TestRealOpenCodeSelfEscalation -v` (in `server/`) needs only the pinned CLI,
no database. A localhost provider with a fake key directs a successful ordinary
write, blocked external-XDG and checkout-metadata writes, and an unavailable
shell attempt after resuming the same session with its history. The test corrects
only the known LAZYAGENTS-26 tool allows so a broken allowlist cannot make this
security regression pass accidentally. Its launcher blocks remote proxy traffic.

`LAZYAGENTS_REAL_OPENCODE=1 TEST_DATABASE_URL=… go test ./internal/runner -run
TestRealOpenCodeAgainstStub` (opt-in, in `server/`) drives the real runner with the
**real `opencode` binary** against a localhost stub provider (no paid call; a
launcher routes the model to the stub, there is no production base-URL setting).
It is evidence of the real wire and runner plumbing, not of a real provider.

`npm run e2e:stack` (in `web/`) drives a real API, runner and Postgres with
fake `opencode`/`claude`/`codex` binaries; see
[`web/e2e-stack/README.md`](../web/e2e-stack/README.md). It proves product
plumbing (catalog, model/harness selection, switching, cancel, history) using
the *synthetic* envelope. It is **not** evidence that real OpenCode 2.2.1 or any
provider works; that needs a separate credentialed run.
