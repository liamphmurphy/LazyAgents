package agentapi

import (
	"slices"
	"testing"
)

// A renamed tool must not silently drop out of a profile.
func TestProfileToolsExist(t *testing.T) {
	for name, list := range map[string][]string{"chat": chatTools, "planning": planningTools, "subagent": subagentTools, "lead hidden": leadHidden} {
		for _, n := range list {
			if _, ok := ByName[n]; !ok {
				t.Errorf("%s profile lists %q, which isn't in Tools", name, n)
			}
		}
	}
}

// Board writes are lead-only (ADR Kanban board, Decision 6): chat, planning
// and delegated runs are neither served nor allowed them.
func TestBoardToolsLeadOnly(t *testing.T) {
	for _, tool := range []string{"estimate_ticket", "check_epic_criterion"} {
		if _, ok := ByName[tool]; !ok {
			t.Fatalf("%s isn't in Tools", tool)
		}
		for name, list := range map[string][]string{"chat": chatTools, "planning": planningTools, "subagent": subagentTools, "lead hidden": leadHidden} {
			if slices.Contains(list, tool) {
				t.Errorf("%s profile allows %s", name, tool)
			}
		}
		for _, p := range []string{profileChat, profilePlanning, profileSubagent} {
			if slices.ContainsFunc(profileTools(p), func(tl Tool) bool { return tl.Name == tool }) {
				t.Errorf("%s profile serves %s", p, tool)
			}
		}
		if !slices.ContainsFunc(profileTools(profileLead), func(tl Tool) bool { return tl.Name == tool }) {
			t.Errorf("lead profile doesn't serve %s", tool)
		}
	}
	// get_ticket (with the estimate and epic payload) stays on every profile.
	for name, list := range map[string][]string{"chat": chatTools, "planning": planningTools, "subagent": subagentTools} {
		if !slices.Contains(list, "get_ticket") {
			t.Errorf("%s profile lost get_ticket", name)
		}
	}
}
