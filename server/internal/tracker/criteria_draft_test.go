package tracker_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func draftFixture(t *testing.T, f fixture, texts ...string) (store.Ticket, []store.Criterion) {
	t.Helper()
	ctx := context.Background()
	tk, err := f.svc.Create(ctx, f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "draft", Criteria: texts})
	if err != nil {
		t.Fatal(err)
	}
	crit, err := f.q.ListCriteria(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	return tk, crit
}

func expectedOf(crit []store.Criterion) []tracker.ExpectedCriterion {
	out := []tracker.ExpectedCriterion{}
	for _, c := range crit {
		out = append(out, tracker.ExpectedCriterion{ID: c.ID, Position: int(c.Position), Text: c.Text, Done: c.Done})
	}
	return out
}

func assertUnchanged(t *testing.T, f fixture, tk store.Ticket, crit []store.Criterion) {
	t.Helper()
	got, err := f.q.ListCriteria(context.Background(), tk.ID)
	if err != nil || len(got) != len(crit) {
		t.Fatalf("criteria changed: %+v %v", got, err)
	}
	for i := range got {
		if got[i].ID != crit[i].ID || got[i].Text != crit[i].Text || got[i].Done != crit[i].Done || got[i].Position != crit[i].Position {
			t.Fatalf("criterion %d changed: %+v vs %+v", i, got[i], crit[i])
		}
	}
	now, _ := tracker.Get(context.Background(), f.q, f.ws, tk.Key)
	if now.Title != tk.Title || now.Revision != tk.Revision {
		t.Fatalf("ticket changed: %+v", now)
	}
}

func TestCriteriaDraftAtomicSave(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tk, crit := draftFixture(t, f, "one", "two", "three", "four")
	rev := int(tk.Revision)
	title := "draft renamed"
	a, b, c := crit[0].ID, crit[1].ID, crit[2].ID
	// reorder (c first), edit a, tick b, drop d, add new
	patch := tracker.Patch{Revision: &rev, Title: &title, Criteria: &tracker.CriteriaDraft{
		Expected: expectedOf(crit),
		Items: []tracker.CriterionDraft{
			{ID: &c, Text: "three"}, {ID: &a, Text: " one edited "}, {ID: &b, Text: "two", Done: true}, {Text: "five", Done: true},
		},
	}}
	got, err := f.svc.Update(ctx, f.ws, f.maya, tk.Key, patch)
	if err != nil || got.Title != title || got.Revision != tk.Revision+1 {
		t.Fatalf("update: %+v %v", got, err)
	}
	after, _ := f.q.ListCriteria(ctx, tk.ID)
	if len(after) != 4 {
		t.Fatalf("criteria: %+v", after)
	}
	want := []struct {
		id   uuid.UUID
		text string
		done bool
	}{{c, "three", false}, {a, "one edited", false}, {b, "two", true}, {uuid.Nil, "five", true}}
	for i, w := range want {
		g := after[i]
		if g.Position != int32(i+1) || g.Text != w.text || g.Done != w.done || (w.id != uuid.Nil && g.ID != w.id) {
			t.Fatalf("item %d: %+v want %+v", i, g, w)
		}
	}
	vs := strings.Join(verbs(t, f, tk.Key), ",")
	for _, v := range []string{tracker.VerbCriterionAdded, tracker.VerbCriterionChecked, tracker.VerbCriterionRemoved} {
		if !strings.Contains(vs, v) {
			t.Fatalf("missing %s in %s", v, vs)
		}
	}

	// criteria-only save keeps existing revision semantics (no field change, no bump)
	rev = int(got.Revision)
	first := after[0].ID
	only, err := f.svc.Update(ctx, f.ws, f.maya, tk.Key, tracker.Patch{Revision: &rev, Criteria: &tracker.CriteriaDraft{
		Expected: expectedOf(after), Items: []tracker.CriterionDraft{{ID: &first, Text: "three"}},
	}})
	if err != nil || only.Revision != got.Revision {
		t.Fatalf("criteria-only: %+v %v", only, err)
	}
	if left, _ := f.q.ListCriteria(ctx, tk.ID); len(left) != 1 || left[0].Position != 1 {
		t.Fatalf("left: %+v", left)
	}
}

