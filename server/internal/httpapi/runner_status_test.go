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

// Host-folder workspaces: the toggle round-trips, each runner says whether it
// has a folder configured, and a run's or chat's folder shows only while the
// checkout exists.
func TestHostFolderWorkspaces(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `INSERT INTO runners (id, backend, max_concurrent, host_workspaces_dir) VALUES
		('with-folder', 'docker', 2, '/Users/me/vesper/workspaces'), ('without', 'docker', 2, '')`); err != nil {
		t.Fatal(err)
	}
	out := e.want("GET", "/runner", nil, http.StatusOK)
	if out["host_folder_workspaces"] != false {
		t.Fatalf("default host_folder_workspaces = %v", out["host_folder_workspaces"])
	}
	dirs := map[string]any{}
	for _, r := range out["runners"].([]any) {
		r := r.(map[string]any)
		dirs[r["id"].(string)] = r["host_workspaces_dir"]
	}
	if dirs["with-folder"] != "/Users/me/vesper/workspaces" || dirs["without"] != nil {
		t.Fatalf("host_workspaces_dir by runner %v", dirs)
	}
	if out := e.want("PUT", "/runner/settings", map[string]any{"host_folder_workspaces": true}, http.StatusOK); out["host_folder_workspaces"] != true {
		t.Fatalf("after PUT %v", out["host_folder_workspaces"])
	}
	if out := e.want("GET", "/runner", nil, http.StatusOK); out["host_folder_workspaces"] != true {
		t.Fatalf("not saved: %v", out["host_folder_workspaces"])
	}

	chat := e.want("POST", "/chats", map[string]any{}, http.StatusCreated)
	var runID string
	if err := e.pool.QueryRow(ctx, `INSERT INTO runs (workspace_id, agent_id, mode, state, resolved_spec, harness, model, purpose, workspace_volume, workspace_host_path)
		SELECT workspace_id, id, 'managed', 'running', '{}', 'claude-code', 'm', 'chat', 'v', '/Users/me/vesper/workspaces/r'
		FROM agents WHERE kind = 'assistant' LIMIT 1 RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE chat_sessions SET current_run_id = $1 WHERE id = $2`, runID, chat["id"]); err != nil {
		t.Fatal(err)
	}
	runOf := func() map[string]any {
		return e.want("GET", "/runs/"+runID, nil, http.StatusOK)["run"].(map[string]any)
	}
	if got := runOf()["workspace_path"]; got != "/Users/me/vesper/workspaces/r" {
		t.Fatalf("run workspace_path = %v", got)
	}
	if got := e.want("GET", "/chats/"+chat["id"].(string), nil, http.StatusOK)["workspace_path"]; got != "/Users/me/vesper/workspaces/r" {
		t.Fatalf("chat workspace_path = %v", got)
	}
	// Once the workspace is removed, the stale path is not offered.
	if _, err := e.pool.Exec(ctx, `UPDATE runs SET workspace_volume = NULL WHERE id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	if got, ok := runOf()["workspace_path"]; ok {
		t.Fatalf("removed run workspace_path = %v", got)
	}
	if got, ok := e.want("GET", "/chats/"+chat["id"].(string), nil, http.StatusOK)["workspace_path"]; ok {
		t.Fatalf("removed chat workspace_path = %v", got)
	}
}
