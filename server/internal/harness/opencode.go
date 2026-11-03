package harness

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
)

// HarnessOpenCode is the public harness id for the pinned OpenCode CLI. For
// returns a fail-closed adapter: the runner sets ConfigIsolated/SessionOwned
// only when it can honor them (docs/contracts/opencode2/ADR.md).
const HarnessOpenCode = agentdef.HarnessOpenCode

// OpenCodeVersion is the only release the contract covers (ADR.md).
const OpenCodeVersion = "2.2.1"

const (
	opencodeConfigFile = "opencode.json"
	opencodeStateDir   = ".lazyagents/opencode"
	opencodeSkillsDir  = ".opencode/skills"
	opencodeTokenEnv   = "LAZYAGENTS_RUN_TOKEN"
)

// OpenCode runs `opencode run --standalone --format json` (contract: 2.2.1).
// Instructions come from AGENTS.md, MCP and policy from a generated
// opencode.json, state from per-run HOME/XDG roots. It fails closed: anything
// the generated policy cannot enforce is rejected before a process starts.
type OpenCode struct {
	// ConfigIsolated attests that the runner suppresses system, ancestor and
	// repository config (managed config plus OS/container isolation). Without
	// it every run (ticket and read-only) is rejected before launch: XDG roots
	// alone do not protect the generated policy from other config layers.
	ConfigIsolated bool
	// SessionOwned reports whether a session id exists and belongs to this
	// run's project. Resuming without it is rejected, because `--session`
	// silently creates a missing session.
	SessionOwned func(id string) bool
	// Prices are exact per-million-token prices keyed by the full model id
	// (provider/model[#variant]). There is no built-in table: another
	// provider's matching model name must never price this one.
	Prices map[string]Price
}

func (OpenCode) Name() string { return HarnessOpenCode }

// OpenCodeCredentialEnv maps a provider-qualified model to the LazyAgents
// secret and environment variable that authenticate it. Only the selected
// provider's key may be injected into the turn.
func OpenCodeCredentialEnv(model string) (secret, env string, err error) {
	provider, err := opencodeProvider(model)
	if err != nil {
		return "", "", err
	}
	switch provider {
	case "anthropic":
		return "harness:anthropic", "ANTHROPIC_API_KEY", nil
	case "openai":
		return "harness:openai", "OPENAI_API_KEY", nil
	}
	return "", "", fmt.Errorf("harness: unsupported opencode provider %q", provider)
}

// opencodeProvider validates provider/model#variant and returns the provider.
// Models keep their provider and variant: no Claude or Codex aliasing applies.
func opencodeProvider(model string) (string, error) {
	p, err := agentdef.OpenCodeProvider(model)
	if err != nil {
		return "", fmt.Errorf("harness: %w", err)
	}
	return p, nil
}

// CheckOpenCodeVersion accepts `opencode --version` output only for the pin.
func CheckOpenCodeVersion(out string) error {
	if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "v")) != OpenCodeVersion {
		return fmt.Errorf("harness: opencode %s is required", OpenCodeVersion)
	}
	return nil
}

var (
	opencodeSession = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
	opencodeName    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
)

// policyStatement is one 2.2.1 policy/permission statement.
type policyStatement struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	Effect   string `json:"effect"`
}

// Actions the pinned 2.2.1 schema knows. The 2.2.3 split actions are ignored
// by 2.2.1, so passing them would be unenforced policy that looks enforced.
var opencodePolicyActions = map[string]bool{"provider.use": true, "tool.use": true, "integration.use": true}

func validatePolicy(ps []policyStatement) error {
	for _, p := range ps {
		if !opencodePolicyActions[p.Action] {
			return fmt.Errorf("harness: opencode policy action %q is unsupported on %s", p.Action, OpenCodeVersion)
		}
		if p.Effect != "allow" && p.Effect != "deny" || p.Resource == "" {
			return fmt.Errorf("harness: invalid opencode policy statement %+v", p)
		}
	}
	return nil
}

