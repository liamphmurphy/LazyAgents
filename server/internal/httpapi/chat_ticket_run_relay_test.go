package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// reply_to_run passes the person's answer to a run their chat started, and
// only once they've answered: the persona can't speak for them before that,
// nor answer runs other chats started.
func TestReplyToRunRelaysThePersonsAnswer(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	ws := e.me.WorkspaceID
	id := e.newChat(nil)["id"].(string)
	atlas, err := e.q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := (&tracker.Service{Pool: e.pool}).Create(ctx, ws, tracker.UserActor(e.me.ID), tracker.NewTicket{ProjectKey: "MOB", Title: "Round payouts half-even", AssigneeAgentID: &atlas.ID})
	if err != nil {
		t.Fatal(err)
	}
	runID := e.want("POST", "/chats/"+id+"/runs", map[string]any{"ticket": ticket.Key}, http.StatusCreated)["run_id"].(string)

	// The run asks; the sweep tells the chat and starts a follow-up turn.
	if _, err := e.pool.Exec(ctx, `UPDATE runs SET state = 'waiting', wait_reason = 'human_reply' WHERE id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Record(ctx, e.q, ws, ticket.ID, nil, tracker.AgentActor(atlas.ID), runs.VerbWaitingForHuman, map[string]any{"run_id": runID, "reason": "Half-even or half-up?"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := runs.AdvanceChatTicketRuns(ctx, e.pool); err != nil {
		t.Fatal(err)
	}
	chat, err := e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: uuid.MustParse(id), WorkspaceID: ws})
	if err != nil || chat.CurrentRunID == nil {
		t.Fatalf("follow-up turn: %+v, %v", chat.CurrentRunID, err)
	}
	token, _, err := agents.IssueToken(ctx, e.q, chat.AgentID, "run", "chat", chat.CurrentRunID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	reply := func(tok string, args map[string]any) (int, string) {
		t.Helper()
		raw, _ := json.Marshal(args)
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/agent/tools/reply_to_run", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, fmt.Sprint(out)
	}
	answer := map[string]any{"ticket": strings.ToLower(ticket.Key), "answer": "Half-even, like the ledger."}

	// The persona can't answer for the person before they've said anything.
	if code, out := reply(token, answer); code != http.StatusConflict || !strings.Contains(out, "wait for the person to answer") {
		t.Fatalf("relay before the person answered = %d %s", code, out)
	}
	if code, out := reply(token, map[string]any{"ticket": ticket.Key, "answer": "  "}); code != http.StatusUnprocessableEntity {
		t.Fatalf("empty answer = %d %s", code, out)
	}

	// Another chat can't answer this chat's run.
	other := e.newChat(nil)["id"].(string)
	if _, err := e.pool.Exec(ctx, `INSERT INTO chat_messages (session_id, seq, role, body_md) VALUES ($1, 1, 'user', 'half-even')`, other); err != nil {
		t.Fatal(err)
	}
	otherRun, err := e.q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: ws, AgentID: chat.AgentID, Mode: "managed", State: "running", ResolvedSpec: json.RawMessage(`{}`), Harness: "claude-code", Model: "m", Purpose: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE chat_sessions SET current_run_id = $2 WHERE id = $1`, other, otherRun.ID); err != nil {
		t.Fatal(err)
	}
	otherToken, _, err := agents.IssueToken(ctx, e.q, chat.AgentID, "run", "chat", &otherRun.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code, out := reply(otherToken, answer); code != http.StatusConflict || !strings.Contains(out, "wasn't started from this chat") {
		t.Fatalf("relay from another chat = %d %s", code, out)
	}

	// Once the person answers, the persona passes it on and the run resumes with it.
	if _, err := e.pool.Exec(ctx, `INSERT INTO chat_messages (session_id, seq, role, body_md, created_at)
		SELECT $1, max(seq) + 1, 'user', 'half-even please', now() + interval '1 second' FROM chat_messages WHERE session_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if code, out := reply(token, answer); code != http.StatusOK || !strings.Contains(out, "resumed") {
		t.Fatalf("relay = %d %s", code, out)
	}
	var state, input string
	if err := e.pool.QueryRow(ctx, `SELECT r.state, rt.input_message FROM runs r JOIN run_turns rt ON rt.run_id = r.id WHERE r.id = $1 ORDER BY rt.seq DESC LIMIT 1`, runID).Scan(&state, &input); err != nil {
		t.Fatal(err)
	}
	if state != "running" || !strings.HasPrefix(input, "@atlas Half-even, like the ledger.") || !strings.Contains(input, "from "+e.me.Name+"'s reply in chat") {
		t.Fatalf("resumed run = %s with %q", state, input)
	}
	var author string
	if err := e.pool.QueryRow(ctx, `SELECT author_type FROM comments WHERE ticket_id = $1 ORDER BY created_at DESC LIMIT 1`, ticket.ID).Scan(&author); err != nil || author != "agent" {
		t.Fatalf("relay comment author = %q, %v", author, err)
	}

	// It's running again, so a second relay has nothing to answer.
	if code, out := reply(token, answer); code != http.StatusConflict || !strings.Contains(out, "isn't waiting for a reply") {
		t.Fatalf("second relay = %d %s", code, out)
	}
}
