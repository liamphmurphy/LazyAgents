package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// An agent in a coding chat starts the chat's playground with a spec; the
// person sees it and asks for a data reset, a stop, and a start again. Specs
// that could reach outside the workspace are refused, and a chat that can't
// run code can't start one.
func TestChatPlaygroundToolsAndAPI(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts}).Handler()
	do := func(method, path, token string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	chatAs := func(code bool) (string, string) {
		t.Helper()
		ps := &personas.Service{Pool: pool}
		name := "Talker"
		if code {
			name = "Builder"
		}
		p, err := ps.Create(ctx, personas.Owner{WorkspaceID: ws, UserID: me.ID}, personas.NewInput{Name: name, Code: &code})
		if err != nil {
			t.Fatal(err)
		}
		chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{PersonaID: &p.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = rs.PostChatMessage(ctx, ws, me.ID, chat.ID, runs.ChatMessageInput{Body: "let me try it"}); err != nil {
			t.Fatal(err)
		}
		chat, _ = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
		runID := *chat.CurrentRunID
		assistant, _ := q.GetAgent(ctx, store.GetAgentParams{ID: chat.AgentID, WorkspaceID: ws})
		token, _, err := agents.IssueToken(ctx, q, assistant.ID, "run", "chat", &runID, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		exec("UPDATE runs SET state='running' WHERE id=$1", runID)
		return chat.ID.String(), token
	}
	chatID, token := chatAs(true)
	tool := "/api/v1/agent/tools/"
	spec := map[string]any{
		"processes": []any{
			map[string]any{"name": "api", "command": "go run ./cmd/vesper serve", "dir": "server", "port": 8000},
			map[string]any{"name": "web", "command": "npm run dev -- -H 0.0.0.0", "dir": "web", "port": 3000},
		},
		"routes":   []any{map[string]any{"path": "/api", "process": "api"}},
		"services": []any{map[string]any{"name": "postgres", "image": "postgres:17-alpine"}},
		"setup":    []any{"cd server && go run ./cmd/vesper migrate"},
		"openapi":  "api/openapi.yaml",
	}

	for label, mutate := range map[string]func(map[string]any){
		"dir outside the workspace": func(s map[string]any) {
			s["processes"] = []any{map[string]any{"name": "x", "command": "y", "dir": "../..", "port": 1}}
		},
		"route to the gateway": func(s map[string]any) { s["routes"] = []any{map[string]any{"path": "/__vesper", "process": "api"}} },
	} {
		bad := map[string]any{}
		for k, v := range spec {
			bad[k] = v
		}
		mutate(bad)
		if code, out := do("POST", tool+"start_playground", token, bad); code != 422 {
			t.Errorf("%s: %d %v, want 422", label, code, out)
		}
	}
	if code, out := do("POST", tool+"playground_status", token, map[string]any{}); code != 409 || !strings.Contains(out["message"].(string), "start one with start_playground") {
		t.Fatalf("status before starting: %d %v", code, out)
	}

	code, out := do("POST", tool+"start_playground", token, spec)
	if code != 200 || out["state"] != "starting" || !strings.Contains(out["tell_the_person"].(string), "tagged playground") {
		t.Fatalf("start: %d %v", code, out)
	}
	_, view := do("GET", "/api/v1/chats/"+chatID+"/playground", "", nil)
	pg := view["playground"].(map[string]any)
	if pg["state"] != "starting" || pg["openapi"] != "api/openapi.yaml" || len(pg["processes"].([]any)) != 2 || len(pg["routes"].([]any)) != 2 || pg["port"] != nil {
		t.Fatalf("person's view %v", pg)
	}

	// Running (as a runner would mark it): the person resets its data, then stops it.
	exec("UPDATE playgrounds SET state='running', runner_id='r', stack='s', port=8090 WHERE chat_id=$1", chatID)
	if code, out := do("POST", "/api/v1/chats/"+chatID+"/playground/reset", "", nil); code != 200 || out["action"] != "reset" {
		t.Fatalf("reset: %d %v", code, out)
	}
	if code, out := do("POST", "/api/v1/chats/"+chatID+"/playground/stop", "", nil); code != 200 || out["action"] != "stop" {
		t.Fatalf("stop: %d %v", code, out)
	}
	exec("UPDATE playgrounds SET state='stopped', runner_id=NULL, stack=NULL, port=NULL, action=NULL WHERE chat_id=$1", chatID)
	if code, out := do("POST", "/api/v1/chats/"+chatID+"/playground/reset", "", nil); code != 409 {
		t.Fatalf("reset while stopped: %d %v", code, out)
	}
	if code, out := do("POST", "/api/v1/chats/"+chatID+"/playground/start", "", nil); code != 200 || out["state"] != "starting" {
		t.Fatalf("start again: %d %v", code, out)
	}

	// Once the chat's segment ended, its workspace is gone: nothing to run.
	exec("UPDATE playgrounds SET state='stopped' WHERE chat_id=$1", chatID)
	exec("UPDATE runs SET state='completed' WHERE id=(SELECT current_run_id FROM chat_sessions WHERE id=$1)", chatID)
	if code, out := do("POST", "/api/v1/chats/"+chatID+"/playground/start", "", nil); code != 409 || !strings.Contains(out["message"].(string), "cleaned up") {
		t.Fatalf("start without a workspace: %d %v", code, out)
	}

	plainChat, plainToken := chatAs(false)
	if code, out := do("POST", tool+"start_playground", plainToken, spec); code != 409 || !strings.Contains(out["message"].(string), "can run code") {
		t.Fatalf("non-coding chat: %d %v", code, out)
	}
	if code, out := do("GET", "/api/v1/chats/"+plainChat+"/playground", "", nil); code != http.StatusOK || out["playground"] != nil {
		t.Fatalf("no playground: %d %v", code, out)
	}
}

// A playground's spec is kept per repository, so a person can start one in
// another chat on the repository without an agent; a document the persona
// inferred lives in one workspace only, so it isn't kept. The persona can
// point the API tab at a document it inferred.
func TestPlaygroundSavedSpecAndInferredDocument(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts}).Handler()
	do := func(method, path, token string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	const repo = "https://github.com/acme/app"
	canCode := true
	p, err := (&personas.Service{Pool: pool}).Create(ctx, personas.Owner{WorkspaceID: ws, UserID: me.ID}, personas.NewInput{Name: "Builder", Code: &canCode, Repos: &[]string{repo}})
	if err != nil {
		t.Fatal(err)
	}
	// newChat opens a chat on the repository; with a message, it has a
	// running segment and an agent token for it.
	newChat := func(message bool) (string, string) {
		t.Helper()
		chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{PersonaID: &p.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE chat_sessions SET repo_url=$2 WHERE id=$1", chat.ID, repo); err != nil {
			t.Fatal(err)
		}
		if !message {
			return chat.ID.String(), ""
		}
		if _, err = rs.PostChatMessage(ctx, ws, me.ID, chat.ID, runs.ChatMessageInput{Body: "show me"}); err != nil {
			t.Fatal(err)
		}
		chat, _ = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
		assistant, _ := q.GetAgent(ctx, store.GetAgentParams{ID: chat.AgentID, WorkspaceID: ws})
		token, _, err := agents.IssueToken(ctx, q, assistant.ID, "run", "chat", chat.CurrentRunID, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE runs SET state='running' WHERE id=$1", *chat.CurrentRunID); err != nil {
			t.Fatal(err)
		}
		return chat.ID.String(), token
	}

	first, token := newChat(true)
	if code, out := do("POST", "/api/v1/agent/tools/start_playground", token, map[string]any{
		"processes": []any{map[string]any{"name": "runner", "command": "vesper runner"}, map[string]any{"name": "web", "command": "npm start", "port": 3000}},
	}); code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}
	if code, out := do("POST", "/api/v1/agent/tools/set_playground_openapi", token, map[string]any{"path": ".lazyagents/playground/openapi.json", "inferred": true, "commit": "9f17116"}); code != 200 {
		t.Fatalf("set document: %d %v", code, out)
	}
	if code, out := do("POST", "/api/v1/agent/tools/set_playground_openapi", token, map[string]any{"path": "../escape.json"}); code != 422 {
		t.Fatalf("a document outside the workspace: %d %v", code, out)
	}
	_, view := do("GET", "/api/v1/chats/"+first+"/playground", "", nil)
	pg := view["playground"].(map[string]any)
	if pg["openapi"] != ".lazyagents/playground/openapi.json" || pg["openapi_inferred"] != true || pg["openapi_commit"] != "9f17116" || view["saved_spec"] != true {
		t.Fatalf("first chat's view %v", view)
	}
	if procs := pg["processes"].([]any); procs[0].(map[string]any)["port"] != float64(0) {
		t.Fatalf("background process %v", procs[0])
	}

	// Another chat on the repository: nothing running, but it can start one.
	second, _ := newChat(true)
	if _, view := do("GET", "/api/v1/chats/"+second+"/playground", "", nil); view["playground"] != nil || view["saved_spec"] != true {
		t.Fatalf("second chat's view %v", view)
	}
	code, out := do("POST", "/api/v1/chats/"+second+"/playground/start", "", nil)
	if code != 200 || out["state"] != "starting" || out["openapi"] != "" || len(out["processes"].([]any)) != 2 {
		t.Fatalf("start from the saved spec: %d %v", code, out)
	}

	// A chat that never ran has no workspace to run it from.
	fresh, _ := newChat(false)
	if code, out := do("POST", "/api/v1/chats/"+fresh+"/playground/start", "", nil); code != 409 || !strings.Contains(out["message"].(string), "no workspace yet") {
		t.Fatalf("start without a workspace: %d %v", code, out)
	}
}
