package tracker_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

func newEpic(t *testing.T, f fixture, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO epics (workspace_id,project_id,name) SELECT $1,id,$2 FROM projects WHERE workspace_id=$1 AND key='PAY' RETURNING id`, f.ws, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func epicTicket(t *testing.T, f fixture, epic uuid.UUID, title string, assignee *uuid.UUID) store.Ticket {
	t.Helper()
	tk, err := f.svc.Create(context.Background(), f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: title, EpicID: &epic, AssigneeAgentID: assignee, Status: "in_progress"})
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func reviewTickets(t *testing.T, f fixture, epic uuid.UUID) []store.Ticket {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT key FROM tickets WHERE epic_id=$1 AND epic_review ORDER BY number`, epic)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []store.Ticket
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		tk, err := tracker.Get(context.Background(), f.q, f.ws, k)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, tk)
	}
	return out
}

func setStatus(t *testing.T, f fixture, key, status string) {
	t.Helper()
	if _, err := f.svc.Update(context.Background(), f.ws, f.maya, key, tracker.Patch{Status: &status}); err != nil {
		t.Fatal(err)
	}
}

func mainAgent(t *testing.T, f fixture) store.Agent {
	t.Helper()
	for _, a := range f.agent {
		if a.Kind != "sub" && a.ArchivedAt == nil {
			return a
		}
	}
	t.Fatal("no main agent")
	return store.Agent{}
}

func TestEpicReviewCreatedWhenLastTicketFinishes(t *testing.T) {
	f := setup(t)
	ag := mainAgent(t, f)
	epic := newEpic(t, f, "Ledger")
	a := epicTicket(t, f, epic, "one", &ag.ID)
	b := epicTicket(t, f, epic, "two", &ag.ID)

	setStatus(t, f, a.Key, "done")
	if n := len(reviewTickets(t, f, epic)); n != 0 {
		t.Fatalf("review created early: %d", n)
	}
	setStatus(t, f, b.Key, "done")
	rs := reviewTickets(t, f, epic)
	if len(rs) != 1 {
		t.Fatalf("reviews: %d", len(rs))
	}
	r := rs[0]
	if !r.EpicReview || r.Status != "todo" || r.EpicID == nil || *r.EpicID != epic ||
		r.AssigneeAgentID == nil || *r.AssigneeAgentID != ag.ID || !strings.HasPrefix(r.Title, "Final review: ") ||
		!strings.Contains(r.DescriptionMd, "[["+a.Key+"]] one (done)") {
		t.Fatalf("review ticket: %+v", r)
	}
	cs, err := f.q.ListCriteria(context.Background(), r.ID)
	if err != nil || len(cs) != 4 {
		t.Fatalf("criteria: %d %v", len(cs), err)
	}

	// Idempotent: reopening and finishing again, and the review's own moves, create nothing.
	setStatus(t, f, b.Key, "in_progress")
	setStatus(t, f, b.Key, "done")
	setStatus(t, f, r.Key, "in_progress")
	setStatus(t, f, r.Key, "done")
	if n := len(reviewTickets(t, f, epic)); n != 1 {
		t.Fatalf("reviews after reopen: %d", n)
	}
}

func TestEpicReviewInReviewCounts(t *testing.T) {
	f := setup(t)
	epic := newEpic(t, f, "Ledger")
	a := epicTicket(t, f, epic, "one", nil)
	setStatus(t, f, a.Key, "in_review")
	rs := reviewTickets(t, f, epic)
	if len(rs) != 1 || rs[0].AssigneeAgentID != nil {
		t.Fatalf("reviews: %+v", rs)
	}
}

