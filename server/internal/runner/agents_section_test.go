package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/harness"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Ticket, delegated and planning runs render exactly as before and never load
// a persona: they get a nil Queries, so any persona (or memory) read panics.
func TestAgentsSectionNoPersonaOutsideChat(t *testing.T) {
	spec := agentdef.Resolved{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5", Instructions: "Be careful.",
		Scopes: []agentdef.ResolvedScope{{Scope: "repo:write", Mode: agentdef.Allowed}}}
	paths := []string{".lazyagents/context/adr.md"}
	parent := uuid.New()
	for _, tc := range []struct {
		name string
		c    runCtx
		want []byte
	}{
		{"ticket", runCtx{run: store.Run{Purpose: "ticket"}, ticket: store.Ticket{Key: "PAY-1"}, spec: spec}, harness.AgentsMD("", spec, paths, "PAY-1")},
		{"delegated", runCtx{run: store.Run{Purpose: "ticket", ParentRunID: &parent}, ticket: store.Ticket{Key: "PAY-1"}, spec: spec}, harness.SubagentAgentsMD("", spec, paths, "PAY-1")},
		{"planning", runCtx{run: store.Run{Purpose: "planning"}, spec: spec}, []byte("Be careful.\nRead these context files first:\n.lazyagents/context/adr.md")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := agentsSection(context.Background(), nil, tc.c, append([]string{}, paths...))
			if err != nil || string(got) != string(tc.want) {
				t.Fatalf("err=%v\n%s\nwant\n%s", err, got, tc.want)
			}
			if strings.Contains(string(got), "## Persona") || strings.Contains(string(got), "In this conversation you are") {
				t.Fatalf("persona content in a %s run:\n%s", tc.name, got)
			}
		})
	}
}
