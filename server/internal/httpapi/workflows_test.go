package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func gatedWorkflowGraph() map[string]any {
	return map[string]any{
		"nodes": []map[string]any{
			{"id": "start", "type": "trigger", "title": "Start", "config": map[string]any{}},
			{"id": "brief", "type": "enrich", "title": "Brief", "config": map[string]any{}},
			{"id": "work", "type": "agent", "title": "Work", "config": map[string]any{"prompt": "Do the brief: {{brief.output}}"}},
			{"id": "review", "type": "gate", "title": "Review", "config": map[string]any{"retry_from": "brief", "max_attempts": 2}},
			{"id": "page", "type": "wiki", "title": "Page", "config": map[string]any{"path": "Workflows/Result"}},
			{"id": "note", "type": "chat", "title": "Tell me", "config": map[string]any{"prompt": "Finished: {{input}}"}},
		},
		"edges": []map[string]any{{"from": "start", "to": "brief"}, {"from": "brief", "to": "work"}, {"from": "work", "to": "review"}, {"from": "review", "to": "page"}, {"from": "review", "to": "note"}},
	}
}

func (e *chatEnv) publishedWorkflow(t *testing.T) string {
	t.Helper()
	id := e.want("POST", "/workflows", map[string]any{"name": "Research to wiki", "description": "Research and file it"}, http.StatusCreated)["id"].(string)
	e.want("PATCH", "/workflows/"+id, map[string]any{"graph": gatedWorkflowGraph()}, 200)
	e.want("POST", "/workflows/"+id+"/publish", nil, 200)
	return id
}

func (e *chatEnv) wfStep(run map[string]any, node string) map[string]any {
	for _, s := range run["steps"].([]any) {
		if m := s.(map[string]any); m["node_id"] == node {
			return m
		}
	}
	return nil
}

// finishWFStep plays the part of the model: submit a result, then exit.
func (e *chatEnv) finishWFStep(t *testing.T, runID, node, output string) map[string]any {
	t.Helper()
	run := e.want("GET", "/workflow-runs/"+runID, nil, 200)
	step := e.wfStep(run, node)
	if step == nil || step["state"] != "running" {
		t.Fatalf("%s isn't running: %v", node, step)
	}
	agent := uuid.MustParse(step["agent_run_id"].(string))
	if err := e.runs.SubmitWorkflowOutput(t.Context(), agent, output); err != nil {
		t.Fatal(err)
	}
	e.setRunState(t, agent, "completed")
	e.advanceWorkflows(t)
	return e.want("GET", "/workflow-runs/"+runID, nil, 200)
}

