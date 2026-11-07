package db_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

func TestCostAndCumulativeUsageMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	gooseTo(t, pool, false, 16)
	// The seed uses current queries, which read agents.archived_at (migration 19).
	mustExec(t, pool, `ALTER TABLE agents ADD COLUMN archived_at timestamptz`)
	// ...and tickets/epics.requires_mockup (migration 24).
	mustExec(t, pool, `ALTER TABLE tickets ADD COLUMN requires_mockup boolean NOT NULL DEFAULT false`)
	mustExec(t, pool, `ALTER TABLE epics ADD COLUMN requires_mockup boolean NOT NULL DEFAULT false`)
	// ...and the ticket revision/estimate and epic status columns (migration 25).
	mustExec(t, pool, `ALTER TABLE tickets ADD COLUMN revision integer NOT NULL DEFAULT 1, ADD COLUMN estimate_points integer,
		ADD COLUMN estimate_rationale text, ADD COLUMN estimated_by_type text, ADD COLUMN estimated_by_id uuid, ADD COLUMN estimated_at timestamptz`)
	mustExec(t, pool, `ALTER TABLE epics ADD COLUMN status text NOT NULL DEFAULT 'open'`)
	// ...and the epic review flag (migration 27).
	mustExec(t, pool, `ALTER TABLE tickets ADD COLUMN epic_review boolean NOT NULL DEFAULT false`)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	var agentID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM agents WHERE workspace_id=$1 AND handle='atlas'`, me.WorkspaceID).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	ticket, _ := q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: me.WorkspaceID, Key: "PAY-214"})
	// Raw SQL: sqlc's CreateRun returns every current column, and this schema is older.
	var runID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO runs (workspace_id, ticket_id, agent_id, mode, state, resolved_spec, harness, model, purpose)
		VALUES ($1,$2,$3,'managed','completed','{}','codex','test','ticket') RETURNING id`, me.WorkspaceID, ticket.ID, agentID).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}
	for seq, total := range []int{100, 150} {
		turn, err := q.StartTurn(ctx, store.StartTurnParams{RunID: runID, Trigger: "resume"})
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
			if err := q.AddRunEvent(ctx, store.AddRunEventParams{RunID: runID, Type: event.kind, Payload: raw}); err != nil {
				t.Fatal(err)
			}
		}
	}
	mustExec(t, pool, `UPDATE runs SET input_tokens=250,output_tokens=25,cost_usd_micros=2500000,cost_source='computed' WHERE id=$1`, runID)
	gooseTo(t, pool, true, 17)
	var input, output, cost int64
	if err := pool.QueryRow(ctx, `SELECT input_tokens, output_tokens, cost_usd_micros FROM runs WHERE id=$1`, runID).Scan(&input, &output, &cost); err != nil {
		t.Fatal(err)
	}
	if input != 150 || output != 15 || cost != 1500000 {
		t.Fatalf("cumulative totals not repaired: input=%d output=%d cost=%d", input, output, cost)
	}
	turns, _ := q.ListTurns(ctx, runID)
	var last map[string]any
	_ = json.Unmarshal(turns[1].Usage, &last)
	if last["input_tokens"] != float64(50) || last["session_input_tokens"] != float64(150) || last["legacy_cumulative_usage"] == nil {
		t.Fatalf("repair lost baseline or audit record: %s", turns[1].Usage)
	}
	events, _ := q.ListRunEvents(ctx, store.ListRunEventsParams{RunID: runID})
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
