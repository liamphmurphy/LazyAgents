# Harness stream fixtures

`*-auth-error.jsonl` were recorded from the real CLIs (Claude Code 2.1.316,
codex-cli 0.166.0) with deliberately invalid API keys, so the envelopes are
exactly what the CLIs emit; long fields (tool lists, cf-ray ids) were trimmed.

`*-turn.jsonl` describe a working turn with tool calls. Their envelopes match
the recorded ones; the tool lines follow each CLI's documented stream
shapes. Re-record them from a real run with keys when convenient (PLAN.md §10
nightly smoke test).
