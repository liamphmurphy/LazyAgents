package tracker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

type fixture struct {
	pool  *pgxpool.Pool
	q     *store.Queries
	svc   *tracker.Service
	ws    uuid.UUID
	maya  tracker.Actor
	agent map[string]store.Agent
	epic  map[string]store.Epic
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	q := store.New(pool)
	u, err := q.GetUserByEmail(ctx, "maya@northwind.dev")
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{pool: pool, q: q, svc: &tracker.Service{Pool: pool}, ws: u.WorkspaceID, maya: tracker.UserActor(u.ID),
		agent: map[string]store.Agent{}, epic: map[string]store.Epic{}}
	agents, _ := q.ListAgents(ctx, f.ws)
	for _, a := range agents {
		f.agent[a.Handle] = a
	}
	epics, _ := q.ListEpics(ctx, f.ws)
	for _, e := range epics {
		f.epic[e.Name] = e
	}
	return f
}

func verbs(t *testing.T, f fixture, key string) []string {
	t.Helper()
	tk, err := tracker.Get(context.Background(), f.q, f.ws, key)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := f.q.ListTicketActivity(context.Background(), &tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Verb
	}
	return out
}

func TestSeedIsOnlyOnce(t *testing.T) {
	f := setup(t)
	res, err := seed.Run(context.Background(), f.pool, seed.Options{Demo: true})
	if err != nil || res.Created {
		t.Fatalf("second seed should be a no-op: created=%v err=%v", res.Created, err)
	}
	tk, err := tracker.Get(context.Background(), f.q, f.ws, "PAY-214")
	if err != nil || tk.Priority != "urgent" || tk.Branch == nil {
		t.Fatalf("PAY-214 not seeded as in the mockup: %+v %v", tk, err)
	}
}

func TestCreateNumbersSequentially(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	keys := make(chan string, 5)
	for range 5 {
		wg.Go(func() {
			tk, err := f.svc.Create(ctx, f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "concurrent"})
			if err != nil {
				t.Error(err)
				return
			}
			keys <- tk.Key
		})
	}
	wg.Wait()
	close(keys)
	seen := map[string]bool{}
	for k := range keys {
		if seen[k] {
			t.Fatalf("duplicate key %s", k)
		}
		seen[k] = true
	}
	for _, want := range []string{"PAY-231", "PAY-232", "PAY-233", "PAY-234", "PAY-235"} {
		if !seen[want] {
			t.Errorf("missing %s in %v", want, seen)
		}
	}
}

func TestPatchRecordsEachChangeInOrder(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	before := len(verbs(t, f, "PAY-226"))
	quill := f.agent["quill"].ID
	_, err := f.svc.Update(ctx, f.ws, f.maya, "PAY-226", tracker.Patch{
		Status:          ptr("in_progress"),
		Priority:        ptr("high"), // unchanged: no event
		AssigneeAgentID: tracker.SetTo(quill),
		DueDate:         tracker.Clear[time.Time](),
		Estimate:        tracker.SetTo("  "), // blank clears; it was already empty: no event
	})
	if err != nil {
		t.Fatal(err)
	}
	got := verbs(t, f, "PAY-226")[before:]
	want := []string{tracker.VerbStatusChanged, tracker.VerbAssigned, tracker.VerbDueChanged}
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}

	// Reassigning logs the hand-off: unassigned then assigned.
	before += len(got)
	if _, err := f.svc.Update(ctx, f.ws, f.maya, "PAY-226", tracker.Patch{AssigneeAgentID: tracker.SetTo(f.agent["atlas"].ID)}); err != nil {
		t.Fatal(err)
	}
	got = verbs(t, f, "PAY-226")[before:]
	if len(got) != 2 || got[0] != tracker.VerbUnassigned || got[1] != tracker.VerbAssigned {
		t.Fatalf("reassign events = %v", got)
	}
}

func TestValidation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var ve *tracker.ValidationError

	_, err := f.svc.Update(ctx, f.ws, f.maya, "PAY-214", tracker.Patch{EpicID: tracker.SetTo(f.epic["Offline mode"].ID)})
	if !errors.As(err, &ve) || ve.Field != "epic_id" {
		t.Errorf("epic from another project: got %v", err)
	}
	_, err = f.svc.Update(ctx, f.ws, f.maya, "PAY-214", tracker.Patch{Status: ptr("shipped")})
	if !errors.As(err, &ve) || ve.Field != "status" {
		t.Errorf("bad status: got %v", err)
	}
	_, err = f.svc.Update(ctx, f.ws, f.maya, "PAY-9999", tracker.Patch{})
	if !errors.Is(err, tracker.ErrNotFound) {
		t.Errorf("missing ticket: got %v", err)
	}
}

func TestBulkIsAllOrNothing(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.svc.Bulk(ctx, f.ws, f.maya, []string{"PAY-219", "PAY-1"}, tracker.Patch{Priority: ptr("urgent")}, nil)
	var ve *tracker.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want validation error, got %v", err)
	}
	tk, _ := tracker.Get(ctx, f.q, f.ws, "PAY-219")
	if tk.Priority != "low" {
		t.Fatalf("PAY-219 changed despite the failed bulk update: %s", tk.Priority)
	}
	if _, err := f.svc.Bulk(ctx, f.ws, f.maya, []string{"PAY-219"}, tracker.Patch{Title: ptr("x")}, nil); !errors.As(err, &ve) {
		t.Fatalf("bulk title edit should be rejected, got %v", err)
	}
}

