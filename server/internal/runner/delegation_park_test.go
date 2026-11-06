package runner

import (
	"context"
	"github.com/google/uuid"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

func TestDelegationReportParksWithLiveChild(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	agent, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	var ticketID uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM tickets LIMIT 1").Scan(&ticketID); err != nil {
		t.Fatal(err)
	}
	create := func(parent *store.Run) store.Run {
		p := store.CreateRunParams{WorkspaceID: me.WorkspaceID, TicketID: &ticketID, AgentID: agent.ID, Mode: "managed", State: "running", ResolvedSpec: []byte(`{}`), Harness: "claude", Model: "test", Purpose: "ticket"}
		if parent != nil {
			p.ParentRunID = &parent.ID
			p.State = "queued"
		}
		run, err := q.CreateRun(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		return run
	}
	parent := create(nil)
	child := create(&parent)
	turn, err := q.StartTurn(ctx, store.StartTurnParams{RunID: parent.ID, Trigger: "delegation", InputMessage: "premature report"})
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{Pool: pool}
	if report := r.delegationReport(ctx, runCtx{run: parent}, nil, int(turn.Seq)); report != "" {
		t.Fatalf("report while child live: %s", report)
	}
	parked, err := r.parkDelegation(ctx, parent, turn)
	if err != nil || !parked {
		t.Fatalf("parked=%v err=%v", parked, err)
	}
	p, err := q.GetRun(ctx, store.GetRunParams{ID: parent.ID, WorkspaceID: me.WorkspaceID})
	if err != nil || p.State != "waiting" || p.WaitReason == nil || *p.WaitReason != "delegation" {
		t.Fatalf("parent %+v err=%v", p, err)
	}
	child, err = q.SetRunState(ctx, store.SetRunStateParams{ID: child.ID, State: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runs.WakeParent(ctx, q, child, "failed", ""); err != nil {
		t.Fatal(err)
	}
	turns, err := q.ListTurns(ctx, parent.ID)
	if err != nil || len(turns) != 2 || turns[0].Status != "done" || turns[1].Status != "pending" {
		t.Fatalf("turns %+v err=%v", turns, err)
	}
}
