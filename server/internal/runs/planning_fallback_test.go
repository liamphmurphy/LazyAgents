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

// OpenCode rejects unknown options, so its read-only planning spec (primary or
// fallback) carries only readonly; every other harness keeps its sandbox too.
func TestPlanningOpenCodeOptionsPassValidation(t *testing.T) {
	r := agentdef.Resolved{Harness: agentdef.HarnessOpenCode, Model: "openai/gpt-5", Fallbacks: []agentdef.Fallback{
		{Harness: agentdef.HarnessOpenCode, Model: "anthropic/claude-sonnet-5-5"}, {Harness: "codex", Model: "x"}}}
	p := PlanningSpec(r)
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(p.HarnessOptions) != 1 || p.HarnessOptions["readonly"] != "true" || p.Fallbacks[1].HarnessOptions["sandbox"] != "read-only" {
		t.Fatalf("%v / %v", p.HarnessOptions, p.Fallbacks)
	}
}
