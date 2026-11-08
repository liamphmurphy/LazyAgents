package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func TestPhaseSevenOverviews(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: &artifacts.Service{Pool: pool}}); err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: pool}
	s := &Server{Pool: pool, Q: q, Tracker: tr, Runs: &runs.Service{Pool: pool, Tracker: tr}}
	handler := s.Handler()
	get := func(path string, dst any, intended int) {
		t.Helper()
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != intended {
			t.Fatalf("%s: %d %s", path, r.Code, r.Body.String())
		}
		if dst != nil {
			if err := json.Unmarshal(r.Body.Bytes(), dst); err != nil {
				t.Fatal(err)
			}
		}
	}
	var dashboard api.Dashboard
	get("/api/v1/dashboard?range=7d", &dashboard, 200)
	if dashboard.Counts.Total != 14 || len(dashboard.Projects) != 4 || len(dashboard.Agents) != 8 || dashboard.Runner.Online {
		t.Fatalf("dashboard data: %+v", dashboard)
	}
	if !dashboard.Stats.Performance.MedianCycleSeconds.IsNull() {
		t.Fatal("missing cycle data must stay null")
	}
	for _, a := range dashboard.Agents {
		if a.Agent.Status != api.AgentStatusIdle {
			t.Fatal("seeded status was mistaken for a live run")
		}
	}
	get("/api/v1/dashboard?range=invalid", nil, 422)
	var epics api.EpicOverviewList
	get("/api/v1/epics", &epics, 200)
	if len(epics.Epics) != 7 {
		t.Fatal("epics missing")
	}
	found := false
	for _, e := range epics.Epics {
		if e.Epic.Name != "Ledger v2" {
			continue
		}
		found = true
		if e.Counts.Total != 5 || e.Owner == nil || e.Source == nil || string(e.Source.Type) != "prd" || e.StartDate == nil || e.TargetDate == nil || len(e.Agents) != 3 {
			t.Fatalf("featured epic lost metadata: %+v", e)
		}
		var list api.TicketList
		get("/api/v1/tickets?view=all&epic="+e.Epic.Id.String(), &list, 200)
		if len(list.Tickets) != e.Counts.Total {
			t.Fatal("epic filter disagrees with aggregate")
		}
		for _, tkt := range list.Tickets {
			if tkt.Epic == nil || tkt.Epic.Id != e.Epic.Id {
				t.Fatal("epic filter leaked another epic")
			}
		}
	}
	if !found {
		t.Fatal("Ledger v2 missing")
	}
	user, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: user.WorkspaceID, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := tracker.Get(ctx, q, user.WorkspaceID, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	run, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: user.WorkspaceID, TicketID: &ticket.ID, AgentID: atlas.ID, Mode: "managed", State: "queued", ResolvedSpec: json.RawMessage(`{}`), Harness: "codex", Model: "project-model", Purpose: "ticket"})
	if err != nil {
		t.Fatal(err)
	}
	var roster api.AgentOverviewList
	get("/api/v1/agent-overviews", &roster, 200)
	inspect := func() api.AgentOverview {
		t.Helper()
		for _, a := range roster.Agents {
			if a.Agent.Id == atlas.ID {
				return a
			}
		}
		t.Fatal("atlas missing")
		return api.AgentOverview{}
	}
	a := inspect()
	if a.Agent.Status != api.AgentStatusIdle || len(a.Assignments) != 1 || a.Assignments[0].Harness != "codex" || a.QueueCount != 1 {
		t.Fatalf("queued assignment: %+v", a)
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	if err = q.InsertPlanStep(ctx, store.InsertPlanStepParams{RunID: run.ID, Position: 0, Text: "Measure latency", State: "current"}); err != nil {
		t.Fatal(err)
	}
	get("/api/v1/agent-overviews", &roster, 200)
	a = inspect()
	if a.Agent.Status != api.AgentStatusRunning || a.Assignments[0].StepPosition != 1 || a.Assignments[0].StepsTotal != 1 {
		t.Fatalf("live plan: %+v", a)
	}
	var boot api.Bootstrap
	get("/api/v1/bootstrap", &boot, 200)
	if boot.Counts.LiveAgents != 1 {
		t.Fatal("sidebar live count disagrees")
	}
	var history api.ListAgentRuns200JSONResponse
	get("/api/v1/agents/atlas/runs", &history, 200)
	if len(history.Runs) != 1 || history.Runs[0].Id != run.ID {
		t.Fatal("run history lost managed run")
	}
	get("/api/v1/agents/atlas/runs?offset=-1", nil, 422)
	get("/api/v1/agents/missing/runs", nil, 404)
	// Pagination has a stable tie-breaker even when every start timestamp matches.
	if _, err = pool.Exec(ctx, `INSERT INTO runs(workspace_id,ticket_id,agent_id,mode,state,resolved_spec,harness,model,started_at,purpose) SELECT $1,$2,$3,'managed','completed','{}','codex','fake','2026-01-01'::timestamptz,'ticket' FROM generate_series(1,52)`, user.WorkspaceID, ticket.ID, atlas.ID); err != nil {
		t.Fatal(err)
	}
	get("/api/v1/agents/atlas/runs", &history, 200)
	if len(history.Runs) != 50 || !history.HasMore {
		t.Fatal("history first page")
	}
	first := map[string]bool{}
	for _, r := range history.Runs {
		first[r.Id.String()] = true
	}
	get("/api/v1/agents/atlas/runs?offset=50", &history, 200)
	if len(history.Runs) != 3 || history.HasMore {
		t.Fatal("history last page")
	}
	for _, r := range history.Runs {
		if first[r.Id.String()] {
			t.Fatal("history pagination duplicated a run")
		}
	}
}

func TestEmptyDashboard(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{}); err != nil {
		t.Fatal(err)
	}
	s := &Server{Pool: pool, Q: store.New(pool)}
	r := httptest.NewRecorder()
	s.Handler().ServeHTTP(r, httptest.NewRequest("GET", "/api/v1/dashboard", nil))
	if r.Code != 200 {
		t.Fatalf("empty dashboard: %d %s", r.Code, r.Body.String())
	}
	var d api.Dashboard
	if err := json.Unmarshal(r.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Counts.Total != 0 || d.Stats.Performance.Completed != 0 || len(d.Agents) != 0 || len(d.Projects) != 0 || d.Attention == nil || d.RecentArtifacts == nil {
		t.Fatalf("invalid empty dashboard: %+v", d)
	}
}
