# Harness stream fixtures

`*-auth-error.jsonl` were recorded from the real CLIs (Claude Code 2.1.316,
codex-cli 0.166.0) with deliberately invalid API keys, so the envelopes are
exactly what the CLIs emit; long fields (tool lists, cf-ray ids) were trimmed.

`*-turn.jsonl` describe a working turn with tool calls. Their envelopes match
the recorded ones; the tool lines follow each CLI's documented stream
shapes. Re-record them from a real run with keys when convenient (docs/dev/plan.md §10
nightly smoke test).

`antigravity-*.jsonl` were recorded from the real `agy` 1.5.8 (linux arm64) by
the opt-in probe in antigravity_probe_test.go: `agy` ran headless with the
configuration Vesper's launcher installs, a fake API key, and
`GOOGLE_GEMINI_BASE_URL` pointed at a local stub that scripted the model's
replies (text, tool calls, a 429 with a RetryInfo). Everything agy printed is
its own; only the model's words and the token counts are the stub's. The
workspace path was rewritten to `/workspace`. `antigravity-turn.jsonl` is a
read-only chat whose policy denied `Bash` and `Agent`.
