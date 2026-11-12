package runner_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// planRollover runs a lead that marks step 1 of a two-step plan done with a
// note, with ROLLOVER_CONTEXT_TOKENS=limit and FAKE_HARNESS_CONTEXT_TOKENS=tokens,
// and returns the finished run and the harness invocations.
func planRollover(t *testing.T, limit int64, tokens string) (env, store.Run, []map[string]string) {
	t.Helper()
	e := setup(t)
	e.runner.Cfg.RolloverContextTokens = limit
	e.runs.RolloverContextTokens = limit
	logPath := filepath.Join(t.TempDir(), "log.jsonl")
	t.Setenv("FAKE_HARNESS_MODE", "plan_rollover")
	t.Setenv("FAKE_HARNESS_LOG", logPath)
	t.Setenv("FAKE_HARNESS_CONTEXT_TOKENS", tokens)
	e.dispatch("PAY-214")
	e.until("completed", e.state("PAY-214=completed"))
	return e, e.run("PAY-214"), readLog(t, logPath)
}

func TestLargeLeadRollsOverAfterPlanStep(t *testing.T) {
	e, run, lines := planRollover(t, 100000, "180000")
	if len(lines) != 2 {
		t.Fatalf("invocations %d: %+v", len(lines), lines)
	}
	second := lines[1]
	if second["resume"] != "" {
		t.Fatalf("rolled-over turn resumed session %q", second["resume"])
	}
	p := second["full_prompt"]
	for _, want := range []string{
		"Your earlier session was closed to keep context small; this brief is your state.",
		"## Plan", "1. [done] Write the change",
		"Note: The setting lives in config.go; keep it off by default.",
		"2. [current] Ship it",
		"## Git", "Branch: ", "Write the change",
		"## Next", "Continue with the plan.",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("rollover prompt lacks %q:\n%s", want, p)
		}
	}
	found := false
	evs, _ := e.q.ListRunEvents(e.ctx, store.ListRunEventsParams{RunID: run.ID, Seq: 0})
	for _, ev := range evs {
		var pl map[string]any
		_ = json.Unmarshal(ev.Payload, &pl)
		if pl["state"] == "session_rollover" {
			found = true
			if pl["reason"] != "plan_step" || pl["context_tokens"] != float64(180000) {
				t.Fatalf("rollover event %s", ev.Payload)
			}
		}
	}
	if !found {
		t.Fatalf("no session_rollover event: %s", e.eventTypes(run.ID))
	}
	if run.RolloverRequested {
		t.Fatal("rollover_requested still set")
	}
	if run.IdleTurns != 0 {
		t.Fatalf("idle_turns %d", run.IdleTurns)
	}
	turns, _ := e.q.ListTurns(e.ctx, run.ID)
	if len(turns) != 2 || turns[1].Trigger != "resume" {
		t.Fatalf("turns %+v", turns)
	}
}

func TestLeadWithoutRolloverAfterPlanStep(t *testing.T) {
	for name, c := range map[string]struct {
		limit  int64
		tokens string
	}{"below": {100000, "50000"}, "disabled": {0, "180000"}} {
		t.Run(name, func(t *testing.T) {
			e, run, lines := planRollover(t, c.limit, c.tokens)
			if len(lines) != 1 {
				t.Fatalf("invocations %d: %+v", len(lines), lines)
			}
			if deref(run.Outcome) != "Done without a rollover" {
				t.Fatalf("outcome %q", deref(run.Outcome))
			}
			if run.RolloverRequested || e.hasLifecycle(run.ID, "session_rollover") {
				t.Fatal("unexpected rollover")
			}
		})
	}
}
