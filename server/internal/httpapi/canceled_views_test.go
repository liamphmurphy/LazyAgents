package httpapi

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

// Canceled tickets are closed work: they leave the attention list, the ready
// view, the overview queue, planning readiness and agents' open-ticket counts.
func TestCanceledTicketsLeaveAttentionAndViews(t *testing.T) {
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
	ready, err := q.ListTickets(ctx, store.ListTicketsParams{WorkspaceID: u.WorkspaceID, Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) == 0 {
		t.Fatal("seed has no ready tickets")
	}
	key := ready[0].Key
	var ticketID uuid.UUID
	var agentID *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id, assignee_agent_id FROM tickets WHERE workspace_id=$1 AND key=$2`, u.WorkspaceID, key).Scan(&ticketID, &agentID); err != nil {
		t.Fatal(err)
	}

	type snapshot struct {
		inbox, ready, overview, planning bool
		open                             int64
	}
	look := func() snapshot {
		t.Helper()
		var s snapshot
		inbox, err := q.ListInbox(ctx, store.ListInboxParams{WorkspaceID: u.WorkspaceID, UserID: u.ID})
		if err != nil {
			t.Fatal(err)
		}
		s.inbox = slices.ContainsFunc(inbox, func(r store.ListInboxRow) bool { return r.TicketKey == key && r.Kind == "dispatch" })
		rows, err := q.ListTickets(ctx, store.ListTicketsParams{WorkspaceID: u.WorkspaceID, Ready: true})
		if err != nil {
			t.Fatal(err)
		}
		s.ready = slices.ContainsFunc(rows, func(r store.ListTicketsRow) bool { return r.Key == key })
		queue, err := q.OverviewAgentTickets(ctx, u.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		s.overview = slices.ContainsFunc(queue, func(r store.OverviewAgentTicketsRow) bool { return r.Key == key })
		keys, err := q.ReadyPlanningTickets(ctx, store.ReadyPlanningTicketsParams{WorkspaceID: u.WorkspaceID, Keys: []string{key}})
		if err != nil {
			t.Fatal(err)
		}
		s.planning = slices.Contains(keys, key)
		if s.open, err = q.CountOpenTicketsForAgent(ctx, agentID); err != nil {
			t.Fatal(err)
		}
		return s
	}

	before := look()
	if !before.inbox || !before.ready || !before.overview || !before.planning || before.open == 0 {
		t.Fatalf("open ticket %s missing before cancel: %+v", key, before)
	}
	if _, err := pool.Exec(ctx, `UPDATE tickets SET status='canceled' WHERE id=$1`, ticketID); err != nil {
		t.Fatal(err)
	}
	after := look()
	if after.inbox || after.ready || after.overview || after.planning || after.open != before.open-1 {
		t.Fatalf("canceled ticket %s still visible: before %+v, after %+v", key, before, after)
	}
}
