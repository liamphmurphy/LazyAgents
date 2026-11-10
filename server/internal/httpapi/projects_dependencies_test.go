package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/projects"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func TestProjectManagementAndDependencies(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	agent, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}
	h := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts}).Handler()
	call := func(method, path string, body any, want int, dst any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		if dst != nil {
			if err := json.Unmarshal(res.Body.Bytes(), dst); err != nil {
				t.Fatal(err)
			}
		}
	}
	create := api.NewProject{Key: "ops", Name: "Operations", Color: "#7c8aff", RepoUrl: "git@github.com:example/operations.git", DefaultBranch: "develop"}
	var project api.Project
	call("POST", "/api/v1/projects", create, 201, &project)
	if project.Key != "OPS" || project.DefaultBranch != "develop" || project.Archived || project.Revision != 1 {
		t.Fatalf("project %+v", project)
	}
	call("POST", "/api/v1/projects", create, 409, nil)
	// An HTTPS GitHub link is accepted and stored in SSH form (LAZYAGENTS-44).
	https := create
	https.Key = "WEBX"
	https.RepoUrl = "https://github.com/example/operations"
	var viaHTTPS api.Project
	call("POST", "/api/v1/projects", https, 201, &viaHTTPS)
	if viaHTTPS.RepoUrl == nil || *viaHTTPS.RepoUrl != "git@github.com:example/operations.git" {
		t.Fatalf("https repo_url stored as %v", viaHTTPS.RepoUrl)
	}
	for _, url := range []string{"http://github.com/example/operations", "https://token@github.com/example/operations", "file:///tmp/project", "git@github.com:example/repo.git?token=secret", "git@gitlab.com:example/repo.git"} {
		bad := create
		bad.Key = "BAD"
		bad.RepoUrl = url
		call("POST", "/api/v1/projects", bad, 422, nil)
	}
	for _, branch := range []string{"--upload-pack=evil", "feature/../bad", "a.lock/b", "a//b"} {
		bad := create
		bad.Key = "BAD"
		bad.DefaultBranch = branch
		call("POST", "/api/v1/projects", bad, 422, nil)
	}
	edit := func(archived bool) api.ProjectUpdate {
		return api.ProjectUpdate{Name: project.Name, Color: project.Color, RepoUrl: deref(project.RepoUrl), DefaultBranch: project.DefaultBranch, Archived: archived, Revision: project.Revision}
	}
	old := edit(false)
	update := edit(false)
	update.Name = "Operations platform"
	call("PUT", "/api/v1/projects/OPS", update, 200, &project)
	call("PUT", "/api/v1/projects/OPS", old, 409, nil)
	makeTicket := func(title string) api.Ticket {
		var out api.Ticket
		call("POST", "/api/v1/tickets", map[string]any{"project_key": "OPS", "title": title, "assignee_agent_id": agent.ID}, 201, &out)
		return out
	}
	a, b, c := makeTicket("Database schema"), makeTicket("API implementation"), makeTicket("Web interface")
	deps := func(ticket *api.Ticket, keys []string, want int) {
		t.Helper()
		var out api.Ticket
		call("PUT", "/api/v1/tickets/"+ticket.Key+"/dependencies", map[string]any{"revision": ticket.DependencyRevision, "blocked_by": keys}, want, &out)
		if want == 200 {
			*ticket = out
		}
	}
	deps(&b, []string{a.Key, a.Key}, 200)
	if len(b.BlockedBy) != 1 {
		t.Fatal("duplicate blockers were not collapsed")
	}
	deps(&c, []string{b.Key}, 200)
	deps(&a, []string{c.Key}, 422)
	deps(&a, []string{a.Key}, 422)
	deps(&a, []string{"UNKNOWN-1"}, 422)
	var detail api.Ticket
	call("GET", "/api/v1/tickets/"+a.Key, nil, 200, &detail)
	if len(detail.Blocks) != 1 || detail.Blocks[0].Key != b.Key {
		t.Fatal("reverse dependency missing")
	}
	ready := func() []api.TicketSummary {
		var out api.TicketList
		call("GET", "/api/v1/tickets?project=OPS&view=all&ready=true", nil, 200, &out)
		return out.Tickets
	}
	if got := ready(); len(got) != 1 || got[0].Key != a.Key {
		t.Fatalf("ready=%+v", got)
	}
	var preview api.DispatchPreview
	call("GET", "/api/v1/tickets/"+b.Key+"/dispatch", nil, 200, &preview)
	if len(preview.Blockers) != 0 || !strings.Contains(strings.Join(preview.QueuedBehind, " "), a.Key) || preview.BaseBranch == nil || *preview.BaseBranch != "develop" {
		t.Fatalf("preview %+v", preview)
	}
	if _, err := rs.Dispatch(ctx, me.WorkspaceID, me.ID, []string{a.Key, b.Key}); err != nil {
		t.Fatalf("blocked ticket should queue: %v", err)
	}
	bid := b.Id
	if r, err := q.ActiveRunForTicket(ctx, &bid); err != nil || r.State != "queued" {
		t.Fatalf("blocked ticket run = %+v, %v", r, err)
	}
	if _, err := rs.Claim(ctx, agents.Caller{Agent: agent, Kind: "unmanaged"}, b.Key); err == nil {
		t.Fatal("unmanaged claim bypassed dependencies")
	}
	for _, id := range []uuid.UUID{a.Id, b.Id} {
		id := id
		r, err := q.ActiveRunForTicket(ctx, &id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rs.SetRunControl(ctx, me.WorkspaceID, me.ID, r.ID, "cancel"); err != nil {
			t.Fatal(err)
		}
	}
	done := "done"
	if _, err := tr.Update(ctx, me.WorkspaceID, tracker.UserActor(me.ID), a.Key, tracker.Patch{Status: &done}); err != nil {
		t.Fatal(err)
	}
	if got := ready(); len(got) != 1 || got[0].Key != b.Key {
		t.Fatalf("completion did not unblock next ticket: %+v", got)
	}
	var inbox api.ListInbox200JSONResponse
	call("GET", "/api/v1/inbox?project=OPS", nil, 200, &inbox)
	found := false
	for _, i := range inbox.Items {
		if i.Kind == "dispatch" && i.TicketKey == b.Key {
			found = true
		}
		if i.Kind == "dispatch" && i.TicketKey == c.Key {
			t.Fatal("blocked ticket offered for dispatch")
		}
	}
	if !found {
		t.Fatal("ready ticket missing from Inbox")
	}
	if err := q.ReadInboxItem(ctx, store.ReadInboxItemParams{UserID: me.ID, ItemID: "dispatch:" + b.Id.String()}); err != nil {
		t.Fatal(err)
	}
	todo := "todo"
	_, _ = tr.Update(ctx, me.WorkspaceID, tracker.UserActor(me.ID), a.Key, tracker.Patch{Status: &todo})
	if len(ready()) != 1 || ready()[0].Key != a.Key {
		t.Fatal("reopening did not restore blocker")
	}
	_, _ = tr.Update(ctx, me.WorkspaceID, tracker.UserActor(me.ID), a.Key, tracker.Patch{Status: &done})
	call("GET", "/api/v1/inbox?project=OPS", nil, 200, &inbox)
	for _, item := range inbox.Items {
		if item.Id == "dispatch:"+b.Id.String() && item.Read {
			t.Fatal("newly unblocked work remained read")
		}
	}
	dispatched, err := rs.Dispatch(ctx, me.WorkspaceID, me.ID, []string{b.Key})
	if err != nil {
		t.Fatal(err)
	}
	frozen, _ := runs.Spec(dispatched[0])
	if frozen.Repository == nil || frozen.Repository.DefaultBranch != "develop" || frozen.Repository.URL == nil || *frozen.Repository.URL != create.RepoUrl {
		t.Fatalf("missing approved repository snapshot: %+v", frozen.Repository)
	}
	call("PUT", "/api/v1/projects/OPS", edit(true), 409, nil)
	deps(&b, []string{}, 409)
	update = edit(false)
	update.DefaultBranch = "release"
	update.RepoUrl = "git@github.com:example/next-repository.git"
	call("PUT", "/api/v1/projects/OPS", update, 200, &project)
	persisted, _ := q.GetRun(ctx, store.GetRunParams{ID: dispatched[0].ID, WorkspaceID: me.WorkspaceID})
	snapshot, _ := runs.Spec(persisted)
	if snapshot.Repository.DefaultBranch != "develop" || *snapshot.Repository.URL != create.RepoUrl {
		t.Fatal("project edit changed approved run")
	}
	if _, err := rs.SetRunControl(ctx, me.WorkspaceID, me.ID, dispatched[0].ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	call("PUT", "/api/v1/projects/OPS", edit(true), 200, &project)
	call("POST", "/api/v1/tickets", map[string]any{"project_key": "OPS", "title": "Must not create"}, 409, nil)
	call("POST", "/api/v1/planning-sessions", map[string]any{"project": "OPS", "agent_id": agent.ID, "goal": "Must not plan"}, 409, nil)
	if _, err := rs.Claim(ctx, agents.Caller{Agent: agent, Kind: "unmanaged"}, c.Key); err == nil {
		t.Fatal("archived project was claimable")
	}
	var all api.TicketList
	call("GET", "/api/v1/tickets?view=all", nil, 200, &all)
	for _, tk := range all.Tickets {
		if tk.ProjectKey == "OPS" {
			t.Fatal("archived work in active list")
		}
	}
	call("GET", "/api/v1/tickets?view=all&project=OPS", nil, 200, &all)
	if len(all.Tickets) != 3 {
		t.Fatal("archiving lost historical tickets")
	}
	call("GET", "/api/v1/tickets/"+b.Key, nil, 200, &detail)
	if !detail.Project.Archived || len(detail.BlockedBy) != 1 {
		t.Fatal("archived ticket lost history")
	}
	var dashboard api.GetDashboard200JSONResponse
	call("GET", "/api/v1/dashboard", nil, 200, &dashboard)
	for _, p := range dashboard.Projects {
		if p.Project.Key == "OPS" {
			t.Fatal("archived project in dashboard")
		}
	}
	call("PUT", "/api/v1/projects/OPS", edit(false), 200, &project)
	if project.Archived || project.Key != "OPS" {
		t.Fatal("restore changed identity")
	}
	// Concurrent inverse edits cannot introduce a cycle.
	d, e := makeTicket("Parallel D"), makeTicket("Parallel E")
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, pair := range [][2]api.Ticket{{d, e}, {e, d}} {
		wg.Add(1)
		go func(pair [2]api.Ticket) {
			defer wg.Done()
			results <- tr.SetDependencies(ctx, me.WorkspaceID, tracker.UserActor(me.ID), pair[0].Key, int32(pair[0].DependencyRevision), []string{pair[1].Key})
		}(pair)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent graph edits: %d succeeded", success)
	}
	// A stale edit is rejected even when it would remove every dependency.
	call("PUT", "/api/v1/tickets/"+b.Key+"/dependencies", map[string]any{"revision": 1, "blocked_by": []string{}}, 409, nil)
}

func TestPlanningDependencyAcceptance(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	a, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr}
	external, err := tr.Create(ctx, me.WorkspaceID, tracker.UserActor(me.ID), tracker.NewTicket{ProjectKey: "PAY", Title: "Existing prerequisite"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := rs.StartPlanning(ctx, me.WorkspaceID, me.ID, runs.PlanningInput{Project: "PAY", AgentID: a.ID, Goal: "Plan a dependent workflow"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: plan.RunID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	caller := agents.Caller{Agent: a, RunID: &plan.RunID}
	for _, bad := range [][]runs.Proposal{
		{{Ref: "same", Title: "One"}, {Ref: "same", Title: "Two"}},
		{{Ref: "one", Title: "One", DependsOn: []string{"missing"}}},
		{{Ref: "one", Title: "One", DependsOn: []string{"one"}}},
		{{Ref: "one", Title: "One", DependsOn: []string{"two"}}, {Ref: "two", Title: "Two", DependsOn: []string{"one"}}},
		{{Ref: "one", Title: "One", BlockedBy: []string{"NOPE-1"}}},
	} {
		if _, err := rs.ProposeTickets(ctx, caller, bad); err == nil {
			t.Fatalf("accepted invalid proposal graph: %+v", bad)
		}
	}
	items := []runs.Proposal{{Ref: "api", Title: "API", AgentID: &a.ID, DependsOn: []string{"schema"}, BlockedBy: []string{external.Key}}, {Ref: "schema", Title: "Schema", AgentID: &a.ID}}
	draft, err := rs.ProposeTickets(ctx, caller, items)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rs.ReviewPlanning(ctx, me.WorkspaceID, me.ID, draft.ID, draft.Revision, "save", items[:1]); err == nil {
		t.Fatal("dropping a referenced proposal silently rewrote dependencies")
	}
	// Reordering proposals retains dependencies by ref, rather than by position.
	items = []runs.Proposal{items[1], items[0]}
	accepted, err := rs.ReviewPlanning(ctx, me.WorkspaceID, me.ID, draft.ID, draft.Revision, "accept", items)
	if err != nil {
		t.Fatal(err)
	}
	if len(accepted.TicketKeys) != 2 {
		t.Fatal("missing accepted tickets")
	}
	schema, _ := tracker.Get(ctx, q, me.WorkspaceID, accepted.TicketKeys[0])
	apiTicket, _ := tracker.Get(ctx, q, me.WorkspaceID, accepted.TicketKeys[1])
	deps, err := q.TicketDependencies(ctx, apiTicket.ID)
	if err != nil || len(deps) != 2 {
		t.Fatalf("dependencies: %+v %v", deps, err)
	}
	found := false
	for _, d := range deps {
		if d.ID == schema.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("reordering redirected dependency")
	}
	ready, err := q.ReadyPlanningTickets(ctx, store.ReadyPlanningTicketsParams{WorkspaceID: me.WorkspaceID, Keys: accepted.TicketKeys})
	if err != nil || len(ready) != 1 || ready[0] != schema.Key {
		t.Fatalf("wrong ready wave: %v %v", ready, err)
	}
	done := "done"
	_, _ = tr.Update(ctx, me.WorkspaceID, tracker.UserActor(me.ID), schema.Key, tracker.Patch{Status: &done})
	_, _ = tr.Update(ctx, me.WorkspaceID, tracker.UserActor(me.ID), external.Key, tracker.Patch{Status: &done})
	ready, err = q.ReadyPlanningTickets(ctx, store.ReadyPlanningTicketsParams{WorkspaceID: me.WorkspaceID, Keys: accepted.TicketKeys})
	if err != nil || len(ready) != 1 || ready[0] != apiTicket.Key {
		t.Fatalf("next wave did not become ready: %v %v", ready, err)
	}
	if _, err := q.ActiveRunForTicket(ctx, &apiTicket.ID); err == nil {
		t.Fatal("dependency completion auto-dispatched a ticket")
	}
}

func TestProjectAndDependencyWorkspaceBoundaries(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	tr := &tracker.Service{Pool: pool}
	svc := projects.Service{Pool: pool}
	foreign, err := q.CreateWorkspace(ctx, store.CreateWorkspaceParams{Name: "Other workspace", Slug: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, foreign.ID, "OUT", projects.Config{Name: "Private", Color: "#112233"}); err != nil {
		t.Fatal(err)
	}
	private, err := tr.Create(ctx, foreign.ID, tracker.Actor{Type: "system"}, tracker.NewTicket{ProjectKey: "OUT", Title: "Private task"})
	if err != nil {
		t.Fatal(err)
	}
	own, err := tr.Create(ctx, me.WorkspaceID, tracker.UserActor(me.ID), tracker.NewTicket{ProjectKey: "PAY", Title: "Own task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.SetDependencies(ctx, me.WorkspaceID, tracker.UserActor(me.ID), own.Key, own.DependencyRevision, []string{private.Key}); err == nil {
		t.Fatal("cross-workspace blocker accepted")
	}
	if err := tr.SetDependencies(ctx, foreign.ID, tracker.Actor{Type: "system"}, own.Key, own.DependencyRevision, []string{}); err == nil {
		t.Fatal("cross-workspace ticket edited")
	}
	if _, err := svc.Update(ctx, me.WorkspaceID, "OUT", projects.Config{Name: "Overwrite", Color: "#112233", Revision: 1}); err == nil {
		t.Fatal("cross-workspace project edited")
	}
	// Queued planners also prevent archiving, and an archived draft cannot create work.
	a, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	rs := &runs.Service{Pool: pool, Tracker: tr}
	plan, err := rs.StartPlanning(ctx, me.WorkspaceID, me.ID, runs.PlanningInput{Project: "PAY", AgentID: a.ID, Goal: "Archive boundary"})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: me.WorkspaceID, Key: "PAY"})
	config := projects.Config{Name: p.Name, Color: p.Color, RepoURL: deref(p.RepoUrl), DefaultBranch: p.DefaultBranch, Revision: p.Revision, Archived: true}
	if _, err := svc.Update(ctx, me.WorkspaceID, p.Key, config); err == nil {
		t.Fatal("archived with a queued planner")
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: plan.RunID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	items := []runs.Proposal{{Ref: "one", Title: "Proposed work"}}
	draft, err := rs.ProposeTickets(ctx, agents.Caller{Agent: a, RunID: &plan.RunID}, items)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := svc.Update(ctx, me.WorkspaceID, p.Key, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rs.ReviewPlanning(ctx, me.WorkspaceID, me.ID, draft.ID, draft.Revision, "accept", items); err == nil {
		t.Fatal("accepted a plan in an archived project")
	}
	untouched, _ := q.GetPlanningSession(ctx, store.GetPlanningSessionParams{ID: draft.ID, WorkspaceID: me.WorkspaceID})
	if untouched.State != "review" || len(untouched.TicketKeys) != 0 {
		t.Fatal("failed acceptance modified draft")
	}
	config.Revision = archived.Revision
	config.Archived = false
	if _, err := svc.Update(ctx, me.WorkspaceID, p.Key, config); err != nil {
		t.Fatal(err)
	}
	if _, err := rs.ReviewPlanning(ctx, me.WorkspaceID, me.ID, draft.ID, draft.Revision, "accept", items); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyRunRepositoryFreezesBeforeProjectEdit(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	a, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: me.WorkspaceID, Handle: "atlas"})
	tk, _ := tracker.Get(ctx, q, me.WorkspaceID, "PAY-224")
	p, _ := q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: me.WorkspaceID, Key: "PAY"})
	legacy, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: me.WorkspaceID, TicketID: &tk.ID, AgentID: a.ID, Mode: "managed", State: "queued", Harness: "claude-code", Model: "test", ResolvedSpec: json.RawMessage(`{}`), Purpose: "ticket"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = (projects.Service{Pool: pool}).Update(ctx, me.WorkspaceID, p.Key, projects.Config{Name: p.Name, Color: p.Color, RepoURL: "git@github.com:example/changed.git", DefaultBranch: "release", Revision: p.Revision})
	if err != nil {
		t.Fatal(err)
	}
	frozen, _ := q.GetRun(ctx, store.GetRunParams{ID: legacy.ID, WorkspaceID: me.WorkspaceID})
	spec, err := runs.Spec(frozen)
	if err != nil || spec.Repository == nil || spec.Repository.DefaultBranch != p.DefaultBranch || deref(spec.Repository.URL) != deref(p.RepoUrl) {
		t.Fatalf("legacy snapshot changed: %+v %v", spec.Repository, err)
	}
}
