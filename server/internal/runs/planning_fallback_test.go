package runs

import (
	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"testing"
)

func TestPlanningFallbacksStayReadOnly(t *testing.T) {
	original := agentdef.Resolved{Fallbacks: []agentdef.Fallback{{Harness: "codex", Model: "test", HarnessOptions: map[string]string{"sandbox": "danger-full-access"}}}}
	planned := PlanningSpec(original)
	if planned.Fallbacks[0].HarnessOptions["sandbox"] != "read-only" || planned.Fallbacks[0].HarnessOptions["readonly"] != "true" {
		t.Fatal("fallback could escape planning restrictions")
	}
	if original.Fallbacks[0].HarnessOptions["sandbox"] != "danger-full-access" {
		t.Fatal("planning mutated original fallback")
	}
}
