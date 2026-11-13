package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPlanningWorkflow(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	a, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts, Wiki: &wiki.Service{Pool: pool, Artifacts: arts, Tracker: tr}}).Handler()
	request := func(method, path string, body any, token string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	requireStatus := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("got %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	count := func() int {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM tickets").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	epicCount := func() int {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM epics").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	beforeEpics := epicCount()
	var source uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM artifacts WHERE type='prd' AND project_id=(SELECT id FROM projects WHERE key='PAY') LIMIT 1").Scan(&source); err != nil {
		t.Fatal(err)
	}
	requireStatus(request("POST", "/api/v1/planning-sessions", map[string]any{"project": "PAY", "agent_id": a.ID, "goal": ""}, ""), 422)
	requireStatus(request("POST", "/api/v1/planning-sessions", map[string]any{"project": "UNKNOWN", "agent_id": a.ID, "goal": "Plan"}, ""), 422)
	// Attached context must belong to the plan's project.
	requireStatus(request("POST", "/api/v1/planning-sessions", map[string]any{"project": "PAY", "agent_id": a.ID, "goal": "Plan", "context_refs": []map[string]any{{"kind": "ticket", "key": "NOPE-1"}}}, ""), 422)
	before := count()
	refs := []map[string]any{{"kind": "ticket", "key": "PAY-214"}, {"kind": "artifact", "id": source}}
	w := request("POST", "/api/v1/planning-sessions", map[string]any{"project": "PAY", "agent_id": a.ID, "goal": "Plan reconciliation", "artifact_id": source, "context_refs": refs}, "")
	requireStatus(w, 201)
	var plan api.PlanningSession
	if err := json.Unmarshal(w.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if count() != before || plan.State != "planning" || plan.RunState != "queued" || plan.SourceVersionId == nil {
		t.Fatalf("start created work prematurely: %+v", plan)
	}
	if len(plan.ContextRefs) != 2 || plan.ContextRefs[0].Key == nil || *plan.ContextRefs[0].Key != "PAY-214" || plan.ProposalCount != 0 {
		t.Fatalf("context refs not echoed: %+v", plan.ContextRefs)
	}
	run, err := q.GetRun(ctx, store.GetRunParams{ID: plan.RunId, WorkspaceID: me.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	// The prompt lists the attached files, and the runner delivers them there.
	var prompt string
	if err := pool.QueryRow(ctx, "SELECT input_message FROM run_turns WHERE run_id=$1", run.ID).Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	refFiles, err := runner.PlanningRefFiles(ctx, q, run)
	if err != nil || len(refFiles) != 2 || refFiles[0].Path != runs.PlanningRefPath(0) || !strings.Contains(string(refFiles[0].Body), "PAY-214") || !strings.Contains(prompt, runs.PlanningRefPath(1)) {
		t.Fatalf("attached context not delivered: %+v, %v, prompt %q", refFiles, err, prompt)
	}
	spec, _ := runs.Spec(run)
	if run.TicketID != nil || spec.Mode("repo:write") != "denied" || len(spec.Setup) != 0 || len(spec.MCPServers) != 0 || spec.HarnessOptions["sandbox"] != "read-only" {
		t.Fatalf("unsafe planning snapshot: %+v", spec)
	}
	// A queued run reads its pinned source, not a newly published version.
	old, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *plan.SourceVersionId, ArtifactID: source, WorkspaceID: me.WorkspaceID, Viewer: pageview.From(ctx).UserID})
	if err != nil {
		t.Fatal(err)
	}
	replacement := "# Replacement source\nThis was published after approval."
	if _, err := arts.AddVersion(ctx, me.WorkspaceID, artifacts.Actor{Type: "user", ID: me.ID}, source, artifacts.NewVersion{Label: "v-next", Content: artifacts.Content{BodyMD: &replacement}}); err != nil {
		t.Fatal(err)
	}
	file, err := runner.PlanningContextFile(ctx, q, run)
	if err != nil || file == nil || string(file.Body) != *old.BodyMd || file.VersionID == nil || *file.VersionID != old.ID {
		t.Fatalf("source changed under queued run: %+v, %v", file, err)
	}
	// The shared run log and cancellation endpoints must work without a ticket.
	requireStatus(request("GET", "/api/v1/runs/"+run.ID.String(), nil, ""), 200)
	// A planner cannot mutate the tracker through any of the Agent API transports.
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	token, _, err := agents.IssueToken(ctx, q, a.ID, "run", "planner", &run.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"create_ticket", "claim_ticket", "comment", "write_note", "request_approval", "complete_run"} {
		w = request("POST", "/api/v1/agent/tools/"+name, map[string]any{"ticket": "PAY-214", "body": "must not post"}, token)
		requireStatus(w, 409)
	}
	requireStatus(request("POST", "/api/v1/agent/tools/get_ticket", map[string]any{"ticket": "PAY-214"}, token), 200)
	// The MCP transport uses the same server-side mutation restriction.
	ts := httptest.NewServer(handler)
	defer ts.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "planning-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp", HTTPClient: &http.Client{Transport: bearer{token, http.DefaultTransport}}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	denied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "comment", Arguments: map[string]any{"ticket": "PAY-214", "body": "A planner must not post this"}})
	// The planning server doesn't register comment at all, so the SDK answers "unknown tool" as a protocol error.
	if err == nil && !denied.IsError {
		t.Fatalf("MCP planning mutation allowed: %+v, %v", denied, err)
	}
	proposals := []runs.Proposal{{Title: "Build reconciliation", Description: "Implement the core", Criteria: []string{"Reconciles a ledger"}, AgentID: &a.ID}, {Title: "Test reconciliation", Description: "Cover edge cases", Criteria: []string{"Duplicate input is safe"}}}
	w = request("POST", "/api/v1/agent/tools/propose_tickets", map[string]any{"tickets": proposals}, token)
	requireStatus(w, 200)
	if count() != before {
		t.Fatal("proposals created real tickets")
	}
	w = request("GET", "/api/v1/planning-sessions/"+plan.Id.String(), nil, "")
	requireStatus(w, 200)
	_ = json.Unmarshal(w.Body.Bytes(), &plan)
	if plan.State != "review" || plan.RunState != "completed" || len(plan.Proposals) != 2 || plan.ProposalCount != 2 {
		t.Fatalf("not ready for review: %+v", plan)
	}
	inbox := request("GET", "/api/v1/inbox", nil, "")
	requireStatus(inbox, 200)
	if !strings.Contains(inbox.Body.String(), "planning:"+plan.Id.String()) {
		t.Fatal("draft missing from Inbox")
	}
	proposals[0].Title = "Edited core implementation"
	proposals[0].RequiresMockup = true
	w = request("PATCH", "/api/v1/planning-sessions/"+plan.Id.String(), map[string]any{"revision": plan.Revision, "action": "save", "proposals": proposals}, "")
	requireStatus(w, 200)
	staleRevision := plan.Revision
	_ = json.Unmarshal(w.Body.Bytes(), &plan)
	requireStatus(request("PATCH", "/api/v1/planning-sessions/"+plan.Id.String(), map[string]any{"revision": staleRevision, "action": "accept", "proposals": proposals}, ""), 409)
	// Force a failure on ticket two, proving all ticket/criteria/activity writes roll back.
	if _, err = pool.Exec(ctx, `CREATE FUNCTION reject_planning_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.title='Test reconciliation' THEN RAISE EXCEPTION 'test failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_planning_test BEFORE INSERT ON tickets FOR EACH ROW EXECUTE FUNCTION reject_planning_test()`); err != nil {
		t.Fatal(err)
	}
	requireStatus(request("PATCH", "/api/v1/planning-sessions/"+plan.Id.String(), map[string]any{"revision": plan.Revision, "action": "accept", "proposals": proposals}, ""), 500)
	if count() != before || epicCount() != beforeEpics {
		t.Fatal("acceptance partially committed tickets or epic")
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER reject_planning_test ON tickets; DROP FUNCTION reject_planning_test()"); err != nil {
		t.Fatal(err)
	}
	// Two reviewers accepting together create exactly one set of tickets.
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- request("PATCH", "/api/v1/planning-sessions/"+plan.Id.String(), map[string]any{"revision": plan.Revision, "action": "accept", "proposals": proposals}, "").Code
		}()
	}
	wg.Wait()
	close(statuses)
	results := map[int]int{}
	for status := range statuses {
		results[status]++
	}
	if results[200] != 1 || results[409] != 1 || count() != before+2 {
		t.Fatalf("duplicate acceptance: %v, count=%d", results, count())
	}
	p, err := q.GetPlanningSession(ctx, store.GetPlanningSessionParams{ID: plan.Id, WorkspaceID: me.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if p.EpicID == nil || epicCount() != beforeEpics+1 {
		t.Fatalf("acceptance should create exactly one epic: %+v", p)
	}
	epic, err := q.GetEpic(ctx, store.GetEpicParams{ID: *p.EpicID, WorkspaceID: me.WorkspaceID})
	if err != nil || epic.Name != p.Goal || epic.Objective != p.Goal || epic.ProjectID != p.ProjectID {
		t.Fatalf("epic context lost: %+v %v", epic, err)
	}
	for _, key := range p.TicketKeys {
		ticket, err := tracker.Get(ctx, q, me.WorkspaceID, key)
		if err != nil || ticket.EpicID == nil || *ticket.EpicID != epic.ID {
			t.Fatalf("ticket not linked to plan epic: %+v %v", ticket, err)
		}
	}
	if len(p.TicketKeys) != 2 {
		t.Fatal("missing created keys")
	}
	tk, err := tracker.Get(ctx, q, me.WorkspaceID, p.TicketKeys[0])
	if err != nil {
		t.Fatal(err)
	}
	cr, _ := q.ListCriteria(ctx, tk.ID)
	docs, _ := q.ListTicketArtifacts(ctx, store.ListTicketArtifactsParams{TicketID: tk.ID, Viewer: pageview.From(ctx).UserID})
	if tk.Title != proposals[0].Title || tk.Status != "backlog" || tk.AssigneeAgentID == nil || *tk.AssigneeAgentID != a.ID || len(cr) != 1 || len(docs) != 1 || docs[0].ID != source {
		t.Fatalf("ticket context lost: %+v, criteria=%v docs=%v", tk, cr, docs)
	}
	if !tk.RequiresMockup {
		t.Fatal("requires_mockup was not carried onto the accepted ticket")
	}
	if other, err := tracker.Get(ctx, q, me.WorkspaceID, p.TicketKeys[1]); err != nil || other.RequiresMockup {
		t.Fatalf("unflagged proposal became gated: %+v %v", other, err)
	}
	if _, err := q.ActiveRunForTicket(ctx, &tk.ID); err == nil {
		t.Fatal("acceptance dispatched work")
	}
	// Cross-workspace IDs cannot read/review a plan.
	if _, err := rs.ReviewPlanning(ctx, uuid.New(), me.ID, plan.Id, p.Revision, "accept", proposals); err == nil {
		t.Fatal("cross-workspace acceptance succeeded")
	}
	w = request(http.MethodGet, "/api/v1/planning-sessions", nil, "")
	requireStatus(w, 200)
	// A dismissed draft creates no tickets and cannot later be accepted.
	dismissed, err := rs.StartPlanning(ctx, me.WorkspaceID, me.ID, runs.PlanningInput{Project: "PAY", AgentID: a.ID, Goal: "Discard this plan"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: dismissed.RunID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	draft, err := rs.ProposeTickets(ctx, agents.Caller{Agent: a, RunID: &dismissed.RunID}, proposals)
	if err != nil {
		t.Fatal(err)
	}
	discarded, err := rs.ReviewPlanning(ctx, me.WorkspaceID, me.ID, draft.ID, draft.Revision, "dismiss", nil)
	if err != nil || discarded.State != "dismissed" || count() != before+2 {
		t.Fatalf("dismissal failed: %+v %v", discarded, err)
	}
	if _, err := rs.ReviewPlanning(ctx, me.WorkspaceID, me.ID, draft.ID, discarded.Revision, "accept", proposals); err == nil {
		t.Fatal("accepted a dismissed plan")
	}
	if epicCount() != beforeEpics+1 {
		t.Fatal("dismissal created an epic")
	}
	// Extending an existing epic must reuse it.
	extension, err := rs.StartPlanning(ctx, me.WorkspaceID, me.ID, runs.PlanningInput{Project: "PAY", AgentID: a.ID, EpicID: p.EpicID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: extension.RunID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	extension, err = rs.ProposeTickets(ctx, agents.Caller{Agent: a, RunID: &extension.RunID}, proposals)
	if err != nil {
		t.Fatal(err)
	}
	extension, err = rs.ReviewPlanning(ctx, me.WorkspaceID, me.ID, extension.ID, extension.Revision, "accept", proposals)
	if err != nil || extension.EpicID == nil || *extension.EpicID != epic.ID || epicCount() != beforeEpics+1 {
		t.Fatalf("existing epic not reused: %+v %v", extension, err)
	}
	// Standalone cancellation leaves no phantom ticket and can be logged normally.
	cancel, err := rs.StartPlanning(ctx, me.WorkspaceID, me.ID, runs.PlanningInput{Project: "PAY", AgentID: a.ID, Goal: "Another plan"})
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(request("POST", "/api/v1/runs/"+cancel.RunID.String()+"/cancel", nil, ""), 200)
}
