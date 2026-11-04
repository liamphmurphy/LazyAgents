package agentdef

import (
	"strings"
	"testing"
)

func openCodeSpec(model string) Spec {
	return Spec{Harness: HarnessOpenCode, Model: model, Instructions: "x"}
}

func TestOpenCodeSelectionValidation(t *testing.T) {
	for _, ok := range []string{"anthropic/claude-sonnet-5-5", "openai/gpt-5#high", "openai/gpt-5-codex"} {
		s := openCodeSpec(ok)
		if err := s.Normalize(); err != nil {
			t.Errorf("%s rejected: %v", ok, err)
		}
	}
	for model, want := range map[string]string{
		"sonnet-5.5":          "provider-qualified",
		"gpt-5":               "provider-qualified",
		"openai/":             "provider-qualified",
		"google/gemini-3":     "not supported",
		"anthropic/x#bad var": "provider-qualified",
	} {
		s := openCodeSpec(model)
		err := s.Normalize()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want %q, got %v", model, want, err)
		}
	}
	s := openCodeSpec("openai/gpt-5")
	s.HarnessOptions = map[string]map[string]string{"opencode": {"sandbox": "read-only"}}
	if err := s.Normalize(); err == nil || !strings.Contains(err.Error(), "sandbox") {
		t.Errorf("unsupported option: %v", err)
	}
	s.HarnessOptions = map[string]map[string]string{"opencode": {"readonly": "maybe"}}
	if err := s.Normalize(); err == nil {
		t.Error("readonly=maybe accepted")
	}
	s.HarnessOptions = map[string]map[string]string{"opencode": {"readonly": "true"}}
	if err := s.Normalize(); err != nil {
		t.Errorf("readonly rejected: %v", err)
	}
}

func TestOpenCodeFallbackValidation(t *testing.T) {
	s := Spec{Harness: HarnessClaudeCode, Model: "sonnet-5.5", Fallbacks: []Fallback{{Harness: HarnessOpenCode, Model: "sonnet-5.5"}}}
	if err := s.Normalize(); err == nil || !strings.Contains(err.Error(), "provider/model") {
		t.Errorf("unqualified fallback: %v", err)
	}
	s = Spec{Harness: HarnessClaudeCode, Model: "sonnet-5.5", Fallbacks: []Fallback{{Harness: HarnessOpenCode, Model: "openai/gpt-5", HarnessOptions: map[string]string{"sandbox": "x"}}}}
	if err := s.Normalize(); err == nil {
		t.Error("fallback option accepted")
	}
	s = Spec{Harness: HarnessOpenCode, Model: "openai/gpt-5", Fallbacks: []Fallback{{Harness: HarnessCodex, Model: "gpt-5-codex"}, {Harness: HarnessClaudeCode, Model: "opus-5.5"}}}
	if err := s.Normalize(); err != nil {
		t.Errorf("three-harness chain: %v", err)
	}
}

// Overrides can't turn a Claude model into an OpenCode one by switching the
// harness alone; the merge must still be runnable.
func TestOpenCodeOverrideAndResolved(t *testing.T) {
	base := Spec{Harness: HarnessClaudeCode, Model: "sonnet-5.5", Instructions: "x"}
	if err := base.Normalize(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOverride(base, Override{Harness: HarnessOpenCode}); err == nil {
		t.Error("harness-only override accepted")
	}
	if err := ValidateOverride(base, Override{Harness: HarnessOpenCode, Model: "anthropic/claude-sonnet-5-5"}); err != nil {
		t.Error(err)
	}
	if err := ValidateOverride(base, Override{Model: "opus-5.5"}); err != nil {
		t.Error(err) // non-OpenCode targets keep their existing freedom
	}
	r := Resolve(base, &Override{Harness: HarnessOpenCode, Model: "bad"})
	if err := r.Validate(); err == nil {
		t.Error("frozen-style invalid target passed Validate")
	}
	r = Resolve(openCodeSpec("openai/gpt-5"), nil)
	if r.Harness != HarnessOpenCode || r.Validate() != nil {
		t.Errorf("%+v", r)
	}
}

func TestOpenCodeCapabilitiesAreConservative(t *testing.T) {
	c := Capabilities[HarnessOpenCode]
	if c.ToolGating || c.PermissionPrompt || c.NativeCost || !c.Resume {
		t.Fatalf("%+v", c)
	}
	if _, how := EnforcementFor("tests:run", RequiresApproval, HarnessOpenCode); !strings.Contains(how, "can't pause") {
		t.Errorf("approval claimed enforceable on opencode: %s", how)
	}
}
