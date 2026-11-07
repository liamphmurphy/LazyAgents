package runs_test

import (
	"context"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

func TestDirectWorkCanFinishWithoutDelegatedReview(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	agent, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	svc := &runs.Service{Pool: pool, Tracker: &tracker.Service{Pool: pool}}
	caller := agents.Caller{Agent: agent, Kind: "unmanaged"}
	run, err := svc.Claim(ctx, caller, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	caller.RunID = &run.ID
	criteria, err := q.ListCriteria(ctx, *run.TicketID)
	if err != nil {
		t.Fatal(err)
	}
	if len(criteria) == 0 {
		t.Fatal("fixture must have acceptance criteria")
	}
	if err := svc.Complete(ctx, caller, "PAY-214", "not verified", "done", ""); err == nil {
		t.Fatal("unchecked criteria were bypassed")
	}
	for i := range criteria {
		if err := svc.CheckCriterion(ctx, caller, "PAY-214", i+1, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Complete(ctx, caller, "PAY-214", "Diff reviewed and criteria verified", "done", ""); err != nil {
		t.Fatal(err)
	}
	tk, err := tracker.Get(ctx, q, me.WorkspaceID, "PAY-214")
	if err != nil || tk.Status != "done" {
		t.Fatalf("%+v %v", tk, err)
	}
	children, err := q.ChildRuns(ctx, &run.ID)
	if err != nil || len(children) != 0 {
		t.Fatalf("unnecessary delegation: %v %v", children, err)
	}
}
