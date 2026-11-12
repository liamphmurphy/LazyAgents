package runner_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A long run's handoff is its state brief plus a short tail, not its history.
func TestHandoffPromptOfLongRunStaysSmall(t *testing.T) {
	e := setup(t)
	e.dispatch("PAY-214")
	e.until("waiting for approval", e.state("PAY-214=waiting"))
	run := e.run("PAY-214")
	// 20 turns of large inputs and many large events.
	for i := 0; i < 20; i++ {
		input := fmt.Sprintf("TURN-%02d ", i) + strings.Repeat("x", 15000)
		if _, err := e.pool.Exec(e.ctx, `INSERT INTO run_turns (run_id, seq, trigger, input_message, status) VALUES ($1, 100+$2, 'comment', $3, 'done')`, run.ID, i, input); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 5; j++ {
			id := fmt.Sprintf("call-%d-%d", i, j)
			call, _ := json.Marshal(map[string]any{"id": id, "tool": "Bash", "input": map[string]any{"command": fmt.Sprintf("CMD-%02d-%d ", i, j) + strings.Repeat("c", 3000)}})
			res, _ := json.Marshal(map[string]any{"id": id, "tool": "Bash", "output": "OUT\n" + strings.Repeat("o", 4000)})
			msg, _ := json.Marshal(map[string]any{"role": "assistant", "text": strings.Repeat("a", 5000)})
			for _, ev := range []store.AddRunEventParams{{RunID: run.ID, Type: "tool_call", Payload: call}, {RunID: run.ID, Type: "tool_result", Payload: res}, {RunID: run.ID, Type: "message", Payload: msg}} {
				if err := e.q.AddRunEvent(e.ctx, ev); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	prompt, err := e.runner.HandoffPrompt(e.ctx, run.ID, "claude-code", "opus", "exhausted its provider usage")
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt) >= 40000 {
		t.Fatalf("handoff is %d characters", len(prompt))
	}
	for _, want := range []string{
		"Continue this existing Vesper run after claude-code (opus) exhausted its provider usage.",
		"## Ticket PAY-214:", "## Acceptance criteria", "## Git", "Branch: ",
		"## Newest input\n\nTURN-19 ", "tool Bash: CMD-19-4 ", "→ OUT o",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("handoff lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "TURN-18") || strings.Contains(prompt, `"payload"`) || strings.Contains(prompt, "Previous input") {
		t.Errorf("handoff carries older inputs or raw events:\n%s", prompt)
	}
	tail := prompt[strings.Index(prompt, "## Last activity"):]
	if n := strings.Count(tail, "\n- "); n > 15 {
		t.Errorf("tail has %d entries", n)
	}
}
