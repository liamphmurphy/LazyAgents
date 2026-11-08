package httpapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func TestRunCacheBreakdownIncludesLegacyAndUnknownInput(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	agent, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	tk, _ := tracker.Get(ctx, q, me.WorkspaceID, "PAY-214")
	run, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: me.WorkspaceID, TicketID: &tk.ID, AgentID: agent.ID, Mode: "managed", State: "completed", ResolvedSpec: json.RawMessage(`{}`), Harness: "codex", Model: "fake", Purpose: "ticket"})
	if err != nil {
		t.Fatal(err)
	}
	for _, usage := range []string{
		`{"input_tokens":1000,"cached_tokens":980,"cache_reported_input_tokens":1000}`,
		`{"input_tokens":1000,"cached_tokens":0,"cache_reported_input_tokens":0}`,
		`{"input_tokens":1000,"cached_tokens":900,"session_input_tokens":3000}`,
	} {
		turn, err := q.StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "start", InputMessage: "fake"})
		if err != nil {
			t.Fatal(err)
		}
		if err = q.FinishTurn(ctx, store.FinishTurnParams{ID: turn.ID, Usage: json.RawMessage(usage)}); err != nil {
			t.Fatal(err)
		}
	}
	run, err = q.AddRunUsage(ctx, store.AddRunUsageParams{ID: run.ID, InputTokens: 3400, OutputTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := q.CacheUsageForRuns(ctx, []uuid.UUID{run.ID})
	if err != nil || len(rows) != 1 {
		t.Fatalf("%v %+v", err, rows)
	}
	if rows[0].ReportedInputTokens != 2000 || rows[0].CachedInputTokens != 1880 {
		t.Fatal(rows)
	}
	got, err := (&Server{Pool: pool, Q: q}).toRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	if *got.CachedInputTokens != 1880 || *got.UncachedInputTokens != 120 || *got.UnreportedInputTokens != 1400 || got.InputTokens != 3400 {
		t.Fatalf("%+v", got)
	}
}

func TestCacheBreakdownClampsPartialCounters(t *testing.T) {
	c, u, n := cacheBreakdown(100, 1000, 500)
	if c != 100 || u != 0 || n != 0 {
		t.Fatal(c, u, n)
	}
	c, u, n = cacheBreakdown(100, 0, 0)
	if c != 0 || u != 0 || n != 100 {
		t.Fatal(c, u, n)
	}
}
