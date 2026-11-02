package seed

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

const atlasInstructions = `You own backend changes in Go and Postgres.

- Post a plan with lazyagents.post_plan before editing anything.
- Every migration gets a dry-run on the staging snapshot first. Attach the
  result with create_artifact(type: report).
- Never drop a column or index in the same PR that stops using it.
- If the spec and the PRD disagree, stop and ask with await_human.
`

func scopes(kv ...string) map[string]agentdef.Mode {
	m := map[string]agentdef.Mode{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = agentdef.Mode(kv[i+1])
	}
	return m
}

const (
	al = string(agentdef.Allowed)
	ap = string(agentdef.RequiresApproval)
	de = string(agentdef.Denied)
)

// seedDefinitions gives every demo agent a definition, Atlas a history that
// matches agent-config.html, and Atlas two project overrides.
func seedDefinitions(ctx context.Context, q *store.Queries, ws uuid.UUID, maya uuid.UUID, agentsByHandle map[string]store.Agent,
	projects map[string]store.Project) error {
	day := func(s string) *time.Time { return date(s) }

	atlasV := func(model string, migrate string, mcp bool, instr string) agentdef.Spec {
		s := agentdef.Spec{
			Harness: agentdef.HarnessClaudeCode, Model: model, Instructions: instr,
			Scopes: scopes("repo:read", al, "repo:write", al, "tests:run", al, "db:migrate", migrate,
				"db:read-staging", al, "deploy:staging", de),
			Budget: agentdef.Budget{MaxRuntime: "2h", MaxTurns: 20},
			HarnessOptions: map[string]map[string]string{
				"claude-code": {"permission_mode": "acceptEdits"},
				"codex":       {"sandbox": "workspace-write"},
			},
		}
		if mcp {
			s.MCPServers = []agentdef.MCPServer{{Name: "context7", URL: "https://mcp.context7.com/mcp", Description: "Docs lookup"}}
		}
		if err := s.Normalize(); err != nil {
			panic(err)
		}
		return s
	}
	oldInstr := "You own backend changes. Post a plan before editing. Never touch prod.\n"
	history := []struct {
		spec agentdef.Spec
		note string
		at   string
	}{
		{atlasV("sonnet-5.5", al, false, oldInstr), "Created", "2026-09-02"},
		{atlasV("sonnet-5.5", al, false, oldInstr+"Run the full test suite before opening a PR.\n"), "Asked for full test runs", "2026-09-15"},
		{atlasV("sonnet-5.5", al, false, oldInstr+"Run the full test suite before opening a PR.\n"), "Added db:read-staging", "2026-09-24"},
		{atlasV("sonnet-5.5", al, false, atlasInstructions), "Rewrote instructions", "2026-10-03"},
		{atlasV("opus-5.5", al, false, atlasInstructions), "Model sonnet-5.5 → opus-5.5", "2026-10-14"},
		{atlasV("opus-5.5", ap, false, atlasInstructions), "db:migrate → Requires approval", "2026-10-22"},
		{atlasV("opus-5.5", ap, true, atlasInstructions), "Added context7 MCP server", "2026-10-30"},
	}
	atlas := agentsByHandle["atlas"]
	id := agents.Identity{Name: atlas.Name, Role: "Backend · migrations & data models", Specialty: atlas.Specialty, ColorIndex: int(atlas.ColorIndex)}
	var err error
	for _, h := range history {
		if atlas, err = agents.WriteVersion(ctx, q, atlas, id, h.spec, h.note, &maya, day(h.at)); err != nil {
			return err
		}
	}
	agentsByHandle["atlas"] = atlas

	others := map[string]agentdef.Spec{
		"quill":  {Model: "sonnet-5.5", Instructions: "You build API endpoints with contract tests. Post a plan first.\n", Scopes: scopes("repo:read", al, "repo:write", al, "tests:run", al, "deploy:staging", de)},
		"sentry": {Model: "opus-5.5", Instructions: "You review specs and docs for conflicts. You don't change code; report findings with create_artifact(type: report).\n", Scopes: scopes("repo:read", al, "repo:write", de, "wiki:write", ap)},
		"juno":   {Model: "sonnet-5.5", Instructions: "You work on the React Native app. Keep screens accessible.\n", Scopes: scopes("repo:read", al, "repo:write", al, "tests:run", al)},
		"nova":   {Model: "sonnet-5.5", Instructions: "You own mobile auth and sessions.\n", Scopes: scopes("repo:read", al, "repo:write", al, "tests:run", al)},
		"relay":  {Model: "sonnet-5.5", Instructions: "You build integrations and event streams.\n", Scopes: scopes("repo:read", al, "repo:write", al, "tests:run", al, "deploy:staging", ap)},
		"pike":   {Model: "sonnet-5.5", Instructions: "You write data backfills. Batch everything and verify with checksums.\n", Scopes: scopes("repo:read", al, "repo:write", al, "tests:run", al, "db:migrate", ap, "db:read-staging", al)},
		"echo":   {Harness: agentdef.HarnessCodex, Model: "haiku-5.5", Instructions: "You do refactors and cleanup. Never change behaviour.\n", Scopes: scopes("repo:read", al, "repo:write", al, "tests:run", al)},
	}
	for handle, spec := range others {
		spec.Budget = agentdef.Budget{MaxRuntime: "2h", MaxTurns: 20}
		if err := spec.Normalize(); err != nil {
			return err
		}
		a := agentsByHandle[handle]
		if agentsByHandle[handle], err = agents.WriteVersion(ctx, q, a, agents.Identity{Name: a.Name, Role: a.Role, Specialty: a.Specialty, ColorIndex: int(a.ColorIndex)},
			spec, "Created", &maya, day("2026-09-10")); err != nil {
			return err
		}
	}

	for key, o := range map[string]agentdef.Override{
		"PAY": {InstructionsAppend: "Follow ADR-012 for partitioning.", MCPServersAdd: []agentdef.MCPServer{{Name: "postgres-staging", URL: "https://pg-staging.northwind.internal/mcp", Description: "Staging schema browser"}}},
		"BIL": {Harness: agentdef.HarnessCodex, Scopes: scopes("prod:*", de)},
	} {
		if err := o.Normalize(); err != nil {
			return err
		}
		raw, _ := json.Marshal(o)
		if _, err := q.UpsertOverride(ctx, store.UpsertOverrideParams{AgentID: atlas.ID, ProjectID: projects[key].ID, Spec: raw, UpdatedByUserID: &maya}); err != nil {
			return err
		}
	}
	return nil
}