// opencodeOptions are the harness options this adapter understands.
func validateOpenCodeOptions(opts map[string]string) error {
	keys := make([]string, 0, len(opts))
	for k := range opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "readonly":
			if opts[k] != "true" && opts[k] != "false" {
				return fmt.Errorf("harness: opencode option readonly must be true or false")
			}
		default:
			// Includes mcp.use, skill.use, plugin.use, integration.connect,
			// permission_mode and sandbox: nothing here can honor them.
			return fmt.Errorf("harness: opencode does not support harness option %q", k)
		}
	}
	return nil
}

func (o OpenCode) Invocation(ws Workspace, t Turn) (Invocation, error) {
	spec := t.Spec
	provider, err := opencodeProvider(spec.Model)
	if err != nil {
		return Invocation{}, err
	}
	if err := validateOpenCodeOptions(spec.HarnessOptions); err != nil {
		return Invocation{}, err
	}
	if strings.TrimSpace(t.Prompt) == "" || strings.HasPrefix(strings.TrimSpace(t.Prompt), "-") {
		return Invocation{}, fmt.Errorf("harness: opencode prompt must be non-empty and must not start with '-'")
	}
	if !filepath.IsAbs(ws.Dir) {
		return Invocation{}, fmt.Errorf("harness: opencode needs an absolute workspace directory")
	}
	if ws.MCPURL == "" || !strings.HasPrefix(ws.MCPURL, "http") {
		return Invocation{}, fmt.Errorf("harness: invalid MCP URL")
	}
	readonly := spec.HarnessOptions["readonly"] == "true"
	if !o.ConfigIsolated {
		return Invocation{}, fmt.Errorf("harness: opencode runs need isolated managed config (system, ancestor and repo config suppressed), which this runner does not provide")
	}
	if readonly && len(spec.MCPServers) > 0 {
		return Invocation{}, fmt.Errorf("harness: opencode read-only runs cannot use external MCP servers")
	}
	for _, s := range spec.Scopes {
		if s.Mode != agentdef.RequiresApproval {
			continue
		}
		info := agentdef.Catalog[s.Scope]
		if !info.Brokered && !info.APIOnly {
			// No authenticated pre-tool bridge exists: a CLI prompt is not
			// LazyAgents approval.
			return Invocation{}, fmt.Errorf("harness: opencode cannot enforce approval for scope %q", s.Scope)
		}
	}
	for _, s := range spec.Skills {
		if !opencodeName.MatchString(s.Name) {
			return Invocation{}, fmt.Errorf("harness: opencode cannot stage skill %q", s.Name)
		}
	}
	cfg, err := o.config(ws, spec, provider, readonly)
	if err != nil {
		return Invocation{}, err
	}

	argv := []string{"opencode", "run", "--standalone", "--format", "json", "--model", spec.Model}
	if t.SessionID != "" {
		if !opencodeSession.MatchString(t.SessionID) {
			return Invocation{}, fmt.Errorf("harness: invalid opencode session id")
		}
		if o.SessionOwned == nil || !o.SessionOwned(t.SessionID) {
			// --session creates a missing session, silently losing context.
			return Invocation{}, fmt.Errorf("harness: opencode session %q is missing or not owned by this project", t.SessionID)
		}
		argv = append(argv, "--session", t.SessionID)
	}
	argv = append(argv, t.Prompt)

	state := filepath.Join(ws.Dir, opencodeStateDir)
	env := map[string]string{
		"PWD":             ws.Dir,
		"HOME":            filepath.Join(state, "home"),
		"XDG_CONFIG_HOME": filepath.Join(state, "config"),
		"XDG_DATA_HOME":   filepath.Join(state, "data"),
		"XDG_CACHE_HOME":  filepath.Join(state, "cache"),
		"XDG_STATE_HOME":  filepath.Join(state, "state"),
		// Referenced by the config as {env:LAZYAGENTS_RUN_TOKEN}; never argv.
		opencodeTokenEnv: ws.Token,
	}

	files := map[string][]byte{opencodeConfigFile: cfg}
	exclude := []string{"/" + opencodeConfigFile, "/" + opencodeStateDir + "/", "/" + opencodeSkillsDir + "/"}
	for _, s := range spec.Skills {
		files[opencodeSkillsDir+"/"+s.Name+"/SKILL.md"] = RenderSkill(s)
		// Inert copy that the AGENTS.md skills index points at.
		files[plainSkillsDir+"/"+s.Name+"/SKILL.md"] = RenderSkill(s)
		exclude = append(exclude, "/"+plainSkillsDir+"/"+s.Name+"/")
	}
	sort.Strings(exclude)
	files[SkillsExcludeFile] = []byte(strings.Join(exclude, "\n") + "\n")
	return Invocation{Argv: argv, Env: env, Files: files}, nil
}

