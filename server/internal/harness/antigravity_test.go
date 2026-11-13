package harness

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

func TestAntigravityTurn(t *testing.T) {
	evs, res := parse(t, Antigravity{}, "antigravity-turn.jsonl", "")
	want := "lifecycle message tool_call:whoami tool_result:whoami tool_call:Bash tool_result:Bash message usage"
	if got := types(evs); got != want {
		t.Fatalf("events:\n got %s\nwant %s", got, want)
	}
	if res.SessionID != "conv-1" || res.Final != "Done." || res.IsError {
		t.Fatalf("result = %+v", res)
	}
	var texts []string
	for _, e := range evs {
		if e.Type == "message" {
			texts = append(texts, e.Payload["text"].(string))
		}
	}
	if !slices.Equal(texts, []string{"I'll check who I am.", "Done."}) {
		t.Fatalf("a step's text deltas must join into one message: %q", texts)
	}
	if res.Usage.ToolCalls != 2 || res.Usage.InputTokens != 1200 || res.Usage.OutputTokens != 200 || res.Usage.CachedTokens != 400 || res.Usage.CostSource != "unknown" {
		t.Fatalf("usage = %+v", res.Usage)
	}
	for _, e := range evs {
		if e.Type == "tool_result" && e.Payload["tool"] == "Bash" && e.Payload["is_error"] != true {
			t.Fatalf("a tool error was lost: %+v", e)
		}
		if e.Type == "usage" {
			if _, ok := e.Payload["cost_usd"]; ok || e.Payload["cost_known"] != false {
				t.Fatalf("agy reports no cost: %+v", e.Payload)
			}
		}
	}
}

func TestAntigravityQuotaFailureIsALimit(t *testing.T) {
	_, res := parse(t, Antigravity{}, "antigravity-error.jsonl", "")
	if !res.IsError || !strings.Contains(res.Error, "exceeded your quota") || !res.UsageLimited {
		t.Fatalf("a quota failure must be a usage limit so fallbacks apply: %+v", res)
	}
	p := (Antigravity{}).NewParser("")
	evs := p.Line([]byte(`AGY_ERROR: {"message":"model overloaded"}`))
	if len(evs) != 1 || evs[0].Type != "error" || p.Result().Error != "model overloaded" {
		t.Fatalf("AGY_ERROR line: %+v %+v", evs, p.Result())
	}
}

func TestAntigravityRememberImagesTheToolSaved(t *testing.T) {
	p := (Antigravity{}).NewParser("")
	p.Line([]byte(`{"event":"step_update","step_update":{"conversation_id":"c","step_index":1,"state":"DONE","step_type":"tool","tool_info":{"name":"generate_image","parameters":{"prompt":"a fox"},"output":"Saved /home/agent/.gemini/antigravity-cli/artifacts/c/fox.png and /etc/cron.d/x.sh"}}}`))
	if got := p.Result().Images; !slices.Equal(got, []string{"/home/agent/.gemini/antigravity-cli/artifacts/c/fox.png"}) {
		t.Fatalf("images = %v: only image files count, the runner decides where they may come from", got)
	}
}

type agyConfigFile struct {
	Install     bool           `json:"install"`
	TokenEnv    string         `json:"token_env"`
	HookCommand string         `json:"hook_command"`
	Settings    map[string]any `json:"settings"`
	MCPServers  map[string]any `json:"mcp_servers"`
}

func agyInvocation(t *testing.T, s agentdef.Resolved, turn Turn) (Invocation, agyConfigFile) {
	t.Helper()
	s.Harness, s.Model = agentdef.HarnessAntigravity, "gemini-3.8-flash-medium"
	turn.Spec = s
	inv, err := (Antigravity{}).Invocation(ws, turn)
	if err != nil {
		t.Fatal(err)
	}
	var cfg agyConfigFile
	if err := json.Unmarshal(inv.Files[agyConfigPath], &cfg); err != nil {
		t.Fatal(err)
	}
	return inv, cfg
}

func rules(cfg agyConfigFile, list string) []string {
	var out []string
	for _, r := range cfg.Settings["permissions"].(map[string]any)[list].([]any) {
		out = append(out, r.(string))
	}
	return out
}

