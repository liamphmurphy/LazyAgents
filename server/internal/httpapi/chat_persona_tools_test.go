package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// personaChat is a running chat on the demo workspace, answered by a persona,
// whose tools a test calls the way the harness does.
type personaChat struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	q       *store.Queries
	rs      *runs.Service
	ws      uuid.UUID
	user    uuid.UUID
	persona uuid.UUID
	chat    store.ChatSession
	token   string
	handler http.Handler
}

func newPersonaChat(t *testing.T, project string) *personaChat {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}
	p, err := q.CreatePersona(ctx, store.CreatePersonaParams{WorkspaceID: me.WorkspaceID, UserID: me.ID, Name: "Hackerman", Tone: "neutral", Color: "#9b8cff", Repos: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := rs.StartChat(ctx, me.WorkspaceID, me.ID, runs.ChatInput{Project: project, Title: "Tools", PersonaID: &p.ID, Message: &runs.ChatMessageInput{Body: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if chat, err = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: me.WorkspaceID}); err != nil || chat.CurrentRunID == nil {
		t.Fatalf("chat run: %v", err)
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: *chat.CurrentRunID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	token, _, err := agents.IssueToken(ctx, q, chat.AgentID, "run", "chat", chat.CurrentRunID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts}).Handler()
	return &personaChat{ctx: ctx, pool: pool, q: q, rs: rs, ws: me.WorkspaceID, user: me.ID, persona: p.ID, chat: chat, token: token, handler: handler}
}

// call runs a tool and decodes its result.
func (c *personaChat) call(t *testing.T, name string, args any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(args)
	req := httptest.NewRequest("POST", "/api/v1/agent/tools/"+name, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+c.token)
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// list is one array of objects from a tool result.
func list(out map[string]any, field string) []map[string]any {
	items, _ := out[field].([]any)
	rows := make([]map[string]any, len(items))
	for i, it := range items {
		rows[i], _ = it.(map[string]any)
	}
	return rows
}

func TestChatListsTicketsAndProjects(t *testing.T) {
	c := newPersonaChat(t, "PAY")

	code, out := c.call(t, "list_tickets", map[string]any{})
	open := list(out, "tickets")
	if code != 200 || len(open) == 0 || out["total"] != float64(len(open)) || out["has_more"] != false {
		t.Fatalf("list_tickets = %d %v", code, out)
	}
	for _, tk := range open {
		if s := tk["status"]; s == "done" || s == "canceled" {
			t.Fatalf("default list has closed ticket %v", tk)
		}
	}

	code, out = c.call(t, "list_tickets", map[string]any{"status": []string{"in_review"}, "project": "pay"})
	for _, tk := range list(out, "tickets") {
		if tk["status"] != "in_review" || tk["project"] != "PAY" {
			t.Fatalf("status filter let through %v", tk)
		}
	}
	if code != 200 || len(list(out, "tickets")) == 0 {
		t.Fatalf("in_review = %d %v", code, out)
	}

	var agent string
	for _, tk := range open {
		if a, _ := tk["agent"].(string); a != "" {
			agent = a
			break
		}
	}
	code, out = c.call(t, "list_tickets", map[string]any{"agent": "@" + agent})
	if code != 200 || len(list(out, "tickets")) == 0 {
		t.Fatalf("agent %s = %d %v", agent, code, out)
	}
	for _, tk := range list(out, "tickets") {
		if tk["agent"] != agent {
			t.Fatalf("agent filter let through %v", tk)
		}
	}

	code, out = c.call(t, "list_tickets", map[string]any{"limit": 2, "offset": 1})
	page := list(out, "tickets")
	if code != 200 || len(page) != 2 || out["has_more"] != true || out["total"] != float64(len(open)) || page[0]["key"] != open[1]["key"] {
		t.Fatalf("page = %d %v", code, out)
	}

	if code, out = c.call(t, "list_tickets", map[string]any{"q": "PAY-21"}); code != 200 || len(list(out, "tickets")) == 0 {
		t.Fatalf("q = %d %v", code, out)
	}
	for _, bad := range []map[string]any{{"status": []string{"shipped"}}, {"project": "NOPE"}, {"agent": "nobody"}, {"limit": 500}} {
		if code, out := c.call(t, "list_tickets", bad); code != 422 {
			t.Errorf("list_tickets %v = %d %v, want 422", bad, code, out)
		}
	}

	// Across every project the board query stops at 500; the total says so.
	if _, err := c.pool.Exec(c.ctx, `INSERT INTO tickets (workspace_id, project_id, number, key, title)
		SELECT p.workspace_id, p.id, 1000 + n, 'BIL-' || (1000 + n), 'Bulk ' || n FROM projects p, generate_series(1, 500) n WHERE p.workspace_id = $1 AND p.key = 'BIL'`, c.ws); err != nil {
		t.Fatal(err)
	}
	if code, out := c.call(t, "list_tickets", map[string]any{}); code != 200 || out["total"] != "at least 500" || out["note"] == nil {
		t.Fatalf("capped total = %d %v", code, out["total"])
	}
	if code, out := c.call(t, "list_tickets", map[string]any{"project": "BIL", "limit": 1}); code != 200 || out["total"] != float64(500) {
		t.Fatalf("one project = %d %v", code, out["total"])
	}

	code, out = c.call(t, "list_projects", map[string]any{})
	projects := list(out, "projects")
	var keys []string
	for _, p := range projects {
		keys = append(keys, fmt.Sprint(p["key"]))
		if (p["key"] == "PAY") != (p["default"] == true) {
			t.Errorf("default flag on %v", p)
		}
	}
	if code != 200 || !strings.Contains(strings.Join(keys, ","), "PAY") || len(projects) < 2 {
		t.Fatalf("list_projects = %d %v", code, out)
	}
}
