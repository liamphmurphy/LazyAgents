package runs_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
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
	if n, err := f.svc.UpdateStep(f.ctx, f.caller, "", 1, "done"); err != nil || n != 2 {
		t.Fatalf("advance = %d %v", n, err)
	}
	if got := f.states(t); !eq(got, []string{"done", "current", "pending"}) {
		t.Fatalf("after step 1 = %v", got)
	}
	// Finishing a step that isn't the current one leaves the current step alone.
	if n, err := f.svc.UpdateStep(f.ctx, f.caller, "", 3, "done"); err != nil || n != 0 {
		t.Fatalf("override = %d %v", n, err)
	}
	if got := f.states(t); !eq(got, []string{"done", "current", "done"}) {
		t.Fatalf("after step 3 = %v", got)
	}
	// Last step done: nothing left to promote.
	if n, err := f.svc.UpdateStep(f.ctx, f.caller, "", 2, "done"); err != nil || n != 0 {
		t.Fatalf("last step = %d %v", n, err)
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
