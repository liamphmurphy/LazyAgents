# OpenCode harness

OpenCode is a third harness next to Claude Code and Codex: public ID `opencode`,
official CLI `@opencode/cli`, executable `opencode`. It is **opt-in and off by
default** (see [Isolation](#isolation-attestation)).

> **Status:** wired end to end, but verified only against a fake CLI and two
> real *error* captures. Real-provider parity is **not yet verified**; see
> [Limitations](#limitations).

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
the attestation if you know the host has none.

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
LazyAgents generates for a run. The runner cannot check this itself. Without it:

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

- **Real-provider parity is not verified.** No paid model call has been made.
  Success, tool/MCP, resume, quota and cancel event shapes, the completion
  marker and whether usage is per-turn or cumulative are **unverified**; the
  parser's fixtures for them are synthetic. Wire was captured only for two
  errors (`provider.no-route`, `provider.auth` 401), which the parser reads.
- **Cost:** a native cost from the CLI is used (`cost_source=harness`, including
  zero). Otherwise cost is computed only from an operator price in
  `RUNNER_MODEL_PRICES` for the exact model id (e.g. `openai/gpt-5#high`), with
  unverified token units. With neither, it is `unknown` (`cost_known=false`)
  and shown as unknown/partial, never as free. Unknown cost can't show a budget
  is unspent; runtime and turn limits still apply.
- **Policy:** the generated `opencode.json` targets the 2.2.1 `integration.use`
  contract. Whether its denials and MCP allowlist are effective, and config-layer
  isolation, are unverified (they need a model call). Newer `mcp.use`,
  `skill.use`, `plugin.use`, `integration.connect` options are rejected.
  OpenCode permission rules are not a security boundary for shell access.
- **No approvals:** with no authenticated pre-tool bridge, scopes set to
  require approval are rejected, except brokered/API-only ones. A CLI prompt is
  not LazyAgents approval. No sandbox guarantees; no `permission_mode`.
- **No unmanaged mode:** OpenCode never runs without the attestation and a key.
- **Chat is read-only** (scratch workspace, project-scoped context tools).
- Docker backend runs against a real image are unexercised; only local with
  the fake CLI was.

## Troubleshooting

| Message (starts with) | Cause | Fix |
| --- | --- | --- |
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

`npm run e2e:stack` (in `web/`) drives a real API, runner and Postgres with
fake `opencode`/`claude`/`codex` binaries; see
[`web/e2e-stack/README.md`](../web/e2e-stack/README.md). It proves product
plumbing (catalog, model/harness selection, switching, cancel, history) using
the *synthetic* envelope. It is **not** evidence that real OpenCode 2.2.1 or any
provider works; that needs a separate credentialed run.
