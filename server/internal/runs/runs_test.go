package runs_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func isConflict(err error) bool {
	var c *apperr.ConflictError
	return errors.As(err, &c)
}

func isInvalid(err error) bool {
	var v *apperr.ValidationError
	return errors.As(err, &v)
}

func verbsAfter(t *testing.T, q *store.Queries, ws uuid.UUID, key string, n int) []string {
	t.Helper()
	tk, _ := tracker.Get(context.Background(), q, ws, key)
	evs, _ := q.ListTicketActivity(context.Background(), &tk.ID)
	var out []string
	for _, e := range evs[n:] {
		out = append(out, e.Verb)
	}
	return out
}

func TestRunLifecycle(t *testing.T) {
	pool := testdb.New(t)
	staging := testdb.New(t) // stands in for the staging database
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(k))
	sec := &secrets.Service{Q: q, Box: box}
	svc := &runs.Service{
		Pool: pool, Secrets: sec, Tracker: &tracker.Service{Pool: pool},
		Artifacts: &artifacts.Service{Pool: pool}, Poll: 20 * time.Millisecond,
	}
	caller := func(handle string) runs.Caller {
		a, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: handle})
		tok, _, err := agents.IssueToken(ctx, q, a.ID, "unmanaged", "", nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		c, err := agents.ResolveToken(ctx, q, tok)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	atlas, quill := caller("atlas"), caller("quill")
	tk, _ := tracker.Get(ctx, q, ws, "PAY-214")
	before, _ := q.ListTicketActivity(ctx, &tk.ID)
	n0 := len(before)

	// Claiming: only the assignee, idempotent, snapshot frozen.
	run, err := svc.Claim(ctx, atlas, "pay-214")
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := svc.Claim(ctx, atlas, "PAY-214"); again.ID != run.ID {
		t.Fatal("re-claim should return the live run")
	}
	if _, err := svc.Claim(ctx, quill, "PAY-214"); !isConflict(err) {
		t.Fatalf("unassigned agent claimed: %v", err)
	}
	spec, _ := runs.Spec(run)
	if spec.Provenance["instructions"] != "project" {
		t.Fatal("run should snapshot the PAY-resolved definition")
	}

	// Plan: one current step at a time.
	if _, err := svc.PostPlan(ctx, atlas, "", []string{"Create partitions", "Backfill", "Apply to staging"}); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.UpdateStep(ctx, atlas, "", 1, "current")
	_, _ = svc.UpdateStep(ctx, atlas, "", 2, "current")
	plan, _ := q.ListPlan(ctx, run.ID)
	if plan[0].State != "pending" || plan[1].State != "current" {
		t.Fatalf("plan states = %s %s", plan[0].State, plan[1].State)
	}
	if _, err := svc.UpdateStep(ctx, atlas, "", 9, "done"); !isInvalid(err) {
		t.Fatal("unknown step accepted")
	}
	if err := svc.LogEvent(ctx, atlas, "", "test_run", map[string]any{"passed": 312.0, "failed": 0.0, "duration_s": 252.0}); err != nil {
		t.Fatal(err)
	}

	// Approval gates follow the run's frozen scopes.
	if out, err := svc.RequestApproval(ctx, atlas, "", "tests:run", "Run the suite", nil, ""); err != nil || !out.NotRequired {
		t.Fatalf("allowed scope should not need approval: %+v %v", out, err)
	}
	if _, err := svc.RequestApproval(ctx, atlas, "", "deploy:staging", "Deploy", nil, ""); !isInvalid(err) {
		t.Fatalf("denied scope: %v", err)
	}
	if _, err := svc.RequestApproval(ctx, atlas, "", "db:migrate", "Migrate", map[string]any{}, ""); !isInvalid(err) {
		t.Fatalf("brokered scope without sql: %v", err)
	}
	sql := "CREATE TABLE ledger_entries_2026_11 (id bigint); INSERT INTO ledger_entries_2026_11 VALUES (1);"
	out, err := svc.RequestApproval(ctx, atlas, "", "db:migrate", "Apply 0042_partition_ledger_entries to staging", map[string]any{"sql": sql}, "18.2M rows, ~4 min, no table lock")
	if err != nil || out.Approval == nil {
		t.Fatal(err)
	}
	if _, err := svc.PostPlan(ctx, atlas, "", []string{"x"}); !isConflict(err) {
		t.Fatal("a waiting run accepted work")
	}
	if _, err := svc.RunGated(ctx, atlas, "", out.Approval.ID); !isConflict(err) {
		t.Fatal("ran before approval")
	}
	a, _ := q.GetAgent(ctx, store.GetAgentParams{ID: atlas.Agent.ID, WorkspaceID: ws})
	if a.Status != "waiting" {
		t.Fatalf("agent status = %s", a.Status)
	}

	// The wait times out while pending, then returns the decision.
	if ap, _ := svc.WaitApproval(ctx, atlas, out.Approval.ID, 50*time.Millisecond); ap.State != "pending" {
		t.Fatal("wait should time out pending")
	}
	go func() {
		time.Sleep(60 * time.Millisecond)
		_, _ = svc.Decide(ctx, ws, me.ID, out.Approval.ID, true, "Go ahead")
	}()
	ap, err := svc.WaitApproval(ctx, atlas, out.Approval.ID, 5*time.Second)
	if err != nil || ap.State != "approved" {
		t.Fatalf("decision = %s %v", ap.State, err)
	}
	if _, err := svc.Decide(ctx, ws, me.ID, out.Approval.ID, false, ""); !isConflict(err) {
		t.Fatal("decided twice")
	}
	turns, _ := q.ListTurns(ctx, run.ID)
	if len(turns) != 2 || turns[1].Trigger != "approval" {
		t.Fatalf("turns = %+v", turns)
	}

	// The broker needs the secret. Without it nothing runs and the approval
	// stays usable, so adding the secret and retrying works.
	if _, err := svc.RunGated(ctx, atlas, "", out.Approval.ID); !isConflict(err) {
		t.Fatalf("no secret: %v", err)
	}
	if _, err := sec.Set(ctx, ws, me.ID, secrets.SetInput{Name: "db:staging", Value: staging.Config().ConnString()}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.RunGated(ctx, atlas, "", out.Approval.ID)
	if err != nil || !res.OK {
		t.Fatalf("gated run: %+v %v", res, err)
	}
	if _, err := svc.RunGated(ctx, atlas, "", out.Approval.ID); !isConflict(err) {
		t.Fatal("approval used twice")
	}
	// A failing migration is reported, rolled back, and uses the approval up.
	bad, _ := svc.RequestApproval(ctx, atlas, "", "db:migrate", "Broken migration", map[string]any{"sql": "SELECT nope FROM missing"}, "")
	_, _ = svc.Decide(ctx, ws, me.ID, bad.Approval.ID, true, "")
	if res, err := svc.RunGated(ctx, atlas, "", bad.Approval.ID); err != nil || res.OK || res.Error == "" {
		t.Fatalf("failing migration: %+v %v", res, err)
	}
	if res, _ := svc.RunGated(ctx, atlas, "", bad.Approval.ID); res.OK {
		t.Fatal("failed approval reused")
	}
	var rows int
	if err := staging.QueryRow(ctx, "SELECT count(*) FROM ledger_entries_2026_11").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("staging not migrated: %d %v", rows, err)
	}

	// Waiting for a person: only an @mention resumes.
	if err := svc.AwaitHuman(ctx, atlas, "", "Spec and PRD disagree on the p95 target"); err != nil {
		t.Fatal(err)
	}
	// Cursors are database timestamps; the host clock can differ from Postgres's.
	var cursor time.Time
	if err := pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Tracker.Comment(ctx, ws, tracker.UserActor(me.ID), "PAY-214", tracker.NewComment{BodyMd: "Looking"}); err != nil {
		t.Fatal(err)
	}
	_ = svc.OnHumanComment(ctx, ws, "PAY-214", "Looking")
	if r, _ := q.GetRun(ctx, store.GetRunParams{ID: run.ID, WorkspaceID: ws}); r.State != "waiting" {
		t.Fatal("a comment without a mention resumed the run")
	}
	sig, err := svc.AwaitSignal(ctx, atlas, "", cursor, 50*time.Millisecond)
	if err != nil || len(sig.Comments) != 1 || sig.State != "waiting" {
		t.Fatalf("signal = %+v %v", sig, err)
	}
	_ = svc.OnHumanComment(ctx, ws, "PAY-214", "@atlas use the PRD's 800ms")
	if r, _ := q.GetRun(ctx, store.GetRunParams{ID: run.ID, WorkspaceID: ws}); r.State != "running" {
		t.Fatal("@mention didn't resume")
	}

	// Code, criteria, report, then complete → In Review.
	if err := svc.SetBranch(ctx, atlas, "", "atlas/pay-214-partition-ledger"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPR(ctx, atlas, "", "https://evil.example/pull/1", false); !isInvalid(err) {
		t.Fatal("non-GitHub PR URL accepted")
	}
	if err := svc.SetPR(ctx, atlas, "", "https://github.com/northwind/payments/pull/491", true); err != nil {
		t.Fatal(err)
	}
	// Rejection leaves the lead running so it can implement or delegate the gaps.
	for _, status := range []string{"", "in_review", "done"} {
		if err := svc.Complete(ctx, atlas, "", "Partial implementation", "", status, ""); !isConflict(err) {
			t.Fatalf("completion with unchecked criteria (%q): %v", status, err)
		}
	}
	if r, err := q.GetRun(ctx, store.GetRunParams{ID: run.ID, WorkspaceID: ws}); err != nil || r.State != "running" {
		t.Fatalf("rejected completion ended run: %+v %v", r, err)
	}
	if ticket, err := tracker.Get(ctx, q, ws, "PAY-214"); err != nil || ticket.Status != "in_progress" {
		t.Fatalf("rejected completion moved ticket: %+v %v", ticket, err)
	}
	criteria, err := q.ListCriteria(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range criteria {
		if err := svc.CheckCriterion(ctx, atlas, "", i+1, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.CheckCriterion(ctx, atlas, "", 4, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateReport(ctx, atlas, "", "Staging migration report", "# Staging migration report\n\nApplied in 140ms.\n"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Complete(ctx, atlas, "", "Migration applied to staging; PR #491 ready", "", "", ""); err != nil {
		t.Fatal(err)
	}
	tk, _ = tracker.Get(ctx, q, ws, "PAY-214")
	a, _ = q.GetAgent(ctx, store.GetAgentParams{ID: atlas.Agent.ID, WorkspaceID: ws})
	if tk.Status != "in_review" || a.Status != "idle" || deref(tk.PrNumber) != 491 {
		t.Fatalf("after complete: ticket %s agent %s pr %v", tk.Status, a.Status, tk.PrNumber)
	}
	got := verbsAfter(t, q, ws, "PAY-214", n0)
	for _, v := range []string{"run_started", "plan_posted", "tests_ran", "approval_requested", "approval_approved", "gated_action_ran",
		"waiting_for_human", "commented", "branch_set", "pr_opened", "criterion_checked", "artifact_attached", "run_completed", "status_changed"} {
		if !contains(got, v) {
			t.Errorf("timeline missing %s (got %v)", v, got)
		}
	}

	// People's controls on a new run.
	run2, err := svc.Claim(ctx, atlas, "PAY-214")
	if err != nil || run2.ID == run.ID {
		t.Fatalf("new run after completion: %v", err)
	}
	if r, err := svc.SetRunControl(ctx, ws, me.ID, run2.ID, "pause"); err != nil || r.State != "paused" {
		t.Fatalf("pause: %v", err)
	}
	if err := svc.SetBranch(ctx, atlas, "", "x"); !isConflict(err) {
		t.Fatal("paused run accepted work")
	}
	if r, err := svc.SetRunControl(ctx, ws, me.ID, run2.ID, "resume"); err != nil || r.State != "running" {
		t.Fatalf("resume: %v", err)
	}
	if r, err := svc.SetRunControl(ctx, ws, me.ID, run2.ID, "cancel"); err != nil || r.State != "cancelled" {
		t.Fatalf("cancel: %v", err)
	} else if r.Outcome == nil || !strings.HasPrefix(*r.Outcome, "Cancelled by ") || strings.Contains(*r.Outcome, "before it started") {
		t.Fatalf("a cancelled run must say who cancelled it: outcome %v", r.Outcome)
	}
	if _, err := svc.PostPlan(ctx, atlas, "", []string{"x"}); !isConflict(err) {
		t.Fatal("cancelled run accepted work")
	}
}

func TestMentions(t *testing.T) {
	cases := map[string]bool{
		"@atlas go ahead":     true,
		"thanks @Atlas.":      true,
		"email me@atlas.dev":  false,
		"@atlasx should not":  false,
		"(cc @atlas)":         true,
		"no mention of atlas": false,
	}
	for text, want := range cases {
		if got := runs.Mentions(text, "atlas"); got != want {
			t.Errorf("Mentions(%q) = %v", text, got)
		}
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}

// A project-added skill reaches the run's frozen snapshot; a removed one doesn't.
func TestRunSnapshotSkillOverrides(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	asvc := &agents.Service{Pool: pool}
	a, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	spec, _, err := agents.CurrentSpec(ctx, q, a)
	if err != nil {
		t.Fatal(err)
	}
	spec.Skills = []agentdef.Skill{
		{Name: "keep-me", Description: "Use when keeping", Body: "keep"},
		{Name: "drop-me", Description: "Use when dropping", Body: "drop"},
	}
	if _, err := asvc.SaveDefinition(ctx, ws, me.ID, "atlas", agents.Identity{Name: a.Name, Role: a.Role, ColorIndex: int(a.ColorIndex)}, spec, "skills"); err != nil {
		t.Fatal(err)
	}
	if err := asvc.SetOverride(ctx, ws, me.ID, "atlas", "PAY", agentdef.Override{
		InstructionsAppend: "Follow PAY playbooks.",
		SkillsRemove:       []string{"drop-me"},
		SkillsAdd:          []agentdef.Skill{{Name: "pay-ledger", Description: "Use when touching the ledger", Body: "ledger rules"}},
	}); err != nil {
		t.Fatal(err)
	}

	k := make([]byte, 32)
	_, _ = rand.Read(k)
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(k))
	svc := &runs.Service{
		Pool: pool, Secrets: &secrets.Service{Q: q, Box: box}, Tracker: &tracker.Service{Pool: pool},
		Artifacts: &artifacts.Service{Pool: pool}, Poll: 20 * time.Millisecond,
	}
	tok, _, err := agents.IssueToken(ctx, q, a.ID, "unmanaged", "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := agents.ResolveToken(ctx, q, tok)
	if err != nil {
		t.Fatal(err)
	}
	run, err := svc.Claim(ctx, caller, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := runs.Spec(run)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range snap.Skills {
		got = append(got, s.Name)
	}
	if len(got) != 2 || got[0] != "keep-me" || got[1] != "pay-ledger" {
		t.Fatalf("snapshot skills = %v", got)
	}
	if snap.Provenance["skills"] != agentdef.FromProject {
		t.Fatalf("provenance = %v", snap.Provenance)
	}
}
