package agentdef

// Caps is what LazyAgents can enforce or observe on a harness (§6.2).
type Caps struct {
	ToolGating       bool `json:"tool_gating"`       // allow/deny individual tools
	PermissionPrompt bool `json:"permission_prompt"` // route permission prompts to LazyAgents
	NativeCost       bool `json:"native_cost"`       // harness reports cost itself
	Resume           bool `json:"resume"`            // resume a session by id
}

// Capabilities per harness. Codex gates by sandbox mode, not per tool.
var Capabilities = map[string]Caps{
	HarnessClaudeCode: {ToolGating: true, PermissionPrompt: true, NativeCost: true, Resume: true},
	HarnessCodex:      {ToolGating: false, PermissionPrompt: false, NativeCost: false, Resume: true},
}

// Enforcement is how strongly a scope holds (§6.3), strongest first:
//   - credential: the agent only gets the secret when the scope is allowed;
//   - brokered:   LazyAgents performs the action (run_gated) after approval;
//   - harness:    best-effort tool rules in the harness;
//   - none:       nothing stops it beyond instructions (e.g. Codex tool rules).
type Enforcement string

const (
	ByCredential Enforcement = "credential"
	ByBroker     Enforcement = "brokered"
	ByHarness    Enforcement = "harness"
	ByNothing    Enforcement = "none"
)

// ScopeInfo describes a well-known scope. Unknown scopes are allowed in
// definitions; they're enforced by the harness only.
type ScopeInfo struct {
	Description string `json:"description"`
	// Secret is the credential this scope gates, if any (workspace secrets
	// grant scopes; Phase 4b). Empty = no credential exists for it.
	Secret string `json:"secret,omitempty"`
	// Brokered scopes can run server-side through run_gated after approval.
	Brokered bool `json:"brokered"`
	// APIOnly scopes cover actions that only exist in the LazyAgents API
	// (wiki edits), so the server enforces every mode itself.
	APIOnly bool `json:"api_only,omitempty"`
}

var Catalog = map[string]ScopeInfo{
	"repo:read":       {Description: "Clone and read the project repo", Secret: "git:pat"},
	"repo:write":      {Description: "Push branches, open PRs", Secret: "git:pat"},
	"tests:run":       {Description: "Run the test suite"},
	"db:migrate":      {Description: "Apply migrations to staging", Secret: "db:staging", Brokered: true},
	"db:read-staging": {Description: "Query the staging snapshot", Secret: "db:staging-readonly"},
	"deploy:staging":  {Description: "Trigger a staging deploy", Secret: "deploy:staging", Brokered: true},
	"wiki:write":      {Description: "Edit wiki pages outside the agent's own memory", APIOnly: true},
}

// EnforcementFor says how a resolved scope is held on a harness, with a
// sentence for the UI's "How" column.
func EnforcementFor(scope string, mode Mode, harness string) (Enforcement, string) {
	info := Catalog[scope]
	caps := Capabilities[harness]
	switch {
	case info.APIOnly && mode == Allowed:
		return ByBroker, "Edits go through LazyAgents and publish directly"
	case info.APIOnly && mode == RequiresApproval:
		return ByBroker, "Edits are saved as suggestions a person accepts"
	case info.APIOnly:
		return ByBroker, "Only the agent's own memory pages can be written"
	case mode == RequiresApproval && info.Brokered:
		return ByBroker, "Runs server-side after a person approves"
	case mode == RequiresApproval:
		if caps.PermissionPrompt {
			return ByHarness, "Each use prompts for approval through LazyAgents"
		}
		return ByNothing, "This harness can't pause for approval; the agent is told to ask first"
	case info.Secret != "" && mode == Denied:
		return ByCredential, "The " + info.Secret + " secret is never injected"
	case info.Secret != "":
		return ByCredential, "The " + info.Secret + " secret is injected into the container"
	case caps.ToolGating && mode == Denied:
		return ByHarness, "Matching tools are disallowed"
	case caps.ToolGating:
		return ByHarness, "Matching tools are allowed"
	default:
		return ByNothing, "Sandbox mode only; the agent is instructed"
	}
}
