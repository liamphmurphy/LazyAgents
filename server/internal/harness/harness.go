// Package harness adapts coding-agent CLIs to LazyAgents runs (PLAN.md
// §6.2): how to invoke one turn (argv, env, files to write) and how to read
// its event stream into normalized run events. Adapters know nothing about
// containers or the database; the runner does both.
package harness

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
)

// Workspace is where a turn runs, as seen from inside its environment.
type Workspace struct {
	Dir      string // absolute path of the checkout, e.g. /workspace
	MCPURL   string // the LazyAgents MCP endpoint, reachable from the turn
	Token    string // the run's agent token (la_…)
	TokenEnv string // env var that carries it, e.g. LAZYAGENTS_TOKEN
}

// Turn is one headless invocation.
type Turn struct {
	Spec      agentdef.Resolved
	Prompt    string
	SessionID string // resume this harness session when set
}

// Invocation is what the runner executes.
type Invocation struct {
	Argv  []string
	Env   map[string]string
	Files map[string][]byte // relative to Workspace.Dir; written before the turn
}

// Event is a normalized run event (run_events.type + payload).
type Event struct {
	Type    string // message | tool_call | tool_result | usage | error | lifecycle
	Payload map[string]any
}

// Usage is what one turn consumed.
type Usage struct {
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CachedTokens int64   `json:"cached_tokens"`
	ToolCalls    int64   `json:"tool_calls"`
	CostUSD      float64 `json:"cost_usd"`
	CostSource   string  `json:"cost_source"` // harness | computed | ""
}

// Result is what a parser learned by the end of a turn.
type Result struct {
	SessionID string
	Usage     Usage
	IsError   bool
	Error     string
	Final     string // the last assistant text
}

// Parser reads one turn's stream, line by line.
type Parser interface {
	Line(b []byte) []Event
	Result() Result
}

// Harness is one CLI adapter.
type Harness interface {
	Name() string
	Invocation(ws Workspace, t Turn) (Invocation, error)
	NewParser(model string) Parser
}

// For returns the adapter for a resolved harness name.
func For(name string) (Harness, error) {
	switch name {
	case agentdef.HarnessClaudeCode:
		return ClaudeCode{}, nil
	case agentdef.HarnessCodex:
		return Codex{}, nil
	}
	return nil, fmt.Errorf("harness: unknown harness %q", name)
}

// ---------------------------------------------------------------- shared

const lazyagentsServer = agentdef.ReservedMCPName

// clip shortens long tool output for the event stream; the raw log keeps it all.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + fmt.Sprintf("… (%d more bytes)", len(s)-cut)
}

var claudeModel = regexp.MustCompile(`^(opus|sonnet|haiku|fable)-(\d+)\.(\d+)$`)

// AnthropicModel maps definition names ("opus-5.5") to API model ids
// ("claude-opus-5-5"); anything else passes through.
func AnthropicModel(m string) string {
	if p := claudeModel.FindStringSubmatch(m); p != nil {
		return "claude-" + p[1] + "-" + p[2] + "-" + p[3]
	}
	return m
}

// AgentsMD renders the instructions file for every harness (PLAN.md §6.2).
// existing is the repo's own AGENTS.md, kept outside our marked section.
func AgentsMD(existing string, spec agentdef.Resolved, contextFiles []string, ticket string) []byte {
	return agentsMD(existing, spec, contextFiles, ticket, false)
}

// SubagentAgentsMD is AgentsMD for a run delegated by a main agent: it works
// in the parent's checkout, doesn't claim the ticket, and reports through
// complete_run.
func SubagentAgentsMD(existing string, spec agentdef.Resolved, contextFiles []string, ticket string) []byte {
	return agentsMD(existing, spec, contextFiles, ticket, true)
}

func agentsMD(existing string, spec agentdef.Resolved, contextFiles []string, ticket string, delegated bool) []byte {
	var b strings.Builder
	b.WriteString(sectionStart + "\n")
	b.WriteString("# LazyAgents run: " + ticket + "\n\n")
	if delegated {
		b.WriteString("You're a sub agent running headless for LazyAgents. A main agent delegated one task to you and is waiting. ")
		b.WriteString("Don't call claim_ticket. Work in this checkout on the current branch, commit what you change, and call complete_run ")
		b.WriteString("with a short summary when you're done (or blocked). If you need a person, call request_approval or await_human and end your turn.\n\n")
	} else {
		b.WriteString("You're running headless for LazyAgents. Use the `lazyagents` MCP tools to report progress: claim_ticket first, ")
		b.WriteString("post_plan before editing, update_step as you go, request_approval before any scope that needs it, and complete_run at the end. ")
		b.WriteString("When you need a person (an approval, an answer), call request_approval or await_human and then end your turn: ")
		b.WriteString("you'll be resumed with their reply.\n\n")
		b.WriteString(delegationSection(spec))
	}
	b.WriteString("## Instructions\n\n" + strings.TrimSpace(spec.Instructions) + "\n\n")
	if len(spec.Scopes) > 0 {
		b.WriteString("## Scopes\n\n")
		for _, s := range spec.Scopes {
			fmt.Fprintf(&b, "- `%s`: %s\n", s.Scope, strings.ReplaceAll(string(s.Mode), "_", " "))
		}
		b.WriteString("\n")
	}
	if len(spec.Skills) > 0 && spec.Harness != agentdef.HarnessClaudeCode {
		b.WriteString(skillsIndex(spec.Skills))
	}
	if len(contextFiles) > 0 {
		b.WriteString("## Context\n\nRead these before planning. They're also available through `read_artifact` and `read_page`, which record citations.\n\n")
		sort.Strings(contextFiles)
		for _, f := range contextFiles {
			b.WriteString("- `" + f + "`\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Keep notes worth remembering with `write_note` under your memory folder, linked with [[…]] to what they're about.\n")
	b.WriteString(sectionEnd + "\n")
	ours := b.String()
	if i, j := strings.Index(existing, sectionStart), strings.Index(existing, sectionEnd); i >= 0 && j > i {
		return []byte(existing[:i] + ours + strings.TrimPrefix(existing[j+len(sectionEnd):], "\n"))
	}
	if strings.TrimSpace(existing) == "" {
		return []byte(ours)
	}
	return []byte(ours + "\n" + existing)
}

const (
	sectionStart = "<!-- lazyagents:start (generated for this run; edits are replaced) -->"
	sectionEnd   = "<!-- lazyagents:end -->"
)

func allowed(spec agentdef.Resolved, scope string) bool {
	for _, s := range spec.Scopes {
		if s.Scope == scope {
			return s.Mode == agentdef.Allowed
		}
	}
	return false
}

func denied(spec agentdef.Resolved, scope string) bool {
	for _, s := range spec.Scopes {
		if s.Scope == scope {
			return s.Mode == agentdef.Denied
		}
	}
	return true // unknown scopes are denied (agentdef.ModeFor)
}

// delegationSection tells a main agent it can hand work to sub agents.
func delegationSection(agentdef.Resolved) string {
	return "If sub agents exist (`list_subagents`), you may hand them self-contained parts of the ticket such as implementation or documentation with " +
		"`delegate`. You wait while one works in this same checkout, then get its summary; review the diff before relying on it. " +
		"You stay responsible for the ticket: only you call claim_ticket and complete_run for it.\n\n"
}
