# Harness stream fixtures

`*-auth-error.jsonl` were recorded from the real CLIs (Claude Code 2.1.316,
codex-cli 0.166.0) with deliberately invalid API keys, so the envelopes are
exactly what the CLIs emit; long fields (tool lists, cf-ray ids) were trimmed.

`*-turn.jsonl` describe a working turn with tool calls. Their envelopes match
the recorded ones; the tool lines follow each CLI's documented stream
shapes. Re-record them from a real run with keys when convenient (PLAN.md §10
nightly smoke test).

`opencode-real-*.jsonl` were recorded from the real OpenCode 2.2.1 binary without any model call
(no credentials, and one invalid fake key), see docs/contracts/opencode2/verification.md.

The other `opencode-*.jsonl` files are SYNTHETIC. The OpenCode 2.2.1 success wire format has not been
captured (docs/contracts/opencode2/ADR.md, "Normalization boundary"); they
follow the assumed envelope documented in `opencode_parser.go`, not real CLI
output. Replace them with sanitized captures (success, auth failure, tool/MCP
failure, resume, cancellation) before enabling the harness. Strings that look
like credentials in them (`sk-ant-…`, `la_…`) are fake and exist to prove redaction.
