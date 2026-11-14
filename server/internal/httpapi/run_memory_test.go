package httpapi

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/memory"
)

// A persona's ticket run may search and read the person's project and work memories and
// the persona's own, never the rest: a run reads untrusted text and writes to the outside
// world, so life and preferences stay out of it. It can't change memory at all.
func TestTicketRunReadsOnlyProjectAndWorkMemory(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	var atlas uuid.UUID
	if err := e.pool.QueryRow(ctx, `SELECT id FROM agents WHERE workspace_id=$1 AND handle='atlas'`, e.me.WorkspaceID).Scan(&atlas); err != nil {
		t.Fatal(err)
	}
	mem := memory.New(e.q)
	person := memory.Owner{WorkspaceID: e.me.WorkspaceID, UserID: e.me.ID}
	save := func(o memory.Owner, title, typ string) string {
		t.Helper()
		r, err := mem.Save(ctx, o, memory.Input{Title: title, Summary: title + " summary", Type: typ}, memory.Source{Label: "test"})
		if err != nil {
			t.Fatal(err)
		}
		return r.Memory.ID.String()
	}
	project := save(person, "Ledger uses one PR per change set", "project")
	work := save(person, "Ledger reviews on Fridays", "work")
	life := save(person, "Ledger of my health appointments", "life")
	pref := save(person, "Ledger notes in Gruvbox", "preference")
	own := save(person.Persona(atlas), "Ledger is my favourite subsystem", "fact")

	var run string
	if err := e.pool.QueryRow(ctx, `INSERT INTO runs (workspace_id, ticket_id, agent_id, mode, state, resolved_spec, harness, model, purpose)
		VALUES ($1, (SELECT id FROM tickets WHERE key=$2), $3, 'managed', 'running', '{}', 'claude-code', 'm', 'ticket') RETURNING id`,
		e.me.WorkspaceID, e.payKey, atlas).Scan(&run); err != nil {
		t.Fatal(err)
	}
	runID := uuid.MustParse(run)
	token, _, err := agents.IssueToken(ctx, e.q, atlas, "run", "test", &runID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	found := func(args map[string]any) map[string]bool {
		t.Helper()
		code, out := e.tool(token, "search_memory", args)
		if code != 200 {
			t.Fatalf("search_memory %v = %d %v", args, code, out)
		}
		ids := map[string]bool{}
		for _, m := range out["memories"].([]any) {
			ids[m.(map[string]any)["id"].(string)] = true
		}
		return ids
	}
	got := found(map[string]any{"query": "ledger", "limit": 50})
	if !got[project] || !got[work] || !got[own] {
		t.Errorf("search found %v, want the project and work memories and the persona's own", got)
	}
	if got[life] || got[pref] {
		t.Errorf("search found a life or preference memory: %v", got)
	}
	if got := found(map[string]any{"about": "persona"}); len(got) != 1 || !got[own] {
		t.Errorf("about persona = %v, want only its own", got)
	}
	if code, _ := e.tool(token, "search_memory", map[string]any{"type": "life"}); code != 422 {
		t.Errorf("search type life = %d, want 422", code)
	}
	for id, want := range map[string]int{project: 200, own: 200, life: 404, pref: 404} {
		if code, _ := e.tool(token, "read_memory", map[string]any{"id": id}); code != want {
			t.Errorf("read_memory %s = %d, want %d", id, code, want)
		}
	}
	if code, _ := e.tool(token, "save_memory", map[string]any{"title": "Runs can't write"}); code == 200 {
		t.Error("a ticket run saved a memory")
	}
}
