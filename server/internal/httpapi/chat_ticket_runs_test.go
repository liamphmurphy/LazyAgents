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

	// One card can offer several tickets: each starts once from it.
	var several []string
	for _, title := range []string{"Shapes", "Themes"} {
		tk, err := tr.Create(ctx, ws, tracker.UserActor(e.me.ID), tracker.NewTicket{ProjectKey: "MOB", Title: title, AssigneeAgentID: &atlas.ID})
		if err != nil {
			t.Fatal(err)
		}
		several = append(several, tk.Key)
	}
	multi := reply(3, "Both are ready.\n\n```dispatch\n"+strings.Join(several, "\n")+"\n```")
	first := e.want("POST", path, map[string]any{"ticket": several[0], "message_id": multi}, http.StatusCreated)
	second := e.want("POST", path, map[string]any{"ticket": several[1], "message_id": multi}, http.StatusCreated)
	if first["message_id"] != multi || second["message_id"] != multi || first["run_id"] == second["run_id"] {
		t.Fatalf("several from one card: %v, %v", first, second)
	}
	e.want("POST", path, map[string]any{"ticket": several[1], "message_id": multi}, http.StatusConflict)
}

// The tray gets what the agent is doing (the tool and, for a shell, only its
// capped command) and any approval the run waits on.
func TestChatTicketRunTrayFields(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	ws := e.me.WorkspaceID
	id := e.newChat(nil)["id"].(string)
	atlas, err := e.q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := (&tracker.Service{Pool: e.pool}).Create(ctx, ws, tracker.UserActor(e.me.ID), tracker.NewTicket{ProjectKey: "MOB", Title: "Migrate", AssigneeAgentID: &atlas.ID})
	if err != nil {
		t.Fatal(err)
	}
	runID := e.want("POST", "/chats/"+id+"/runs", map[string]any{"ticket": ticket.Key}, http.StatusCreated)["run_id"].(string)
	tray := func() map[string]any {
		t.Helper()
		return e.want("GET", "/chats/"+id+"/runs", nil, http.StatusOK)["runs"].([]any)[0].(map[string]any)
	}
	if r := tray(); r["activity"] != nil || r["approval"] != nil {
		t.Fatalf("fresh run = %v", r)
	}
	long := strings.Repeat("x", 500)
	for _, ev := range []string{`{"tool":"Write","input":{"file_path":"secret.go","content":"` + long + `"}}`, `{"tool":"Bash","input":{"command":"go test ./... ` + long + `"}}`} {
		if _, err := e.pool.Exec(ctx, `INSERT INTO run_events (run_id, type, payload) VALUES ($1, 'tool_call', $2)`, runID, ev); err != nil {
			t.Fatal(err)
		}
	}
	act := tray()["activity"].(map[string]any)
	if act["tool"] != "Bash" || !strings.HasPrefix(act["command"].(string), "go test ./...") || len(act["command"].(string)) != 300 {
		t.Fatalf("activity = %v", act)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO approval_requests (workspace_id, run_id, ticket_id, agent_id, scope, action_summary, action_detail, risk_summary)
		VALUES ($1, $2, $3, $4, 'db:migrate', 'Run migrations on staging', '{"env":"staging"}', 'Reversible')`, ws, runID, ticket.ID, atlas.ID); err != nil {
		t.Fatal(err)
	}
	ap := tray()["approval"].(map[string]any)
	if ap["scope"] != "db:migrate" || ap["action"] != "Run migrations on staging" || !strings.Contains(ap["detail"].(string), "staging") || ap["risk"] != "Reversible" {
		t.Fatalf("approval = %v", ap)
	}
}
