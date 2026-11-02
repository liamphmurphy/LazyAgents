// Package agentdef defines what an agent is, independent of any harness
// (PLAN.md §6.1): the versioned definition, per-project overrides, the merge
// that resolves them, and how strongly each scope can be enforced (§6.3).
package agentdef

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// Mode is a scope's permission. Modes are ordered: a project may only move a
// scope rightwards (tighten), never leftwards.
type Mode string

const (
	Allowed          Mode = "allowed"
	RequiresApproval Mode = "requires_approval"
	Denied           Mode = "denied"
)

func (m Mode) rank() int {
	switch m {
	case Allowed:
		return 0
	case RequiresApproval:
		return 1
	case Denied:
		return 2
	}
	return -1
}

// Stricter returns the tighter of two modes.
func Stricter(a, b Mode) Mode {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

const (
	HarnessClaudeCode = "claude-code"
	HarnessCodex      = "codex"
)

var Harnesses = []string{HarnessClaudeCode, HarnessCodex}

// Budget is an optional runaway guard (no spend caps in v1). Zero = unset.
type Budget struct {
	MaxRuntime string `json:"max_runtime,omitempty"` // Go duration, e.g. "2h"
	MaxTurns   int    `json:"max_turns,omitempty"`
}

type MCPServer struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// Spec is one immutable definition version, stored as jsonb.
type Spec struct {
	Harness        string                       `json:"harness"`
	Model          string                       `json:"model"`
	Instructions   string                       `json:"instructions"`
	Scopes         map[string]Mode              `json:"scopes"`
	Budget         Budget                       `json:"budget"`
	MaxConcurrency int                          `json:"max_concurrency"`
	MCPServers     []MCPServer                  `json:"mcp_servers"`
	HarnessOptions map[string]map[string]string `json:"harness_options"`
	Memory         *Memory                      `json:"memory"` // nil (saved before memory existed) = DefaultMemory
}

// Memory says what wiki context a run gets (PLAN.md §12.3). Inject levels
// are cumulative: index (the agent's Memory page) → linked (plus pages
// linked to or from the ticket and its artifacts) → search (plus the top
// search hits for the ticket title).
type Memory struct {
	Enabled   bool   `json:"enabled"`
	Inject    string `json:"inject"`
	MaxTokens int    `json:"max_tokens"`
}

const (
	InjectIndex  = "index"
	InjectLinked = "linked"
	InjectSearch = "search"
)

// DefaultMemory applies when a definition doesn't say.
var DefaultMemory = Memory{Enabled: true, Inject: InjectLinked, MaxTokens: 4000}

// Override is a project's partial definition, merged over the agent's.
type Override struct {
	Harness            string          `json:"harness,omitempty"`
	Model              string          `json:"model,omitempty"`
	InstructionsAppend string          `json:"instructions_append,omitempty"`
	Scopes             map[string]Mode `json:"scopes,omitempty"`
	Budget             Budget          `json:"budget"`
	MCPServersAdd      []MCPServer     `json:"mcp_servers_add,omitempty"`
	Setup              []string        `json:"setup,omitempty"`
}

// Empty reports whether the override changes nothing.
func (o Override) Empty() bool {
	return o.Harness == "" && o.Model == "" && strings.TrimSpace(o.InstructionsAppend) == "" && len(o.Scopes) == 0 &&
		o.Budget == (Budget{}) && len(o.MCPServersAdd) == 0 && len(o.Setup) == 0
}

// The lazyagents MCP server is always injected by the runner; definitions
// can't remove or shadow it.
const ReservedMCPName = "lazyagents"

var (
	scopeRe = regexp.MustCompile(`^[a-z][a-z0-9-]*:([a-z0-9][a-z0-9-]*|\*)$`)
	nameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)
)

