package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func TestRunnerEnvironmentSnapshots(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := tracker.Get(ctx, q, me.WorkspaceID, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	makeRun := func(workspace uuid.UUID, status string) uuid.UUID {
		t.Helper()
		r, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: workspace, TicketID: &ticket.ID, AgentID: agent.ID, Mode: "managed", State: "completed", ResolvedSpec: json.RawMessage(`{}`), Harness: "codex", Model: "fake", Purpose: "ticket"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, "UPDATE runs SET runner_id='previous-runner' WHERE id=$1", r.ID); err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(map[string]any{"state": "environment_snapshot", "runner_id": "runner-one", "backend": "local", "report": map[string]any{"status": status, "manifest": ".lazyagents-deps.json", "tools": []map[string]any{{"name": "go", "available": true, "version": "go1.26.0"}}}})
		if err = q.AddRunEvent(ctx, store.AddRunEventParams{RunID: r.ID, Type: "lifecycle", Payload: payload}); err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	first := makeRun(me.WorkspaceID, "ready")
	second := makeRun(me.WorkspaceID, "failed")
	// A report in another workspace must never appear, even when it references
	// the same ticket. Workspace ownership is scoped on the run itself.
	var otherWorkspace uuid.UUID
	if err = pool.QueryRow(ctx, "INSERT INTO workspaces(name,slug) VALUES('Other','other-env') RETURNING id").Scan(&otherWorkspace); err != nil {
		t.Fatal(err)
	}
	other := makeRun(otherWorkspace, "ready")
	if err = q.Heartbeat(ctx, store.HeartbeatParams{ID: "runner-one", Backend: "local", MaxConcurrent: 2}); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Pool: pool, Q: q}).Handler()
	fetch := func(query string) api.RunnerEnvironment {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/runner/environment"+query, nil))
		if rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		var result api.RunnerEnvironment
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	latest := fetch("")
	if !latest.Runner.Online || len(latest.Snapshots) != 1 || latest.Snapshots[0].RunnerId != "runner-one" || latest.Snapshots[0].RunId != second || latest.Snapshots[0].Report.Status != "failed" {
		t.Fatalf("%+v", latest)
	}
	exact := fetch("?run_id=" + first.String())
	if len(exact.Snapshots) != 1 || exact.Snapshots[0].Report.Tools[0].Name != "go" || exact.Snapshots[0].RunId != first {
		t.Fatalf("%+v", exact)
	}
	if exact.Snapshots[0].Report.Caches == nil || exact.Snapshots[0].Report.Packages == nil {
		t.Fatal("empty collections must be arrays")
	}
	if got := fetch("?run_id=" + other.String()); len(got.Snapshots) != 0 {
		t.Fatal("cross-workspace report leaked")
	}
	if err = q.DeleteRunner(ctx, "runner-one"); err != nil {
		t.Fatal(err)
	}
	historical := fetch("?run_id=" + first.String())
	if historical.Runner.Online || len(historical.Snapshots) != 1 {
		t.Fatal("snapshot disappeared with offline runner")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/runner/environment?run_id=invalid", nil))
	if rec.Code != 400 {
		t.Fatalf("invalid run id: %d", rec.Code)
	}
}