// config renders the protected run-specific opencode.json.
func (OpenCode) config(ws Workspace, spec agentdef.Resolved, provider string, readonly bool) ([]byte, error) {
	servers := map[string]any{
		lazyagentsServer: map[string]any{
			"type": "remote", "url": ws.MCPURL, "oauth": false, "codemode": false,
			"headers": map[string]string{"Authorization": "Bearer {env:" + opencodeTokenEnv + "}"},
		},
	}
	policies := []policyStatement{
		{"tool.use", "*", "deny"},
		{"provider.use", "*", "deny"},
		{"provider.use", provider, "allow"},
		{"integration.use", "mcp:*", "deny"},
		{"integration.use", "mcp:" + lazyagentsServer, "allow"},
	}
	perms := []policyStatement{{"*", "*", "deny"}}
	allowTool := func(name string) {
		perms = append(perms, policyStatement{name, "*", "allow"})
		policies = append(policies, policyStatement{"tool.use", name, "allow"})
	}
	for _, t := range []string{"read", "glob", "grep", lazyagentsServer + "_*"} {
		allowTool(t)
	}
	for _, m := range spec.MCPServers {
		if !opencodeName.MatchString(m.Name) || m.Name == lazyagentsServer {
			return nil, fmt.Errorf("harness: opencode cannot use MCP server name %q", m.Name)
		}
		if !strings.HasPrefix(m.URL, "https://") && !strings.HasPrefix(m.URL, "http://") {
			return nil, fmt.Errorf("harness: invalid URL for MCP server %q", m.Name)
		}
		servers[m.Name] = map[string]any{"type": "remote", "url": m.URL, "oauth": false}
		policies = append(policies, policyStatement{"integration.use", "mcp:" + m.Name, "allow"})
		allowTool(m.Name + "_*")
	}
	if !readonly {
		allowTool("edit")
		allowTool("write")
		shell := spec.Mode("tests:run") != agentdef.Denied || spec.Mode("repo:write") != agentdef.Denied || spec.Mode("repo:read") != agentdef.Denied
		if shell {
			allowTool("bash")
			if denied(spec, "repo:write") {
				// Last match wins, so these follow the bash allow.
				perms = append(perms,
					policyStatement{"bash", "git push*", "deny"},
					policyStatement{"bash", "gh pr create*", "deny"})
			}
		}
	}
	policies = append(policies,
		policyStatement{"integration.use", "skill:*", "deny"},
		policyStatement{"integration.use", "plugin:*", "deny"})
	for _, s := range spec.Skills { // exact exceptions after the blanket deny
		policies = append(policies, policyStatement{"integration.use", "skill:" + s.Name, "allow"})
	}
	if err := validatePolicy(policies); err != nil {
		return nil, err
	}
	providers := map[string]any{}
	if provider == "openai" {
		providers["openai"] = map[string]any{"env": []string{"OPENAI_API_KEY"}}
	}
	return json.MarshalIndent(map[string]any{
		"providers":   providers,
		"mcp":         map[string]any{"servers": servers},
		"permissions": perms,
		"policies":    policies,
	}, "", "  ")
}
