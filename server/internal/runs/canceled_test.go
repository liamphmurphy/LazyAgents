package runs_test

import (
	"context"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func TestCanceledTicketCannotBeClaimedOrDispatched(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	u, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: u.WorkspaceID, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := agents.IssueToken(ctx, q, a.ID, "unmanaged", "test", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := agents.ResolveToken(ctx, q, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tickets SET status='canceled' WHERE workspace_id=$1 AND key='PAY-214'`, u.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	svc := &runs.Service{Pool: pool, Tracker: &tracker.Service{Pool: pool}}
	if _, err := svc.Claim(ctx, caller, "PAY-214"); !isConflict(err) {
		t.Fatalf("claim: expected conflict, got %v", err)
	}
	if _, err := svc.Dispatch(ctx, u.WorkspaceID, u.ID, []string{"PAY-214"}); !isConflict(err) {
		t.Fatalf("dispatch: expected conflict, got %v", err)
	}
	queue, err := q.AgentQueue(ctx, &a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ticket := range queue {
		if ticket.Key == "PAY-214" {
			t.Fatal("canceled ticket appeared in agent queue")
		}
	}
}
