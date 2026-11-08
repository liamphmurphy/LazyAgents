package httpapi

import (
	"context"
	"net/http"
	"testing"
)

// The Runs nav count leaves out Personal Assistant chat runs: they live in Chat.
func TestRunnerStatusLiveSkipsChatRuns(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `INSERT INTO runs (workspace_id, ticket_id, agent_id, mode, state, resolved_spec, harness, model, purpose)
		SELECT t.workspace_id, t.id, a.id, 'managed', s.state, '{}', 'claude-code', 'm', 'ticket'
		FROM (SELECT *, row_number() OVER (ORDER BY created_at) n FROM tickets) t
		JOIN unnest(ARRAY['running', 'starting', 'waiting', 'completed']) WITH ORDINALITY s(state, n) ON s.n = t.n,
		(SELECT id FROM agents WHERE kind <> 'assistant' LIMIT 1) a`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO runs (workspace_id, agent_id, mode, state, resolved_spec, harness, model, purpose)
		SELECT workspace_id, id, 'managed', 'running', '{}', 'claude-code', 'm', 'chat' FROM agents WHERE kind = 'assistant' LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	out := e.want("GET", "/runner", nil, http.StatusOK)
	if out["live"] != float64(2) {
		t.Fatalf("live = %v, want 2 (running + starting ticket runs, no chat)", out["live"])
	}
}
