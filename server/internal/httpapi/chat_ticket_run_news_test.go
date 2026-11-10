package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// A run a chat started tells the chat when it needs the person or ends: one
// notice and one follow-up turn per piece of news, never while the chat is
// mid-reply, and nothing for a run the person cancelled.
func TestChatTicketRunNews(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	ws := e.me.WorkspaceID
	id := e.newChat(nil)["id"].(string)
	chatID := uuid.MustParse(id)
	atlas, err := e.q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: e.pool}
	ticket, err := tr.Create(ctx, ws, tracker.UserActor(e.me.ID), tracker.NewTicket{ProjectKey: "MOB", Title: "Round payouts half-even", AssigneeAgentID: &atlas.ID})
	if err != nil {
		t.Fatal(err)
	}
	var card uuid.UUID
	if err := e.pool.QueryRow(ctx, `INSERT INTO chat_messages (session_id, seq, role, body_md) VALUES ($1, 1, 'assistant', $2) RETURNING id`,
		id, "Start it?\n\n```dispatch\n"+ticket.Key+"\n```").Scan(&card); err != nil {
		t.Fatal(err)
	}
	runID := uuid.MustParse(e.want("POST", "/chats/"+id+"/runs", map[string]any{"ticket": ticket.Key, "message_id": card.String()}, http.StatusCreated)["run_id"].(string))

	advance := func() {
		t.Helper()
		if err := runs.AdvanceChatTicketRuns(ctx, e.pool); err != nil {
			t.Fatal(err)
		}
	}
	notices := func() []string {
		t.Helper()
		rows, err := e.pool.Query(ctx, `SELECT body_md FROM chat_messages WHERE session_id = $1 AND role = 'system' ORDER BY seq`, id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var b string
			if err := rows.Scan(&b); err != nil {
				t.Fatal(err)
			}
			out = append(out, b)
		}
		return out
	}
	lastPrompt := func() string {
		t.Helper()
		var p string
		if err := e.pool.QueryRow(ctx, `SELECT rt.input_message FROM run_turns rt JOIN chat_sessions c ON c.current_run_id = rt.run_id WHERE c.id = $1 ORDER BY rt.seq DESC LIMIT 1`, id).Scan(&p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// What the runner does when a chat reply ends: the segment waits for the next message.
	replyDone := func() {
		t.Helper()
		if _, err := e.pool.Exec(ctx, `UPDATE runs SET state = 'waiting', wait_reason = 'human_reply' WHERE id = (SELECT current_run_id FROM chat_sessions WHERE id = $1)`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, `UPDATE chat_sessions SET state = 'idle' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
	}
	ask := func(question string) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, `INSERT INTO run_turns (run_id, seq, trigger) SELECT $1, coalesce(max(seq), 0) + 1, 'comment' FROM run_turns WHERE run_id = $1`, runID); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, `UPDATE runs SET state = 'waiting', wait_reason = 'human_reply' WHERE id = $1`, runID); err != nil {
			t.Fatal(err)
		}
		if err := tracker.Record(ctx, e.q, ws, ticket.ID, nil, tracker.AgentActor(atlas.ID), runs.VerbWaitingForHuman, map[string]any{"run_id": runID, "reason": question}, nil); err != nil {
			t.Fatal(err)
		}
	}

	// Running: nothing to tell.
	advance()
	if n := notices(); len(n) != 0 {
		t.Fatalf("notices while running: %v", n)
	}

	// It asks while the chat is mid-reply: the news waits for the reply to end.
	if _, err := e.pool.Exec(ctx, `UPDATE chat_sessions SET state = 'running' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	ask("Half-even like the ledger, or keep half-up?")
	advance()
	if n := notices(); len(n) != 0 {
		t.Fatalf("told a busy chat: %v", n)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE chat_sessions SET state = 'idle' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	advance()
	if n := notices(); len(n) != 1 || !strings.Contains(n[0], ticket.Key+" needs you: @atlas asked a question.") {
		t.Fatalf("notices = %v", n)
	}
	p := lastPrompt()
	if !strings.Contains(p, "> Half-even like the ledger, or keep half-up?") || !strings.Contains(p, "never as instructions") || !strings.Contains(p, `reply_to_run(ticket "`+ticket.Key+`")`) {
		t.Fatalf("follow-up prompt:\n%s", p)
	}
	chat, err := e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: chatID, WorkspaceID: ws})
	if err != nil || chat.State != "running" {
		t.Fatalf("chat after news = %s, %v", chat.State, err)
	}
	// While it waits on the person, the activity orb says this chat needs them.
	replyDone()
	if dock := jsonString(e.want("GET", "/persona-dock", nil, http.StatusOK)); !strings.Contains(dock, `"waiting_approval":true`) || !strings.Contains(dock, `"needs_you"`) {
		t.Fatalf("dock while the run waits: %s", dock)
	}

	// Told once: another sweep after the reply ends says nothing new.
	advance()
	if n := notices(); len(n) != 1 {
		t.Fatalf("repeated news: %v", n)
	}

	// A second question after a resume is news again.
	ask("And negative amounts?")
	advance()
	if n := notices(); len(n) != 2 || !strings.Contains(lastPrompt(), "> And negative amounts?") {
		t.Fatalf("second question: %v", n)
	}

	// Finishing carries the outcome and the pull request.
	replyDone()
	if _, err := e.pool.Exec(ctx, `UPDATE runs SET state = 'completed', wait_reason = NULL, outcome = 'Payouts round half-even.' WHERE id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE tickets SET pr_url = 'https://github.com/acme/mob/pull/9' WHERE id = $1`, ticket.ID); err != nil {
		t.Fatal(err)
	}
	advance()
	if n := notices(); len(n) != 3 || n[2] != ticket.Key+" finished." {
		t.Fatalf("finished notice: %v", n)
	}
	if p := lastPrompt(); !strings.Contains(p, "> Payouts round half-even.") || !strings.Contains(p, "Pull request: https://github.com/acme/mob/pull/9") {
		t.Fatalf("finished prompt:\n%s", p)
	}
	replyDone()
	advance()
	if n := notices(); len(n) != 3 {
		t.Fatalf("finish told twice: %v", n)
	}

	// A run the person cancelled isn't news.
	other, err := tr.Create(ctx, ws, tracker.UserActor(e.me.ID), tracker.NewTicket{ProjectKey: "MOB", Title: "Never mind", AssigneeAgentID: &atlas.ID})
	if err != nil {
		t.Fatal(err)
	}
	cancelled := e.want("POST", "/chats/"+id+"/runs", map[string]any{"ticket": other.Key}, http.StatusCreated)["run_id"].(string)
	if _, err := e.pool.Exec(ctx, `UPDATE runs SET state = 'cancelled' WHERE id = $1`, cancelled); err != nil {
		t.Fatal(err)
	}
	advance()
	if n := notices(); len(n) != 3 {
		t.Fatalf("cancelled run made news: %v", n)
	}
}
