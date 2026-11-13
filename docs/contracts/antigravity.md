# Antigravity (`agy`) contract

What Vesper's Antigravity harness (`server/internal/harness/antigravity.go`)
assumes about Google's Antigravity CLI, and how sure we are.

> **Status: from Google's documentation, not from a real run.** No `agy` binary
> was available when the adapter was written, so nothing below was recorded from
> the CLI. `server/internal/harness/testdata/antigravity-*.jsonl` follow the
> documented shapes. Before relying on the adapter, run the opt-in probe
> (`AGY_PROBE=1 AGY_BIN=$(command -v agy) go test ./internal/harness -run AgyProbe -v`),
> record real output with `AGY_RECORD=<dir>`, and replace what is marked
> **unverified**. The probe skips when `agy` is absent.

Sources (all antigravity.google/docs unless noted): `cli/headless`,
`cli/reference`, `permissions`, `settings`, `hooks`, `mcp`, `install`, and
github.com/google-antigravity/antigravity-cli (README, CHANGELOG).

## Credentials: an API key, never the sign-in

Google's terms allow calling `agy -p` from a tool but not extracting, storing or
intercepting the user's OAuth credentials. Vesper therefore never handles the
Google login `agy` keeps in the OS keychain. A turn authenticates the way the docs
describe for CI: `settings.json` `modelProvider: "gemini"` plus `GEMINI_API_KEY`.

- The key is the workspace secret `harness:google` (`chatmodels.KeySecret`),
  injected as `GEMINI_API_KEY` (`runner.HarnessKeys`).
- There is no `subscription` kind and no sign-in flow for it; onboarding refuses
  one (`httpapi.checkHarnessCredential`).
- Models are listed from the Gemini API (`chatmodels.discoverGemini`, key in the
  `x-goog-api-key` header). **Unverified:** that `agy --model` takes the Gemini
  API's model ids when `modelProvider` is `gemini`. `agy models` is the CLI's own
  list (the fake backend uses it, `chatmodels.ParseAgyModels`; its output format
  is undocumented, so the parser takes slug-first lines only).

## Invocation

```
python3 .lazyagents/agy-launch.py agy -p "<prompt>" --output-format stream-json \
  [--model M] [--effort E] [--conversation ID]
```

| Flag | Source | Status |
| --- | --- | --- |
| `-p/--print/--prompt`, `--output-format stream-json` | headless | documented |
| `--model`, `--effort` (low…max, per model), `--conversation ID` (resume) | headless | documented; resume with `-p` **unverified** |
| `--dangerously-skip-permissions` | headless | **never passed**: it would defeat the hook and deny rules |
| `--sandbox` | headless | not used: the container is the boundary, as for Codex |

`--effort` is sent only if a definition sets it; the catalog offers no efforts for
Antigravity (`chatmodels.fallbackEfforts`) because which models accept one isn't
documented.

## Event stream

NDJSON, one object per line, an `event` field on each (`init`, `step_update`,
`result`). The parser (`agyParser`) reads:

- `init`: `init.model`. Becomes a `harness_started` lifecycle event.
- `step_update`: `conversation_id` (the session id), `step_index`, `state`
  (`ACTIVE`/`DONE`), `step_type`. `agent_response` steps stream `text_delta`s,
  joined and emitted as one assistant message when the step is `DONE`. `tool`
  steps carry `tool_name` and `tool_info` (`name`, `parameters`, `output`,
  `error.{type,message}`): a `tool_call` when first seen, a `tool_result` at `DONE`.
- `result`: `status` (`SUCCESS`, else a failure), `response`, `error`,
  `conversation_id`, `usage` (`input_tokens`, `output_tokens`, `thinking_tokens`,
  `cache_read_tokens`). The envelope may sit under `result` or at the top level;
  both are read.
- `AGY_ERROR: {...}` on a line is a failure. It is documented as printing to
  stderr, which the runner keeps in the raw log but does not parse; `result.error`
  is what normally carries it.

