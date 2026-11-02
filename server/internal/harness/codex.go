package harness

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
)

// Codex runs `codex exec --json` (checked against codex-cli 0.166.0).
// Codex reads AGENTS.md itself; MCP servers and policy go in -c overrides.
// It authenticates headless with CODEX_API_KEY.
type Codex struct{}

func (Codex) Name() string { return agentdef.HarnessCodex }

func tomlString(s string) string { return strconv.Quote(s) }

func (Codex) Invocation(ws Workspace, t Turn) (Invocation, error) {
	spec := t.Spec
	sandbox := spec.HarnessOptions["sandbox"]
	if sandbox == "" {
		sandbox = "workspace-write"
	}
	opts := []string{"--json", "--skip-git-repo-check"}
	if spec.Model != "" {
		opts = append(opts, "-m", spec.Model)
	}
	cfg := []string{
		"approval_policy=" + tomlString("never"), // nobody answers prompts headless
		"sandbox_mode=" + tomlString(sandbox),
		"mcp_servers." + lazyagentsServer + ".url=" + tomlString(ws.MCPURL),
		"mcp_servers." + lazyagentsServer + ".bearer_token_env_var=" + tomlString(ws.TokenEnv),
	}
	for _, m := range spec.MCPServers {
		cfg = append(cfg, "mcp_servers."+m.Name+".url="+tomlString(m.URL))
	}
	// Pushing and opening PRs needs the network, which workspace-write
	// blocks by default.
	if sandbox == "workspace-write" && allowed(spec, "repo:write") {
		cfg = append(cfg, "sandbox_workspace_write.network_access=true")
	}
	for _, c := range cfg {
		opts = append(opts, "-c", c)
	}
	argv := []string{"codex", "exec"}
	if t.SessionID != "" {
		argv = append(argv, "resume")
		argv = append(argv, opts...)
		argv = append(argv, t.SessionID, t.Prompt)
	} else {
		argv = append(argv, opts...)
		argv = append(argv, t.Prompt)
	}
	return Invocation{Argv: argv, Env: map[string]string{ws.TokenEnv: ws.Token}}, nil
}

func (Codex) NewParser(model string) Parser { return &codexParser{model: model} }

type codexParser struct {
	model string
	res   Result
}

type codexItem struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Message          string          `json:"message"`
	Command          string          `json:"command"`
	AggregatedOutput string          `json:"aggregated_output"`
	ExitCode         *int            `json:"exit_code"`
	Status           string          `json:"status"`
	Server           string          `json:"server"`
	Tool             string          `json:"tool"`
	Arguments        json.RawMessage `json:"arguments"`
	Result           json.RawMessage `json:"result"`
	Error            json.RawMessage `json:"error"`
	Changes          []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"changes"`
	Query string `json:"query"`
}

type codexLine struct {
	Type     string     `json:"type"`
	ThreadID string     `json:"thread_id"`
	Message  string     `json:"message"`
	Item     *codexItem `json:"item"`
	Usage    *struct {
		InputTokens       int64 `json:"input_tokens"`
		CachedInputTokens int64 `json:"cached_input_tokens"`
		OutputTokens      int64 `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (p *codexParser) Line(b []byte) []Event {
	var l codexLine
	if err := json.Unmarshal(b, &l); err != nil || l.Type == "" {
		return nil
	}
	switch l.Type {
	case "thread.started":
		p.res.SessionID = l.ThreadID
		return []Event{{"lifecycle", map[string]any{"state": "harness_started", "harness": agentdef.HarnessCodex, "model": p.model, "session_id": l.ThreadID}}}
	case "turn.completed":
		if l.Usage != nil {
			p.res.Usage.InputTokens += l.Usage.InputTokens
			p.res.Usage.CachedTokens += l.Usage.CachedInputTokens
			p.res.Usage.OutputTokens += l.Usage.OutputTokens
		}
		cost, known := ComputeCost(p.model, p.res.Usage)
		if known {
			p.res.Usage.CostUSD, p.res.Usage.CostSource = cost, "computed"
		}
		return []Event{{"usage", map[string]any{
			"input_tokens": p.res.Usage.InputTokens, "output_tokens": p.res.Usage.OutputTokens,
			"cached_tokens": p.res.Usage.CachedTokens, "cost_usd": p.res.Usage.CostUSD, "cost_source": p.res.Usage.CostSource,
		}}}
	case "turn.failed":
		msg := "turn failed"
		if l.Error != nil {
			msg = l.Error.Message
		}
		p.res.IsError, p.res.Error = true, msg
		return []Event{{"error", map[string]any{"message": msg}}}
	case "error":
		// Transient ("Reconnecting… 2/5") errors are noise until the last.
		if strings.HasPrefix(l.Message, "Reconnecting") {
			return nil
		}
		p.res.Error = l.Message
		return []Event{{"error", map[string]any{"message": l.Message}}}
	case "item.started", "item.completed":
		if l.Item == nil {
			return nil
		}
		return p.item(l.Type == "item.completed", *l.Item)
	}
	return nil
}

