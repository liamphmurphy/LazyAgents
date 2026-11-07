package tracker_test

import (
	"context"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

func TestScopedTicketListsAreComplete(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	original, err := f.q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: f.ws, Key: "PAY-214"})
	if err != nil {
		t.Fatal(err)
	}
	// Bulk fixture crosses the old 500-row cap without 501 service round trips.
	_, err = f.pool.Exec(ctx, `INSERT INTO tickets (workspace_id, project_id, number, key, title, epic_id)
 SELECT $1, $2, 10000+n, 'PAY-' || (10000+n)::text, 'map completeness', $3 FROM generate_series(1,501) n`, f.ws, original.ProjectID, original.EpicID)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []struct {
		name   string
		params store.ListTicketsParams
	}{
		{"project", store.ListTicketsParams{WorkspaceID: f.ws, ProjectID: &original.ProjectID}},
		{"epic", store.ListTicketsParams{WorkspaceID: f.ws, EpicID: original.EpicID}},
	} {
		t.Run(scope.name, func(t *testing.T) {
			tickets, err := f.q.ListTickets(ctx, scope.params)
			if err != nil {
				t.Fatal(err)
			}
			if len(tickets) < 501 {
				t.Fatalf("scoped list truncated: %d", len(tickets))
			}
		})
	}
	tickets, err := f.q.ListTickets(ctx, store.ListTicketsParams{WorkspaceID: f.ws})
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 500 {
		t.Fatalf("workspace search cap changed: %d", len(tickets))
	}
}
