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
//	chat-read           chat turn; a prompt "read <artifact id> [section]" reads it through MCP
//	idle                end the turn without saying what's next
//	crash               exit 3 with an error result
//	slow                wait for SIGINT (pause), then end the turn
//	delegate            claim, delegate to @scribe, end the turn; the delegated run
//	                    writes child.txt in the shared checkout and completes
//	delegate_contract   delegate with a contract (allowed_paths docs/, a passing
//	                    test_command); the resumed parent reports the checks it was given
//	delegate_script     delegate as FAKE_HARNESS_DELEGATIONS (a JSON array of delegate
//	                    arguments) says, optionally writing an uncommitted dirty.txt first
//	                    (FAKE_HARNESS_DIRTY); on resume run FAKE_HARNESS_ROUND2 once (a
//	                    "$1" continue_run_id is the first delegated run's id), then
//	                    complete_run with the whole resume message as the outcome
//
// A delegated run (any delegate* mode) writes the file named by FILE=<path> in its
// task (default child.txt) in its worktree and commits it. Test knobs: FAKE_HARNESS_LOG
// appends one JSON line per invocation (cwd, resume session, first prompt line);
// FAKE_HARNESS_SYNC=<dir> makes delegated runs wait until FAKE_HARNESS_SYNC_N (2) of
// them are running at once; FAKE_HARNESS_HOLD=<file> writes <file>.started and waits
// for <file> before working; FAKE_HARNESS_REJECT_RESUME fails a --resume like a CLI
// that lost the session.
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
	"path/filepath"
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
	}
	if path := os.Getenv("FAKE_HARNESS_LOG"); path != "" {
		wd, _ := os.Getwd()
		line, _ := json.Marshal(map[string]any{"cwd": wd, "resume": resume, "prompt": firstLine(prompt), "mode": os.Getenv("FAKE_HARNESS_MODE")})
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, _ = f.Write(append(line, '\n'))
			f.Close()
		}
	}
	if codex {
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
	if strings.HasPrefix(os.Getenv("FAKE_HARNESS_MODE"), "delegate") && strings.Contains(prompt, "delegated part of") {
		// A delegated run: no claim; work in its worktree, commit, and report back.
		if resume != "" && os.Getenv("FAKE_HARNESS_REJECT_RESUME") != "" {
			result(true, "No conversation found with session ID: "+resume)
			os.Exit(1)
		}
		if dir := os.Getenv("FAKE_HARNESS_SYNC"); dir != "" {
			_ = os.WriteFile(fmt.Sprintf("%s/%d.start", dir, os.Getpid()), nil, 0o644)
			want := 2
			fmt.Sscan(os.Getenv("FAKE_HARNESS_SYNC_N"), &want)
			if !waitFor(func() bool { m, _ := os.ReadDir(dir); return len(m) >= want }) {
				result(true, "the other delegated runs never started: not running in parallel")
				os.Exit(2)
			}
		}
		if hold := os.Getenv("FAKE_HARNESS_HOLD"); hold != "" {
			_ = os.WriteFile(hold+".started", nil, 0o644)
			if !waitFor(func() bool { _, err := os.Stat(hold); return err == nil }) {
				result(true, "never released")
				os.Exit(2)
			}
		}
		file := "child.txt"
		if _, rest, ok := strings.Cut(prompt, "FILE="); ok && len(strings.Fields(rest)) > 0 {
			file = strings.Fields(rest)[0]
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			result(true, err.Error())
			os.Exit(2)
		}
		if err := os.WriteFile(file, []byte("from the sub agent\n"), 0o644); err != nil {
			result(true, err.Error())
			os.Exit(2)
		}
		for _, args := range [][]string{{"add", file}, {"commit", "-q", "-m", "Add " + file}} {
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				result(true, "git "+args[0]+": "+string(out))
				os.Exit(2)
			}
		}
		call("complete_run", map[string]any{"outcome": "Wrote " + file})
		result(false, "Done.")
		return
	}
	if resume != "" {
		if os.Getenv("FAKE_HARNESS_MODE") == "delegate_script" {
			if _, err := os.Stat(".lazyagents/round2"); err != nil && os.Getenv("FAKE_HARNESS_ROUND2") != "" {
				_ = os.WriteFile(".lazyagents/round2", nil, 0o600)
				delegateScript(call, "FAKE_HARNESS_ROUND2")
				text("Delegated again. Ending my turn until it reports back.")
				result(false, "Waiting for the sub agent.")
				return
			}
			state := ""
			for _, args := range [][]string{{"log", "--pretty=%s"}, {"branch", "--list"}, {"ls-files"}, {"status", "--porcelain"}, {"rev-parse", "-q", "--verify", "MERGE_HEAD"}} {
				out, _ := exec.Command("git", args...).CombinedOutput()
				state += "\n[git " + args[0] + "]\n" + string(out)
			}
			call("complete_run", map[string]any{"outcome": "RESUMED\n" + prompt + "\n=== checkout" + state})
			result(false, "Done.")
			return
		}
		if os.Getenv("FAKE_HARNESS_MODE") == "delegate_contract" {
			call("complete_run", map[string]any{"outcome": fmt.Sprintf("checks failed=%v outside=%v test_passed=%v criteria=%v",
				strings.Contains(prompt, "**FAILED**"), strings.Contains(prompt, "Changed outside the allowed paths:\n- child.txt"),
				strings.Contains(prompt, "passed."), strings.Contains(prompt, "Automated checks"))})
			result(false, "Done.")
			return
		}
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
		if !codex {
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
	if os.Getenv("FAKE_HARNESS_MODE") == "delegate_contract" {
		call("delegate", map[string]any{"agent": "scribe", "task": "Write child.txt", "acceptance_criteria": []string{"child.txt exists"},
			"allowed_paths": []string{"docs/"}, "test_command": "test -f child.txt"})
		text("Delegated to @scribe. Ending my turn until it reports back.")
		result(false, "Waiting for the sub agent.")
		return
	}
	if os.Getenv("FAKE_HARNESS_MODE") == "delegate_script" {
		if os.Getenv("FAKE_HARNESS_DIRTY") != "" {
			_ = os.WriteFile("dirty.txt", []byte("uncommitted\n"), 0o644)
		}
		delegateScript(call, "FAKE_HARNESS_DELEGATIONS")
		text("Delegated. Ending my turn until the sub agents report back.")
		result(false, "Waiting for the sub agents.")
		return
	}
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

// delegateScript issues the delegate calls in the JSON array named by env. A
// continue_run_id of "$N" means the Nth run_id this parent has been given.
func delegateScript(call func(string, map[string]any) map[string]any, env string) {
	var list []map[string]any
	if err := json.Unmarshal([]byte(os.Getenv(env)), &list); err != nil {
		return
	}
	for _, a := range list {
		raw, _ := os.ReadFile(".lazyagents/children")
		ids := strings.Fields(string(raw))
		if v, ok := a["continue_run_id"].(string); ok && strings.HasPrefix(v, "$") {
			var n int
			fmt.Sscan(v[1:], &n)
			if n >= 1 && n <= len(ids) {
				a["continue_run_id"] = ids[n-1]
			}
		}
		out := call("delegate", a)
		if id, ok := out["run_id"].(string); ok {
			f, _ := os.OpenFile(".lazyagents/children", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			fmt.Fprintln(f, id)
			f.Close()
		}
	}
}

// waitFor polls cond for up to 30 seconds.
func waitFor(cond func() bool) bool {
	for i := 0; i < 600; i++ {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
