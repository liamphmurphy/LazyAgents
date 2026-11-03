package runs

import (
	"reflect"
	"strings"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
)

func TestChatSpecReadOnly(t *testing.T) {
	in := agentdef.Resolved{
		Instructions: "Agent persona", Scopes: []agentdef.ResolvedScope{{Scope: "repo:*", Mode: agentdef.Allowed}, {Scope: "custom:write", Mode: agentdef.Allowed}, {Scope: "wiki:write", Mode: agentdef.Allowed}},
		Setup: []string{"exit 79"}, MCPServers: []agentdef.MCPServer{{Name: "external", URL: "https://example.com"}},
		HarnessOptions: map[string]string{"sandbox": "danger-full-access"}, Fallbacks: []agentdef.Fallback{{Harness: "codex", Model: "other"}},
		Skills: []agentdef.Skill{{Name: "context", Body: "Read sources"}},
	}
	got := ChatSpec(in)
	for _, scope := range []string{"repo:read", "repo:write", "tests:run", "wiki:write", "db:migrate", "deploy:staging", "custom:write", "future:scope"} {
		if got.Mode(scope) != agentdef.Denied {
			t.Errorf("%s is not denied", scope)
		}
	}
	if len(got.Setup)+len(got.MCPServers)+len(got.Fallbacks) != 0 || got.HarnessOptions["readonly"] != "true" || got.HarnessOptions["sandbox"] != "read-only" {
		t.Fatalf("unsafe chat spec: %+v", got)
	}
	if !strings.Contains(got.Instructions, "Agent persona") || !reflect.DeepEqual(got.Skills, in.Skills) {
		t.Fatal("lost persona or skills")
	}
	if in.Scopes[0].Mode != agentdef.Allowed || in.HarnessOptions["sandbox"] != "danger-full-access" || len(in.Fallbacks) != 1 {
		t.Fatal("mutated original definition")
	}
}