func TestCriteriaDraftRejections(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tk, crit := draftFixture(t, f, "one", "two")
	other, ocrit := draftFixture(t, f, "elsewhere")
	rev := int(tk.Revision)
	title := "must not land"
	id0, id1 := crit[0].ID, crit[1].ID
	draft := func(items ...tracker.CriterionDraft) *tracker.CriteriaDraft {
		return &tracker.CriteriaDraft{Expected: expectedOf(crit), Items: items}
	}
	field := func(err error) string {
		var ve *tracker.ValidationError
		var ce *apperr.ConflictError
		switch {
		case errors.As(err, &ve):
			return "422:" + ve.Field
		case errors.As(err, &ce):
			return "409:" + ce.Field
		}
		return "other:" + errString(err)
	}
	tooMany := make([]tracker.CriterionDraft, 201)
	for i := range tooMany {
		tooMany[i] = tracker.CriterionDraft{Text: "x"}
	}
	stale := rev + 5
	cases := []struct {
		name string
		p    tracker.Patch
		want string
	}{
		{"missing revision", tracker.Patch{Title: &title, Criteria: draft(tracker.CriterionDraft{ID: &id0, Text: "one"})}, "422:revision"},
		{"stale revision", tracker.Patch{Revision: &stale, Title: &title, Criteria: draft(tracker.CriterionDraft{ID: &id0, Text: "one"})}, "409:revision"},
		{"duplicate id", tracker.Patch{Revision: &rev, Title: &title, Criteria: draft(tracker.CriterionDraft{ID: &id0, Text: "a"}, tracker.CriterionDraft{ID: &id0, Text: "b"})}, "422:criteria"},
		{"foreign id", tracker.Patch{Revision: &rev, Title: &title, Criteria: draft(tracker.CriterionDraft{ID: &ocrit[0].ID, Text: "a"})}, "422:criteria"},
		{"unknown id", tracker.Patch{Revision: &rev, Title: &title, Criteria: draft(tracker.CriterionDraft{ID: ptr(uuid.New()), Text: "a"})}, "422:criteria"},
		{"empty text", tracker.Patch{Revision: &rev, Title: &title, Criteria: draft(tracker.CriterionDraft{ID: &id1, Text: "  "})}, "422:criteria"},
		{"too many", tracker.Patch{Revision: &rev, Title: &title, Criteria: draft(tooMany...)}, "422:criteria"},
		{"snapshot mismatch", tracker.Patch{Revision: &rev, Title: &title, Criteria: &tracker.CriteriaDraft{
			Expected: expectedOf(crit[:1]), Items: []tracker.CriterionDraft{{ID: &id0, Text: "one"}}}}, "409:criteria"},
	}
	for _, c := range cases {
		_, err := f.svc.Update(ctx, f.ws, f.maya, tk.Key, c.p)
		if got := field(err); got != c.want {
			t.Fatalf("%s: got %s want %s", c.name, got, c.want)
		}
		assertUnchanged(t, f, tk, crit)
	}
	assertUnchanged(t, f, other, ocrit)

	// A legacy write between load and save invalidates the snapshot.
	if _, err := f.svc.UpdateCriterion(ctx, f.ws, f.maya, id0, nil, ptr(true)); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.Update(ctx, f.ws, f.maya, tk.Key, tracker.Patch{Revision: &rev, Title: &title, Criteria: draft(tracker.CriterionDraft{ID: &id0, Text: "one"})})
	if field(err) != "409:criteria" {
		t.Fatalf("legacy interleave: %v", err)
	}
	now, _ := tracker.Get(ctx, f.q, f.ws, tk.Key)
	if now.Title != tk.Title {
		t.Fatal("title changed on conflict")
	}

	// A criterion the draft still keeps was deleted concurrently: stale (409), not invalid (422).
	if err := f.svc.DeleteCriterion(ctx, f.ws, f.maya, id1); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.Update(ctx, f.ws, f.maya, tk.Key, tracker.Patch{Revision: &rev, Title: &title, Criteria: draft(tracker.CriterionDraft{ID: &id1, Text: "two"})})
	if field(err) != "409:criteria" {
		t.Fatalf("concurrent delete: got %s", field(err))
	}

	// Bulk rejects criteria.
	_, err = f.svc.Bulk(ctx, f.ws, f.maya, []string{tk.Key}, tracker.Patch{Criteria: draft()}, nil)
	if field(err) != "422:criteria" {
		t.Fatalf("bulk: %v", err)
	}
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func TestOmittedCriteriaLeavesPatchUnchanged(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tk, crit := draftFixture(t, f, "one", "two")
	rev := int(tk.Revision)
	title := "only title"
	if _, err := f.svc.Update(ctx, f.ws, f.maya, tk.Key, tracker.Patch{Revision: &rev, Title: &title}); err != nil {
		t.Fatal(err)
	}
	after, _ := f.q.ListCriteria(ctx, tk.ID)
	if len(after) != 2 || after[0].ID != crit[0].ID || after[1].ID != crit[1].ID {
		t.Fatalf("criteria touched: %+v", after)
	}
}

// Legacy writers must wait behind a held ticket row lock (as a draft save holds it).
func TestLegacyCriterionWritersTakeTicketLock(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tk, crit := draftFixture(t, f, "one")
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := store.New(tx).LockTicketByKey(ctx, store.LockTicketByKeyParams{WorkspaceID: f.ws, Key: tk.Key}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 3)
	go func() { _, err := f.svc.AddCriterion(ctx, f.ws, f.maya, tk.Key, "late"); done <- err }()
	go func() { _, err := f.svc.UpdateCriterion(ctx, f.ws, f.maya, crit[0].ID, nil, ptr(true)); done <- err }()
	go func() { done <- f.svc.DeleteCriterion(ctx, f.ws, f.maya, crit[0].ID) }()
	select {
	case err := <-done:
		t.Fatalf("legacy writer ran while ticket locked: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := <-done; err != nil && !errors.Is(err, tracker.ErrNotFound) {
			t.Fatal(err)
		}
	}
}
