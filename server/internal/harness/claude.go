package harness

import (
	"encoding/json"
	"strings"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
)

// ClaudeCode runs `claude -p --output-format stream-json` (checked against
// Claude Code 2.1.316). Instructions come from AGENTS.md, MCP from a
// generated --mcp-config, tool rules from scopes.
type ClaudeCode struct{}

func (ClaudeCode) Name() string { return agentdef.HarnessClaudeCode }

const claudeMCPFile = ".lazyagents/mcp.json"

type mcpEntry struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (ClaudeCode) Invocation(ws Workspace, t Turn) (Invocation, error) {
	spec := t.Spec
	servers := map[string]mcpEntry{
		// The run token is written into the (git-excluded) config; it's
		// revoked when the run ends.
		lazyagentsServer: {Type: "http", URL: ws.MCPURL, Headers: map[string]string{"Authorization": "Bearer " + ws.Token}},
	}
	for _, m := range spec.MCPServers {
		servers[m.Name] = mcpEntry{Type: "http", URL: m.URL}
	}
	cfg, err := json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
	if err != nil {
		return Invocation{}, err
	}

	mode := spec.HarnessOptions["permission_mode"]
	if mode == "" {
		mode = "acceptEdits"
	}
	allow := []string{"mcp__" + lazyagentsServer, "Read", "Edit", "Write", "Glob", "Grep", "TodoWrite"}
	for _, m := range spec.MCPServers {
		allow = append(allow, "mcp__"+m.Name)
	}
	// Shell access backs tests and git; it's granted when either scope is.
	if allowed(spec, "tests:run") || allowed(spec, "repo:write") || allowed(spec, "repo:read") {
		allow = append(allow, "Bash")
	}
	var deny []string
	if denied(spec, "repo:write") {
		deny = append(deny, "Bash(git push:*)", "Bash(gh pr create:*)")
	}
	if denied(spec, "tests:run") && !allowed(spec, "repo:write") {
		deny = append(deny, "Bash")
	}

	argv := []string{"claude", "-p", t.Prompt,
		"--output-format", "stream-json", "--verbose",
		"--model", AnthropicModel(spec.Model),
		"--mcp-config", claudeMCPFile, "--strict-mcp-config",
		"--permission-mode", mode,
		// Nobody answers prompts headless: anything not allowed is denied
		// and the agent is told to use request_approval.
		"--permission-prompts", "none",
		"--allowedTools", strings.Join(allow, ","),
	}
	if len(deny) > 0 {
		argv = append(argv, "--disallowedTools", strings.Join(deny, ","))
	}
	if t.SessionID != "" {
		argv = append(argv, "--resume", t.SessionID)
	}
	return Invocation{
		Argv:  argv,
		Env:   map[string]string{ws.TokenEnv: ws.Token},
		Files: map[string][]byte{claudeMCPFile: cfg},
	}, nil
}

func (ClaudeCode) NewParser(string) Parser { return &claudeParser{tools: map[string]string{}} }

type claudeParser struct {
	res   Result
	tools map[string]string // tool_use id → name
}

type claudeLine struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	SessionID string          `json:"session_id"`
	Model     string          `json:"model"`
	Message   json.RawMessage `json:"message"`
	// result
	IsError    bool                            `json:"is_error"`
	Result     string                          `json:"result"`
	TotalCost  float64                         `json:"total_cost_usd"`
	NumTurns   int                             `json:"num_turns"`
	DurationMS int64                           `json:"duration_ms"`
	Terminal   string                          `json:"terminal_reason"`
	Usage      *claudeUsage                    `json:"usage"`
	Denials    []json.RawMessage               `json:"permission_denials"`
	MCPServers []struct{ Name, Status string } `json:"mcp_servers"`
}

