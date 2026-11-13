package runs

import (
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

// A refreshed segment takes the persona's new capabilities and instructions
// but keeps what it chose itself, and the model is told what changed.
func TestRefreshChatSpecAndNotice(t *testing.T) {
	have := agentdef.Resolved{Harness: agentdef.HarnessClaudeCode, Model: "m", Instructions: "old",
		MCPServers:     []agentdef.MCPServer{{Name: "linear", URL: "vesper", Connector: true}},
		HarnessOptions: map[string]string{"sandbox": "read-only", "readonly": "true", "effort": "high", agentdef.OptionWebSearch: "true"}}
	want := agentdef.Resolved{Harness: agentdef.HarnessClaudeCode, Model: "other", Instructions: "new",
		MCPServers:     []agentdef.MCPServer{{Name: "github", URL: "vesper", Connector: true}, {Name: agentdef.SecretsMCPName, URL: "vesper", Connector: true}},
		HarnessOptions: map[string]string{"sandbox": "read-only", "readonly": "true", agentdef.OptionWebSearch: "true"}}
	if !sameWorkspace(want, have) {
		t.Fatal("connectors and instructions don't need a new workspace")
	}
	got := refreshChatSpec(have, want)
	if got.Model != "m" || got.HarnessOptions["effort"] != "high" || got.Instructions != "new" || len(got.MCPServers) != 2 {
		t.Fatalf("refreshed spec %+v", got)
	}
	if have.HarnessOptions["effort"] != "high" || len(have.MCPServers) != 1 {
		t.Fatal("refresh changed the segment's old spec in place")
	}
	note := capabilityNotice(have, got)
	for _, part := range []string{"now use: github", "no longer use: linear", "API secrets", "instructions were updated"} {
		if !strings.Contains(note, part) {
			t.Errorf("notice %q lacks %q", note, part)
		}
	}
	if capabilityNotice(got, got) != "" {
		t.Error("an unchanged spec got a notice")
	}
	coding := want
	coding.HarnessOptions = map[string]string{"sandbox": "workspace-write"}
	if sameWorkspace(coding, have) {
		t.Error("turning code on must need a fresh segment")
	}
}

func TestToolNotice(t *testing.T) {
	if toolNotice(nil, []string{"a"}) != "" {
		t.Error("a segment that never recorded its tools can't be told what changed")
	}
	if toolNotice([]string{"a"}, []string{"a"}) != "" {
		t.Error("unchanged tools got a notice")
	}
	if n := toolNotice([]string{"a", "b"}, []string{"a", "c"}); !strings.Contains(n, "New: c.") || !strings.Contains(n, "Gone: b.") {
		t.Errorf("notice %q", n)
	}
}
