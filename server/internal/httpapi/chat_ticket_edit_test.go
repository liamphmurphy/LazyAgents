package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// A persona working on a ticket from a chat keeps it current with
// update_ticket and comment_ticket (LAZYAGENTS-100/101): edits are revision
// checked, attributed to the chat, and leave a live run's ticket to the run.
func TestChatUpdatesExistingTickets(t *testing.T) {
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
	runID := *chat.CurrentRunID
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: runID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	token, _, err := agents.IssueToken(ctx, q, chat.AgentID, "run", "chat", &runID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	call := func(name string, args map[string]any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(args)
		req := httptest.NewRequest("POST", "/api/v1/agent/tools/"+name, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	tk, err := tr.Create(ctx, ws, tracker.UserActor(me.ID), tracker.NewTicket{ProjectKey: "PAY", Title: "Worked on in chat"})
	if err != nil {
		t.Fatal(err)
	}
	rev := int(tk.Revision)

	// Starting work moves the ticket, and the event names the chat it came from.
	code, out := call("update_ticket", map[string]any{"ticket": tk.Key, "revision": rev, "status": "in_progress"})
	if code != 200 || out["status"] != "in_progress" {
		t.Fatalf("update_ticket status = %d %v", code, out)
	}
	events, _ := q.ListTicketActivity(ctx, &tk.ID)
	var via map[string]any
	for _, e := range events {
		if e.Verb == tracker.VerbStatusChanged {
			var p map[string]any
			_ = json.Unmarshal(e.Payload, &p)
			via, _ = p["via"].(map[string]any)
		}
	}
	if via == nil || via["chat_id"] != chat.ID.String() {
		t.Fatalf("status event via = %v, want chat %s", via, chat.ID)
	}

	// The old revision is stale now: the persona must re-read first.
	if code, out := call("update_ticket", map[string]any{"ticket": tk.Key, "revision": rev, "title": "Clobbered"}); code != 409 || !strings.Contains(fmt.Sprint(out), "stale") {
		t.Fatalf("stale revision = %d %v", code, out)
	}
	if code, out := call("update_ticket", map[string]any{"ticket": tk.Key, "status": "todo"}); code != 422 {
		t.Fatalf("missing revision = %d %v", code, out)
	}
	cur, _ := tracker.Get(ctx, q, ws, tk.Key)
	if code, out := call("update_ticket", map[string]any{"ticket": tk.Key, "revision": cur.Revision, "status": "canceled"}); code != 409 {
		t.Fatalf("cancel from chat = %d %v", code, out)
	}

	// Linking the PR hands the ticket to PR sync, which closes it on merge.
	code, out = call("update_ticket", map[string]any{"ticket": tk.Key, "revision": cur.Revision, "pr_url": "https://github.com/northwind/payments/pull/77"})
	if code != 200 {
		t.Fatalf("pr_url = %d %v", code, out)
	}
	cur, _ = tracker.Get(ctx, q, ws, tk.Key)
	if cur.PrUrl == nil || *cur.PrUrl != "https://github.com/northwind/payments/pull/77" || cur.PrNumber == nil || *cur.PrNumber != 77 {
		t.Fatalf("ticket PR = %v %v", cur.PrUrl, cur.PrNumber)
	}
	if code, out := call("update_ticket", map[string]any{"ticket": tk.Key, "revision": cur.Revision, "pr_url": "https://example.com/pr/1"}); code != 422 {
		t.Fatalf("bad pr_url = %d %v", code, out)
	}

	if code, out := call("comment_ticket", map[string]any{"ticket": tk.Key, "body": "Repro: no runs, still in backlog."}); code != 200 {
		t.Fatalf("comment_ticket = %d %v", code, out)
	}
	if code, out := call("comment_ticket", map[string]any{"ticket": "PAY-99999", "body": "x"}); code != 422 {
		t.Fatalf("comment on unknown ticket = %d %v", code, out)
	}

	// A ticket with a live run is the run's: no status or PR edits, though its
	// description can still be corrected.
	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	busy, err := tr.Create(ctx, ws, tracker.UserActor(me.ID), tracker.NewTicket{ProjectKey: "PAY", Title: "Running", Status: "todo", AssigneeAgentID: &atlas.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rs.Dispatch(ctx, ws, me.ID, []string{busy.Key}); err != nil {
		t.Fatal(err)
	}
	busy, _ = tracker.Get(ctx, q, ws, busy.Key)
	if code, out := call("update_ticket", map[string]any{"ticket": busy.Key, "revision": busy.Revision, "status": "done"}); code != 409 || !strings.Contains(fmt.Sprint(out), "live run") {
		t.Fatalf("status on a live run's ticket = %d %v", code, out)
	}
	if after, _ := tracker.Get(ctx, q, ws, busy.Key); after.Status == "done" {
		t.Fatal("refused status edit still committed")
	}
	if code, out := call("update_ticket", map[string]any{"ticket": busy.Key, "revision": busy.Revision, "description": "More context"}); code != 200 {
		t.Fatalf("description on a live run's ticket = %d %v", code, out)
	}
}
