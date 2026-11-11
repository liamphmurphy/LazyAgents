package httpapi

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/playground"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"net/http"
	"testing"
)

func TestChatRepositoryAccessIsNotPersonaAllowlist(t *testing.T) {
	e := newChatEnv(t)
	p := e.want("POST", "/personas", map[string]any{"name": "Repo coder", "capabilities": map[string]any{"code": true, "images": false}}, 201)
	chat := e.want("POST", "/chats", map[string]any{"persona_id": p["id"], "repo_url": "https://github.com/acme/unlisted"}, 201)
	id := chat["id"].(string)
	e.want("PUT", "/chats/"+id+"/repository", map[string]any{"repository": "acme/second"}, 200)
	got := e.want("GET", "/chats/"+id, nil, 200)
	if got["id"] != id || got["repo_url"] != "https://github.com/acme/second.git" {
		t.Fatalf("switch = %v", got)
	}
	e.want("PUT", "/chats/"+id+"/repository", map[string]any{"repository": "acme/unlisted"}, 200)
	got = e.want("GET", "/chats/"+id, nil, 200)
	if len(got["repositories"].([]any)) != 2 {
		t.Fatalf("attachments = %v", got)
	}
}

func TestRepositorySelectionQueuesAndChecksCapabilities(t *testing.T) {
	e := newChatEnv(t)
	p := e.want("POST", "/personas", map[string]any{"name": "Coder", "capabilities": map[string]any{"code": true, "images": false}}, 201)
	c := e.want("POST", "/chats", map[string]any{"persona_id": p["id"], "repo_url": "acme/first"}, 201)
	id := c["id"].(string)
	if _, err := e.pool.Exec(t.Context(), `UPDATE chat_sessions SET state='running' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	next := e.want("PUT", "/chats/"+id+"/repository", map[string]any{"repository": "acme/second"}, 200)
	if next["repo_url"] != "https://github.com/acme/first.git" || next["pending_repo_url"] != "https://github.com/acme/second.git" {
		t.Fatal(next)
	}
	if _, err := e.pool.Exec(t.Context(), `UPDATE chat_sessions SET state='idle' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	e.want("POST", "/chats/"+id+"/messages", map[string]any{"body": "next turn"}, 202)
	next = e.want("GET", "/chats/"+id, nil, 200)
	if next["repo_url"] != "https://github.com/acme/second.git" || next["pending_repo_url"] != nil {
		t.Fatal(next)
	}
	writer := e.want("POST", "/personas", map[string]any{"name": "Writer", "capabilities": map[string]any{"code": false, "images": false}}, 201)
	plain := e.want("POST", "/chats", map[string]any{"persona_id": writer["id"]}, 201)
	e.want("PUT", "/chats/"+plain["id"].(string)+"/repository", map[string]any{"repository": "acme/third"}, 409)
	e.want("PUT", "/chats/00000000-0000-0000-0000-000000000001/repository", map[string]any{"repository": "acme/third"}, 404)
}

func TestRepositorySourcesAndLegacyAliases(t *testing.T) {
	e := newChatEnv(t)
	p := e.want("POST", "/personas", map[string]any{"name": "Legacy coder", "capabilities": map[string]any{"code": true, "images": false}, "repos": []string{"git@github.com:Acme/App.git"}}, 201)
	c := e.want("POST", "/chats", map[string]any{"persona_id": p["id"]}, 201)
	id := c["id"].(string)
	got := e.want("PUT", "/chats/"+id+"/repository", map[string]any{"repository": "acme/app"}, 200)
	repos := got["repositories"].([]any)
	if len(repos) != 1 || repos[0].(map[string]any)["directory"] != "." {
		t.Fatal("legacy checkout duplicated", repos)
	}
	e.want("DELETE", "/repositories/sources", map[string]any{"kind": "repository", "name": "acme/app"}, 204)
	// Removing a shortcut leaves the active checkout usable.
	e.want("PUT", "/chats/"+id+"/repository", map[string]any{"repository": "acme/app"}, 200)
	e.want("PUT", "/repositories/sources", map[string]any{"kind": "organization", "name": "https://github.com/acme/"}, 204)
	e.want("PUT", "/repositories/sources", map[string]any{"kind": "organization", "name": "../bad"}, 422)
}

func TestRepositoryToolUsesFrozenAndCurrentCodingPermissions(t *testing.T) {
	e := newChatEnv(t)
	p := e.want("POST", "/personas", map[string]any{"name": "Tool coder", "capabilities": map[string]any{"code": true, "images": false}}, 201)
	c := e.want("POST", "/chats", map[string]any{"persona_id": p["id"], "message": map[string]any{"body": "work in another repository"}}, 201)
	id := uuid.MustParse(c["id"].(string))
	chat, err := e.q.GetChatSession(t.Context(), store.GetChatSessionParams{ID: id, WorkspaceID: e.me.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.q.SetRunState(t.Context(), store.SetRunStateParams{ID: *chat.CurrentRunID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	token, _, err := agents.IssueToken(t.Context(), e.q, chat.AgentID, "run", "test", chat.CurrentRunID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	call := func(want int) map[string]any {
		t.Helper()
		req, err := http.NewRequest("POST", e.srv.URL+"/api/v1/agent/tools/use_repository", bytes.NewBufferString(`{"repository":"acme/private"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := e.srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		out := map[string]any{}
		if err = json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("tool status %d want %d: %v", res.StatusCode, want, out)
		}
		return out
	}
	out := call(200)
	if out["directory"] != ".lazyagents/repos/acme/private" || out["setup_command"] == nil {
		t.Fatal(out)
	}
	// A stale run token cannot widen a capability the person has revoked.
	if _, err = e.pool.Exec(t.Context(), `UPDATE personas SET can_code=false WHERE id=$1`, p["id"]); err != nil {
		t.Fatal(err)
	}
	call(409)
}

func TestRepositoryPlaygroundUsesSelectedCheckoutAndEnvironment(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	p := e.want("POST", "/personas", map[string]any{"name": "Playground coder", "capabilities": map[string]any{"code": true, "images": false}}, 201)
	personaID := uuid.MustParse(p["id"].(string))
	chat, err := e.runs.StartChat(ctx, e.me.WorkspaceID, e.me.ID, runs.ChatInput{PersonaID: &personaID})
	if err != nil {
		t.Fatal(err)
	}
	for _, repo := range []string{"acme/alpha", "acme/beta", "acme/alpha"} {
		chat, err = e.runs.SetChatRepository(ctx, e.me.WorkspaceID, e.me.ID, chat.ID, repo, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = e.q.PutTestEnvironment(ctx, store.PutTestEnvironmentParams{WorkspaceID: e.me.WorkspaceID, Repo: "github.com/acme/beta", Recipe: json.RawMessage(`{"services":[{"name":"beta-db","image":"postgres:17-alpine"}],"nix_packages":[]}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err = e.runs.PostChatMessage(ctx, e.me.WorkspaceID, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "preview beta"}); err != nil {
		t.Fatal(err)
	}
	chat, err = e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: e.me.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.q.SetRunState(ctx, store.SetRunStateParams{ID: *chat.CurrentRunID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	pg, err := e.runs.StartChatPlayground(ctx, e.me.WorkspaceID, *chat.CurrentRunID, playground.Spec{Repository: "acme/beta", Setup: []string{"npm ci"}, OpenAPI: "api/openapi.yaml", Processes: []playground.Process{{Name: "web", Command: "npm run dev", Dir: "web", Port: 3000}}})
	if err != nil {
		t.Fatal(err)
	}
	var spec playground.Spec
	if err = json.Unmarshal(pg.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Directory != ".lazyagents/repos/acme/beta" || spec.Processes[0].Dir != "web" || spec.OpenAPI != "api/openapi.yaml" || len(spec.Services) != 1 || spec.Services[0].Name != "beta-db" {
		t.Fatalf("wrong checkout or environment: %+v", spec)
	}
	// A saved spec binds to another chat's checkout path without rewriting commands.
	other, err := e.runs.StartChat(ctx, e.me.WorkspaceID, e.me.ID, runs.ChatInput{PersonaID: &personaID, RepoURL: "acme/beta"})
	if err != nil {
		t.Fatal(err)
	}
	saved, found, err := e.runs.SavedPlaygroundSpec(ctx, other)
	if err != nil || !found || saved.Directory != "." || saved.Processes[0].Dir != "web" {
		t.Fatalf("saved spec: %+v found=%v err=%v", saved, found, err)
	}
}
