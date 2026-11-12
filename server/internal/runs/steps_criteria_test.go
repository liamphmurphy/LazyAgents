package runs_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

type stepFixture struct {
	ctx    context.Context
	q      *store.Queries
	svc    *runs.Service
	tr     *tracker.Service
	caller agents.Caller
	run    store.Run
	ws     uuid.UUID
	user   uuid.UUID
}

func newStepFixture(t *testing.T) *stepFixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.New(t)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	agent, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	tr := &tracker.Service{Pool: pool}
	svc := &runs.Service{Pool: pool, Tracker: tr}
	caller := agents.Caller{Agent: agent, Kind: "unmanaged"}
	run, err := svc.Claim(ctx, caller, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	caller.RunID = &run.ID
	return &stepFixture{ctx: ctx, q: q, svc: svc, tr: tr, caller: caller, run: run, ws: me.WorkspaceID, user: me.ID}
}

func (f *stepFixture) states(t *testing.T) []string {
	t.Helper()
	plan, err := f.q.ListPlan(f.ctx, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(plan))
	for i, s := range plan {
		out[i] = s.State
	}
	return out
}

func (f *stepFixture) criteria(t *testing.T) []bool {
	t.Helper()
	cr, err := f.q.ListCriteria(f.ctx, *f.run.TicketID)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]bool, len(cr))
	for i, c := range cr {
		out[i] = c.Done
	}
	return out
}

