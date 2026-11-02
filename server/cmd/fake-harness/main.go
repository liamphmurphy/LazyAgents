// Command fake-harness stands in for the `claude` and `codex` CLIs in runner
// tests and demos without API keys (PLAN.md §10). Invoked as `claude -p …`
// it emits Claude's stream-json and reads the MCP server from --mcp-config;
// invoked as `codex exec [resume] … --json` it emits Codex's JSONL and reads
// the server from -c mcp_servers.lazyagents.* overrides. Either way it does
// real work through the LazyAgents MCP server.
//
// FAKE_HARNESS_MODE picks the first turn's script:
//
//	approval (default)  claim, plan, request db:migrate approval, end turn
//	complete            claim, plan, complete_run
//	idle                end the turn without saying what's next
//	crash               exit 3 with an error result
//	slow                wait for SIGINT (pause), then end the turn
//
// Resumed turns (--resume) finish the run with complete_run, unless
// FAKE_HARNESS_RESUME=idle.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", b.token)
	return http.DefaultTransport.RoundTrip(r)
}

var (
	session string
	codex   bool // speak Codex's JSONL instead of Claude's stream-json
	items   int
)

func emit(v map[string]any) {
	if !codex {
		v["session_id"] = session
	}
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

func item(event string, it map[string]any) {
	emit(map[string]any{"type": event, "item": it})
}

func text(s string) {
	if codex {
		items++
		item("item.completed", map[string]any{"id": fmt.Sprintf("item_%d", items), "type": "agent_message", "text": s})
		return
	}
	emit(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": s}}}})
}

func result(isError bool, msg string) {
	if codex {
		if isError {
			emit(map[string]any{"type": "turn.failed", "error": map[string]any{"message": msg}})
			return
		}
		emit(map[string]any{"type": "turn.completed", "usage": map[string]any{"input_tokens": 1000, "cached_input_tokens": 500, "output_tokens": 200}})
		return
	}
	emit(map[string]any{"type": "result", "subtype": "success", "is_error": isError, "result": msg, "num_turns": 3,
		"total_cost_usd": 0.0125, "usage": map[string]any{"input_tokens": 1000, "output_tokens": 200, "cache_read_input_tokens": 500}})
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func main() {
	var prompt, resume, cfgPath string
	var mcpURL, tokenEnv string
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "exec" {
		codex = true
		args = args[1:]
		isResume := len(args) > 0 && args[0] == "resume"
		if isResume {
			args = args[1:]
		}
		var pos []string
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "-c":
				kv := args[i+1]
				i++
				k, v, _ := strings.Cut(kv, "=")
				v = strings.Trim(v, `"`)
				switch k {
				case "mcp_servers.lazyagents.url":
					mcpURL = v
				case "mcp_servers.lazyagents.bearer_token_env_var":
					tokenEnv = v
				}
			case "-m", "-s", "-C":
				i++
			default:
				if !strings.HasPrefix(args[i], "-") {
					pos = append(pos, args[i])
				}
			}
		}
		if isResume && len(pos) == 2 {
			resume, prompt = pos[0], pos[1]
		} else if len(pos) > 0 {
			prompt = pos[len(pos)-1]
		}
	}
	for i := 0; !codex && i < len(args); i++ {
		switch args[i] {
		case "-p", "--print":
			if i+1 < len(args) {
				prompt = args[i+1]
				i++
			}
		case "--resume":
			resume = args[i+1]
			i++
		case "--mcp-config":
			cfgPath = args[i+1]
			i++
		}
	}
	session = resume
	if session == "" {
		session = newUUID()
	}
	if codex {
		emit(map[string]any{"type": "thread.started", "thread_id": session})
		emit(map[string]any{"type": "turn.started"})
	} else {
		emit(map[string]any{"type": "system", "subtype": "init", "model": "fake", "mcp_servers": []any{map[string]any{"name": "lazyagents", "status": "connected"}}})
	}

	var cfg struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	var err error
	if codex {
		cfg.MCPServers = map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		}{"lazyagents": {URL: mcpURL, Headers: map[string]string{"Authorization": "Bearer " + os.Getenv(tokenEnv)}}}
	} else {
		var raw []byte
		raw, err = os.ReadFile(cfgPath)
		if err == nil {
			err = json.Unmarshal(raw, &cfg)
		}
	}
	srv := cfg.MCPServers["lazyagents"]
	if err != nil || srv.URL == "" {
		result(true, "no lazyagents MCP server configured")
		os.Exit(2)
	}
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "fake-harness", Version: "1"}, nil)
	sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: srv.URL, HTTPClient: &http.Client{Transport: bearer{srv.Headers["Authorization"]}}, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		result(true, "connect: "+err.Error())
		os.Exit(2)
	}
	defer sess.Close()

	n := 0
	call := func(name string, a map[string]any) map[string]any {
		n++
		id := fmt.Sprintf("toolu_%02d", n)
		if codex {
			items++
			id = fmt.Sprintf("item_%d", items)
			item("item.started", map[string]any{"id": id, "type": "mcp_tool_call", "server": "lazyagents", "tool": name, "arguments": a, "status": "in_progress"})
		} else {
			emit(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "id": id, "name": "mcp__lazyagents__" + name, "input": a}}}})
		}
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: a})
		out := map[string]any{}
		body, isErr := "", false
		switch {
		case err != nil:
			body, isErr = err.Error(), true
		case res.IsError:
			body, isErr = res.Content[0].(*mcp.TextContent).Text, true
		default:
			b, _ := json.Marshal(res.StructuredContent)
			body = string(b)
			_ = json.Unmarshal(b, &out)
		}
		if codex {
			status := "completed"
			if isErr {
				status = "failed"
			}
			item("item.completed", map[string]any{"id": id, "type": "mcp_tool_call", "server": "lazyagents", "tool": name, "arguments": a,
				"result": map[string]any{"content": []any{map[string]any{"type": "text", "text": body}}}, "status": status})
		} else {
			emit(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": id, "content": body, "is_error": isErr}}}})
		}
		return out
	}
	ticket := os.Getenv("LAZYAGENTS_TICKET")
	if os.Getenv("FAKE_HARNESS_MODE") == "planning" {
		who := call("whoami", map[string]any{})
		call("get_ticket", map[string]any{"ticket": "PAY-214"})
		call("propose_tickets", map[string]any{"tickets": []any{map[string]any{"title": "Planned implementation", "description": "An editable draft from the fake harness", "criteria": []string{"Works safely"}, "agent_id": who["id"]}}})
		result(false, "Draft submitted.")
		return
	}

	if resume != "" && os.Getenv("FAKE_HARNESS_RESUME") == "idle" {
		text("Still looking around.")
		result(false, "Still looking around.")
		return
	}
	if resume != "" {
		text("Resumed: " + firstLine(prompt))
		status := "in_review"
		if strings.HasPrefix(prompt, "Denied") {
			status = "todo"
		}
		call("complete_run", map[string]any{"outcome": "Finished after: " + firstLine(prompt), "status": status})
		result(false, "Done.")
		return
	}

	switch os.Getenv("FAKE_HARNESS_MODE") {
	case "crash":
		text("Something went wrong.")
		result(true, "API Error: overloaded")
		os.Exit(3)
	case "idle":
		text("I looked around and stopped.")
		result(false, "I looked around and stopped.")
		return
	case "slow":
		call("claim_ticket", map[string]any{"ticket": ticket})
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sig:
			text("Interrupted; stopping here.")
			result(false, "Interrupted")
			os.Exit(130)
		case <-time.After(60 * time.Second):
			result(false, "timed out")
			return
		}
	}

	call("claim_ticket", map[string]any{"ticket": ticket})
	call("post_plan", map[string]any{"steps": []string{"Read context", "Make the change", "Apply to staging"}})
	if os.Getenv("FAKE_HARNESS_MODE") == "complete" {
		call("complete_run", map[string]any{"outcome": "Done in one turn"})
		result(false, "Done.")
		return
	}
	ap := call("request_approval", map[string]any{"scope": "db:migrate", "action": "Apply the partition migration to staging",
		"detail": map[string]any{"sql": "SELECT 1"}, "risk": "Locks ledger_entries briefly"})
	text(fmt.Sprintf("Asked for approval (%v). Ending my turn until a person decides.", ap["state"]))
	result(false, "Waiting for approval.")
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
