// Command fake-harness stands in for the `claude`, `codex` and `opencode` CLIs in runner
// tests and demos without API keys (PLAN.md §10). Invoked as `claude -p …`
// it emits Claude's stream-json and reads the MCP server from --mcp-config;
// invoked as `codex exec [resume] … --json` it emits Codex's JSONL and reads
// the server from -c mcp_servers.lazyagents.* overrides. Either way it does
// real work through the LazyAgents MCP server. Invoked as `opencode run …` it
// emits the SYNTHETIC OpenCode envelope the adapter's parser assumes, reads the
// server from the generated ./opencode.json, and answers `opencode --version`.
//
// FAKE_HARNESS_MODE picks the first turn's script:
//
//	approval (default)  claim, plan, request db:migrate approval, end turn
//	complete            claim, plan, complete_run
//	chat-read           chat turn; a prompt "read <artifact id> [section]" reads it through MCP
//	idle                end the turn without saying what's next
//	crash               exit 3 with an error result
//	slow                wait for SIGINT (pause), then end the turn
//	delegate            claim, delegate to @scribe, end the turn; the delegated run
//	                    writes child.txt in the shared checkout and completes
//
// Resumed turns (--resume) finish the run with complete_run, unless
// FAKE_HARNESS_RESUME=idle.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
	session  string
	codex    bool // speak Codex's JSONL instead of Claude's stream-json
	opencode bool // speak the (synthetic) OpenCode JSONL
	items    int
)