**Unverified:** every field name above beyond the documented list, whether a
tool's `parameters` arrive at `ACTIVE` or `DONE`, whether `usage` is per process
or cumulative across a resumed conversation (it is documented cumulative only for
multi-turn stream input), and the exact quota-error wording
(`harness.UsageLimit` matches "quota exceeded", "exceeded your quota",
"rate limit"…).

Cost is `unknown`: `agy` reports tokens, and Vesper keeps no Gemini price list.

## Permissions: two layers, fail closed

`agy` has no read-only mode. Vesper writes `~/.gemini/antigravity-cli/settings.json`
in the turn container (`agy-launch.py` from `.lazyagents/agy-config.json`):

1. **Deny rules from the spec** (`agyDenyRules`). Deny beats ask beats allow and
   applies in every mode, so these hold even if the hook doesn't load:
   `command(*)` unless the run may test or write; `write_file(*)` unless it may
   write (not for an image step, whose generator saves elsewhere);
   `read_url(*)`/`execute_url(*)` without web access; everything for a no-tools turn.
2. **A PreToolUse hook** (`agy_policy.py`, installed in
   `~/.gemini/config/hooks.json`) that asks Vesper's policy
   (`runs.CheckTool`), answering `auto_approve` only when Vesper says `allow`.
   An action with no rule is "ask", which headless `agy` denies, so an unloaded
   hook leaves a turn able to do nothing but what the allow rules name
   (`mcp(<server>/*)` for Vesper's own tools).

Tool names are renamed to Claude Code's vocabulary before the policy sees them
(`antigravityTool` / `canonical`; a test runs both). **Unverified:** the real tool
names and argument keys (`run_command` `CommandLine`, `view_file` `AbsolutePath`,
`write_to_file` `TargetFile`, MCP tools as `mcp_<server>_<tool>`, the built-in
generator as `generate_image`). An unrecognized tool keeps its name and so needs
the `tests:run` scope: restricted, not open. If MCP tools arrive under another
name, chats will be denied their tools: visible, not unsafe.

Also **unverified:** that a workspace-trust prompt can't stall a headless run
(`trustedWorkspaces` lists the workspace), that the global `hooks.json` loads
headless, and whether `auto_approve` overrides an "ask" rule.

## MCP

`~/.gemini/config/mcp_config.json`: `{"mcpServers": {name: {"serverUrl", "headers",
"timeoutSeconds"}}}` (Streamable HTTP only). The run token is substituted at launch
from the environment, so it is never in a workspace file.

## Images

`agy` hands image requests to a built-in generator subagent that saves to the
conversation's artifacts. Vesper does not let a chat copy from there past the
policy hook (as Codex does); the runner collects instead
(`collectAntigravityScript`): image paths named in the generator's tool output, or
the files in a directory named for the conversation id, strictly under
`$HOME/.gemini`, into the outbox as `agy-<conversation>-<name>`. **Unverified:**
the artifacts' location and layout, and that the generator works with an API key
rather than only an account (Google's changelog notes image generation is
global-endpoint only).

A chat on Antigravity does not generate images itself: like a Claude chat, it
asks for one with `generate_image`, and a one-shot step runs on the image engine
(workspace setting, persona override, workflow step field; Codex or Antigravity).

## Usage and quota

`/usage` is a TUI slash command; stream input rejects CLI-handled slash commands.
With a Gemini API key there is no plan allowance to read, so the orb shows
Antigravity with a bare ring and "Usage unknown".

## Install

`curl -fsSL https://antigravity.google/cli/install.sh | bash` (binary at
`~/.local/bin/agy`). The agent image runs it at build time, unpinned and
non-fatally (`deploy/agent-base/Dockerfile`, `INSTALL_ANTIGRAVITY`). Record
`agy --version` from the image whenever you verify the adapter.
