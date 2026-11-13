# Harness stream fixtures

`*-auth-error.jsonl` were recorded from the real CLIs (Claude Code 2.1.316,
codex-cli 0.166.0) with deliberately invalid API keys, so the envelopes are
exactly what the CLIs emit; long fields (tool lists, cf-ray ids) were trimmed.

`*-turn.jsonl` describe a working turn with tool calls. Their envelopes match
the recorded ones; the tool lines follow each CLI's documented stream
shapes. Re-record them from a real run with keys when convenient (docs/dev/plan.md §10
nightly smoke test).

`antigravity-*.jsonl` are NOT recorded: no `agy` binary was available when they
were written. They follow the stream-json shapes in Google's headless docs
(docs/contracts/antigravity.md). Replace them with recordings from a real
run (the opt-in probe in antigravity_probe_test.go) before relying on them.
