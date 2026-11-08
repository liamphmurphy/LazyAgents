# Harness stream fixtures

`*-auth-error.jsonl` were recorded from the real CLIs (Claude Code 2.1.316,
codex-cli 0.166.0) with deliberately invalid API keys, so the envelopes are
exactly what the CLIs emit; long fields (tool lists, cf-ray ids) were trimmed.

`*-turn.jsonl` describe a working turn with tool calls. Their envelopes match
the recorded ones; the tool lines follow each CLI's documented stream
shapes. Re-record them from a real run with keys when convenient (docs/dev/plan.md §10
nightly smoke test).

`opencode-real-*.jsonl` were recorded from the real OpenCode 2.2.1 binary without any model call
(no credentials, and one invalid fake key), see docs/contracts/opencode2/verification.md.

`opencode-real-stub-*.jsonl` were recorded from the real OpenCode 2.2.1 binary against a localhost
stub provider (no paid call; paths sanitized to `/work/ws`): `text` (plain turn), `tools-blocked` (exact
Vesper config: every file tool is blocked by policy, an MCP call completes), `resume`, `quota-error`
(stub 400), `cancel` (SIGINT mid-stream), and `tools-completed` / `shell-denied`, which were recorded with a
corrected policy (`<tool>:*` resources, `shell`) to show completed tools and the `git push` deny. The model
output, token numbers and the stub's `ses_`/`msg_` ids are the stub's or the CLI's own; they prove the
envelope, not a real provider. See docs/contracts/opencode2/verification.md.

The other `opencode-*.jsonl` files (not `opencode-real-*`) are SYNTHETIC. They follow the assumed
envelope documented at the end of the `opencode_parser.go` header comment, which the real CLI does not
emit (the real success envelope is the `opencode-real-stub-*` one; a real-provider capture is still
missing). Keep them only for the synthetic parser path. Strings that look like credentials in them
(`sk-ant-…`, `la_…`) are fake and exist to prove redaction.