func (e *chatEnv) advanceWorkflows(t *testing.T) {
	t.Helper()
	if err := runs.NewWorkflowEngine(e.pool, e.runs.Artifacts, e.runs.Tracker).AdvanceAll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowPublishNeedsAValidGraph(t *testing.T) {
	e := newChatEnv(t)
	created := e.want("POST", "/workflows", map[string]any{"name": "Empty"}, http.StatusCreated)
	if len(created["issues"].([]any)) == 0 || created["published_version"].(float64) != 0 {
		t.Fatalf("%v", created)
	}
	e.invalid("POST", "/workflows/"+created["id"].(string)+"/publish", nil, "graph", "at least one step")
}

func TestWorkflowRunsToAGateThenWritesTheWikiOnApproval(t *testing.T) {
	e := newChatEnv(t)
	wf := e.publishedWorkflow(t)
	chat := e.newChat(nil)["id"].(string)
	run := e.want("POST", "/chats/"+chat+"/workflows", map[string]any{"workflow_id": wf, "input": "Compare queues"}, http.StatusCreated)
	id := run["id"].(string)
	if run["state"] != "running" || e.wfStep(run, "start")["output"] != "Compare queues" {
		t.Fatalf("%v", run)
	}

	// Steps are read-only and carry no repository.
	var spec []byte
	var purpose string
	agent := e.wfStep(run, "brief")["agent_run_id"].(string)
	if err := e.pool.QueryRow(t.Context(), `SELECT purpose, resolved_spec FROM runs WHERE id = $1`, agent).Scan(&purpose, &spec); err != nil {
		t.Fatal(err)
	}
	if purpose != "workflow" || !containsBytes(spec, `"readonly": "true"`) || containsBytes(spec, `"web_search"`) {
		t.Fatalf("%s %s", purpose, spec)
	}

	e.finishWFStep(t, id, "brief", "A sharper brief")
	var prompt string
	work := e.wfStep(e.want("GET", "/workflow-runs/"+id, nil, 200), "work")["agent_run_id"].(string)
	if err := e.pool.QueryRow(t.Context(), `SELECT input_message FROM run_turns WHERE run_id = $1`, work).Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Do the brief: A sharper brief") {
		t.Fatal(prompt)
	}
	run = e.finishWFStep(t, id, "work", "The findings")
	if run["state"] != "waiting" || e.wfStep(run, "review")["state"] != "waiting" || e.wfStep(run, "page") != nil {
		t.Fatalf("%v", run)
	}
	inbox := e.want("GET", "/inbox", nil, 200)["items"].([]any)
	found := false
	for _, i := range inbox {
		found = found || i.(map[string]any)["kind"] == "workflow_gate"
	}
	if !found {
		t.Fatal("the gate isn't in the Inbox")
	}

	run = e.want("POST", "/workflow-runs/"+id+"/gate", map[string]any{"node_id": "review", "decision": "approve"}, 200)
	if run["state"] != "done" || run["finished_at"] == nil {
		t.Fatalf("%v", run)
	}
	var body string
	if err := e.pool.QueryRow(t.Context(), `SELECT v.body_md FROM artifacts a JOIN artifact_versions v ON v.id = a.current_version_id WHERE a.path = 'Workflows/Result'`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if body != "The findings" {
		t.Fatal(body)
	}
	var posted string
	if err := e.pool.QueryRow(t.Context(), `SELECT body_md FROM chat_messages WHERE session_id = $1 ORDER BY seq DESC LIMIT 1`, chat).Scan(&posted); err != nil || posted != "Finished: Compare queues" {
		t.Fatalf("%q %v", posted, err)
	}
	e.want("POST", "/workflow-runs/"+id+"/gate", map[string]any{"node_id": "review", "decision": "approve"}, http.StatusConflict)
}

func TestWorkflowRequestChangesRerunsAsANewAttemptUpToTheLimit(t *testing.T) {
	e := newChatEnv(t)
	wf := e.publishedWorkflow(t)
	chat := e.newChat(nil)["id"].(string)
	id := e.want("POST", "/chats/"+chat+"/workflows", map[string]any{"workflow_id": wf, "input": "X"}, http.StatusCreated)["id"].(string)
	e.finishWFStep(t, id, "brief", "b1")
	e.finishWFStep(t, id, "work", "w1")

	e.invalid("POST", "/workflow-runs/"+id+"/gate", map[string]any{"node_id": "review", "decision": "changes"}, "note", "should change")
	run := e.want("POST", "/workflow-runs/"+id+"/gate", map[string]any{"node_id": "review", "decision": "changes", "note": "Cite sources"}, 200)
	brief := e.wfStep(run, "brief")
	if run["state"] != "running" || brief["attempt"].(float64) != 2 || brief["state"] != "running" {
		t.Fatalf("%v", run)
	}
	var prompt string
	if err := e.pool.QueryRow(t.Context(), `SELECT input_message FROM run_turns WHERE run_id = $1`, brief["agent_run_id"]).Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Cite sources") {
		t.Fatal(prompt)
	}
	e.finishWFStep(t, id, "brief", "b2")
	run = e.finishWFStep(t, id, "work", "w2")
	if g := e.wfStep(run, "review"); g["attempt"].(float64) != 2 || g["state"] != "waiting" {
		t.Fatalf("%v", g)
	}
	e.want("POST", "/workflow-runs/"+id+"/gate", map[string]any{"node_id": "review", "decision": "changes", "note": "Again"}, http.StatusConflict)
	run = e.want("POST", "/workflow-runs/"+id+"/gate", map[string]any{"node_id": "review", "decision": "reject"}, 200)
	if run["state"] != "stopped" {
		t.Fatalf("%v", run)
	}
	var pages int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM artifacts WHERE path = 'Workflows/Result'`).Scan(&pages); err != nil || pages != 0 {
		t.Fatalf("rejected run wrote the wiki: %d %v", pages, err)
	}
}

func TestWorkflowStepThatSubmitsNothingFailsTheRun(t *testing.T) {
	e := newChatEnv(t)
	wf := e.publishedWorkflow(t)
	chat := e.newChat(nil)["id"].(string)
	run := e.want("POST", "/chats/"+chat+"/workflows", map[string]any{"workflow_id": wf, "input": "X"}, http.StatusCreated)
	e.setRunState(t, uuid.MustParse(e.wfStep(run, "brief")["agent_run_id"].(string)), "completed")
	e.advanceWorkflows(t)
	run = e.want("GET", "/workflow-runs/"+run["id"].(string), nil, 200)
	if run["state"] != "failed" || !strings.Contains(run["error"].(string), "without submitting") {
		t.Fatalf("%v", run)
	}
}

func TestWorkflowStartFromAReplyOnlyOnceAndOnlyIfProposed(t *testing.T) {
	e := newChatEnv(t)
	wf := e.publishedWorkflow(t)
	chat := e.newChat(nil)["id"].(string)
	add := func(body string) string {
		m, err := e.q.AppendChatMessage(t.Context(), store.AppendChatMessageParams{SessionID: uuid.MustParse(chat), Role: "assistant", BodyMd: body, ContextRefs: []byte("[]"), Citations: []byte("[]")})
		if err != nil {
			t.Fatal(err)
		}
		return m.ID.String()
	}
	plain := add("no card here")
	e.invalid("POST", "/chats/"+chat+"/workflows", map[string]any{"workflow_id": wf, "input": "X", "message_id": plain}, "message_id", "doesn't propose")
	msg := add("That fits.\n\n```workflow-run\n" + wf + "\nCompare queues\n```\n")
	e.want("POST", "/chats/"+chat+"/workflows", map[string]any{"workflow_id": wf, "input": "X", "message_id": msg}, http.StatusCreated)
	e.want("POST", "/chats/"+chat+"/workflows", map[string]any{"workflow_id": wf, "input": "X", "message_id": msg}, http.StatusConflict)
}

func TestWorkflowRunKeepsTheGraphItStartedWith(t *testing.T) {
	e := newChatEnv(t)
	wf := e.publishedWorkflow(t)
	chat := e.newChat(nil)["id"].(string)
	run := e.want("POST", "/chats/"+chat+"/workflows", map[string]any{"workflow_id": wf, "input": "X"}, http.StatusCreated)
	g := gatedWorkflowGraph()
	g["nodes"] = append(g["nodes"].([]map[string]any), map[string]any{"id": "extra", "type": "chat", "title": "Extra", "config": map[string]any{}})
	g["edges"] = append(g["edges"].([]map[string]any), map[string]any{"from": "review", "to": "extra"})
	e.want("PATCH", "/workflows/"+wf, map[string]any{"graph": g}, 200)
	updated := e.want("POST", "/workflows/"+wf+"/publish", nil, 200)
	if updated["published_version"].(float64) != 2 {
		t.Fatalf("%v", updated)
	}
	got := e.want("GET", "/workflow-runs/"+run["id"].(string), nil, 200)
	if n := len(got["graph"].(map[string]any)["nodes"].([]any)); n != 6 || got["version"].(float64) != 1 {
		t.Fatalf("nodes %d: %v", n, got["version"])
	}
}

func TestWorkflowProposalAppliesOnlyWhenThePersonSays(t *testing.T) {
	e := newChatEnv(t)
	wf := e.publishedWorkflow(t)
	id := uuid.MustParse(wf)
	ctx := t.Context()
	ops := []runs.WorkflowOp{
		{Op: "add_node", Node: &runs.WorkflowNode{ID: "check", Type: runs.NodeAgent, Title: "Check", Config: runs.WorkflowConfig{Prompt: "Check it"}}},
		{Op: "remove_edge", From: "work", To: "review"},
		{Op: "add_edge", From: "work", To: "check"},
		{Op: "add_edge", From: "check", To: "review"},
	}
	workspace := e.me.WorkspaceID
	if _, err := e.runs.ProposeWorkflowEdit(ctx, workspace, e.me.ID, nil, runs.WorkflowProposalInput{WorkflowID: &id, Summary: "Bad", Ops: []runs.WorkflowOp{{Op: "add_edge", From: "page", To: "start"}}}); err == nil || !strings.Contains(err.Error(), "invalid workflow") {
		t.Fatalf("got %v", err)
	}
	p, err := e.runs.ProposeWorkflowEdit(ctx, workspace, e.me.ID, nil, runs.WorkflowProposalInput{WorkflowID: &id, Summary: "Add a check", Ops: ops})
	if err != nil {
		t.Fatal(err)
	}
	got := e.want("GET", "/workflows/"+wf, nil, 200)
	if n := len(got["draft"].(map[string]any)["nodes"].([]any)); n != 6 || len(got["proposals"].([]any)) != 1 {
		t.Fatalf("a proposal changed the draft: %d", n)
	}
	got = e.want("POST", "/workflows/"+wf+"/proposals/"+p.ID.String()+"/apply", nil, 200)
	if n := len(got["draft"].(map[string]any)["nodes"].([]any)); n != 7 || got["unpublished_changes"] != true || len(got["proposals"].([]any)) != 0 {
		t.Fatalf("%v", got)
	}
	e.want("POST", "/workflows/"+wf+"/proposals/"+p.ID.String()+"/discard", nil, http.StatusConflict)
}

func TestWorkflowNoteListsOnlyPublished(t *testing.T) {
	e := newChatEnv(t)
	e.want("POST", "/workflows", map[string]any{"name": "Draft only"}, http.StatusCreated)
	wf := e.publishedWorkflow(t)
	chat, err := e.q.GetChatSession(t.Context(), store.GetChatSessionParams{ID: uuid.MustParse(e.newChat(nil)["id"].(string)), WorkspaceID: e.me.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	note, err := runs.WorkflowNote(t.Context(), e.q, chat)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note, wf) || !strings.Contains(note, "Research and file it") || strings.Contains(note, "Draft only") {
		t.Fatal(note)
	}
}
