# Switching harness or model mid-chat (LAZYAGENTS-10)

The runner side of switching. `chat_sessions.harness/model` is the selection;
`postChatMessage` (`internal/runs/chat.go`) applies it when the next message is posted.

## Behavior (current segment run vs. the chat's stored selection)

| Stored selection | Effect on next message |
|---|---|
| same harness, same model | resume the segment (unchanged) |
| same harness, new model | update `runs.model` and `resolved_spec.model`; keep run, workspace and CLI session id. Next argv carries the new model (`--model` via `harness.AnthropicModel`, `-m` for Codex, `--model` for Antigravity) |
| different harness (incl. switching back) | complete the old segment run (`chat_segment_ended` event; the runner `Sweep` removes its workspace), create a fresh segment run with no resume session, and send a transcript handoff |

Every harness switch starts fresh; old native sessions are never resumed because the
transcript has moved on. Only `waiting` segments switch; a busy chat still returns 409.
Lock order is unchanged (chat row, then run) and the cancel claim boundary is untouched.

## Handoff

The first turn of a segment that follows earlier messages gets `RenderHandoff`
(`internal/runs/handoff.go`) as its prompt: header, prior messages oldest first, then
`New message from the user` + the user's text. The stored chat message keeps only the
user's text. Budget: `runs.Service.HandoffChars` (default 24000 characters, header and
history together). Truncation keeps the newest messages; the header says how many were
omitted. Query is bounded: newest 40 user/assistant rows, bodies clipped to 8000 chars in
SQL, plus a count for the omitted number. A single oversized newest message is clipped.

## Attribution

Each message stores the harness/model of the run that produced it (`run.Harness/Model`
after any model update), so history stays truthful across switches.

## Integration point for LAZYAGENTS-11

After validating harness/model, call `Queries.SetChatSelection(id, harness, model)` while
holding the chat row lock (`LockChatSession`). Do not touch the run; the runner applies
the selection on the next `PostChatMessage`. If a selection is set while a turn is
running, it applies to the next message after the turn ends. No migration needed.

## Capability and tool changes (no switch)

`startChatTurn` rebuilds the chat's spec from the speaker's current capabilities
(`ChatSpec`) on every message to an idle segment and compares it to the frozen one:

| What changed | Effect on next message |
|---|---|
| coding on/off, sandbox, aside, Codex image generation (`sameWorkspace`) | complete the segment (`chat_segment_ended`, reason `capabilities_changed`) and start a fresh one with a handoff, as before: the runner prepared the workspace for the old spec |
| connectors, secrets, wiki:write, web search, instructions | update `runs.resolved_spec` in place (`SetRunSpec`, event `chat_capabilities_refreshed`), keep the session, and prefix the turn's prompt with a `[Vesper: …]` note of what changed |
| the chat tool set (`runs.ChatTools`) since the segment last recorded it (`runs.chat_tool_names`) | prefix the prompt with the new and removed tool names, then record the current set |

The notes go only into the turn's prompt; the stored chat message keeps the
person's text. A fresh segment records its tools and gets no note. Segments
from before `chat_tool_names` existed (NULL) are told nothing about tools until
their set is recorded on their next message.