func emit(v map[string]any) {
	if !codex && !opencode {
		v["session_id"] = session
	}
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

func item(event string, it map[string]any) {
	emit(map[string]any{"type": event, "item": it})
}

func text(s string) {
	if opencode {
		emit(map[string]any{"type": "text", "text": s})
		return
	}
	if codex {
		items++
		item("item.completed", map[string]any{"id": fmt.Sprintf("item_%d", items), "type": "agent_message", "text": s})
		return
	}
	emit(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": s}}}})
}

func result(isError bool, msg string) {
	if opencode {
		if isError {
			emit(map[string]any{"type": "error", "message": msg})
			return
		}
		emit(map[string]any{"type": "completed", "usage": map[string]any{"input_tokens": 1000, "output_tokens": 200, "cached_tokens": 500}})
		return
	}
	if codex {
		if isError {
			emit(map[string]any{"type": "turn.failed", "error": map[string]any{"message": msg}})
			return
		}
		counts := map[string]int{}
		raw, _ := os.ReadFile(".lazyagents/fake-codex-usage.json")
		_ = json.Unmarshal(raw, &counts)
		counts[session]++
		raw, _ = json.Marshal(counts)
		_ = os.WriteFile(".lazyagents/fake-codex-usage.json", raw, 0600)
		count := counts[session]
		emit(map[string]any{"type": "turn.completed", "usage": map[string]any{"input_tokens": 1000 * count, "cached_input_tokens": 500 * count, "output_tokens": 200 * count}})
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
	var prompt, resume, cfgPath, model string
	var mcpURL, tokenEnv, mcpApproval string
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "app-server" {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var request struct {
				ID     any    `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal(scanner.Bytes(), &request) != nil || request.ID == nil {
				continue
			}
			result := any(map[string]any{})
			if request.Method == "hooks/list" {
				result = map[string]any{"data": []any{map[string]any{"errors": []any{}, "hooks": []any{map[string]any{"enabled": true, "source": "sessionFlags", "handlerType": "command", "command": os.Getenv("LAZYAGENTS_POLICY_HOOK_COMMAND"), "eventName": "preToolUse", "matcher": "*", "trustStatus": "untrusted"}}}}}
			}
			b, _ := json.Marshal(map[string]any{"id": request.ID, "result": result})
			fmt.Println(string(b))
		}
		return
	}
	if len(args) > 0 && args[0] == "--version" {
		fmt.Println("opencode v2.2.1")
		return
	}
	if len(args) > 0 && args[0] == "run" {
		opencode = true
		var pos []string
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--standalone":
			case "--format":
				i++
			case "--model", "-m":
				model = args[i+1]
				i++
			case "--session", "-s":
				resume = args[i+1]
				i++
			default:
				pos = append(pos, args[i])
			}
		}
		if len(pos) > 0 {
			prompt = pos[len(pos)-1]
		}
		if err := opencodeEnv(model); err != "" {
			session = "ses_envcheck"
			emit(map[string]any{"type": "session", "session_id": session})
			result(true, err)
			os.Exit(1)
		}
	}
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
				case "mcp_servers.lazyagents.default_tools_approval_mode":
					mcpApproval = v
				}
			case "-m":
				model = args[i+1]
				i++
			case "-s", "-C":
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
	if codex && mcpApproval != "approve" {
		result(true, "MCP tool call requires approval, but approval policy is never")
		os.Exit(1)
	}
	for i := 0; !codex && !opencode && i < len(args); i++ {
		switch args[i] {
		case "-p", "--print":
			if i+1 < len(args) {
				prompt = args[i+1]
				i++
			}
		case "--resume":
			resume = args[i+1]
			i++
		case "--model":
			model = args[i+1]
			i++
		case "--mcp-config":
			cfgPath = args[i+1]
			i++
		}
	}
	session = resume
	if session == "" {
		session = newUUID()
		if opencode {
			session = "ses_" + strings.ReplaceAll(session, "-", "")
		}
	}
	if opencode {
		emit(map[string]any{"type": "session", "session_id": session})
	} else if codex {
		emit(map[string]any{"type": "thread.started", "thread_id": session})
		emit(map[string]any{"type": "turn.started"})
	} else {
		emit(map[string]any{"type": "system", "subtype": "init", "model": "fake", "mcp_servers": []any{map[string]any{"name": "lazyagents", "status": "connected"}}})
	}

	// A chat can answer successfully without connecting to MCP or calling tools.
	if os.Getenv("FAKE_HARNESS_MODE") == "chat" {
		if prompt == "slow" && resume == "" {
			time.Sleep(60 * time.Second)
		}
		if prompt == "quota" {
			result(true, "You have hit your usage limit")
			os.Exit(1)
		}
		answer := fmt.Sprintf("Chat reply (resumed=%v, session=%s, model=%s): %s", resume != "", session, model, prompt)
		text(answer)
		result(false, answer)
		return
	}

	var cfg struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	var err error
	if opencode {
		cfg.MCPServers, err = opencodeServers()
	} else if codex {
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
	var call func(string, map[string]any) map[string]any
	call = func(name string, a map[string]any) map[string]any {
		// Simulate successful verification for main-run lifecycle fixtures.
		// Delegated fixtures report only their assigned task to the parent.
		if name == "complete_run" && a["status"] != "todo" && !strings.Contains(prompt, "delegated part of") {
			tk := call("get_ticket", map[string]any{"ticket": os.Getenv("LAZYAGENTS_TICKET")})
			if criteria, ok := tk["criteria"].([]any); ok {
				for i := range criteria {
					call("check_criterion", map[string]any{"n": i + 1, "done": true})
				}
			}
		}
		n++
		id := fmt.Sprintf("toolu_%02d", n)
		if opencode {
			id = fmt.Sprintf("call_%02d", n)
			emit(map[string]any{"type": "tool_start", "call_id": id, "tool": "lazyagents_" + name, "input": a})
		} else if codex {
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
		if opencode {
			emit(map[string]any{"type": "tool_result", "call_id": id, "output": body, "is_error": isErr})
		} else if codex {
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
	// chat-read: a prompt of "read <artifact id> [section]" reads it through MCP; any other prompt reads nothing.
	if os.Getenv("FAKE_HARNESS_MODE") == "chat-read" {
		if f := strings.Fields(prompt); len(f) >= 2 && f[0] == "read" {
			args := map[string]any{"artifact": f[1]}
			if len(f) > 2 {
				args["section"] = f[2]
			}
			call("read_artifact", args)
		}
		answer := "Chat reply: " + prompt
		text(answer)
		result(false, answer)
		return
	}
	if os.Getenv("FAKE_HARNESS_MODE") == "planning" {
		who := call("whoami", map[string]any{})
		call("get_ticket", map[string]any{"ticket": "PAY-214"})
		call("propose_tickets", map[string]any{"tickets": []any{map[string]any{"title": "Planned implementation", "description": "An editable draft from the fake harness", "criteria": []string{"Works safely"}, "agent_id": who["id"]}}})
		result(false, "Draft submitted.")
		return
	}

	if os.Getenv("FAKE_HARNESS_MODE") == "policy" {
		call("claim_ticket", map[string]any{"ticket": ticket})
		check := exec.Command("python3", ".lazyagents/tool-policy.py")
		check.Stdin = strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}`)
		body, err := check.CombinedOutput()
		if err != nil {
			result(true, "policy hook failed")
			os.Exit(2)
		}
		if strings.Contains(string(body), `"deny"`) {
			text("Tool blocked; waiting for LazyAgents approval.")
			result(false, "Waiting for tool approval")
			return
		}
		_ = os.WriteFile("approved-action.txt", []byte("ran after approval"), 0o644)
		call("complete_run", map[string]any{"outcome": "Exact tool action approved and completed"})
		result(false, "Done")
		return
	}
	if resume != "" && os.Getenv("FAKE_HARNESS_RESUME") == "idle" {
		text("Still looking around.")
		result(false, "Still looking around.")
		return
	}
	if os.Getenv("FAKE_HARNESS_MODE") == "delegate" && strings.Contains(prompt, "delegated part of") {
		// A delegated run: no claim; work in the parent's checkout and report back.
		if err := os.WriteFile("child.txt", []byte("from the sub agent\n"), 0o644); err != nil {
			result(true, err.Error())
			os.Exit(2)
		}
		call("complete_run", map[string]any{"outcome": "Wrote child.txt"})
		result(false, "Done.")
		return
	}
	if resume != "" {
		if os.Getenv("FAKE_HARNESS_MODE") == "delegate" {
			_, statErr := os.Stat("child.txt")
			call("complete_run", map[string]any{"outcome": fmt.Sprintf("Reviewed: child.txt present=%v; %s", statErr == nil, firstLine(prompt))})
			result(false, "Done.")
			return
		}
		text("Resumed: " + firstLine(prompt))
		status := "in_review"
		if strings.HasPrefix(prompt, "Denied") {
			status = "todo"
		}
		call("complete_run", map[string]any{"outcome": "Finished after: " + firstLine(prompt), "status": status})
		result(false, "Done.")
		return
	}

	if os.Getenv("FAKE_HARNESS_MODE") == "quota" {
		// Claude always hits the limit and Codex never does (the existing
		// fixtures); OpenCode hits it until it is a fallback's fresh session.
		if (!codex && !opencode) || (opencode && !strings.Contains(prompt, "fresh session")) {
			call("claim_ticket", map[string]any{"ticket": ticket})
			_ = os.WriteFile("before-fallback.txt", []byte("work preserved"), 0o644)
			result(true, "You have hit your usage limit")
			os.Exit(1)
		}
		if _, err := os.Stat("before-fallback.txt"); err != nil {
			result(true, "checkout was lost")
			os.Exit(2)
		}
		if resume != "" || !strings.Contains(prompt, "fresh session") {
			result(true, "missing handoff")
			os.Exit(2)
		}
		call("complete_run", map[string]any{"outcome": "Completed on fallback with preserved checkout"})
		result(false, "Done")
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
	if os.Getenv("FAKE_HARNESS_MODE") == "delegate" {
		call("delegate", map[string]any{"agent": "scribe", "task": "Write child.txt"})
		text("Delegated to @scribe. Ending my turn until it reports back.")
		result(false, "Waiting for the sub agent.")
		return
	}
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

// opencodeEnv checks what the adapter promised about the environment and
// records which credential variables were present (names only) so tests can
// assert that only the selected provider's key was injected.
func opencodeEnv(model string) string {
	var names []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasSuffix(k, "_API_KEY") || strings.HasSuffix(k, "_TOKEN") {
			names = append(names, k)
		}
	}
	_ = os.MkdirAll(".lazyagents", 0o755)
	f, err := os.OpenFile(".lazyagents/fake-opencode-env.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err == nil {
		fmt.Fprintf(f, "%s %s\n", model, strings.Join(names, ","))
		f.Close()
	}
	if !strings.Contains(os.Getenv("HOME"), "/.lazyagents/opencode/") || !strings.Contains(os.Getenv("XDG_CONFIG_HOME"), "/.lazyagents/opencode/") {
		return "HOME and XDG roots are not isolated"
	}
	if _, err := os.Stat("opencode.json"); err != nil {
		return "no generated opencode.json"
	}
	return ""
}

// opencodeServers reads the MCP server from the generated opencode.json,
// resolving {env:NAME} the way the CLI does.
func opencodeServers() (map[string]struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}, error) {
	var cfg struct {
		MCP struct {
			Servers map[string]struct {
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
			} `json:"servers"`
		} `json:"mcp"`
	}
	raw, err := os.ReadFile("opencode.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	for name, srv := range cfg.MCP.Servers {
		for k, v := range srv.Headers {
			for {
				i := strings.Index(v, "{env:")
				if i < 0 {
					break
				}
				j := strings.Index(v[i:], "}")
				v = v[:i] + os.Getenv(v[i+5:i+j]) + v[i+j+1:]
			}
			srv.Headers[k] = v
		}
		cfg.MCP.Servers[name] = srv
	}
	return cfg.MCP.Servers, nil
}
