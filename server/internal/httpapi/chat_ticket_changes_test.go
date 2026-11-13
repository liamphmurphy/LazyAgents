package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentapi"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// allowTicketEdits lets the chat's persona edit tickets without asking, for
// tests of the edits themselves rather than of their approval.
func allowTicketEdits(t *testing.T, q *store.Queries, chat store.ChatSession) {
	t.Helper()
	speaker := runs.ChatSpeakerID(chat)
	if speaker == nil {
		t.Fatal("chat has no persona")
	}
	if _, err := q.SetPersonaTicketEdits(context.Background(), store.SetPersonaTicketEditsParams{ID: *speaker, WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy, TicketEdits: agentapi.TicketEditsAllow}); err != nil {
		t.Fatal(err)
	}
}

// A persona that hasn't been let edit tickets without asking proposes each
// edit: the chat lists it with a before/after preview, and nothing changes
// until the person applies it. Applying runs it as the persona with the
// person's approval on the timeline; a ticket that changed since is refused,
// not overwritten; "always" lets the persona edit without asking from then on.
func TestChatTicketEditsWaitForThePerson(t *testing.T) {
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

	chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{Project: "PAY", Title: "Edits", Message: &runs.ChatMessageInput{Body: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	chat, _ = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
	persona := runs.ChatSpeakerID(chat)
	if persona == nil {
		t.Fatal("chat has no persona")
	}
	runID := *chat.CurrentRunID
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: runID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	token, _, err := agents.IssueToken(ctx, q, chat.AgentID, "run", "chat", &runID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path, auth string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	call := func(tool string, args map[string]any) (int, map[string]any) {
		t.Helper()
		return do("POST", "/api/v1/agent/tools/"+tool, token, args)
	}
	pending := func() []any {
		t.Helper()
		code, out := do("GET", "/api/v1/chats/"+chat.ID.String()+"/ticket-changes", "", nil)
		if code != 200 {
			t.Fatalf("list ticket changes = %d %v", code, out)
		}
		return out["requests"].([]any)
	}
	decide := func(id any, decision string) (int, map[string]any) {
		t.Helper()
		return do("POST", fmt.Sprintf("/api/v1/ticket-changes/%v", id), "", map[string]any{"decision": decision})
	}
	get := func(key string) store.Ticket {
		t.Helper()
		tk, err := tracker.Get(ctx, q, ws, key)
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}

	project, err := q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: ws, Key: "PAY"})
	if err != nil {
		t.Fatal(err)
	}
	epic, err := tr.CreateEpic(ctx, store.CreateEpicParams{WorkspaceID: ws, ProjectID: project.ID, Name: "Refunds", Health: "on_track"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateLabel(ctx, store.CreateLabelParams{WorkspaceID: ws, Name: "backend", Color: "#336699"}); err != nil {
		t.Fatal(err)
	}
	tk, err := tr.Create(ctx, ws, tracker.UserActor(me.ID), tracker.NewTicket{ProjectKey: "PAY", Title: "Proposed edits", EpicID: &epic.ID, Criteria: []string{"Refund posts", "Email sent"}})
	if err != nil {
		t.Fatal(err)
	}

	// Proposed: the ticket is untouched and the chat has a card with the diff.
	code, out := call("update_ticket", map[string]any{"ticket": tk.Key, "revision": tk.Revision, "status": "in_progress",
		"remove_from_epic": true, "labels": []string{"Backend"}, "due_date": "2026-12-01", "tick_criteria": []map[string]any{{"n": 2, "done": true}}})
	if code != 200 || out["state"] != "proposed" {
		t.Fatalf("update_ticket = %d %v", code, out)
	}
	if after := get(tk.Key); after.Status != tk.Status || after.EpicID == nil || after.Revision != tk.Revision {
		t.Fatalf("a proposal changed the ticket: %+v", after)
	}
	cards := pending()
	if len(cards) != 1 {
		t.Fatalf("pending = %v", cards)
	}
	card := cards[0].(map[string]any)
	fields := map[string]map[string]any{}
	for _, c := range card["changes"].([]any) {
		ch := c.(map[string]any)
		fields[ch["field"].(string)] = ch
	}
	if card["ticket_key"] != tk.Key || card["persona"] == nil ||
		fields["status"]["before"] != "backlog" || fields["status"]["after"] != "in_progress" ||
		fields["epic"]["before"] != "Refunds" || fields["epic"]["after"] != "none" ||
		fields["labels"]["after"] != "backend" || fields["due_date"]["after"] != "2026-12-01" ||
		fields["criteria"]["after"] != "- [ ] Refund posts\n- [x] Email sent" {
		t.Fatalf("card = %v", card)
	}

	// Applied as the persona, with the person's approval on the timeline.
	if code, out := decide(card["id"], "allow"); code != 200 || out["state"] != "applied" {
		t.Fatalf("allow = %d %v", code, out)
	}
	after := get(tk.Key)
	if after.Status != "in_progress" || after.EpicID != nil || after.DueDate == nil || after.DueDate.Format("2006-01-02") != "2026-12-01" {
		t.Fatalf("applied ticket = %+v", after)
	}
	crit, _ := q.ListCriteria(ctx, tk.ID)
	labels, _ := q.ListLabelsForTickets(ctx, []uuid.UUID{tk.ID})
	if len(crit) != 2 || crit[0].Done || !crit[1].Done || len(labels) != 1 || labels[0].Name != "backend" {
		t.Fatalf("criteria %+v labels %+v", crit, labels)
	}
	events, _ := q.ListTicketActivity(ctx, &tk.ID)
	var approved bool
	for _, e := range events {
		if e.Verb != tracker.VerbStatusChanged {
			continue
		}
		var p map[string]any
		_ = json.Unmarshal(e.Payload, &p)
		via, _ := p["via"].(map[string]any)
		approved = e.ActorType == "agent" && via["approved_by"] == me.ID.String() && via["chat_id"] == chat.ID.String()
	}
	if !approved {
		t.Fatal("status event doesn't record the persona and the person's approval")
	}
	if code, out := decide(card["id"], "allow"); code != 409 {
		t.Fatalf("deciding twice = %d %v", code, out)
	}

	// Declined: nothing is written.
	if code, out := call("comment_ticket", map[string]any{"ticket": tk.Key, "body": "Found the cause."}); code != 200 || out["state"] != "proposed" {
		t.Fatalf("comment_ticket = %d %v", code, out)
	}
	if code, out := decide(pending()[0].(map[string]any)["id"], "deny"); code != 200 || out["state"] != "declined" {
		t.Fatalf("deny = %d %v", code, out)
	}
	if comments, _ := q.ListTicketComments(ctx, tk.ID); len(comments) != 0 {
		t.Fatalf("declined comment was posted: %v", comments)
	}

	// The ticket moved on after the proposal: applying it is refused, not a clobber.
	cur := get(tk.Key)
	if code, out := call("update_ticket", map[string]any{"ticket": tk.Key, "revision": cur.Revision, "title": "Persona's title"}); code != 200 || out["state"] != "proposed" {
		t.Fatalf("title proposal = %d %v", code, out)
	}
	title := "Person's title"
	if _, err := tr.Update(ctx, ws, tracker.UserActor(me.ID), tk.Key, tracker.Patch{Title: &title}); err != nil {
		t.Fatal(err)
	}
	stale := pending()[0].(map[string]any)
	if code, out := decide(stale["id"], "allow"); code != 409 || !strings.Contains(fmt.Sprint(out), "stale") {
		t.Fatalf("stale apply = %d %v", code, out)
	}
	if get(tk.Key).Title != title || len(pending()) != 0 {
		t.Fatal("stale proposal overwrote the ticket or kept waiting")
	}
	if r, _ := q.GetTicketChangeRequest(ctx, store.GetTicketChangeRequestParams{ID: uuid.MustParse(stale["id"].(string)), WorkspaceID: ws, UserID: me.ID}); r.TicketChangeRequest.State != "failed" || r.TicketChangeRequest.Error == nil {
		t.Fatalf("stale request = %+v", r.TicketChangeRequest)
	}

	// A cancel the person approves goes through, though only people cancel.
	cur = get(tk.Key)
	if code, out := call("update_ticket", map[string]any{"ticket": tk.Key, "revision": cur.Revision, "status": "canceled"}); code != 200 || out["state"] != "proposed" {
		t.Fatalf("cancel proposal = %d %v", code, out)
	}
	// "Always" applies it and lets the persona edit without asking from now on.
	if code, out := decide(pending()[0].(map[string]any)["id"], "always"); code != 200 || out["state"] != "applied" {
		t.Fatalf("always = %d %v", code, out)
	}
	if get(tk.Key).Status != "canceled" {
		t.Fatal("approved cancel didn't apply")
	}
	if p, _ := q.GetPersona(ctx, store.GetPersonaParams{ID: *persona, WorkspaceID: ws, UserID: me.ID}); p.TicketEdits != agentapi.TicketEditsAllow {
		t.Fatalf("persona ticket_edits = %q after always", p.TicketEdits)
	}
	if code, out := call("comment_ticket", map[string]any{"ticket": tk.Key, "body": "Canceled: duplicate of the refunds epic."}); code != 200 || out["event_id"] == nil {
		t.Fatalf("comment after always = %d %v", code, out)
	}
	// Reopening a canceled ticket still waits for the person.
	cur = get(tk.Key)
	if code, out := call("update_ticket", map[string]any{"ticket": tk.Key, "revision": cur.Revision, "status": "todo"}); code != 200 || out["state"] != "proposed" {
		t.Fatalf("reopen = %d %v", code, out)
	}
}
