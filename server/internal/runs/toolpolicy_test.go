package runs_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

func TestToolPolicyExactApproval(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	a, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	svc := &runs.Service{Pool: pool, Tracker: &tracker.Service{Pool: pool}}
	c := agents.Caller{Agent: a, Kind: "unmanaged"}
	run, err := svc.Claim(ctx, c, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	c.RunID = &run.ID
	spec, _ := runs.Spec(run)
	for i := range spec.Scopes {
		if spec.Scopes[i].Scope == "tests:run" {
			spec.Scopes[i].Mode = "requires_approval"
		}
	}
	raw, _ := json.Marshal(spec)
	if _, err := pool.Exec(ctx, "UPDATE runs SET resolved_spec=$2 WHERE id=$1", run.ID, raw); err != nil {
		t.Fatal(err)
	}
	action := runs.ToolCheck{Tool: "Bash", Input: map[string]any{"command": "go test ./..."}}
	d, err := svc.CheckTool(ctx, c, action)
	if err != nil || d.Decision != "deny" {
		t.Fatalf("unapproved action: %+v %v", d, err)
	}
	aps, err := q.ListApprovalsForRun(ctx, run.ID)
	if err != nil || len(aps) != 1 {
		t.Fatalf("approvals=%d err=%v", len(aps), err)
	}
	if _, err := svc.Decide(ctx, me.WorkspaceID, me.ID, aps[0].ID, true, ""); err != nil {
		t.Fatal(err)
	}
	d, err = svc.CheckTool(ctx, c, runs.ToolCheck{Tool: "Read", Input: map[string]any{"file_path": "README.md"}})
	if err != nil || d.Decision != "allow" {
		t.Fatalf("read: %+v %v", d, err)
	}
	changed := runs.ToolCheck{Tool: "Bash", Input: map[string]any{"command": "go test ./other"}}
	changedDecision, changedErr := svc.CheckTool(ctx, c, changed)
	if changedErr != nil || changedDecision.Decision != "deny" {
		t.Fatalf("approval widened to another input: %+v %v", changedDecision, changedErr)
	}
	changedApprovals, _ := q.ListApprovalsForRun(ctx, run.ID)
	if len(changedApprovals) != 2 {
		t.Fatal("changed action must request its own approval")
	}
	if _, err := svc.Decide(ctx, me.WorkspaceID, me.ID, changedApprovals[1].ID, false, ""); err != nil {
		t.Fatal(err)
	}
	d, err = svc.CheckTool(ctx, c, action)
	if err != nil || d.Decision != "allow" {
		t.Fatalf("approved: %+v %v", d, err)
	}
	ap, _ := q.GetApproval(ctx, store.GetApprovalParams{ID: aps[0].ID, WorkspaceID: me.WorkspaceID})
	if ap.ExecutedAt == nil {
		t.Fatal("exact approval was not consumed")
	}
	// Repeating the exact command needs a new decision.
	d, err = svc.CheckTool(ctx, c, action)
	if err != nil || d.Decision != "deny" {
		t.Fatalf("approval reused: %+v %v", d, err)
	}
	aps, _ = q.ListApprovalsForRun(ctx, run.ID)
	if len(aps) != 3 {
		t.Fatalf("expected another request, got %d", len(aps))
	}
	if _, err := svc.Decide(ctx, me.WorkspaceID, me.ID, aps[len(aps)-1].ID, false, ""); err != nil {
		t.Fatal(err)
	}
	d, err = svc.CheckTool(ctx, c, action)
	if err != nil || d.Decision != "deny" {
		t.Fatalf("denial ignored: %+v %v", d, err)
	}
	// Multiple required scopes are consumed together, after all decisions exist.
	for i := range spec.Scopes {
		if spec.Scopes[i].Scope == "repo:write" {
			spec.Scopes[i].Mode = "requires_approval"
		}
	}
	raw, _ = json.Marshal(spec)
	if _, err := pool.Exec(ctx, "UPDATE runs SET resolved_spec=$2 WHERE id=$1", run.ID, raw); err != nil {
		t.Fatal(err)
	}
	multi := runs.ToolCheck{Tool: "Bash", Input: map[string]any{"command": "git push origin HEAD"}}
	for i := 0; i < 2; i++ {
		decision, err := svc.CheckTool(ctx, c, multi)
		if err != nil || decision.Decision != "deny" {
			t.Fatalf("multi-scope action was allowed early: %+v %v", decision, err)
		}
		all, _ := q.ListApprovalsForRun(ctx, run.ID)
		latest := all[len(all)-1]
		for _, a := range all {
			if a.ActionSummary == "Allow Bash for tests:run: git push origin HEAD" && a.ExecutedAt != nil {
				t.Fatal("first scope consumed before second approval")
			}
		}
		if _, err := svc.Decide(ctx, me.WorkspaceID, me.ID, latest.ID, true, ""); err != nil {
			t.Fatal(err)
		}
	}
	decision, err := svc.CheckTool(ctx, c, multi)
	if err != nil || decision.Decision != "allow" {
		t.Fatalf("approved scopes not applied: %+v %v", decision, err)
	}
	// Planning can inspect supplied files without inheriting execution access.
	spec = runs.PlanningSpec(spec)
	raw, _ = json.Marshal(spec)
	if _, err := pool.Exec(ctx, "UPDATE runs SET resolved_spec=$2 WHERE id=$1", run.ID, raw); err != nil {
		t.Fatal(err)
	}
	decision, err = svc.CheckTool(ctx, c, runs.ToolCheck{Tool: "Read", Input: map[string]any{"file_path": ".lazyagents/context/page.md"}})
	if err != nil || decision.Decision != "allow" {
		t.Fatalf("planning read blocked: %+v %v", decision, err)
	}
	decision, err = svc.CheckTool(ctx, c, action)
	if err != nil || decision.Decision != "deny" {
		t.Fatalf("planning execution allowed: %+v %v", decision, err)
	}
	decision, err = svc.CheckTool(ctx, c, runs.ToolCheck{Tool: "Edit", Input: map[string]any{"file_path": "app.go"}})
	if err != nil || decision.Decision != "deny" {
		t.Fatalf("planning edit allowed: %+v %v", decision, err)
	}

}
