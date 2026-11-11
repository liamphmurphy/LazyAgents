package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
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

// A persona planning an epic from a chat can do what the person does on the
// ticket and epic pages: set acceptance criteria and blockers on tickets, and
// change an epic's criteria and mockup requirement.
func TestChatPlansTicketsWithCriteriaAndBlockers(t *testing.T) {
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

	chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{Project: "PAY", Title: "Plan", Message: &runs.ChatMessageInput{Body: "hello"}})
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
	blockers := func(key string) []string {
		t.Helper()
		tk, err := tracker.Get(ctx, q, ws, key)
		if err != nil {
			t.Fatal(err)
		}
		deps, err := q.TicketDependencies(ctx, tk.ID)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, d := range deps {
			out = append(out, d.Key)
		}
		return out
	}
	criteria := func(key string) []store.Criterion {
		t.Helper()
		tk, err := tracker.Get(ctx, q, ws, key)
		if err != nil {
			t.Fatal(err)
		}
		cs, err := q.ListCriteria(ctx, tk.ID)
		if err != nil {
			t.Fatal(err)
		}
		return cs
	}
	ticketsTitled := func(title string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE workspace_id = $1 AND title = $2`, ws, title).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// create_ticket takes criteria and blockers, and sets both with the ticket.
	code, out := call("create_ticket", map[string]any{"title": "Foundation"})
	if code != 200 {
		t.Fatalf("create foundation: %d %v", code, out)
	}
	foundation := fmt.Sprint(out["ticket"])
	code, out = call("create_ticket", map[string]any{"title": "Builds on it", "criteria": []string{"Renders the deck", "Exports a PDF"}, "blocked_by": []string{strings.ToLower(foundation)}})
	if code != 200 {
		t.Fatalf("create with blockers: %d %v", code, out)
	}
	dependent := fmt.Sprint(out["ticket"])
	if got := blockers(dependent); !slices.Equal(got, []string{foundation}) {
		t.Fatalf("blocked_by = %v, want [%s]", got, foundation)
	}
	if cs := criteria(dependent); len(cs) != 2 || cs[0].Text != "Renders the deck" || cs[1].Text != "Exports a PDF" {
		t.Fatalf("criteria = %+v", cs)
	}

	// An unknown blocker refuses the whole ticket: none is left half-made.
	if code, out := call("create_ticket", map[string]any{"title": "Orphan", "blocked_by": []string{"PAY-99999"}}); code != 422 || !strings.Contains(fmt.Sprint(out), "PAY-99999") {
		t.Fatalf("unknown blocker: %d %v", code, out)
	}
	if n := ticketsTitled("Orphan"); n != 0 {
		t.Fatalf("a ticket with an unknown blocker was still created (%d)", n)
	}
	if code, out := call("create_ticket", map[string]any{"title": "Blank criterion", "criteria": []string{"Fine", "  "}}); code != 422 {
		t.Fatalf("blank criterion: %d %v", code, out)
	}
	if n := ticketsTitled("Blank criterion"); n != 0 {
		t.Fatalf("a ticket with a blank criterion was still created")
	}

	// update_ticket_blockers adds and removes without a revision, and refuses cycles.
	code, out = call("create_ticket", map[string]any{"title": "Another blocker"})
	if code != 200 {
		t.Fatalf("create: %d %v", code, out)
	}
	another := fmt.Sprint(out["ticket"])
	if code, out := call("update_ticket_blockers", map[string]any{"ticket": dependent, "add": []string{another}}); code != 200 {
		t.Fatalf("add blocker: %d %v", code, out)
	}
	if got := blockers(dependent); !slices.Contains(got, foundation) || !slices.Contains(got, another) {
		t.Fatalf("after add, blocked_by = %v", got)
	}
	code, out = call("update_ticket_blockers", map[string]any{"ticket": dependent, "remove": []string{foundation}})
	if code != 200 || fmt.Sprint(out["blocked_by"]) != "["+another+"]" {
		t.Fatalf("remove blocker: %d %v", code, out)
	}
	if code, out := call("update_ticket_blockers", map[string]any{"ticket": another, "add": []string{dependent}}); code != 422 || !strings.Contains(fmt.Sprint(out), "cycle") {
		t.Fatalf("cycle: %d %v", code, out)
	}
	if code, out := call("update_ticket_blockers", map[string]any{"ticket": dependent}); code != 422 {
		t.Fatalf("nothing to change: %d %v", code, out)
	}
	var via string
	if err := pool.QueryRow(ctx, `SELECT coalesce(e.payload->'via'->>'chat_id', '') FROM activity_events e JOIN tickets t ON t.id = e.ticket_id
		WHERE t.key = $1 AND e.verb = 'dependencies_changed' ORDER BY e.seq DESC LIMIT 1`, dependent).Scan(&via); err != nil {
		t.Fatal(err)
	}
	if via != chat.ID.String() {
		t.Fatalf("blocker edit not attributed to the chat: via chat_id = %q", via)
	}

	// update_ticket replaces criteria; an unchanged criterion keeps its tick.
	cs := criteria(dependent)
	if _, err := tr.UpdateCriterion(ctx, ws, tracker.UserActor(me.ID), cs[0].ID, nil, ptr(true)); err != nil {
		t.Fatal(err)
	}
	tk, _ := tracker.Get(ctx, q, ws, dependent)
	code, out = call("update_ticket", map[string]any{"ticket": dependent, "revision": tk.Revision, "criteria": []string{"Opens in LibreOffice", "Renders the deck"}})
	if code != 200 {
		t.Fatalf("replace criteria: %d %v", code, out)
	}
	cs = criteria(dependent)
	if len(cs) != 2 || cs[0].Text != "Opens in LibreOffice" || cs[0].Done || cs[1].Text != "Renders the deck" || !cs[1].Done {
		t.Fatalf("criteria after replace = %+v", cs)
	}

	// get_epic and update_epic: criteria by number, the mockup requirement, and done stays gated.
	code, out = call("create_epic", map[string]any{"name": "Planned in chat", "criteria": []string{"First", "Second", "Third"}})
	if code != 200 {
		t.Fatalf("create epic: %d %v", code, out)
	}
	epicID := fmt.Sprint(out["epic_id"])
	code, out = call("update_epic", map[string]any{"epic_id": epicID, "requires_mockup": true,
		"edit_criteria": []map[string]any{{"n": 1, "text": "First, sharpened"}}, "remove_criteria": []int{2}, "add_criteria": []string{"Fourth"}})
	if code != 200 {
		t.Fatalf("update epic: %d %v", code, out)
	}
	var texts []string
	for _, c := range out["criteria"].([]any) {
		texts = append(texts, fmt.Sprint(c.(map[string]any)["text"]))
	}
	if !slices.Equal(texts, []string{"First, sharpened", "Third", "Fourth"}) || out["requires_mockup"] != true {
		t.Fatalf("epic after update: %v", out)
	}
	// A bad number is refused before anything changes.
	if code, out := call("update_epic", map[string]any{"epic_id": epicID, "add_criteria": []string{"Never added"}, "remove_criteria": []int{9}}); code != 422 {
		t.Fatalf("bad criterion number: %d %v", code, out)
	}
	if code, out := call("update_epic", map[string]any{"epic_id": epicID, "edit_criteria": []map[string]any{{"n": 1, "text": "x"}}, "remove_criteria": []int{1}}); code != 422 {
		t.Fatalf("edit and remove the same criterion: %d %v", code, out)
	}
	if code, out := call("update_epic", map[string]any{"epic_id": epicID, "status": "done"}); code != 409 {
		t.Fatalf("closing an epic with unticked criteria: %d %v", code, out)
	}
	code, out = call("get_epic", map[string]any{"epic_id": epicID})
	if code != 200 || len(out["criteria"].([]any)) != 3 {
		t.Fatalf("get_epic: %d %v", code, out)
	}
}
