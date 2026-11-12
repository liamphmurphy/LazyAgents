package agentapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// list_subagents tells the lead what each sub agent may do, and lists one whose
// definition doesn't resolve as unavailable instead of failing.
func TestListSubagentsCapabilities(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	agentSvc := &agents.Service{Pool: pool}
	readOnly := agents.DefaultSpec()
	readOnly.Scopes["repo:write"] = agentdef.Denied
	for handle, spec := range map[string]*agentdef.Spec{"scribe": nil, "critic": &readOnly, "broken": nil} {
		if _, err := agentSvc.Create(ctx, ws, me.ID, handle, agents.Identity{Name: handle, Kind: agents.KindSub}, spec); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_definition_versions v SET spec = jsonb_set(v.spec, '{harness}', '"nope"')
		FROM agents a WHERE a.current_definition_version_id = v.id AND a.handle = 'broken' AND a.workspace_id = $1`, ws); err != nil {
		t.Fatal(err)
	}
	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	tok, _, err := agents.IssueToken(ctx, q, atlas.ID, "unmanaged", "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := agents.ResolveToken(ctx, q, tok)
	if err != nil {
		t.Fatal(err)
	}
	d := Deps{Q: q, Runs: &runs.Service{Pool: pool, Tracker: &tracker.Service{Pool: pool}, Poll: 20 * time.Millisecond}}
	out, err := ByName["list_subagents"].call(ctx, d, c, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]any{}
	for _, it := range out.(map[string]any)["subagents"].([]map[string]any) {
		got[it["handle"].(string)] = it
	}
	if s := got["scribe"]; s["can_write"] != true || s["can_run_tests"] != true || s["harness"] != agentdef.HarnessClaudeCode || s["model"] == "" ||
		s["scopes"].(map[string]string)["repo:write"] != "allowed" {
		t.Errorf("scribe = %v", s)
	}
	if s := got["critic"]; s["can_write"] != false || s["can_run_tests"] != true || s["scopes"].(map[string]string)["repo:write"] != "denied" {
		t.Errorf("critic = %v", s)
	}
	if s := got["broken"]; s["unavailable"] == nil || s["unavailable"] == "" || s["can_write"] != nil {
		t.Errorf("broken = %v", s)
	}
}
