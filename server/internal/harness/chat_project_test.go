package harness

import (
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

func TestChatAgentsMDNamesTheAttachedProject(t *testing.T) {
	with := string(ChatAgentsMD(agentdef.Resolved{}, nil, "PAY", "", nil))
	if !strings.Contains(with, "# Vesper chat: project PAY") {
		t.Fatalf("missing project heading:\n%s", with)
	}
	without := string(ChatAgentsMD(agentdef.Resolved{}, nil, "", "", nil))
	if strings.Contains(without, "project \n") || !strings.Contains(without, "No project is attached") {
		t.Fatalf("no-project heading:\n%s", without)
	}
}
