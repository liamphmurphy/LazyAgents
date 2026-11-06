package agentapi

import "testing"

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
