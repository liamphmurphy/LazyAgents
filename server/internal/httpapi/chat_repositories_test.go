package httpapi

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/agents"
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
