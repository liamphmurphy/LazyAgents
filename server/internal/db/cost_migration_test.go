package db_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

func TestCostAndCumulativeUsageMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	gooseTo(t, pool, false, 16)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	agent, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	ticket, _ := q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: me.WorkspaceID, Key: "PAY-214"})
	run, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: me.WorkspaceID, TicketID: &ticket.ID, AgentID: agent.ID, Mode: "managed", State: "completed", ResolvedSpec: []byte(`{}`), Harness: "codex", Model: "test", Purpose: "ticket"})
	if err != nil {
		t.Fatal(err)
	}
	for seq, total := range []int{100, 150} {
		turn, err := q.StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "resume"})
		if err != nil {
			t.Fatal(err)
		}
		usage, _ := json.Marshal(map[string]any{"input_tokens": total, "output_tokens": total / 10, "cached_tokens": total / 5, "cost_usd": float64(total) / 100, "cost_source": "computed"})
		if err := q.FinishTurn(ctx, store.FinishTurnParams{ID: turn.ID, Usage: usage}); err != nil {
			t.Fatal(err)
		}
		for _, event := range []struct {
			kind    string
			payload map[string]any
		}{
			{"lifecycle", map[string]any{"state": "turn_started", "turn": seq + 1}},
			{"lifecycle", map[string]any{"state": "harness_started", "harness": "codex", "model": "test", "session_id": "session-a"}},
			{"usage", map[string]any{"input_tokens": total, "output_tokens": total / 10, "cost_usd": float64(total) / 100}},
		} {
			raw, _ := json.Marshal(event.payload)
			if err := q.AddRunEvent(ctx, store.AddRunEventParams{RunID: run.ID, Type: event.kind, Payload: raw}); err != nil {
				t.Fatal(err)
			}
		}
	}
	mustExec(t, pool, `UPDATE runs SET input_tokens=250,output_tokens=25,cost_usd_micros=2500000,cost_source='computed' WHERE id=$1`, run.ID)
	gooseTo(t, pool, true, 17)
	repaired, err := q.GetRun(ctx, store.GetRunParams{ID: run.ID, WorkspaceID: me.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if repaired.InputTokens != 150 || repaired.OutputTokens != 15 || repaired.CostUsdMicros != 1500000 {
		t.Fatalf("cumulative totals not repaired: input=%d output=%d cost=%d", repaired.InputTokens, repaired.OutputTokens, repaired.CostUsdMicros)
	}
	turns, _ := q.ListTurns(ctx, run.ID)
	var last map[string]any
	_ = json.Unmarshal(turns[1].Usage, &last)
	if last["input_tokens"] != float64(50) || last["session_input_tokens"] != float64(150) || last["legacy_cumulative_usage"] == nil {
		t.Fatalf("repair lost baseline or audit record: %s", turns[1].Usage)
	}
	events, _ := q.ListRunEvents(ctx, store.ListRunEventsParams{RunID: run.ID})
	for _, event := range events {
		if event.Type != "usage" {
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(event.Payload, &payload)
		if payload["input_tokens"] == float64(150) {
			t.Fatal("historical usage log still shows cumulative total")
		}
	}
}
