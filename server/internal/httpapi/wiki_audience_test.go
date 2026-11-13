package httpapi

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"strings"
	"testing"
)

// Exercise the actual REST/MCP tool boundary: identity must come from chat
// ownership, and caller-supplied audience/owner fields never grant sharing.
func TestWikiAudienceToolBoundary(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	ws := e.me.WorkspaceID
	private := e.want("POST", "/wiki/pages", map[string]any{"path": "Old private alias", "body_md": "Private payload 75b4"}, 201)
	id := private["id"].(string)
	if private["audience"] != "private" {
		t.Fatalf("new page default: %v", private)
	}
	e.want("PUT", "/wiki/pages/"+id, map[string]any{"path": "Vault/Secret title 4218"}, 200)
	shared := e.want("POST", "/wiki/pages", map[string]any{"path": "Shared entry", "body_md": "See [[Old private alias]].", "audience": "shared"}, 201)
	atlas, err := e.q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	tk, err := e.q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: ws, Key: e.payKey})
	if err != nil {
		t.Fatal(err)
	}
	run, err := e.q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: ws, TicketID: &tk.ID, AgentID: atlas.ID, Mode: "managed", State: "running", Purpose: "ticket", Harness: "claude", Model: "test", ResolvedSpec: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := agents.IssueToken(ctx, e.q, atlas.ID, "run", "audience", &run.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	code, out := e.tool(token, "read_page", map[string]any{"ref": "Shared entry"})
	raw, _ := json.Marshal(out)
	if code != 200 || !strings.Contains(string(raw), "Old private alias") || strings.Contains(string(raw), "Secret title") || strings.Contains(string(raw), "Private payload") || strings.Contains(string(raw), id) {
		t.Fatalf("ghost link response: %d %s", code, raw)
	}
	for _, ref := range []string{id, "Old private alias", "Vault/Secret title 4218"} {
		if code, out := e.tool(token, "read_page", map[string]any{"ref": ref}); code == 200 {
			t.Fatalf("run read private page: %v", out)
		}
	}
	// An agent cannot turn a shared page private via an extra tool argument.
	if code, out := e.tool(token, "write_note", map[string]any{"path": "Shared entry", "body": "Agent version", "audience": "private", "owner_user_id": e.me.ID.String()}); code != 200 {
		t.Fatalf("agent write: %d %v", code, out)
	}
	if got := e.want("GET", "/wiki/page?path=Shared%20entry", nil, 200); got["audience"] != "shared" {
		t.Fatalf("agent changed audience: %v", got)
	}
	chat := e.newChat(nil)
	chatID := uuid.MustParse(chat["id"].(string))
	persona := e.canvasToken(chatID)
	if code, out := e.tool(persona, "read_page", map[string]any{"ref": id}); code != 200 {
		t.Fatalf("owner's persona cannot read: %d %v", code, out)
	}
	if code, out := e.tool(persona, "write_note", map[string]any{"path": "Vault/Secret title 4218", "body": "Persona private edit", "audience": "shared"}); code != 200 {
		t.Fatalf("persona edit: %d %v", code, out)
	}
	if got := e.want("GET", "/wiki/page?path=Vault/Secret%20title%204218", nil, 200); got["audience"] != "private" || got["body_md"] != "Persona private edit" {
		t.Fatalf("persona changed audience or didn't edit directly: %v", got)
	}
	// Sharing is an explicit person action on the REST API.
	e.want("PUT", "/wiki/pages/"+id, map[string]any{"audience": "shared"}, 200)
	if code, out := e.tool(token, "read_page", map[string]any{"ref": id}); code != 200 {
		t.Fatalf("shared page unavailable: %d %v", code, out)
	}
	// Agent knowledge discovery is opt-in for the person's API requests.
	if code, out := e.tool(token, "write_note", map[string]any{"path": "agents/atlas/Discoveryneedle", "body": "Discoveryneedle [[Shared entry]]"}); code != 200 {
		t.Fatalf("agent memory: %d %v", code, out)
	}
	for _, route := range []string{"/knowledge/search?q=Discoveryneedle", "/knowledge/suggest?q=Discoveryneedle", "/knowledge/graph?kind=artifact&id=" + shared["id"].(string)} {
		hidden := e.want("GET", route, nil, 200)
		included := e.want("GET", route+"&include_agent=true", nil, 200)
		a, _ := json.Marshal(hidden)
		b, _ := json.Marshal(included)
		if strings.Contains(string(a), "Discoveryneedle") || !strings.Contains(string(b), "Discoveryneedle") {
			t.Fatalf("discovery %s: hidden=%s included=%s", route, a, b)
		}
	}
}

func TestWikiAudienceSavesUsePageID(t *testing.T) {
	e := newChatEnv(t)
	shared := e.want("POST", "/wiki/pages", map[string]any{"path": "Overlapping path", "body_md": "Shared body", "audience": "shared"}, 201)
	private := e.want("POST", "/wiki/pages", map[string]any{"path": "Overlapping path", "body_md": "Private body"}, 201)
	sid := shared["id"].(string)
	pid := private["id"].(string)
	e.want("PUT", "/wiki/pages/"+sid, map[string]any{"body_md": "Updated shared body"}, 200)
	if got := e.want("GET", "/artifacts/"+sid, nil, 200); got["body_md"] != "Updated shared body" || got["audience"] != "shared" {
		t.Fatalf("wrong shared save: %v", got)
	}
	if got := e.want("GET", "/artifacts/"+pid, nil, 200); got["body_md"] != "Private body" || got["audience"] != "private" {
		t.Fatalf("private page was overwritten: %v", got)
	}
	e.want("PUT", "/wiki/pages/"+pid, map[string]any{"audience": "shared"}, 409)
}