func TestAntigravityInvocation(t *testing.T) {
	s := spec("repo:read", "allowed")
	s.HarnessOptions["effort"] = "high"
	inv, cfg := agyInvocation(t, s, Turn{Prompt: "go", SessionID: "conv-9"})
	if !slices.Equal(inv.Argv[:3], []string{"python3", agyLaunchPath, "agy"}) || flag(inv.Argv, "--output-format") != "stream-json" ||
		flag(inv.Argv, "--model") != "gemini-3.8-flash-medium" || flag(inv.Argv, "--effort") != "high" || flag(inv.Argv, "--conversation") != "conv-9" {
		t.Fatalf("argv = %q", inv.Argv)
	}
	if i := slices.Index(inv.Argv, "-p"); i < 0 || !strings.HasSuffix(inv.Argv[i+1], "go") || !strings.Contains(inv.Argv[i+1], "AGENTS.md") {
		t.Fatalf("prompt = %q", inv.Argv)
	}
	if slices.Contains(inv.Argv, "--dangerously-skip-permissions") {
		t.Fatal("approving every tool would defeat the policy hook and the deny rules")
	}
	if inv.Env["LAZYAGENTS_TOKEN"] != "la_tok" || inv.Env["LAZYAGENTS_POLICY_TOKEN"] != "la_tok" || !strings.HasSuffix(inv.Env["LAZYAGENTS_POLICY_HOOK_COMMAND"], "agy-policy.py'") {
		t.Fatalf("env = %v", inv.Env)
	}
	if cfg.Settings["modelProvider"] != "gemini" || cfg.Settings["toolPermission"] != "request-review" || cfg.Install {
		t.Fatalf("settings = %v install=%v", cfg.Settings, cfg.Install)
	}
	// The token reaches agy only through the launcher, never a workspace file.
	for p, body := range inv.Files {
		if strings.Contains(string(body), "la_tok") {
			t.Errorf("%s contains the run token", p)
		}
	}
	srv := cfg.MCPServers["lazyagents"].(map[string]any)
	if srv["serverUrl"] != "http://api:8000/mcp" || srv["headers"].(map[string]any)["Authorization"] != "Bearer {{token}}" {
		t.Fatalf("mcp = %v", srv)
	}
	if !slices.Contains(rules(cfg, "allow"), "mcp(lazyagents/*)") {
		t.Fatalf("allow = %v", rules(cfg, "allow"))
	}
}

func TestAntigravityDenyRulesFollowTheSpec(t *testing.T) {
	cases := []struct {
		name    string
		spec    agentdef.Resolved
		want    []string
		notWant []string
	}{
		{"read-only chat", func() agentdef.Resolved {
			s := spec("repo:read", "allowed")
			s.HarnessOptions["readonly"] = "true"
			return s
		}(), []string{"command(*)", "write_file(*)", "read_url(*)"}, nil},
		{"coding run", spec("repo:read", "allowed", "repo:write", "allowed", "tests:run", "allowed", "web:read", "allowed"), nil, []string{"command(*)", "write_file(*)", "read_url(*)"}},
		{"approval-gated writes stay denied until approved", spec("repo:read", "allowed", "repo:write", "requires_approval", "tests:run", "allowed"), []string{"write_file(*)"}, []string{"command(*)"}},
		{"image step writes elsewhere", func() agentdef.Resolved {
			s := spec()
			s.HarnessOptions["readonly"], s.HarnessOptions[agentdef.OptionImageGeneration] = "true", "true"
			return s
		}(), []string{"command(*)"}, []string{"write_file(*)"}},
		{"no tools", func() agentdef.Resolved {
			s := spec("repo:read", "allowed")
			s.HarnessOptions[agentdef.OptionNoTools] = "true"
			return s
		}(), []string{"command(*)", "read_file(*)", "write_file(*)", "mcp(*)"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg := agyInvocation(t, c.spec, Turn{Prompt: "x"})
			deny := rules(cfg, "deny")
			for _, w := range c.want {
				if !slices.Contains(deny, w) {
					t.Errorf("deny %v lacks %s", deny, w)
				}
			}
			for _, w := range c.notWant {
				if slices.Contains(deny, w) {
					t.Errorf("deny %v has %s", deny, w)
				}
			}
		})
	}
	// A no-tools turn gets no MCP server at all and isn't told to read AGENTS.md.
	s := spec("repo:read", "allowed")
	s.HarnessOptions[agentdef.OptionNoTools] = "true"
	inv, cfg := agyInvocation(t, s, Turn{Prompt: "summarize"})
	if len(cfg.MCPServers) != 0 || inv.Argv[slices.Index(inv.Argv, "-p")+1] != "summarize" {
		t.Fatalf("no-tools turn: mcp=%v argv=%q", cfg.MCPServers, inv.Argv)
	}
}

