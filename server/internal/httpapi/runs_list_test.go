package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

// The Runs index lists top-level ticket, planning and chat runs with their
// work and project, and filters by state, agent, project and purpose.
func TestListRuns(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: &artifacts.Service{Pool: pool}}); err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: pool}
	s := &Server{Pool: pool, Q: q, Tracker: tr, Runs: &runs.Service{Pool: pool, Tracker: tr}}
	handler := s.Handler()
	list := func(query string, intended int) api.ListRuns200JSONResponse {
		t.Helper()
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", "/api/v1/runs"+query, nil))
		if r.Code != intended {
			t.Fatalf("%s: %d %s", query, r.Code, r.Body.String())
		}
		var out api.ListRuns200JSONResponse
		if intended == 200 {
			if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	ids := func(out api.ListRuns200JSONResponse) map[uuid.UUID]api.Run {
		m := map[uuid.UUID]api.Run{}
		for _, r := range out.Runs {
			m[r.Id] = r
		}
		return m
	}

	user, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ws := user.WorkspaceID
	agent := func(handle string) store.Agent {
		a, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: handle})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	atlas := agent("atlas")
	ticket, err := tracker.Get(ctx, q, ws, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	newRun := func(a store.Agent, state, purpose string, ticketID, parent *uuid.UUID) store.Run {
		t.Helper()
		r, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: ws, TicketID: ticketID, AgentID: a.ID, Mode: "managed", State: state, ResolvedSpec: json.RawMessage(`{}`), Harness: "codex", Model: "m", ParentRunID: parent, Purpose: purpose})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	ticketRun := newRun(atlas, "waiting", "ticket", &ticket.ID, nil)
	child := newRun(atlas, "running", "ticket", &ticket.ID, &ticketRun.ID)

	var other struct {
		ID  uuid.UUID
		Key string
	}
	if err := pool.QueryRow(ctx, `SELECT id, key FROM projects WHERE workspace_id = $1 AND key <> 'PAY' ORDER BY key LIMIT 1`, ws).Scan(&other.ID, &other.Key); err != nil {
		t.Fatal(err)
	}
	planRun := newRun(atlas, "completed", "planning", nil, nil)
	var planID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO planning_sessions (workspace_id, project_id, goal, run_id, created_by) VALUES ($1, $2, 'Harden the ledger', $3, $4) RETURNING id`, ws, other.ID, planRun.ID, user.ID).Scan(&planID); err != nil {
		t.Fatal(err)
	}
	// A chat's earlier segment is found through its messages, not current_run_id.
	chatOld := newRun(atlas, "completed", "chat", nil, nil)
	chatRun := newRun(atlas, "running", "chat", nil, nil)
	var chatID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO chat_sessions (workspace_id, project_id, title, created_by, agent_id, harness, model, current_run_id) VALUES ($1, $2, 'Refund questions', $3, $4, 'codex', 'm', $5) RETURNING id`, ws, ticket.ProjectID, user.ID, atlas.ID, chatRun.ID).Scan(&chatID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO chat_messages (session_id, seq, role, body_md, run_id) VALUES ($1, 1, 'user', 'hi', $2)`, chatID, chatOld.ID); err != nil {
		t.Fatal(err)
	}

	var foreignWS uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO workspaces (name,slug) VALUES ('Other','other-runs') RETURNING id").Scan(&foreignWS); err != nil {
		t.Fatal(err)
	}
	foreign, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: foreignWS, AgentID: atlas.ID, Mode: "managed", State: "running", ResolvedSpec: json.RawMessage(`{}`), Harness: "codex", Model: "m", Purpose: "planning"})
	if err != nil {
		t.Fatal(err)
	}
	orphan := newRun(atlas, "running", "planning", nil, nil)
	live := ids(list("", 200))
	if _, ok := live[foreign.ID]; ok {
		t.Fatal("cross-workspace run leaked")
	}
	if _, ok := live[orphan.ID]; !ok {
		t.Fatal("legacy planning run without a session must not fail the index")
	}
	if _, ok := live[child.ID]; ok {
		t.Fatal("delegated runs belong to their parent, not the index")
	}
	if _, ok := live[planRun.ID]; ok {
		t.Fatal("live filter leaked a completed run")
	}
	tr0, ok := live[ticketRun.ID]
	if !ok || live[chatRun.ID].Id == uuid.Nil {
		t.Fatalf("live runs missing: %+v", live)
	}
	if tr0.Purpose != api.RunPurposeTicket || tr0.ProjectKey == nil || *tr0.ProjectKey != "PAY" || tr0.WorkTitle == nil || *tr0.WorkTitle != ticket.Title || tr0.ChildCount == nil || *tr0.ChildCount != 1 {
		t.Fatalf("ticket run not enriched: %+v", tr0)
	}
	cr := live[chatRun.ID]
	if cr.ChatId == nil || *cr.ChatId != chatID || cr.WorkTitle == nil || *cr.WorkTitle != "Refund questions" || cr.ProjectKey == nil || *cr.ProjectKey != "PAY" {
		t.Fatalf("chat run not enriched: %+v", cr)
	}

	all := ids(list("?state=all", 200))
	pr := all[planRun.ID]
	if pr.PlanningId == nil || *pr.PlanningId != planID || pr.ProjectKey == nil || *pr.ProjectKey != other.Key || pr.WorkTitle == nil || *pr.WorkTitle != "Harden the ledger" {
		t.Fatalf("planning run not enriched: %+v", pr)
	}
	if old := all[chatOld.ID]; old.ChatId == nil || *old.ChatId != chatID {
		t.Fatalf("earlier chat segment lost its chat: %+v", old)
	}

	if got := ids(list("?state=all&project="+other.Key, 200)); len(got) != 1 || got[planRun.ID].Id == uuid.Nil {
		t.Fatalf("project filter: %+v", got)
	}
	if got := ids(list("?state=all&project=PAY&purpose=chat", 200)); len(got) != 2 || got[chatRun.ID].Id == uuid.Nil || got[chatOld.ID].Id == uuid.Nil {
		t.Fatalf("project + purpose filter: %+v", got)
	}
	if got := ids(list("?state=waiting", 200)); got[ticketRun.ID].Id == uuid.Nil || got[chatRun.ID].Id != uuid.Nil {
		t.Fatalf("single state filter: %+v", got)
	}
	if got := list("?state=all&agent=atlas", 200); len(got.Runs) < 4 {
		t.Fatalf("agent filter dropped runs: %d", len(got.Runs))
	}
	if got := list("?agent=nobody", 200); len(got.Runs) != 0 {
		t.Fatal("unknown agent must match nothing")
	}
	if got := list("?project=NOPE", 200); len(got.Runs) != 0 {
		t.Fatal("unknown project must match nothing")
	}
	list("?state=bogus", 422)
	list("?purpose=bogus", 422)
	list("?offset=-1", 422)
}