func eq[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPlanStepsAdvanceThemselves(t *testing.T) {
	f := newStepFixture(t)
	if _, err := f.svc.PostPlan(f.ctx, f.caller, "", []string{"One", "Two", "Three"}); err != nil {
		t.Fatal(err)
	}
	if got := f.states(t); !eq(got, []string{"current", "pending", "pending"}) {
		t.Fatalf("fresh plan = %v", got)
	}
	if r, err := f.svc.UpdateStep(f.ctx, f.caller, "", 1, "done", ""); err != nil || r.Current != 2 {
		t.Fatalf("advance = %d %v", r.Current, err)
	}
	if got := f.states(t); !eq(got, []string{"done", "current", "pending"}) {
		t.Fatalf("after step 1 = %v", got)
	}
	// Finishing a step that isn't the current one leaves the current step alone.
	if r, err := f.svc.UpdateStep(f.ctx, f.caller, "", 3, "done", ""); err != nil || r.Current != 0 {
		t.Fatalf("override = %d %v", r.Current, err)
	}
	if got := f.states(t); !eq(got, []string{"done", "current", "done"}) {
		t.Fatalf("after step 3 = %v", got)
	}
	// Last step done: nothing left to promote.
	if r, err := f.svc.UpdateStep(f.ctx, f.caller, "", 2, "done", ""); err != nil || r.Current != 0 {
		t.Fatalf("last step = %d %v", r.Current, err)
	}
	if got := f.states(t); !eq(got, []string{"done", "done", "done"}) {
		t.Fatalf("finished plan = %v", got)
	}
}

func TestCheckCriteria(t *testing.T) {
	f := newStepFixture(t)
	total := len(f.criteria(t))
	if total < 4 {
		t.Fatalf("fixture has %d criteria", total)
	}
	// Start from a clean slate: the demo seed ticks some criteria already.
	all := make([]int, total)
	for i := range all {
		all[i] = i + 1
	}
	if err := f.svc.CheckCriteria(f.ctx, f.caller, "", all, false); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.CheckCriteria(f.ctx, f.caller, "", []int{1, 3, 3}, true); err != nil {
		t.Fatal(err)
	}
	got := f.criteria(t)
	if !got[0] || got[1] || !got[2] {
		t.Fatalf("after ticking 1,3 = %v", got)
	}
	// One bad number rejects the whole call.
	for _, bad := range []int{0, total + 1} {
		if err := f.svc.CheckCriteria(f.ctx, f.caller, "", []int{2, bad}, true); !isInvalid(err) {
			t.Fatalf("bad number %d: %v", bad, err)
		}
		if got := f.criteria(t); got[1] {
			t.Fatalf("criterion 2 ticked despite invalid %d: %v", bad, got)
		}
	}
	if err := f.svc.CheckCriteria(f.ctx, f.caller, "", []int{1, 3}, false); err != nil {
		t.Fatal(err)
	}
	if got := f.criteria(t); got[0] || got[2] {
		t.Fatalf("after unticking = %v", got)
	}
}

func TestCheckCriteriaRequiresMockup(t *testing.T) {
	f := newStepFixture(t)
	yes := true
	if _, err := f.tr.Update(f.ctx, f.ws, tracker.UserActor(f.user), "PAY-214", tracker.Patch{RequiresMockup: &yes}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.CheckCriteria(f.ctx, f.caller, "", []int{1, 2}, true); !isConflict(err) {
		t.Fatalf("ticking without mockup: %v", err)
	}
	if err := f.svc.CheckCriteria(f.ctx, f.caller, "", []int{1, 2}, false); err != nil {
		t.Fatalf("unticking is allowed: %v", err)
	}
}

func TestStepNoteAndRollover(t *testing.T) {
	f := newStepFixture(t)
	f.svc.RolloverContextTokens = 100000
	if _, err := f.svc.PostPlan(f.ctx, f.caller, "", []string{"One", "Two", "Three", "Four"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UpdateStep(f.ctx, f.caller, "", 1, "done", strings.Repeat("é", runs.StepNoteLimit+1)); !isInvalid(err) {
		t.Fatalf("over-long note: %v", err)
	}
	set := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.svc.Pool.Exec(f.ctx, sql, append([]any{f.run.ID}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	flag := func() bool {
		t.Helper()
		run, err := f.q.GetRun(f.ctx, store.GetRunParams{ID: f.run.ID, WorkspaceID: f.ws})
		if err != nil {
			t.Fatal(err)
		}
		return run.RolloverRequested
	}
	// A delegated run never rolls over, however large.
	var parent uuid.UUID
	if err := f.svc.Pool.QueryRow(f.ctx, `INSERT INTO runs (workspace_id, ticket_id, agent_id, mode, state, resolved_spec, harness, model, purpose)
		SELECT workspace_id, ticket_id, agent_id, mode, 'completed', resolved_spec, harness, model, purpose FROM runs WHERE id=$1 RETURNING id`, f.run.ID).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	set(`UPDATE runs SET mode='managed', context_tokens=180000, parent_run_id=$2 WHERE id=$1`, parent)
	if r, err := f.svc.UpdateStep(f.ctx, f.caller, "", 1, "done", "  ledger.go holds it  "); err != nil || r.Rollover || r.Current != 2 || flag() {
		t.Fatalf("delegated run: %+v %v", r, err)
	}
	plan, _ := f.q.ListPlan(f.ctx, f.run.ID)
	if plan[0].Note == nil || *plan[0].Note != "ledger.go holds it" {
		t.Fatalf("note %v", plan[0].Note)
	}
	// A lead below the setting doesn't.
	set(`UPDATE runs SET context_tokens=50000, parent_run_id=NULL WHERE id=$1`)
	if r, err := f.svc.UpdateStep(f.ctx, f.caller, "", 2, "done", ""); err != nil || r.Rollover || flag() {
		t.Fatalf("small lead: %+v %v", r, err)
	}
	// A large lead does, with the size that triggered it.
	set(`UPDATE runs SET context_tokens=100000 WHERE id=$1`)
	if r, err := f.svc.UpdateStep(f.ctx, f.caller, "", 3, "done", ""); err != nil || !r.Rollover || r.Current != 4 || r.ContextTokens != 100000 || !flag() {
		t.Fatalf("large lead: %+v %v", r, err)
	}
	// With the setting off, nothing does.
	set(`UPDATE runs SET rollover_requested=false WHERE id=$1`)
	f.svc.RolloverContextTokens = 0
	if _, err := f.svc.UpdateStep(f.ctx, f.caller, "", 3, "current", ""); err != nil {
		t.Fatal(err)
	}
	if r, err := f.svc.UpdateStep(f.ctx, f.caller, "", 3, "done", ""); err != nil || r.Rollover || flag() {
		t.Fatalf("setting off: %+v %v", r, err)
	}
}