func TestCommentRepliesStayOneLevel(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	root, err := f.svc.Comment(ctx, f.ws, f.maya, "PAY-219", tracker.NewComment{BodyMd: "root"})
	if err != nil {
		t.Fatal(err)
	}
	rootID := payloadID(t, root, "comment_id")
	reply, err := f.svc.Comment(ctx, f.ws, f.maya, "PAY-219", tracker.NewComment{BodyMd: "reply", ParentCommentID: &rootID})
	if err != nil {
		t.Fatal(err)
	}
	replyID := payloadID(t, reply, "comment_id")
	nested, err := f.svc.Comment(ctx, f.ws, f.maya, "PAY-219", tracker.NewComment{BodyMd: "nested", ParentCommentID: &replyID})
	if err != nil {
		t.Fatal(err)
	}
	if got := payloadID(t, nested, "parent_comment_id"); got != rootID {
		t.Fatalf("reply to a reply should attach to the root comment")
	}
}

func TestTicketRevisionAndEstimate(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ticket, err := f.svc.Create(ctx, f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "revision"})
	if err != nil {
		t.Fatal(err)
	}
	revision := int(ticket.Revision)
	unchanged, err := f.svc.Update(ctx, f.ws, f.maya, ticket.Key, tracker.Patch{Revision: &revision, Title: &ticket.Title})
	if err != nil || unchanged.Revision != ticket.Revision {
		t.Fatalf("no-op: %+v %v", unchanged, err)
	}
	updated, err := f.svc.Update(ctx, f.ws, f.maya, ticket.Key, tracker.Patch{Revision: &revision, EstimatePoints: tracker.SetTo(5)})
	if err != nil || updated.Revision != ticket.Revision+1 || updated.EstimatePoints == nil || *updated.EstimatePoints != 5 {
		t.Fatalf("estimate: %+v %v", updated, err)
	}
	if _, err := f.svc.Update(ctx, f.ws, f.maya, ticket.Key, tracker.Patch{Revision: &revision}); err == nil {
		t.Fatal("stale no-op accepted")
	}
	if _, err := f.svc.Update(ctx, f.ws, f.maya, ticket.Key, tracker.Patch{EstimatePoints: tracker.SetTo(4)}); err == nil {
		t.Fatal("off-scale estimate accepted")
	}
	if _, err := f.svc.Bulk(ctx, f.ws, f.maya, []string{ticket.Key}, tracker.Patch{Revision: &revision}, nil); err == nil {
		t.Fatal("bulk revision accepted")
	}
	agent := f.agent["quill"].ID
	if _, err := f.svc.Update(ctx, f.ws, tracker.Actor{Type: "agent", ID: &agent}, ticket.Key, tracker.Patch{EstimatePoints: tracker.SetTo(8)}); err == nil {
		t.Fatal("agent overwrote human estimate")
	}
}

func TestHumanConfirmsAgentEstimate(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ticket, err := f.svc.Create(ctx, f.ws, f.maya, tracker.NewTicket{ProjectKey: "PAY", Title: "ownership"})
	if err != nil {
		t.Fatal(err)
	}
	agentID := f.agent["quill"].ID
	agent := tracker.AgentActor(agentID)
	rationale := "agent assessment"
	estimated, err := f.svc.Update(ctx, f.ws, agent, ticket.Key, tracker.Patch{EstimatePoints: tracker.SetTo(5), EstimateRationale: &rationale})
	if err != nil {
		t.Fatal(err)
	}
	before := len(verbs(t, f, ticket.Key))
	confirmed, err := f.svc.Update(ctx, f.ws, f.maya, ticket.Key, tracker.Patch{EstimatePoints: tracker.SetTo(5)})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.EstimatedByType == nil || *confirmed.EstimatedByType != "user" || confirmed.EstimatedByID == nil || *confirmed.EstimatedByID != *f.maya.ID || confirmed.EstimateRationale != nil || confirmed.Revision != estimated.Revision+1 {
		t.Fatalf("confirmation did not transfer ownership: %+v", confirmed)
	}
	events, err := f.q.ListTicketActivity(ctx, &ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != before+1 || events[len(events)-1].Verb != tracker.VerbEstimateChanged || events[len(events)-1].ActorType != "user" {
		t.Fatalf("missing confirmation activity: %+v", events)
	}
	again, err := f.svc.Update(ctx, f.ws, f.maya, ticket.Key, tracker.Patch{EstimatePoints: tracker.SetTo(5)})
	if err != nil || again.Revision != confirmed.Revision || len(verbs(t, f, ticket.Key)) != len(events) {
		t.Fatalf("repeat confirmation: %+v %v", again, err)
	}
	if _, err = f.svc.Update(ctx, f.ws, agent, ticket.Key, tracker.Patch{EstimatePoints: tracker.SetTo(8)}); err == nil {
		t.Fatal("agent overwrote confirmed human estimate")
	}
}