func TestAntigravityConnectorsAndInstallInContainers(t *testing.T) {
	s := spec("repo:read", "allowed")
	s.MCPServers = []agentdef.MCPServer{{Name: "github", Connector: true}, {Name: "docs", URL: "https://docs.example/mcp"}}
	c := ws
	c.ContainerSandbox = true
	s.Harness, s.Model = agentdef.HarnessAntigravity, "m"
	inv, err := (Antigravity{}).Invocation(c, Turn{Spec: s, Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	var cfg agyConfigFile
	_ = json.Unmarshal(inv.Files[agyConfigPath], &cfg)
	gh, docs := cfg.MCPServers["github"].(map[string]any), cfg.MCPServers["docs"].(map[string]any)
	if !cfg.Install || gh["serverUrl"] != "http://api:8000/mcp/connectors/github" || gh["headers"] == nil || docs["serverUrl"] != "https://docs.example/mcp" || docs["headers"] != nil {
		t.Fatalf("install=%v mcp=%v", cfg.Install, cfg.MCPServers)
	}
	if !slices.Contains(rules(cfg, "allow"), "mcp(github/*)") || !slices.Contains(rules(cfg, "allow"), "mcp(docs/*)") {
		t.Fatalf("allow = %v", rules(cfg, "allow"))
	}
}

func TestAntigravityReferenceImagesAreNamedInThePrompt(t *testing.T) {
	inv, _ := agyInvocation(t, spec("repo:read", "allowed"), Turn{Prompt: "draw", Images: []string{".lazyagents/references/persona.png"}})
	if p := inv.Argv[slices.Index(inv.Argv, "-p")+1]; !strings.Contains(p, ".lazyagents/references/persona.png") {
		t.Fatalf("prompt = %q", p)
	}
}

// TestAgyPolicyHookAgreesWithTheParser runs the real hook script against a stub
// policy server: each tool call must reach the policy as the same name and input
// the parser shows in the run's events, and Vesper's answer maps to agy's.
func TestAgyPolicyHookAgreesWithTheParser(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	var got struct {
		Tool  string         `json:"tool"`
		Input map[string]any `json:"input"`
	}
	answer := `{"decision":"allow"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer la_tok" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(answer))
	}))
	defer srv.Close()
	dir := t.TempDir()
	script := filepath.Join(dir, "agy-policy.py")
	if err := os.WriteFile(script, agyPolicyHook, 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(name string, args map[string]any, url string) map[string]any {
		event, _ := json.Marshal(map[string]any{"toolCall": map[string]any{"name": name, "args": args}})
		cmd := exec.Command("python3", "-I", script)
		cmd.Stdin = strings.NewReader(string(event))
		cmd.Env = []string{"LAZYAGENTS_POLICY_URL=" + url, "LAZYAGENTS_POLICY_TOKEN=la_tok"}
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var d map[string]any
		if err := json.Unmarshal(out, &d); err != nil {
			t.Fatalf("%s printed %q", name, out)
		}
		return d
	}
	for _, c := range []struct {
		name string
		args map[string]any
	}{
		// The tools agy 1.5.8 declares to the model, with their real argument keys.
		{"run_command", map[string]any{"CommandLine": "go test ./...", "toolSummary": "Run tests", "WaitMsBeforeAsync": 500.0}},
		{"read_file", map[string]any{"Path": "/workspace/a.go", "StartLine": 1.0}},
		{"write_file", map[string]any{"Path": "/workspace/b.go", "Content": "package b"}},
		{"edit_file", map[string]any{"Path": "/workspace/c.go", "Target": "a", "Replacement": "b"}},
		{"call_mcp_tool", map[string]any{"ServerName": "lazyagents", "ToolName": "get_ticket", "Arguments": map[string]any{"ticket": "X-1"}}},
		{"call_mcp_tool", map[string]any{"ServerName": "lazyagents"}},
		{"invoke_subagent", map[string]any{"TypeName": "general", "Role": "helper", "Prompt": "run ls"}},
		{"swarm", map[string]any{"action": "add"}},
		{"search_web", map[string]any{"query": "go slog"}},
		// agy's internal names (init's tool list), should a step report one.
		{"view_file", map[string]any{"AbsolutePath": "/workspace/a.go"}},
		{"write_to_file", map[string]any{"TargetFile": "/workspace/b.go"}},
		{"replace_file_content", map[string]any{"TargetFile": "/workspace/c.go"}},
		{"list_dir", map[string]any{"DirectoryPath": "/workspace"}},
		{"grep_search", map[string]any{"Query": "TODO", "SearchPath": "/workspace"}},
		{"find_by_name", map[string]any{"Pattern": "*.go", "SearchPath": "/workspace"}},
		{"read_url_content", map[string]any{"Url": "https://go.dev"}},
		{"mcp_lazyagents_get_ticket", map[string]any{"ticket": "X-1"}},
		{"browser_subagent", map[string]any{"Task": "x"}},
	} {
		got.Tool, got.Input = "", nil
		d := run(c.name, c.args, srv.URL)
		wantTool, wantInput := antigravityTool(c.name, c.args)
		wantJSON, _ := json.Marshal(wantInput)
		gotJSON, _ := json.Marshal(got.Input)
		if got.Tool != wantTool || string(gotJSON) != string(wantJSON) {
			t.Errorf("%s: hook sent %s %s, parser says %s %s", c.name, got.Tool, gotJSON, wantTool, wantJSON)
		}
		if d["decision"] != "auto_approve" {
			t.Errorf("%s: allow became %v", c.name, d)
		}
	}
	answer = `{"decision":"deny","reason":"repo:write is denied for this run"}`
	if d := run("write_to_file", map[string]any{"TargetFile": "x"}, srv.URL); d["decision"] != "deny" || d["reason"] != "repo:write is denied for this run" {
		t.Fatalf("deny = %v", d)
	}
	// Anything but an explicit allow, and any failure, denies.
	answer = `{"decision":"ask"}`
	if d := run("run_command", map[string]any{}, srv.URL); d["decision"] != "deny" {
		t.Fatalf("unknown answer = %v", d)
	}
	if d := run("run_command", map[string]any{}, "http://127.0.0.1:1/"); d["decision"] != "deny" {
		t.Fatalf("unreachable policy = %v", d)
	}
}

func TestAntigravityToolNamesInEvents(t *testing.T) {
	for in, want := range map[string]string{
		"run_command": "Bash", "read_file": "Read", "write_file": "Write", "edit_file": "Edit", "invoke_subagent": "Agent",
		"view_file": "Read", "write_to_file": "Write", "replace_file_content": "Edit", "call_mcp_tool": "call_mcp_tool",
		"mcp_lazyagents_search_knowledge": "mcp__lazyagents_search_knowledge", "browser_subagent": "browser_subagent",
	} {
		if got, _ := antigravityTool(in, nil); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
	// Every MCP tool arrives through call_mcp_tool; the policy must see it as
	// that server's tool, or a read-only chat is refused its own tools.
	tool, input := antigravityTool("call_mcp_tool", map[string]any{"ServerName": "lazyagents", "ToolName": "list_tickets", "Arguments": map[string]any{"q": "x"}})
	if tool != "mcp__lazyagents__list_tickets" || input["q"] != "x" {
		t.Fatalf("call_mcp_tool → %s %v", tool, input)
	}
}
