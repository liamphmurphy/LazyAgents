package agentapi

import (
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

func TestSummariseRunEfficiency(t *testing.T) {
	t0 := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	ev := func(s int, typ, state, id, tool, cmd string, ctx int64) store.RunEfficiencyEventsRow {
		return store.RunEfficiencyEventsRow{Type: typ, At: at(s), State: state, CallID: id, Tool: tool, Command: cmd, ContextTokens: ctx}
	}
	got := summariseRunEfficiency([]store.RunEfficiencyEventsRow{
		ev(0, "lifecycle", "starting", "", "", "", 0),
		ev(95, "lifecycle", "workspace_ready", "", "", "", 0),
		ev(100, "tool_call", "", "a", "Read", "", 40_000),
		ev(101, "tool_call", "", "b", "Bash", "cd server && go test ./internal/harness 2>&1 | tail", 120_000),
		ev(131, "tool_result", "", "b", "Bash", "", 0),
		ev(132, "tool_call", "", "c", "Bash", "git status", 130_000),
		ev(133, "tool_call", "", "d", "Edit", "", 160_000),
		ev(140, "tool_call", "", "e", "shell", "npx playwright test e2e/foo.spec.ts", 0), // no context recorded (Codex)
		ev(200, "tool_result", "", "e", "shell", "", 0),
		ev(201, "tool_call", "", "f", "Write", "", 90_000), // a rollover shrank it
		// A second setup (a resumed workspace).
		ev(300, "lifecycle", "starting", "", "", "", 0),
		ev(305, "lifecycle", "workspace_ready", "", "", "", 0),
	})
	if got.SetupMs != 100_000 || got.Setups != 2 {
		t.Errorf("setup = %d ms over %d, want 100000 over 2", got.SetupMs, got.Setups)
	}
	if got.ToolCalls != 6 || got.CallsBeforeFirstEdit != 3 || got.ContextAtFirstEdit != 160_000 {
		t.Errorf("calls=%d before edit=%d context at edit=%d, want 6/3/160000", got.ToolCalls, got.CallsBeforeFirstEdit, got.ContextAtFirstEdit)
	}
	if got.PeakContext != 160_000 || got.ContextCrossed["100000"] != 2 || got.ContextCrossed["150000"] != 4 || len(got.ContextCrossed) != 2 {
		t.Errorf("peak=%d crossed=%v, want 160000 and 100k@2, 150k@4", got.PeakContext, got.ContextCrossed)
	}
	if got.TestRuns != 2 || got.TestMs != 90_000 || len(got.Tests) != 2 || got.Tests[0].Ms != 30_000 || got.Tests[1].Ms != 60_000 {
		t.Errorf("tests = %d runs %d ms %+v, want 2 runs 90000 ms (30s, 60s)", got.TestRuns, got.TestMs, got.Tests)
	}

	// No edits, no context, no setup: the unknowns stay unknown.
	empty := summariseRunEfficiency([]store.RunEfficiencyEventsRow{ev(0, "tool_call", "", "a", "Grep", "", 0)})
	if empty.CallsBeforeFirstEdit != -1 || empty.PeakContext != 0 || empty.ContextCrossed != nil || empty.Setups != 0 {
		t.Errorf("empty = %+v", empty)
	}
}
