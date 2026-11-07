package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
	"github.com/liamphmurphy/lazyagents/server/internal/wiki"
)

type estimateEnv struct {
	t       *testing.T
	ctx     context.Context
	pool    any
	q       *store.Queries
	tr      *tracker.Service
	handler http.Handler
	ws      uuid.UUID
	me      store.User
	atlas   store.Agent
}

func newEstimateEnv(t *testing.T) *estimateEnv {
	pool := testdb.New(t)
	ctx := context.Background()
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}
	h := (&Server{Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts, Wiki: &wiki.Service{Pool: pool, Artifacts: arts, Tracker: tr}}).Handler()
	return &estimateEnv{t: t, ctx: ctx, q: q, tr: tr, handler: h, ws: me.WorkspaceID, me: me, atlas: atlas}
}

func (e *estimateEnv) request(method, path string, body any, token string) (int, string) {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func (e *estimateEnv) tool(token, name string, args any) (int, string) {
	return e.request("POST", "/api/v1/agent/tools/"+name, args, token)
}

// runToken issues a run token for a new atlas run with the given resolved spec.
func (e *estimateEnv) runToken(ticketID, parent *uuid.UUID, purpose, spec string) string {
	e.t.Helper()
	run, err := e.q.CreateRun(e.ctx, store.CreateRunParams{WorkspaceID: e.ws, TicketID: ticketID, AgentID: e.atlas.ID, Mode: "managed", State: "running", ResolvedSpec: json.RawMessage(spec), Harness: "claude", Model: "test", ParentRunID: parent, Purpose: purpose})
	if err != nil {
		e.t.Fatal(err)
	}
	tok, _, err := agents.IssueToken(e.ctx, e.q, e.atlas.ID, "run", "estimates", &run.ID, nil, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return tok
}

func (e *estimateEnv) ticket(project, title string, epic *uuid.UUID) store.Ticket {
	e.t.Helper()
	tk, err := e.tr.Create(e.ctx, e.ws, tracker.UserActor(e.me.ID), tracker.NewTicket{ProjectKey: project, Title: title, EpicID: epic})
	if err != nil {
		e.t.Fatal(err)
	}
	return tk
}

func (e *estimateEnv) expect(label string, code int, body string, want int, contains string) {
	e.t.Helper()
	if code != want || !strings.Contains(body, contains) {
		e.t.Errorf("%s = %d %s, want %d containing %q", label, code, body, want, contains)
	}
}

func TestMCPEstimateTicket(t *testing.T) {
	e := newEstimateEnv(t)
	own := e.ticket("PAY", "Own ticket", nil)
	sibling := e.ticket("PAY", "Sibling", nil)
	other := e.ticket("MOB", "Other project", nil)
	lead := e.runToken(&own.ID, nil, "ticket", `{}`)

	// Success: own ticket, rationale stored on the ticket and in activity.
	code, body := e.tool(lead, "estimate_ticket", map[string]any{"ticket": own.Key, "points": 5, "rationale": "  Two handlers and a migration  "})
	e.expect("estimate own", code, body, 200, `"estimate_points":5`)
	got, _ := tracker.Get(e.ctx, e.q, e.ws, own.Key)
	if got.EstimatePoints == nil || *got.EstimatePoints != 5 || got.EstimateRationale == nil || *got.EstimateRationale != "Two handlers and a migration" ||
		got.EstimatedByType == nil || *got.EstimatedByType != "agent" || *got.EstimatedByID != e.atlas.ID {
		t.Fatalf("estimate not stored: %+v", got)
	}
	// Same project (ADR Decision 6) is allowed too.
	code, body = e.tool(lead, "estimate_ticket", map[string]any{"ticket": sibling.Key, "points": 3, "rationale": "Small"})
	e.expect("estimate sibling", code, body, 200, `"estimate_points":3`)

	// get_ticket shows it.
	code, body = e.tool(lead, "get_ticket", map[string]any{"ticket": own.Key})
	e.expect("get_ticket estimate", code, body, 200, `"estimate_points":5`)
	if !strings.Contains(body, `"rationale":"Two handlers and a migration"`) || !strings.Contains(body, `"epic":null`) {
		t.Errorf("get_ticket estimate payload: %s", body)
	}

	// Rejections.
	for _, p := range []int{0, 4, 21, -1} {
		code, body = e.tool(lead, "estimate_ticket", map[string]any{"ticket": own.Key, "points": p, "rationale": "x"})
		e.expect("off scale", code, body, 422, "off the scale")
	}
	code, body = e.tool(lead, "estimate_ticket", map[string]any{"ticket": own.Key, "points": 8, "rationale": "   "})
	e.expect("empty rationale", code, body, 422, "rationale")
	code, body = e.tool(lead, "estimate_ticket", map[string]any{"ticket": own.Key, "points": 8, "rationale": strings.Repeat("a", 2001)})
	e.expect("long rationale", code, body, 422, "2000")
	code, body = e.tool(lead, "estimate_ticket", map[string]any{"ticket": other.Key, "points": 8, "rationale": "x"})
	e.expect("other project", code, body, 422, "in this project")
	code, body = e.tool(lead, "estimate_ticket", map[string]any{"ticket": "PAY-99999", "points": 8, "rationale": "x"})
	e.expect("unknown ticket", code, body, 422, "in this project")

	// A person's estimate wins.
	if _, err := e.tr.Update(e.ctx, e.ws, tracker.UserActor(e.me.ID), sibling.Key, tracker.Patch{EstimatePoints: tracker.SetTo(2)}); err != nil {
		t.Fatal(err)
	}
	code, body = e.tool(lead, "estimate_ticket", map[string]any{"ticket": sibling.Key, "points": 8, "rationale": "x"})
	e.expect("human estimate", code, body, 409, "a person estimated this")

	// Closed tickets can't be estimated by agents.
	closed := e.ticket("PAY", "Closed", nil)
	done := "done"
	if _, err := e.tr.Update(e.ctx, e.ws, tracker.UserActor(e.me.ID), closed.Key, tracker.Patch{Status: &done}); err != nil {
		t.Fatal(err)
	}
	code, body = e.tool(lead, "estimate_ticket", map[string]any{"ticket": closed.Key, "points": 2, "rationale": "x"})
	e.expect("closed ticket", code, body, 409, "closed tickets")

	// Chat, planning and delegated tokens are refused by profile.
	code, body = e.tool(e.runToken(nil, nil, "chat", `{}`), "estimate_ticket", map[string]any{"ticket": own.Key, "points": 2, "rationale": "x"})
	e.expect("chat", code, body, 409, "isn't available in chats")
	code, body = e.tool(e.runToken(nil, nil, "planning", `{}`), "estimate_ticket", map[string]any{"ticket": own.Key, "points": 2, "rationale": "x"})
	e.expect("planning", code, body, 409, "planning runs can read context and propose tickets only")
	leadRun, _ := e.q.ActiveRunForTicket(e.ctx, &own.ID)
	code, body = e.tool(e.runToken(&own.ID, &leadRun.ID, "ticket", `{}`), "estimate_ticket", map[string]any{"ticket": own.Key, "points": 2, "rationale": "x"})
	e.expect("delegated", code, body, 409, "isn't available to delegated runs")

	// Unmanaged sessions (no run token) must hold a live run on a ticket in the project.
	unmanaged, _, _ := agents.IssueToken(e.ctx, e.q, e.atlas.ID, "unmanaged", "estimates", nil, nil, nil)
	code, body = e.tool(unmanaged, "estimate_ticket", map[string]any{"ticket": own.Key, "points": 8, "rationale": "Bigger than thought"})
	e.expect("unmanaged own", code, body, 200, `"estimate_points":8`)
	if code, body = e.tool(unmanaged, "estimate_ticket", map[string]any{"ticket": other.Key, "points": 2, "rationale": "x"}); code == 200 {
		t.Errorf("unmanaged other project = %d %s", code, body)
	}
	if code, body = e.tool(unmanaged, "check_epic_criterion", map[string]any{"ticket": own.Key, "epic": "anything", "n": 1}); code != 422 || !strings.Contains(body, "tracker:epics") {
		t.Errorf("unmanaged check_epic_criterion without scope = %d %s", code, body)
	}
	// A finished run's token can't estimate.
	finished, err := e.q.CreateRun(e.ctx, store.CreateRunParams{WorkspaceID: e.ws, TicketID: &sibling.ID, AgentID: e.atlas.ID, Mode: "managed", State: "completed", ResolvedSpec: json.RawMessage(`{}`), Harness: "claude", Model: "test", Purpose: "ticket"})
	if err != nil {
		t.Fatal(err)
	}
	finishedTok, _, _ := agents.IssueToken(e.ctx, e.q, e.atlas.ID, "run", "finished", &finished.ID, nil, nil)
	code, body = e.tool(finishedTok, "estimate_ticket", map[string]any{"ticket": own.Key, "points": 2, "rationale": "x"})
	e.expect("completed run", code, body, 409, "the run is completed")

	// create_ticket validates and stores optional points.
	code, body = e.tool(lead, "create_ticket", map[string]any{"title": "Follow-up", "points": 4})
	e.expect("create_ticket off scale", code, body, 422, "off the scale")
	code, body = e.tool(lead, "create_ticket", map[string]any{"title": "Follow-up", "points": 2, "estimate_rationale": "Copy change"})
	e.expect("create_ticket points", code, body, 200, `"ticket"`)
	var res struct{ Ticket string }
	_ = json.Unmarshal([]byte(body), &res)
	created, _ := tracker.Get(e.ctx, e.q, e.ws, res.Ticket)
	if created.EstimatePoints == nil || *created.EstimatePoints != 2 || *created.EstimateRationale != "Copy change" || *created.EstimatedByType != "agent" {
		t.Fatalf("create_ticket estimate: %+v", created)
	}
}

func TestMCPEpicCriteria(t *testing.T) {
	e := newEstimateEnv(t)
	pay, _ := e.q.GetProjectByKey(e.ctx, store.GetProjectByKeyParams{WorkspaceID: e.ws, Key: "PAY"})
	mob, _ := e.q.GetProjectByKey(e.ctx, store.GetProjectByKeyParams{WorkspaceID: e.ws, Key: "MOB"})
	mkEpic := func(project uuid.UUID, name string, criteria ...string) store.Epic {
		t.Helper()
		ep, err := e.q.CreateEpic(e.ctx, store.CreateEpicParams{WorkspaceID: e.ws, ProjectID: project, Name: name, Health: "on_track"})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range criteria {
			if _, err := e.tr.CreateEpicCriterion(e.ctx, e.ws, ep.ID, c); err != nil {
				t.Fatal(err)
			}
		}
		return ep
	}
	epic := mkEpic(pay.ID, "Board v1", "Columns per status", "Points rollup")
	otherEpic := mkEpic(mob.ID, "Mobile epic", "Only one")
	own := e.ticket("PAY", "Verify board", &epic.ID)
	if _, err := e.tr.Update(e.ctx, e.ws, tracker.AgentActor(e.atlas.ID), own.Key, tracker.Patch{EstimatePoints: tracker.SetTo(3), EstimateRationale: ptr("verification")}); err != nil {
		t.Fatal(err)
	}
	allowed := `{"scopes":[{"scope":"tracker:epics","mode":"allowed"}]}`
	lead := e.runToken(&own.ID, nil, "ticket", allowed)

	// get_ticket includes the parent epic with numbered criteria.
	code, body := e.tool(lead, "get_ticket", map[string]any{"ticket": own.Key})
	e.expect("get_ticket epic", code, body, 200, `"estimate_points":3`)
	var gt struct {
		Epic struct {
			Key, Title, Name, Status string
			PointsTotal              int `json:"points_total"`
			Criteria                 []struct {
				N    int
				Text string
				Done bool
			}
		}
	}
	if err := json.Unmarshal([]byte(body), &gt); err != nil {
		t.Fatal(err)
	}
	if gt.Epic.Key != epic.ID.String() || gt.Epic.Title != "Board v1" || gt.Epic.Name != "Board v1" || gt.Epic.Status != "open" || gt.Epic.PointsTotal != 3 ||
		len(gt.Epic.Criteria) != 2 || gt.Epic.Criteria[1].N != 2 || gt.Epic.Criteria[1].Text != "Points rollup" || gt.Epic.Criteria[1].Done {
		t.Fatalf("get_ticket epic payload: %s", body)
	}

	// Success, by exact name and by id; the check is attributed to the agent.
	code, body = e.tool(lead, "check_epic_criterion", map[string]any{"epic": "Board v1", "n": 2})
	e.expect("check by name", code, body, 200, `"done":true`)
	cs, _ := e.q.ListEpicCriteria(e.ctx, epic.ID)
	if !cs[1].Done || cs[1].CheckedByType == nil || *cs[1].CheckedByType != "agent" || *cs[1].CheckedByID != e.atlas.ID || cs[0].Done {
		t.Fatalf("criterion not checked by agent: %+v", cs)
	}
	code, body = e.tool(lead, "check_epic_criterion", map[string]any{"epic": epic.ID.String(), "n": 2, "done": false})
	e.expect("uncheck by id", code, body, 200, `"done":false`)
	code, body = e.tool(lead, "get_ticket", map[string]any{"ticket": own.Key})
	e.expect("get_ticket after", code, body, 200, `"done":false`)

	// Rejections.
	code, body = e.tool(lead, "check_epic_criterion", map[string]any{"epic": "Board v1", "n": 3})
	e.expect("out of range", code, body, 422, "between 1 and 2")
	code, body = e.tool(lead, "check_epic_criterion", map[string]any{"epic": "Board v1", "n": 0})
	e.expect("zero", code, body, 422, "between 1 and 2")
	code, body = e.tool(lead, "check_epic_criterion", map[string]any{"epic": "No such epic", "n": 1})
	e.expect("unknown epic", code, body, 422, "no epic")
	code, body = e.tool(lead, "check_epic_criterion", map[string]any{"epic": otherEpic.ID.String(), "n": 1})
	e.expect("other project by id", code, body, 422, "no epic")
	code, body = e.tool(lead, "check_epic_criterion", map[string]any{"epic": "Mobile epic", "n": 1})
	e.expect("other project by name", code, body, 422, "no epic")

	for label, spec := range map[string]string{
		"no scope":          `{}`,
		"denied":            `{"scopes":[{"scope":"tracker:epics","mode":"denied"}]}`,
		"requires_approval": `{"scopes":[{"scope":"tracker:epics","mode":"requires_approval"}]}`,
	} {
		tk := e.ticket("PAY", "Scope "+label, &epic.ID)
		code, body = e.tool(e.runToken(&tk.ID, nil, "ticket", spec), "check_epic_criterion", map[string]any{"epic": "Board v1", "n": 1})
		e.expect(label, code, body, 422, "tracker:epics scope is allowed")
	}
	code, body = e.tool(e.runToken(nil, nil, "chat", allowed), "check_epic_criterion", map[string]any{"epic": "Board v1", "n": 1})
	e.expect("chat", code, body, 409, "isn't available in chats")
	code, body = e.tool(e.runToken(nil, nil, "planning", allowed), "check_epic_criterion", map[string]any{"epic": "Board v1", "n": 1})
	e.expect("planning", code, body, 409, "planning runs can read context and propose tickets only")
	leadRun, _ := e.q.ActiveRunForTicket(e.ctx, &own.ID)
	code, body = e.tool(e.runToken(&own.ID, &leadRun.ID, "ticket", allowed), "check_epic_criterion", map[string]any{"epic": "Board v1", "n": 1})
	e.expect("delegated", code, body, 409, "isn't available to delegated runs")

	// A done epic refuses checks.
	if _, err := e.q.SetEpicStatus(e.ctx, store.SetEpicStatusParams{ID: epic.ID, WorkspaceID: e.ws, Status: "done"}); err != nil {
		t.Fatal(err)
	}
	code, body = e.tool(lead, "check_epic_criterion", map[string]any{"epic": "Board v1", "n": 1})
	e.expect("done epic", code, body, 409, "epic is done")
}

func TestPlanningProposalPoints(t *testing.T) {
	e := newEstimateEnv(t)
	code, body := e.request("POST", "/api/v1/planning-sessions", map[string]any{"project": "PAY", "agent_id": e.atlas.ID, "goal": "Plan estimates"}, "")
	if code != 201 {
		t.Fatalf("start = %d %s", code, body)
	}
	var plan api.PlanningSession
	_ = json.Unmarshal([]byte(body), &plan)
	if _, err := e.q.SetRunState(e.ctx, store.SetRunStateParams{ID: plan.RunId, State: "running"}); err != nil {
		t.Fatal(err)
	}
	token, _, _ := agents.IssueToken(e.ctx, e.q, e.atlas.ID, "run", "planner", &plan.RunId, nil, nil)

	// Planning tokens can't use the lead-only tools.
	code, body = e.tool(token, "estimate_ticket", map[string]any{"ticket": "PAY-214", "points": 3, "rationale": "x"})
	e.expect("planning estimate_ticket", code, body, 409, "planning runs")

	drafts := []map[string]any{
		{"ref": "a", "title": "Estimated", "description": "d", "criteria": []string{"c"}, "points": 5, "estimate_rationale": " Two endpoints "},
		{"ref": "b", "title": "Unestimated", "description": "d", "criteria": []string{"c"}},
		{"ref": "c", "title": "Not selected", "description": "d", "criteria": []string{"c"}},
	}
	bad := []map[string]any{{"title": "Bad", "description": "d", "criteria": []string{"c"}, "points": 4}}
	code, body = e.tool(token, "propose_tickets", map[string]any{"tickets": bad})
	e.expect("off-scale draft", code, body, 422, "off the scale")
	code, body = e.tool(token, "propose_tickets", map[string]any{"tickets": drafts})
	e.expect("propose", code, body, 200, `"review"`)

	code, body = e.request("GET", "/api/v1/planning-sessions/"+plan.Id.String(), nil, "")
	_ = json.Unmarshal([]byte(body), &plan)
	if len(plan.Proposals) != 3 || plan.Proposals[0].EstimatePoints == nil || *plan.Proposals[0].EstimatePoints != 5 ||
		plan.Proposals[0].EstimateRationale == nil || *plan.Proposals[0].EstimateRationale != "Two endpoints" || plan.Proposals[1].EstimatePoints != nil {
		t.Fatalf("draft estimates not persisted: %d %s", code, body)
	}
	// A reviewer can't save an off-scale value either.
	edited := append([]api.TicketProposal{}, plan.Proposals...)
	four := api.EstimatePoints(4)
	edited[1].EstimatePoints = &four
	code, body = e.request("PATCH", "/api/v1/planning-sessions/"+plan.Id.String(), map[string]any{"revision": plan.Revision, "action": "save", "proposals": edited}, "")
	e.expect("save off scale", code, body, 422, "estimate_points")

	// Saving a changed estimate must not replace the planner's baseline.
	eightSaved := api.EstimatePoints(8)
	saved := append([]api.TicketProposal{}, plan.Proposals...)
	saved[0].EstimatePoints = &eightSaved
	saved[1].EstimatePoints = &eightSaved
	saved[1].PlannerEstimate = plan.Proposals[0].PlannerEstimate
	code, body = e.request("PATCH", "/api/v1/planning-sessions/"+plan.Id.String(), map[string]any{"revision": plan.Revision, "action": "save", "proposals": saved}, "")
	e.expect("save estimate", code, body, 200, "planner_estimate")
	_ = json.Unmarshal([]byte(body), &plan)
	if plan.Proposals[0].PlannerEstimate == nil || *plan.Proposals[0].PlannerEstimate.Points != 5 {
		t.Fatal("planner baseline lost")
	}
	// Resetting to the original value after saving still credits the planner.
	plan.Proposals[0].EstimatePoints = ptr(api.EstimatePoints(5))
	// The reviewer adds an estimate to the second draft: that one is the person's.
	accepted := append([]api.TicketProposal{}, plan.Proposals[:2]...)
	eight := api.EstimatePoints(8)
	accepted[1].EstimatePoints = &eight
	code, body = e.request("PATCH", "/api/v1/planning-sessions/"+plan.Id.String(), map[string]any{"revision": plan.Revision, "action": "accept", "proposals": accepted}, "")
	if code != 200 {
		t.Fatalf("accept = %d %s", code, body)
	}
	_ = json.Unmarshal([]byte(body), &plan)
	if len(plan.TicketKeys) != 2 || plan.ProposalCount != 3 {
		t.Fatalf("subset count: %+v", plan)
	}
	estimated, _ := tracker.Get(e.ctx, e.q, e.ws, plan.TicketKeys[0])
	if estimated.EstimatePoints == nil || *estimated.EstimatePoints != 5 || *estimated.EstimateRationale != "Two endpoints" ||
		*estimated.EstimatedByType != "agent" || *estimated.EstimatedByID != e.atlas.ID {
		t.Fatalf("accepted estimate: %+v", estimated)
	}
	reviewed, _ := tracker.Get(e.ctx, e.q, e.ws, plan.TicketKeys[1])
	if reviewed.EstimatePoints == nil || *reviewed.EstimatePoints != 8 || *reviewed.EstimatedByType != "user" || *reviewed.EstimatedByID != e.me.ID {
		t.Fatalf("reviewer's estimate not credited to the reviewer: %+v", reviewed)
	}
}

func ptr[T any](v T) *T { return &v }
