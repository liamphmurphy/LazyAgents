package runs_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

func TestRepositoryForEpicReview(t *testing.T) {
	ctx := context.Background()
	pool, _, u, _, _ := boardRaceFixture(t)
	q := store.New(pool)
	tr := &tracker.Service{Pool: pool}
	var epic uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO epics (workspace_id,project_id,name) SELECT $1,id,'Ledger' FROM projects WHERE workspace_id=$1 AND key='PAY' RETURNING id`, u.WorkspaceID).Scan(&epic); err != nil {
		t.Fatal(err)
	}
	a, err := tr.Create(ctx, u.WorkspaceID, tracker.UserActor(u.ID), tracker.NewTicket{ProjectKey: "PAY", Title: "one", EpicID: &epic})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tickets SET branch='pay-1', pr_url='https://github.com/o/r/pull/3', pr_state='open' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Update(ctx, u.WorkspaceID, tracker.UserActor(u.ID), a.Key, tracker.Patch{Status: ptr("in_review")}); err != nil {
		t.Fatal(err)
	}
	var reviewKey string
	if err := pool.QueryRow(ctx, `SELECT key FROM tickets WHERE epic_id=$1 AND epic_review`, epic).Scan(&reviewKey); err != nil {
		t.Fatal(err)
	}
	p, err := q.GetProject(ctx, store.GetProjectParams{ID: a.ProjectID, WorkspaceID: u.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}

	repo, err := runs.RepositoryFor(ctx, q, p, mustTicket(t, q, u.WorkspaceID, reviewKey))
	if err != nil {
		t.Fatal(err)
	}
	if !repo.EpicReview || repo.EpicBranch == "" || len(repo.EpicTickets) != 1 {
		t.Fatalf("review repo: %+v", repo)
	}
	got := repo.EpicTickets[0]
	if got.Key != a.Key || got.Status != "in_review" || got.Branch != "pay-1" || got.PRURL != "https://github.com/o/r/pull/3" || got.PRState != "open" {
		t.Fatalf("epic ticket: %+v", got)
	}

	repo, err = runs.RepositoryFor(ctx, q, p, mustTicket(t, q, u.WorkspaceID, a.Key))
	if err != nil || repo.EpicReview || len(repo.EpicTickets) != 0 {
		t.Fatalf("regular ticket repo: %+v %v", repo, err)
	}
}
