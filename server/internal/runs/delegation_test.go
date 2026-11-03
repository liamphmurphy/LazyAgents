package runs_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

func TestDelegationToSubagents(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	svc := &runs.Service{Pool: pool, Tracker: &tracker.Service{Pool: pool}, Artifacts: &artifacts.Service{Pool: pool}, Poll: 20 * time.Millisecond}
	agentSvc := &agents.Service{Pool: pool}
	trk := &tracker.Service{Pool: pool}

	scribe, err := agentSvc.Create(ctx, ws, me.ID, "scribe", agents.Identity{Name: "Scribe", Role: "Docs", Kind: agents.KindSub}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if scribe.Kind != agents.KindSub {
		t.Fatalf("kind = %q", scribe.Kind)
	}
	callerFor := func(a store.Agent, runID *uuid.UUID) runs.Caller {
		kind := "unmanaged"
		var exp *time.Time
		if runID != nil {
			kind = "run"
			e := time.Now().Add(time.Hour)
			exp = &e
		}
		tok, _, err := agents.IssueToken(ctx, q, a.ID, kind, "", runID, nil, exp)
		if err != nil {
			t.Fatal(err)
		}
		c, err := agents.ResolveToken(ctx, q, tok)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	// Sub agents can't be assigned tickets.
	st := "PAY-214"
	if _, err := trk.Update(ctx, ws, tracker.UserActor(me.ID), st, tracker.Patch{AssigneeAgentID: tracker.Opt[uuid.UUID]{Set: true, Value: &scribe.ID}}); !isInvalid(err) {
		t.Fatalf("assigned a sub agent: %v", err)
	}
	// ...and an agent with open tickets can't become one.
	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	if _, err := agentSvc.SaveDefinition(ctx, ws, me.ID, "atlas", agents.Identity{Name: atlas.Name, Kind: agents.KindSub}, agents.DefaultSpec(), ""); !isConflict(err) {
		t.Fatalf("made an agent with open tickets a sub agent: %v", err)
	}

	// A managed run for atlas whose workspace is ready.
	runsOut, err := svc.Dispatch(ctx, ws, me.ID, []string{st})
	if err != nil {
		t.Fatal(err)
	}
	parent := runsOut[0]
	vol := parent.ID.String()
	if err := q.SetRunWorkspace(ctx, store.SetRunWorkspaceParams{ID: parent.ID, WorkspaceVolume: &vol}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: parent.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	main := callerFor(atlas, &parent.ID)
	tk, _ := tracker.Get(ctx, q, ws, st)

	if _, err := svc.Delegate(ctx, main, "", "scribe", "  "); !isInvalid(err) {
		t.Fatalf("empty task: %v", err)
	}
	if _, err := svc.Delegate(ctx, main, "", "nobody", "do it"); !isInvalid(err) {
		t.Fatalf("unknown agent: %v", err)
	}
	if _, err := svc.Delegate(ctx, main, "", "quill", "do it"); !isInvalid(err) {
		t.Fatalf("delegated to a main agent: %v", err)
	}
	if subs, err := svc.ListSubagents(ctx, main); err != nil || len(subs) != 1 || subs[0].Handle != "scribe" {
		t.Fatalf("subagents = %v, %v", subs, err)
	}

	d, err := svc.Delegate(ctx, main, "", "@Scribe", "Write the migration notes")
	if err != nil {
		t.Fatal(err)
	}
	child := d.Run
	if child.ParentRunID == nil || *child.ParentRunID != parent.ID || child.State != "queued" || *child.TicketID != tk.ID {
		t.Fatalf("child = %+v", child)
	}
	p, _ := q.GetRun(ctx, store.GetRunParams{ID: parent.ID, WorkspaceID: ws})
	if p.State != "waiting" || deref(p.WaitReason) != "delegation" {
		t.Fatalf("parent = %s/%s", p.State, deref(p.WaitReason))
	}
	if _, err := svc.Delegate(ctx, callerFor(atlas, &parent.ID), "", "scribe", "again"); !isConflict(err) {
		t.Fatalf("second live delegation: %v", err)
	}
	// The ticket's live run is still the parent's.
	if live, err := q.ActiveRunForTicket(ctx, &tk.ID); err != nil || live.ID != parent.ID {
		t.Fatalf("active run = %v, %v", live.ID, err)
	}
	// Sub agents can't claim tickets or delegate.
	if _, err := svc.Claim(ctx, callerFor(scribe, nil), st); !isConflict(err) {
		t.Fatalf("sub agent claimed: %v", err)
	}
	if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: child.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	sub := callerFor(scribe, &child.ID)
	if _, err := svc.Delegate(ctx, sub, "", "scribe", "x"); !isConflict(err) {
		t.Fatalf("sub agent delegated: %v", err)
	}

	// Finishing reports to the parent and leaves the ticket where it was.
	before, _ := tracker.Get(ctx, q, ws, st)
	if err := svc.Complete(ctx, sub, "", "Wrote docs/migrations.md", ""); err != nil {
		t.Fatal(err)
	}
	after, _ := tracker.Get(ctx, q, ws, st)
	if after.Status != before.Status {
		t.Fatalf("ticket moved %s → %s", before.Status, after.Status)
	}
	c, _ := q.GetRun(ctx, store.GetRunParams{ID: child.ID, WorkspaceID: ws})
	p, _ = q.GetRun(ctx, store.GetRunParams{ID: parent.ID, WorkspaceID: ws})
	if c.State != "completed" || p.State != "running" {
		t.Fatalf("child %s, parent %s", c.State, p.State)
	}
	turns, _ := q.ListTurns(ctx, parent.ID)
	last := turns[len(turns)-1]
	if last.Trigger != "delegation" || !strings.Contains(last.InputMessage, "Wrote docs/migrations.md") || !strings.Contains(last.InputMessage, "@scribe") {
		t.Fatalf("parent turn = %+v", last)
	}

	// A failed child tells the parent; cancelling a parent takes its child down.
	if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: parent.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	d2, err := svc.Delegate(ctx, callerFor(atlas, &parent.ID), "", "scribe", "Second task")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetRunControl(ctx, ws, me.ID, d2.Run.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	p, _ = q.GetRun(ctx, store.GetRunParams{ID: parent.ID, WorkspaceID: ws})
	if p.State != "running" {
		t.Fatalf("parent after child cancelled = %s", p.State)
	}
	turns, _ = q.ListTurns(ctx, parent.ID)
	if m := turns[len(turns)-1].InputMessage; !strings.Contains(m, "cancelled") {
		t.Fatalf("parent told %q", m)
	}
	d3, err := svc.Delegate(ctx, callerFor(atlas, &parent.ID), "", "scribe", "Third task")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetRunControl(ctx, ws, me.ID, parent.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	c3, _ := q.GetRun(ctx, store.GetRunParams{ID: d3.Run.ID, WorkspaceID: ws})
	if c3.State != "cancelled" {
		t.Fatalf("child of a cancelled parent = %s", c3.State)
	}
}
