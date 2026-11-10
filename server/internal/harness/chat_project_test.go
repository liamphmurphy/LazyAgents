package harness

import (
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

func TestChatAgentsMDNamesTheDefaultProject(t *testing.T) {
	with := string(ChatAgentsMD(agentdef.Resolved{}, nil, "PAY", "", nil))
	if !strings.Contains(with, "# Vesper chat: default project PAY") || !strings.Contains(with, "unless you pass another project key") {
		t.Fatalf("missing default project:\n%s", with)
	}
	without := string(ChatAgentsMD(agentdef.Resolved{}, nil, "", "", nil))
	if strings.Contains(without, "project \n") || !strings.Contains(without, "no default project") || strings.Contains(without, "attach a project") {
		t.Fatalf("no-project preamble:\n%s", without)
	}
	for _, md := range []string{with, without} {
		if !strings.Contains(md, "in any project") {
			t.Fatalf("reads aren't described as workspace-wide:\n%s", md)
		}
	}
}
