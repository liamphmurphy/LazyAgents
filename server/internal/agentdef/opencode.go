package agentdef

import (
	"fmt"
	"regexp"
)

// OpenCode models are provider-qualified (provider/model[#variant]) and keep
// that form end to end: no Claude or Codex aliasing applies. Only providers
// with a mapped API-key credential are accepted (docs/contracts/opencode2/ADR.md).
var openCodeModel = regexp.MustCompile(`^([a-z][a-z0-9-]*)/([A-Za-z0-9][A-Za-z0-9._:/-]*)(#[A-Za-z0-9._-]+)?$`)

// OpenCodeProviders are the model providers a run can authenticate.
var OpenCodeProviders = []string{"anthropic", "openai"}

// OpenCodeProvider validates an OpenCode model id and returns its provider.
func OpenCodeProvider(model string) (string, error) {
	m := openCodeModel.FindStringSubmatch(model)
	if m == nil {
		return "", fmt.Errorf("opencode model %q must be provider-qualified: provider/model[#variant], e.g. anthropic/claude-sonnet-5-5", model)
	}
	for _, p := range OpenCodeProviders {
		if m[1] == p {
			return p, nil
		}
	}
	return "", fmt.Errorf("opencode provider %q is not supported (supported: anthropic, openai)", m[1])
}

// ValidateTarget checks a harness/model/options combination that a run could
// execute, as a definition, override, fallback or frozen spec.
func ValidateTarget(harness, model string, opts map[string]string) error {
	if harness != HarnessOpenCode {
		return nil
	}
	if _, err := OpenCodeProvider(model); err != nil {
		return err
	}
	for k, v := range opts {
		if k != "readonly" {
			return fmt.Errorf("opencode does not support harness option %q (supported: readonly)", k)
		}
		if v != "true" && v != "false" {
			return fmt.Errorf("opencode option readonly must be true or false")
		}
	}
	return nil
}

// ValidateOverride checks that an override still names a runnable target once
// merged over the agent's spec: for instance harness=opencode needs a
// provider-qualified model, which the agent's own claude model is not.
func ValidateOverride(s Spec, o Override) error {
	r := Resolve(s, &o)
	if err := ValidateTarget(r.Harness, r.Model, r.HarnessOptions); err != nil {
		return invalid("model", "%s", err.Error())
	}
	return nil
}

// Validate checks a resolved definition's primary and fallback targets.
func (r Resolved) Validate() error {
	if err := ValidateTarget(r.Harness, r.Model, r.HarnessOptions); err != nil {
		return err
	}
	for _, f := range r.Fallbacks {
		if err := ValidateTarget(f.Harness, f.Model, f.HarnessOptions); err != nil {
			return fmt.Errorf("fallback: %w", err)
		}
	}
	return nil
}