func (p *codexParser) item(done bool, it codexItem) []Event {
	switch it.Type {
	case "agent_message":
		if !done || strings.TrimSpace(it.Text) == "" {
			return nil
		}
		p.res.Final = it.Text
		return []Event{{"message", map[string]any{"role": "assistant", "text": it.Text}}}
	case "command_execution":
		if !done {
			p.res.Usage.ToolCalls++
			return []Event{{"tool_call", map[string]any{"id": it.ID, "tool": "shell", "input": map[string]any{"command": it.Command}}}}
		}
		exit := 0
		if it.ExitCode != nil {
			exit = *it.ExitCode
		}
		return []Event{{"tool_result", map[string]any{"id": it.ID, "tool": "shell", "is_error": exit != 0 || it.Status == "failed",
			"exit_code": exit, "output": clip(it.AggregatedOutput, 4000)}}}
	case "mcp_tool_call":
		tool := it.Tool
		if it.Server != "" && it.Server != lazyagentsServer {
			tool = it.Server + "." + it.Tool
		}
		if !done {
			p.res.Usage.ToolCalls++
			var args any
			_ = json.Unmarshal(it.Arguments, &args)
			return []Event{{"tool_call", map[string]any{"id": it.ID, "tool": tool, "input": args}}}
		}
		out := string(it.Result)
		if len(it.Error) > 0 && string(it.Error) != "null" {
			out = string(it.Error)
		}
		return []Event{{"tool_result", map[string]any{"id": it.ID, "tool": tool, "is_error": it.Status == "failed", "output": clip(out, 4000)}}}
	case "file_change":
		if !done {
			return nil
		}
		files := make([]string, len(it.Changes))
		for i, c := range it.Changes {
			files[i] = c.Kind + " " + c.Path
		}
		p.res.Usage.ToolCalls++
		return []Event{{"tool_call", map[string]any{"id": it.ID, "tool": "apply_patch", "input": map[string]any{"files": files}, "status": it.Status}}}
	case "web_search":
		if done {
			p.res.Usage.ToolCalls++
			return []Event{{"tool_call", map[string]any{"id": it.ID, "tool": "web_search", "input": map[string]any{"query": it.Query}}}}
		}
	case "error":
		if done {
			return []Event{{"error", map[string]any{"message": it.Message}}}
		}
	}
	return nil // reasoning, todo lists: raw log only
}

func (p *codexParser) Result() Result { return p.res }

// ---------------------------------------------------------------- prices

// Price is USD per million tokens.
type Price struct{ Input, CachedInput, Output float64 }

// Prices is the model_prices table for harnesses that don't report cost
// (Codex). Unknown models leave cost unset rather than guessing; add rows
// as models change.
var Prices = map[string]Price{
	"gpt-5":         {Input: 1.25, CachedInput: 0.125, Output: 10},
	"gpt-5-codex":   {Input: 1.25, CachedInput: 0.125, Output: 10},
	"gpt-5-mini":    {Input: 0.25, CachedInput: 0.025, Output: 2},
	"gpt-5.1-codex": {Input: 1.25, CachedInput: 0.125, Output: 10},
}

// ComputeCost prices usage for a model; known is false for unknown models.
func ComputeCost(model string, u Usage) (cost float64, known bool) {
	pr, ok := Prices[model]
	if !ok {
		return 0, false
	}
	uncached := u.InputTokens - u.CachedTokens
	if uncached < 0 {
		uncached = 0
	}
	return (float64(uncached)*pr.Input + float64(u.CachedTokens)*pr.CachedInput + float64(u.OutputTokens)*pr.Output) / 1e6, true
}