// ValidationError names the offending field (mapped to 422 by the API).
type ValidationError struct{ Field, Msg string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

func invalid(field, format string, a ...any) error {
	return &ValidationError{Field: field, Msg: fmt.Sprintf(format, a...)}
}

func validMode(field string, m Mode) error {
	if m.rank() < 0 {
		return invalid(field, "mode must be allowed, requires_approval or denied")
	}
	return nil
}

func validScopes(field string, scopes map[string]Mode) error {
	for s, m := range scopes {
		if !scopeRe.MatchString(s) {
			return invalid(field, "%q is not a scope like repo:write or prod:*", s)
		}
		if err := validMode(field+"."+s, m); err != nil {
			return err
		}
	}
	return nil
}

func validBudget(field string, b Budget) error {
	if b.MaxRuntime != "" {
		d, err := time.ParseDuration(b.MaxRuntime)
		if err != nil || d <= 0 {
			return invalid(field+".max_runtime", "must be a duration like 2h or 45m")
		}
	}
	if b.MaxTurns < 0 {
		return invalid(field+".max_turns", "can't be negative")
	}
	return nil
}

func validMCP(field string, servers []MCPServer) error {
	seen := map[string]bool{}
	for _, s := range servers {
		if !nameRe.MatchString(s.Name) {
			return invalid(field, "server name %q must be lowercase letters, digits, - or _", s.Name)
		}
		if s.Name == ReservedMCPName {
			return invalid(field, "%q is reserved for the built-in server", s.Name)
		}
		if seen[s.Name] {
			return invalid(field, "duplicate server %q", s.Name)
		}
		seen[s.Name] = true
		if !strings.HasPrefix(s.URL, "http://") && !strings.HasPrefix(s.URL, "https://") {
			return invalid(field, "server %q needs an http(s) URL", s.Name)
		}
	}
	return nil
}

// Normalize fills defaults and validates. Saved specs are always normalized.
func (s *Spec) Normalize() error {
	if s.Harness == "" {
		s.Harness = HarnessClaudeCode
	}
	if !slices.Contains(Harnesses, s.Harness) {
		return invalid("harness", "must be one of %s", strings.Join(Harnesses, ", "))
	}
	s.Model = strings.TrimSpace(s.Model)
	if s.Model == "" {
		return invalid("model", "is required")
	}
	if s.MaxConcurrency == 0 {
		s.MaxConcurrency = 1
	}
	if s.MaxConcurrency < 1 || s.MaxConcurrency > 16 {
		return invalid("max_concurrency", "must be between 1 and 16")
	}
	if s.Scopes == nil {
		s.Scopes = map[string]Mode{}
	}
	if s.MCPServers == nil {
		s.MCPServers = []MCPServer{}
	}
	if s.HarnessOptions == nil {
		s.HarnessOptions = map[string]map[string]string{}
	}
	for h := range s.HarnessOptions {
		if !slices.Contains(Harnesses, h) {
			return invalid("harness_options", "unknown harness %q", h)
		}
	}
	if err := validScopes("scopes", s.Scopes); err != nil {
		return err
	}
	if err := validBudget("budget", s.Budget); err != nil {
		return err
	}
	if s.Memory == nil {
		m := DefaultMemory
		s.Memory = &m
	}
	switch s.Memory.Inject {
	case "":
		s.Memory.Inject = InjectLinked
	case InjectIndex, InjectLinked, InjectSearch:
	default:
		return invalid("memory.inject", "must be index, linked or search")
	}
	if s.Memory.MaxTokens == 0 {
		s.Memory.MaxTokens = DefaultMemory.MaxTokens
	}
	if s.Memory.MaxTokens < 500 || s.Memory.MaxTokens > 50000 {
		return invalid("memory.max_tokens", "must be between 500 and 50000")
	}
	return validMCP("mcp_servers", s.MCPServers)
}

func (o *Override) Normalize() error {
	if o.Harness != "" && !slices.Contains(Harnesses, o.Harness) {
		return invalid("harness", "must be one of %s", strings.Join(Harnesses, ", "))
	}
	o.Model = strings.TrimSpace(o.Model)
	if err := validScopes("scopes", o.Scopes); err != nil {
		return err
	}
	if err := validBudget("budget", o.Budget); err != nil {
		return err
	}
	if err := validMCP("mcp_servers_add", o.MCPServersAdd); err != nil {
		return err
	}
	cleaned := o.Setup[:0]
	for _, c := range o.Setup {
		if c = strings.TrimSpace(c); c != "" {
			cleaned = append(cleaned, c)
		}
	}
	o.Setup = cleaned
	return nil
}

// sortedScopes returns scope names in a stable display order.
func sortedScopes(m map[string]Mode) []string {
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// MemoryOrDefault is the spec's memory settings, defaulted.
func (s Spec) MemoryOrDefault() Memory {
	if s.Memory == nil {
		return DefaultMemory
	}
	return *s.Memory
}