func TestEpicReviewBulk(t *testing.T) {
	f := setup(t)
	epic := newEpic(t, f, "Ledger")
	a := epicTicket(t, f, epic, "one", nil)
	b := epicTicket(t, f, epic, "two", nil)
	if _, err := f.svc.Bulk(context.Background(), f.ws, f.maya, []string{a.Key, b.Key}, tracker.Patch{Status: ptr("done")}, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(reviewTickets(t, f, epic)); n != 1 {
		t.Fatalf("reviews: %d", n)
	}
}

func TestEpicReviewNotTriggered(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	pending := newEpic(t, f, "Pending")
	a := epicTicket(t, f, pending, "one", nil)
	epicTicket(t, f, pending, "waiting", nil)
	if _, err := f.pool.Exec(ctx, `UPDATE tickets SET status='todo' WHERE epic_id=$1 AND title='waiting'`, pending); err != nil {
		t.Fatal(err)
	}
	setStatus(t, f, a.Key, "done")
	if n := len(reviewTickets(t, f, pending)); n != 0 {
		t.Fatalf("unfinished epic triggered: %d", n)
	}

	closed := newEpic(t, f, "Closed")
	b := epicTicket(t, f, closed, "one", nil)
	if _, err := f.pool.Exec(ctx, `UPDATE epics SET status='done' WHERE id=$1`, closed); err != nil {
		t.Fatal(err)
	}
	setStatus(t, f, b.Key, "done")
	if n := len(reviewTickets(t, f, closed)); n != 0 {
		t.Fatalf("done epic triggered: %d", n)
	}

	dropped := newEpic(t, f, "Dropped")
	c := epicTicket(t, f, dropped, "one", nil)
	setStatus(t, f, c.Key, "canceled")
	if n := len(reviewTickets(t, f, dropped)); n != 0 {
		t.Fatalf("all-canceled epic triggered: %d", n)
	}
}

func TestEpicReviewWhenTicketLeavesEpic(t *testing.T) {
	f := setup(t)
	epic := newEpic(t, f, "Ledger")
	a := epicTicket(t, f, epic, "one", nil)
	b := epicTicket(t, f, epic, "stray", nil)
	setStatus(t, f, a.Key, "done")
	if n := len(reviewTickets(t, f, epic)); n != 0 {
		t.Fatalf("early: %d", n)
	}
	if _, err := f.svc.Update(context.Background(), f.ws, f.maya, b.Key, tracker.Patch{EpicID: tracker.Clear[uuid.UUID]()}); err != nil {
		t.Fatal(err)
	}
	if n := len(reviewTickets(t, f, epic)); n != 1 {
		t.Fatalf("reviews: %d", n)
	}
}

func TestEpicReviewMovesAndArchive(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// A finished ticket moving into an epic whose other work is finished triggers there.
	target := newEpic(t, f, "Target")
	a := epicTicket(t, f, target, "one", nil)
	if _, err := f.pool.Exec(ctx, `UPDATE tickets SET status='todo' WHERE key=$1`, a.Key); err != nil {
		t.Fatal(err)
	}
	loose, err := f.svc.Create(ctx, f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "loose", Status: "done"})
	if err != nil {
		t.Fatal(err)
	}
	setStatus(t, f, a.Key, "canceled")
	if n := len(reviewTickets(t, f, target)); n != 0 {
		t.Fatalf("all-canceled target triggered: %d", n)
	}
	if _, err := f.svc.Update(ctx, f.ws, f.maya, loose.Key, tracker.Patch{EpicID: tracker.SetTo(target)}); err != nil {
		t.Fatal(err)
	}
	reviews := reviewTickets(t, f, target)
	if len(reviews) != 1 {
		t.Fatalf("finished ticket moving in: %d reviews", len(reviews))
	}

	// The review can't leave its epic (that would allow a second one).
	other := newEpic(t, f, "Other")
	if _, err := f.svc.Update(ctx, f.ws, f.maya, reviews[0].Key, tracker.Patch{EpicID: tracker.SetTo(other)}); err == nil {
		t.Fatal("review ticket changed epics")
	}

	// An archived project gets no review, and finishing its last ticket still works.
	arch := newEpic(t, f, "Archived")
	b := epicTicket(t, f, arch, "one", nil)
	if _, err := f.pool.Exec(ctx, `UPDATE projects SET archived=true WHERE workspace_id=$1 AND key='PAY'`, f.ws); err != nil {
		t.Fatal(err)
	}
	setStatus(t, f, b.Key, "done")
	if n := len(reviewTickets(t, f, arch)); n != 0 {
		t.Fatalf("archived project triggered: %d", n)
	}
}

func TestEnsureEpicReview(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	epic := newEpic(t, f, "Ledger")
	a := epicTicket(t, f, epic, "one", nil)
	first, err := f.svc.EnsureEpicReview(ctx, f.ws, epic)
	if err != nil || !first.EpicReview || first.EpicID == nil || *first.EpicID != epic {
		t.Fatalf("ensure: %+v %v", first, err)
	}
	again, err := f.svc.EnsureEpicReview(ctx, f.ws, epic)
	if err != nil || again.ID != first.ID {
		t.Fatalf("not idempotent: %+v %v", again, err)
	}
	if n := len(reviewTickets(t, f, epic)); n != 1 {
		t.Fatalf("reviews: %d", n)
	}

	// The blocker names open tickets and clears once they finish.
	bl, err := tracker.HardBlockers(ctx, f.q, first)
	if err != nil || len(bl) != 1 || !strings.Contains(bl[0], "Final review waits for the epic's other tickets to finish: "+a.Key+" (in_progress)") {
		t.Fatalf("blockers: %v %v", bl, err)
	}
	setStatus(t, f, a.Key, "done")
	if bl, err = tracker.HardBlockers(ctx, f.q, first); err != nil || len(bl) != 0 {
		t.Fatalf("blockers after finish: %v %v", bl, err)
	}

	// Done epic and archived project conflict (when no review exists yet).
	done := newEpic(t, f, "Done epic")
	if _, err := f.pool.Exec(ctx, `UPDATE epics SET status='done' WHERE id=$1`, done); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.EnsureEpicReview(ctx, f.ws, done); !errors.As(err, new(*apperr.ConflictError)) {
		t.Fatalf("done epic: %v", err)
	}
	arch := newEpic(t, f, "Archived")
	if _, err := f.pool.Exec(ctx, `UPDATE projects SET archived=true WHERE workspace_id=$1 AND key='PAY'`, f.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.EnsureEpicReview(ctx, f.ws, arch); !errors.As(err, new(*apperr.ConflictError)) {
		t.Fatalf("archived project: %v", err)
	}
	if _, err := f.svc.EnsureEpicReview(ctx, f.ws, uuid.New()); !errors.Is(err, tracker.ErrNotFound) {
		t.Fatalf("unknown epic: %v", err)
	}
}

func TestEpicReviewPrefersTechLead(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ag := mainAgent(t, f)
	lead := ""
	for h, a := range f.agent {
		if a.ID != ag.ID && a.Kind != "sub" && a.ArchivedAt == nil {
			lead = h
			if _, err := f.pool.Exec(ctx, `UPDATE agents SET handle='tech-lead' WHERE id=$1`, a.ID); err != nil {
				t.Fatal(err)
			}
			epic := newEpic(t, f, "Ledger")
			epicTicket(t, f, epic, "one", &ag.ID)
			r, err := f.svc.EnsureEpicReview(ctx, f.ws, epic)
			if err != nil || r.AssigneeAgentID == nil || *r.AssigneeAgentID != a.ID {
				t.Fatalf("assignee: %+v %v", r, err)
			}
			break
		}
	}
	if lead == "" {
		t.Fatal("no second main agent")
	}
}