type claudeUsage struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheReadTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func (p *claudeParser) Line(b []byte) []Event {
	var l claudeLine
	if err := json.Unmarshal(b, &l); err != nil || l.Type == "" {
		return nil // stderr noise or a partial line; the raw log keeps it
	}
	if l.SessionID != "" {
		p.res.SessionID = l.SessionID
	}
	switch l.Type {
	case "system":
		if l.Subtype != "init" {
			return nil
		}
		pl := map[string]any{"state": "harness_started", "harness": agentdef.HarnessClaudeCode, "model": l.Model, "session_id": l.SessionID}
		for _, m := range l.MCPServers {
			if m.Status != "connected" {
				return []Event{{"lifecycle", pl}, {"error", map[string]any{"message": "MCP server " + m.Name + " is " + m.Status}}}
			}
		}
		return []Event{{"lifecycle", pl}}
	case "assistant", "user":
		var msg struct {
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(l.Message, &msg) != nil {
			return nil
		}
		var blocks []claudeBlock
		if json.Unmarshal(msg.Content, &blocks) != nil {
			var s string // user messages can be a plain string
			if json.Unmarshal(msg.Content, &s) == nil && s != "" && l.Type == "assistant" {
				p.res.Final = s
				return []Event{{"message", map[string]any{"role": "assistant", "text": s}}}
			}
			return nil
		}
		var out []Event
		for _, bl := range blocks {
			switch bl.Type {
			case "text":
				if l.Type == "assistant" && strings.TrimSpace(bl.Text) != "" {
					p.res.Final = bl.Text
					out = append(out, Event{"message", map[string]any{"role": "assistant", "text": bl.Text}})
				}
			case "thinking":
				// Not shown: thinking stays in the raw log.
			case "tool_use":
				p.tools[bl.ID] = bl.Name
				p.res.Usage.ToolCalls++
				var input any
				_ = json.Unmarshal(bl.Input, &input)
				out = append(out, Event{"tool_call", map[string]any{"id": bl.ID, "tool": bl.Name, "input": input}})
			case "tool_result":
				out = append(out, Event{"tool_result", map[string]any{
					"id": bl.ToolUseID, "tool": p.tools[bl.ToolUseID], "is_error": bl.IsError, "output": clip(blockText(bl.Content), 4000),
				}})
			}
		}
		return out
	case "result":
		p.res.IsError = l.IsError
		if l.Result != "" && !l.IsError {
			p.res.Final = l.Result
		}
		if l.IsError {
			p.res.Error = l.Result
			if p.res.Error == "" {
				p.res.Error = l.Terminal
			}
		}
		if l.Usage != nil {
			p.res.Usage.InputTokens = l.Usage.InputTokens + l.Usage.CacheCreationTokens + l.Usage.CacheReadTokens
			p.res.Usage.CachedTokens = l.Usage.CacheReadTokens
			p.res.Usage.OutputTokens = l.Usage.OutputTokens
		}
		p.res.Usage.CostUSD, p.res.Usage.CostSource = l.TotalCost, "harness"
		out := []Event{{"usage", map[string]any{
			"input_tokens": p.res.Usage.InputTokens, "output_tokens": p.res.Usage.OutputTokens, "cached_tokens": p.res.Usage.CachedTokens,
			"cost_usd": l.TotalCost, "num_turns": l.NumTurns, "duration_ms": l.DurationMS,
		}}}
		for _, d := range l.Denials {
			var den struct {
				ToolName  string          `json:"tool_name"`
				ToolUseID string          `json:"tool_use_id"`
				Input     json.RawMessage `json:"tool_input"`
			}
			if json.Unmarshal(d, &den) == nil {
				var input any
				_ = json.Unmarshal(den.Input, &input)
				out = append(out, Event{"tool_call", map[string]any{"id": den.ToolUseID, "tool": den.ToolName, "input": input, "denied_by_harness": true}})
			}
		}
		if l.IsError {
			out = append(out, Event{"error", map[string]any{"message": p.res.Error}})
		}
		return out
	}
	return nil
}

// blockText flattens a tool_result's content (a string or text blocks).
func blockText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

func (p *claudeParser) Result() Result { return p.res }
