package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// A persona's ```dispatch card starts a ticket's run only when the person
// presses Start: as them, once per card, in any project, and the chat lists it.
func TestStartChatTicketRun(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	ws := e.me.WorkspaceID
	id := e.newChat(nil)["id"].(string) // a PAY chat; the ticket is in MOB
	atlas, err := e.q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: e.pool}
	ticket, err := tr.Create(ctx, ws, tracker.UserActor(e.me.ID), tracker.NewTicket{ProjectKey: "MOB", Title: "Round payouts half-even", AssigneeAgentID: &atlas.ID})
	if err != nil {
		t.Fatal(err)
	}
	unassigned, err := tr.Create(ctx, ws, tracker.UserActor(e.me.ID), tracker.NewTicket{ProjectKey: "MOB", Title: "Nobody's yet"})
	if err != nil {
		t.Fatal(err)
	}
	reply := func(seq int, body string) string {
		t.Helper()
		var msg uuid.UUID
		if err := e.pool.QueryRow(ctx, `INSERT INTO chat_messages (session_id, seq, role, body_md) VALUES ($1, $2, 'assistant', $3) RETURNING id`, id, seq, body).Scan(&msg); err != nil {
			t.Fatal(err)
		}
		return msg.String()
	}
	card := reply(1, "Filed it. Start it when you're ready.\n\n```dispatch\n"+ticket.Key+"\n```")
	plain := reply(2, "No card here.")
	path := "/chats/" + id + "/runs"

	e.invalid("POST", path, map[string]any{"ticket": ticket.Key, "message_id": plain}, "message_id", "doesn't offer")
	e.invalid("POST", path, map[string]any{"ticket": unassigned.Key, "message_id": card}, "message_id", "doesn't offer")
	e.invalid("POST", path, map[string]any{"ticket": unassigned.Key}, "ticket", "has no agent")
	e.invalid("POST", path, map[string]any{"ticket": "NOPE-1"}, "ticket", "no ticket")

	started := e.want("POST", path, map[string]any{"ticket": strings.ToLower(ticket.Key), "message_id": card}, http.StatusCreated)
	if started["ticket"] != ticket.Key || started["agent"] != "atlas" || started["state"] != "queued" || started["message_id"] != card {
		t.Fatalf("started = %v", started)
	}
	var dispatcher *uuid.UUID
	if err := e.pool.QueryRow(ctx, `SELECT dispatched_by_user_id FROM runs WHERE id = $1`, started["run_id"]).Scan(&dispatcher); err != nil || dispatcher == nil || *dispatcher != e.me.ID {
		t.Fatalf("dispatcher = %v, %v", dispatcher, err)
	}

	// A card starts one run; a second Start is refused, as is a second live run.
	e.want("POST", path, map[string]any{"ticket": ticket.Key, "message_id": card}, http.StatusConflict)
	e.want("POST", path, map[string]any{"ticket": ticket.Key}, http.StatusConflict)

	listed := e.want("GET", path, nil, http.StatusOK)["runs"].([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["run_id"] != started["run_id"] || listed[0].(map[string]any)["steps_total"] != float64(0) {
		t.Fatalf("listed = %v", listed)
	}
	e.want("GET", "/chats/"+uuid.NewString()+"/runs", nil, http.StatusNotFound)
}
