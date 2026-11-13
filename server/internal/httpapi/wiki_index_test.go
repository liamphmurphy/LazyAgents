package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

// The wiki indexes: pattern tag filter and facet, ADR status and open
// questions, and which runs and chats read a page.
func TestWikiIndexes(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	w := &wiki.Service{Pool: pool, Artifacts: arts, Tracker: &tracker.Service{Pool: pool}}
	save := func(path, body string) wiki.Result {
		t.Helper()
		res, err := w.SaveAsUser(ctx, ws, me.ID, wiki.Save{Path: path, Body: body, Audience: "shared"})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	save("Patterns/Architecture/Branch on purpose", "Topic: Architecture\nTags: runner, purpose, mcp\n\n## When to use\n\nNew run kinds.\n")
	save("Patterns/API/Allowlist MCP tools", "Topic: API\nTags: mcp, security\n\n## When to use\n\nShared MCP servers.\n")
	save("Patterns/Go/Wrap errors", "Topic: Go\nTags: errors, context\n\n## When to use\n\nBoundaries.\n")
	page := save("Architecture/ADR - Chat sessions", "# ADR: Chat sessions\n\nStatus: Accepted, with open questions (2026-11-02). Ticket: [[PAY-214]].\n\n## Decision\n\n- Use runs.\n\n## Open questions (on [[PAY-214]])\n\n1. Which agent identity\n   chats use.\n2. Memory writes.\n\n## After\n\n- not a question\n")
	save("Architecture/ADR - Push branches", "# ADR\n\nStatus: Accepted (2026-11-01).\n")
	save("Architecture/Glossary", "Status: Accepted\n")

	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q}).Handler()
	get := func(url string, code int, out any) {
		t.Helper()
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", url, nil))
		if r.Code != code {
			t.Fatalf("%s: %d %s", url, r.Code, r.Body.String())
		}
		if out != nil {
			if err := json.Unmarshal(r.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Patterns: AND tag filter; the facet ignores the tag filter.
	var pats api.FindPatterns200JSONResponse
	get("/api/v1/wiki/patterns?tag=mcp&limit=50", 200, &pats)
	if len(pats.Patterns) != 2 {
		t.Fatalf("tag=mcp: %+v", pats.Patterns)
	}
	if len(pats.Tags) == 0 || pats.Tags[0] != (api.TagCount{Tag: "mcp", Count: 2}) {
		t.Fatalf("facet: %+v", pats.Tags)
	}
	get("/api/v1/wiki/patterns?tag=mcp&tag=%23Runner&limit=50", 200, &pats)
	if len(pats.Patterns) != 1 || pats.Patterns[0].Title != "Branch on purpose" {
		t.Fatalf("tag=mcp&tag=runner: %+v", pats.Patterns)
	}
	get("/api/v1/wiki/patterns?topic=Go&tag=mcp", 200, &pats)
	if len(pats.Patterns) != 0 || len(pats.Tags) != 2 {
		t.Fatalf("topic=Go&tag=mcp: %+v %+v", pats.Patterns, pats.Tags)
	}

	// ADRs: open questions first, status without "with open questions".
	var adrs api.ListWikiADRs200JSONResponse
	get("/api/v1/wiki/adrs", 200, &adrs)
	byTitle := map[string]api.WikiADR{}
	for _, a := range adrs.Adrs {
		byTitle[a.Title] = a
	}
	if _, ok := byTitle["Glossary"]; ok {
		t.Fatal("Glossary isn't an ADR")
	}
	chat, push := byTitle["ADR - Chat sessions"], byTitle["ADR - Push branches"]
	if chat.Status != "Accepted" || chat.State != api.WikiADRStateAccepted || deref(chat.Date) != "2026-11-02" || deref(chat.Ticket) != "PAY-214" ||
		len(chat.OpenQuestions) != 2 || chat.OpenQuestions[0] != "Which agent identity chats use." || chat.Href != "/wiki/Architecture/ADR%20-%20Chat%20sessions" {
		t.Fatalf("chat ADR: %+v", chat)
	}
	if !chat.HasOpenQuestions || push.HasOpenQuestions {
		t.Fatalf("has_open_questions: %v %v", chat.HasOpenQuestions, push.HasOpenQuestions)
	}
	if push.Status != "Accepted" || len(push.OpenQuestions) != 0 || deref(push.Date) != "2026-11-01" {
		t.Fatalf("push ADR: %+v", push)
	}
	if adrs.Adrs[0].Title != "ADR - Chat sessions" {
		t.Fatalf("open questions sort first: %+v", adrs.Adrs[0])
	}

	// Citations: reads grouped by run, chats resolved, run-less reads apart.
	atlas, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := tracker.Get(ctx, q, ws, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	ticketRun, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: ws, TicketID: &ticket.ID, AgentID: atlas.ID, Mode: "managed", State: "completed", ResolvedSpec: json.RawMessage(`{}`), Harness: "codex", Model: "fake", Purpose: "ticket"})
	if err != nil {
		t.Fatal(err)
	}
	chatRun, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: ws, AgentID: atlas.ID, Mode: "managed", State: "waiting", ResolvedSpec: json.RawMessage(`{}`), Harness: "codex", Model: "fake", Purpose: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := q.CreateChatSession(ctx, store.CreateChatSessionParams{WorkspaceID: ws, ProjectID: &ticket.ProjectID, Title: "Why read-only?", CreatedBy: me.ID, AgentID: atlas.ID, Harness: "codex", Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: sess.ID, CurrentRunID: &chatRun.ID, State: "idle"}); err != nil {
		t.Fatal(err)
	}
	// Another person's private chat read the page too: it's counted, never named.
	other, err := q.CreateUser(ctx, store.CreateUserParams{WorkspaceID: ws, Name: "Sam", Email: "sam@example.com", Initials: "S"})
	if err != nil {
		t.Fatal(err)
	}
	otherRun, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: ws, AgentID: atlas.ID, Mode: "managed", State: "waiting", ResolvedSpec: json.RawMessage(`{}`), Harness: "codex", Model: "fake", Purpose: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	otherSess, err := q.CreateChatSession(ctx, store.CreateChatSessionParams{WorkspaceID: ws, ProjectID: &ticket.ProjectID, Title: "Sam's secret plans", CreatedBy: other.ID, AgentID: atlas.ID, Harness: "codex", Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: otherSess.ID, CurrentRunID: &otherRun.ID, State: "idle"}); err != nil {
		t.Fatal(err)
	}
	a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: page.Note.ID, WorkspaceID: ws, Viewer: pageview.From(ctx).UserID})
	if err != nil {
		t.Fatal(err)
	}
	for _, rid := range []*uuid.UUID{&ticketRun.ID, &ticketRun.ID, &chatRun.ID, nil, &otherRun.ID} {
		if err := artifacts.RecordRead(ctx, q, a, atlas.ID, rid, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	var cites api.GetWikiCitations200JSONResponse
	get("/api/v1/wiki/pages/"+a.ID.String()+"/citations", 200, &cites)
	if cites.Runs != 1 || cites.Chats != 2 || len(cites.Citations) != 3 || len(cites.Unlinked) != 1 || cites.Unlinked[0].Reads != 1 {
		t.Fatalf("citations: %+v", cites)
	}
	if body, _ := json.Marshal(cites); strings.Contains(string(body), "secret") || strings.Contains(string(body), otherSess.ID.String()) {
		t.Fatalf("another person's chat leaked: %s", body)
	}
	for _, c := range cites.Citations {
		switch c.Purpose {
		case api.WikiCitationPurposeTicket:
			if deref(c.TicketKey) != "PAY-214" || c.Reads != 2 || c.ChatId != nil {
				t.Fatalf("ticket citation: %+v", c)
			}
		case api.WikiCitationPurposeChat:
			if c.RunId == otherRun.ID {
				if c.ChatId != nil || c.ChatTitle != nil {
					t.Fatalf("other chat citation: %+v", c)
				}
				continue
			}
			if c.ChatId == nil || *c.ChatId != sess.ID || deref(c.ChatTitle) != "Why read-only?" || c.Reads != 1 {
				t.Fatalf("chat citation: %+v", c)
			}
		}
	}
	// Generic run routes must not bypass private chat ownership.
	get("/api/v1/runs/"+otherRun.ID.String(), 404, nil)
	get("/api/v1/runs/"+otherRun.ID.String()+"/turns/1/log", 404, nil)
	get("/api/v1/runs/"+chatRun.ID.String(), 200, nil)
	// Ownership must also work for past chat segments, after current_run_id changes.
	if _, err := pool.Exec(ctx, `INSERT INTO chat_messages (session_id, seq, role, body_md, run_id) VALUES ($1, 1, 'assistant', 'answer', $2), ($3, 1, 'assistant', 'private answer', $4)`, sess.ID, chatRun.ID, otherSess.ID, otherRun.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE chat_sessions SET current_run_id = NULL WHERE id = ANY($1::uuid[])`, []uuid.UUID{sess.ID, otherSess.ID}); err != nil {
		t.Fatal(err)
	}
	get("/api/v1/runs/"+chatRun.ID.String(), 200, nil)
	get("/api/v1/runs/"+otherRun.ID.String(), 404, nil)
	control := httptest.NewRecorder()
	handler.ServeHTTP(control, httptest.NewRequest("POST", "/api/v1/runs/"+otherRun.ID.String()+"/cancel", nil))
	if control.Code != 404 {
		t.Fatalf("private run control: %d %s", control.Code, control.Body.String())
	}

	otherWS, err := q.CreateWorkspace(ctx, store.CreateWorkspaceParams{Name: "Other", Slug: "other"})
	if err != nil {
		t.Fatal(err)
	}
	foreignPath := "Private"
	foreignPage, err := q.CreateArtifact(ctx, store.CreateArtifactParams{Audience: "shared", WorkspaceID: otherWS.ID, Type: "note", Title: "Private workspace page", Path: &foreignPath, AuthorType: "user", AuthorID: me.ID})
	if err != nil {
		t.Fatal(err)
	}
	get("/api/v1/wiki/pages/"+foreignPage.ID.String()+"/citations", 404, nil)
}
