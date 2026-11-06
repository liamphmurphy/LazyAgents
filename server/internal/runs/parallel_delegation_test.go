package runs_test

import (
	"context"
	"encoding/json"
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

func TestPathsOverlap(t *testing.T) {
	for _, c := range []struct {
		a, b    []string
		overlap bool
	}{
		{[]string{"docs/"}, []string{"src/"}, false},
		{[]string{"docs/"}, []string{"docs/"}, true},
		{[]string{"docs"}, []string{"docs/guide.md"}, true},
		{[]string{"docs/guide.md"}, []string{"docs/"}, true},
		{[]string{"docs/a.md"}, []string{"docs/b.md"}, false},
		{[]string{"go.mod"}, []string{"docs/", "internal/"}, false},
		{[]string{"docs-extra/"}, []string{"docs/"}, false},
		{[]string{"docs/*.md"}, []string{"src/"}, false},
		{[]string{"docs/*.md"}, []string{"docs/x.go"}, true},
		{[]string{"*.md"}, []string{"src/"}, true},
		{[]string{"**/*.go"}, []string{"src/"}, true},
		{[]string{"src/"}, []string{"[a-z]*"}, true},
		{[]string{"a/", "b/"}, []string{"c/", "b/x"}, true},
	} {
		if got := runs.PathsOverlap(c.a, c.b); got != c.overlap || runs.PathsOverlap(c.b, c.a) != c.overlap {
			t.Errorf("PathsOverlap(%v, %v) = %v, want %v", c.a, c.b, got, c.overlap)
		}
	}
}

func TestParallelDelegationRulesAndWake(t *testing.T) {
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
	scribe, err := agentSvc.Create(ctx, ws, me.ID, "scribe", agents.Identity{Name: "Scribe", Role: "Docs", Kind: agents.KindSub}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tester, err := agentSvc.Create(ctx, ws, me.ID, "tester", agents.Identity{Name: "Tester", Role: "QA", Kind: agents.KindSub}, nil)
	if err != nil {
		t.Fatal(err)
	}
	callerFor := func(a store.Agent, runID *uuid.UUID) runs.Caller {
		exp := time.Now().Add(time.Hour)
		tok, _, err := agents.IssueToken(ctx, q, a.ID, "run", "", runID, nil, &exp)
		if err != nil {
			t.Fatal(err)
		}
		c, err := agents.ResolveToken(ctx, q, tok)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	out, err := svc.Dispatch(ctx, ws, me.ID, []string{"PAY-214"})
	if err != nil {
		t.Fatal(err)
	}
	parent := out[0]
	vol := parent.ID.String()
	if err := q.SetRunWorkspace(ctx, store.SetRunWorkspaceParams{ID: parent.ID, WorkspaceVolume: &vol}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: parent.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	main := callerFor(atlas, &parent.ID)
	delegate := func(handle, task string, c runs.Contract) (runs.Delegation, error) {
		return svc.DelegateWithContract(ctx, main, "", handle, task, c)
	}
	state := func(id uuid.UUID) store.Run {
		r, err := q.GetRun(ctx, store.GetRunParams{ID: id, WorkspaceID: ws})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	// First delegation, with paths, parks the parent.
	d1, err := delegate("scribe", "docs", runs.Contract{AllowedPaths: []string{"docs/"}})
	if err != nil {
		t.Fatal(err)
	}
	if p := state(parent.ID); p.State != "waiting" {
		t.Fatalf("parent %s", p.State)
	}
	// A parked parent whose process has gone can't delegate again...
	if _, err := delegate("tester", "x", runs.Contract{AllowedPaths: []string{"src/"}}); !isConflict(err) {
		t.Fatalf("delegated from a parked parent with no live turn: %v", err)
	}
	// ...but within the same turn (process still running) it can.
	if _, err := pool.Exec(ctx, "UPDATE run_turns SET status = 'running' WHERE run_id = $1", parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := delegate("tester", "no paths", runs.Contract{}); err == nil || !strings.Contains(err.Error(), "allowed_paths") {
		t.Fatalf("concurrent child without allowed_paths: %v", err)
	}
	for _, overlap := range []string{"docs/guide.md", "docs", "*.md", "docs/*.md"} {
		if _, err := delegate("tester", "overlap", runs.Contract{AllowedPaths: []string{overlap}}); !isConflict(err) || !strings.Contains(err.Error(), "overlap") {
			t.Fatalf("overlapping %q: %v", overlap, err)
		}
	}
	d2, err := delegate("tester", "src", runs.Contract{AllowedPaths: []string{"src/"}, TestCommand: "go test ./..."})
	if err != nil {
		t.Fatal(err)
	}
	if p := state(parent.ID); p.State != "waiting" || deref(p.WaitReason) != "delegation" {
		t.Fatalf("parent %s/%s", p.State, deref(p.WaitReason))
	}
	d3, err := delegate("scribe", "third", runs.Contract{AllowedPaths: []string{"web/"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := delegate("scribe", "fourth", runs.Contract{AllowedPaths: []string{"api/"}}); !isConflict(err) || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("4th concurrent child: %v", err)
	}

	// The parent stays parked until the last child ends, then resumes once.
	finish := func(d runs.Delegation, a store.Agent, outcome string) {
		if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: d.Run.ID, State: "running"}); err != nil {
			t.Fatal(err)
		}
		if err := svc.Complete(ctx, callerFor(a, &d.Run.ID), "", outcome, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	finish(d1, scribe, "docs done")
	finish(d3, scribe, "web done")
	if p := state(parent.ID); p.State != "waiting" {
		t.Fatalf("parent resumed with a child still live: %s", p.State)
	}
	finish(d2, tester, "tests done")
	if p := state(parent.ID); p.State != "running" {
		t.Fatalf("parent after all children: %s", p.State)
	}
	turns, _ := q.ListTurns(ctx, parent.ID)
	var wakes []store.RunTurn
	for _, tn := range turns {
		if tn.Trigger == "delegation" {
			wakes = append(wakes, tn)
		}
	}
	if len(wakes) != 1 {
		t.Fatalf("parent got %d delegation turns", len(wakes))
	}
	for _, want := range []string{"3 delegated runs have ended", "docs done", "web done", "tests done", "@scribe", "@tester", "continue_run_id"} {
		if !strings.Contains(wakes[0].InputMessage, want) {
			t.Errorf("resume message lacks %q:\n%s", want, wakes[0].InputMessage)
		}
	}

	// continue_run_id: only a finished run of this parent and this sub agent that got a worktree.
	if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: parent.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	cont := func(handle string, id string) error {
		_, err := delegate(handle, "fix it", runs.Contract{ContinueRunID: id})
		return err
	}
	for name, c := range map[string]struct{ handle, id string }{
		"not a uuid":    {"scribe", "nope"},
		"unknown run":   {"scribe", uuid.NewString()},
		"other agent":   {"tester", d1.Run.ID.String()},
		"never started": {"scribe", d1.Run.ID.String()},
		"the parent":    {"scribe", parent.ID.String()},
	} {
		err := cont(c.handle, c.id)
		if err == nil || isConflict(err) || !strings.Contains(err.Error(), "continue_run_id") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Give d1 a worktree, a branch and a session, as the runner would.
	raw, _ := json.Marshal(runs.Contract{AllowedPaths: []string{"docs/"}, Branch: "atlas/pay-214--scribe-abc", Worktree: ".lazyagents/worktrees/" + d1.Run.ID.String()})
	if _, err := q.SetRunContract(ctx, store.SetRunContractParams{ID: d1.Run.ID, Contract: raw}); err != nil {
		t.Fatal(err)
	}
	sid := "session-1"
	if err := q.SetRunSession(ctx, store.SetRunSessionParams{ID: d1.Run.ID, HarnessSessionID: &sid}); err != nil {
		t.Fatal(err)
	}
	d4, err := delegate("scribe", "fix docs", runs.Contract{ContinueRunID: d1.Run.ID.String(), AllowedPaths: []string{"docs/"}})
	if err != nil {
		t.Fatal(err)
	}
	var c4 runs.Contract
	if err := json.Unmarshal(d4.Run.Contract, &c4); err != nil || c4.Worktree == "" || c4.Branch != "atlas/pay-214--scribe-abc" || c4.ContinueRunID != d1.Run.ID.String() || !c4.ResumeSession {
		t.Fatalf("continued contract %s (%v)", d4.Run.Contract, err)
	}
	if got := state(d4.Run.ID); deref(got.HarnessSessionID) != sid || got.ParentRunID == nil || *got.ParentRunID != parent.ID || got.ID == d1.Run.ID {
		t.Fatalf("continued run %+v", got)
	}
	// The same worktree can't be continued twice, nor while the continuation runs.
	if err := cont("scribe", d1.Run.ID.String()); err == nil || !strings.Contains(err.Error(), "continue_run_id") {
		t.Fatalf("second continuation of one run: %v", err)
	}
	if err := cont("scribe", d4.Run.ID.String()); err == nil || !strings.Contains(err.Error(), "still in progress") {
		t.Fatalf("continuing a live run: %v", err)
	}
	// A different harness starts fresh: no session is copied.
	finish(d4, scribe, "fixed")
	if _, err := pool.Exec(ctx, "UPDATE runs SET harness = 'codex' WHERE id = $1", d4.Run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: parent.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	d5, err := delegate("scribe", "again", runs.Contract{ContinueRunID: d4.Run.ID.String(), AllowedPaths: []string{"docs/"}})
	if err != nil {
		t.Fatal(err)
	}
	var c5 runs.Contract
	if err := json.Unmarshal(d5.Run.Contract, &c5); err != nil || c5.ResumeSession || c5.Worktree != c4.Worktree || state(d5.Run.ID).HarnessSessionID != nil {
		t.Fatalf("contract %s (%v)", d5.Run.Contract, err)
	}
}
