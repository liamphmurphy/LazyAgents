package tracker_test

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
	"testing"
	"time"
)

func TestEpicCriteriaAndGate(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO epics (workspace_id,project_id,name) SELECT $1,id,'test epic' FROM projects WHERE workspace_id=$1 AND key='PAY' RETURNING id`, f.ws).Scan(&id); err != nil {
		t.Fatal(err)
	}
	a, err := f.svc.CreateEpicCriterion(ctx, f.ws, id, "first")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.svc.CreateEpicCriterion(ctx, f.ws, id, "second")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.PatchEpicCriterion(ctx, f.ws, f.maya, b.ID, nil, ptr(true), ptr(1)); err != nil {
		t.Fatal(err)
	}
	cs, err := f.svc.ListEpicCriteria(ctx, f.ws, id)
	if err != nil || len(cs) != 2 || cs[0].ID != b.ID || cs[1].ID != a.ID || cs[0].CheckedByID == nil || *cs[0].CheckedByID != *f.maya.ID || cs[0].CheckedAt == nil {
		t.Fatalf("criteria: %+v %v", cs, err)
	}
	child, err := f.svc.Create(ctx, f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "child", EpicID: &id})
	if err != nil {
		t.Fatal(err)
	}
	blockers, err := f.svc.EpicBlockers(ctx, f.ws, id)
	if err != nil || len(blockers.UncheckedPositions) != 1 || blockers.UncheckedPositions[0] != 2 || len(blockers.OpenTicketKeys) != 1 || blockers.OpenTicketKeys[0] != child.Key {
		t.Fatalf("blockers: %+v %v", blockers, err)
	}
	var conflict *apperr.ConflictError
	if _, err = f.svc.SetEpicStatus(ctx, f.ws, id, "done"); !errors.As(err, &conflict) {
		t.Fatalf("close: %v", err)
	}
	if _, err = f.svc.PatchEpicCriterion(ctx, f.ws, f.maya, a.ID, nil, ptr(true), nil); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.Update(ctx, f.ws, f.maya, child.Key, tracker.Patch{Status: ptr("done")}); err != nil {
		t.Fatal(err)
	}
	// Finishing the last ticket created the epic's final review; it gates the close too.
	if _, err = f.svc.SetEpicStatus(ctx, f.ws, id, "done"); !errors.As(err, &conflict) {
		t.Fatalf("close with review open: %v", err)
	}
	var reviewKey string
	if err = f.pool.QueryRow(ctx, `SELECT key FROM tickets WHERE epic_id=$1 AND epic_review`, id).Scan(&reviewKey); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.Update(ctx, f.ws, f.maya, reviewKey, tracker.Patch{Status: ptr("done")}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.SetEpicStatus(ctx, f.ws, id, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.PatchEpicCriterion(ctx, f.ws, f.maya, b.ID, nil, ptr(false), nil); !errors.As(err, &conflict) {
		t.Fatalf("closed criterion: %v", err)
	}
	if _, err = f.svc.Create(ctx, f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "blocked", EpicID: &id}); !errors.As(err, &conflict) {
		t.Fatalf("closed create: %v", err)
	}
	if _, err = f.svc.Update(ctx, f.ws, f.maya, child.Key, tracker.Patch{Status: ptr("todo")}); !errors.As(err, &conflict) {
		t.Fatalf("closed reopen: %v", err)
	}
	if _, err = f.svc.SetEpicStatus(ctx, f.ws, id, "open"); err != nil {
		t.Fatal(err)
	}
	c, err := f.svc.PatchEpicCriterion(ctx, f.ws, f.maya, b.ID, nil, ptr(false), nil)
	if err != nil || c.CheckedAt != nil || c.CheckedByID != nil || c.CheckedByType != nil {
		t.Fatalf("uncheck: %+v %v", c, err)
	}
	if err = f.svc.DeleteEpicCriterion(ctx, f.ws, b.ID); err != nil {
		t.Fatal(err)
	}
	cs, err = f.svc.ListEpicCriteria(ctx, f.ws, id)
	if err != nil || len(cs) != 1 || cs[0].Position != 1 {
		t.Fatalf("compact: %+v %v", cs, err)
	}
	if _, err = f.svc.ListEpicCriteria(ctx, uuid.New(), id); !errors.Is(err, tracker.ErrNotFound) {
		t.Fatalf("scope: %v", err)
	}
}

func TestEpicCloseSerializesWithTicketCreate(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var id, project uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO epics (workspace_id,project_id,name) SELECT $1,id,'race epic' FROM projects WHERE workspace_id=$1 AND key='PAY' RETURNING id,project_id`, f.ws).Scan(&id, &project); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var pid int32
	if err = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err = store.New(tx).LockProject(ctx, store.LockProjectParams{ID: project, WorkspaceID: f.ws}); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := f.svc.SetEpicStatus(ctx, f.ws, id, "done"); result <- err }()
	for {
		var blocked bool
		err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid)))`, pid).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
	}
	if _, err = tracker.CreateInTx(ctx, store.New(tx), f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "race child", EpicID: &id}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var conflict *apperr.ConflictError
	if err = <-result; !errors.As(err, &conflict) {
		t.Fatalf("close after create: %v", err)
	}
}

func TestProjectLockSerializesEpicChangesAndStalePatch(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var id, project uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO epics (workspace_id,project_id,name) SELECT $1,id,'race epic' FROM projects WHERE workspace_id=$1 AND key='PAY' RETURNING id,project_id`, f.ws).Scan(&id, &project); err != nil {
		t.Fatal(err)
	}
	child, err := f.svc.Create(ctx, f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "child", Status: "done", EpicID: &id})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.svc.Create(ctx, f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "other"})
	if err != nil {
		t.Fatal(err)
	}
	criterion, err := f.svc.CreateEpicCriterion(ctx, f.ws, id, "check")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		action func() error
	}{
		{"create", func() error {
			_, e := f.svc.Create(ctx, f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "new", EpicID: &id})
			return e
		}},
		{"reopen", func() error {
			_, e := f.svc.Update(ctx, f.ws, f.maya, child.Key, tracker.Patch{Status: ptr("todo")})
			return e
		}},
		{"move", func() error {
			_, e := f.svc.Update(ctx, f.ws, f.maya, other.Key, tracker.Patch{EpicID: tracker.SetTo(id)})
			return e
		}},
		{"criterion", func() error {
			_, e := f.svc.PatchEpicCriterion(ctx, f.ws, f.maya, criterion.ID, ptr("changed"), nil, nil)
			return e
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			var pid int32
			if err = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			if _, err = store.New(tx).LockProject(ctx, store.LockProjectParams{ID: project, WorkspaceID: f.ws}); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `UPDATE epics SET status='done' WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { result <- tc.action() }()
			waitForProjectBlock(t, ctx, f, pid)
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var conflict *apperr.ConflictError
			if err = <-result; !errors.As(err, &conflict) {
				t.Fatalf("expected conflict: %v", err)
			}
			if _, err = f.svc.SetEpicStatus(ctx, f.ws, id, "open"); err != nil {
				t.Fatal(err)
			}
		})
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var pid int32
	if err = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err = store.New(tx).LockProject(ctx, store.LockProjectParams{ID: project, WorkspaceID: f.ws}); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE tickets SET revision=revision+1 WHERE id=$1`, other.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, e := f.svc.Update(ctx, f.ws, f.maya, other.Key, tracker.Patch{Revision: ptr(int(other.Revision)), Title: ptr("stale")})
		result <- e
	}()
	waitForProjectBlock(t, ctx, f, pid)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var conflict *apperr.ConflictError
	if err = <-result; !errors.As(err, &conflict) {
		t.Fatalf("stale patch: %v", err)
	}
}
func waitForProjectBlock(t *testing.T, ctx context.Context, f fixture, pid int32) {
	t.Helper()
	for {
		var blocked bool
		if err := f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid)))`, pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
	}
}
