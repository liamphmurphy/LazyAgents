package harness

import (
	"bufio"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
)

func parse(t *testing.T, h Harness, file, model string) ([]Event, Result) {
	t.Helper()
	f, err := os.Open("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := h.NewParser(model)
	var evs []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		evs = append(evs, p.Line(sc.Bytes())...)
	}
	evs = append(evs, p.Line([]byte("not json"))...) // stderr noise is ignored
	return evs, p.Result()
}

func types(evs []Event) string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Type
		if tool, ok := e.Payload["tool"].(string); ok {
			out[i] += ":" + tool
		}
	}
	return strings.Join(out, " ")
}

func TestClaudeTurn(t *testing.T) {
	evs, res := parse(t, ClaudeCode{}, "claude-turn.jsonl", "")
	want := "lifecycle message tool_call:mcp__lazyagents__claim_ticket tool_result:mcp__lazyagents__claim_ticket tool_call:Bash tool_result:Bash message usage tool_call:Bash"
	if types(evs) != want {
		t.Fatalf("events:\n got %s\nwant %s", types(evs), want)
	}
	if res.SessionID != "5d1f0c2a-8b3e-4f6a-9c7d-1e2f3a4b5c6d" || res.IsError || res.Usage.ToolCalls != 2 {
		t.Fatalf("result %+v", res)
	}
	if res.Usage.CostUSD != 0.4182 || res.Usage.CostSource != "harness" || res.Usage.InputTokens != 46200 || res.Usage.CachedTokens != 42000 || res.Usage.OutputTokens != 2100 {
		t.Fatalf("usage %+v", res.Usage)
	}
	if evs[3].Payload["output"] != `{"run_id":"r1","state":"running"}` {
		t.Fatalf("tool result text: %v", evs[3].Payload["output"])
	}
	if evs[len(evs)-1].Payload["denied_by_harness"] != true {
		t.Fatal("permission denial not flagged")
	}
}

func TestClaudeAuthError(t *testing.T) {
	evs, res := parse(t, ClaudeCode{}, "claude-auth-error.jsonl", "")
	if !res.IsError || !strings.Contains(res.Error, "Invalid API key") {
		t.Fatalf("result %+v", res)
	}
	if types(evs) != "lifecycle error message usage error" {
		t.Fatalf("events %s", types(evs)) // MCP failure, the API error text, usage, error
	}
}

func TestCodexTurn(t *testing.T) {
	evs, res := parse(t, Codex{}, "codex-turn.jsonl", "gpt-5-codex")
	want := "lifecycle tool_call:claim_ticket tool_result:claim_ticket tool_call:shell tool_result:shell tool_call:apply_patch message usage"
	if types(evs) != want {
		t.Fatalf("events:\n got %s\nwant %s", types(evs), want)
	}
	if res.SessionID != "01a19db0-1111-7222-8333-944455556666" || res.Usage.ToolCalls != 3 || res.Final != "Fixed the failing test and asked for approval." {
		t.Fatalf("result %+v", res)
	}
	if evs[4].Payload["is_error"] != true || evs[4].Payload["exit_code"] != 2 {
		t.Fatalf("failed command: %v", evs[4].Payload)
	}
	// 8000 uncached × 1.25 + 12000 cached × 0.125 + 3000 × 10, per million.
	if res.Usage.CostSource != "computed" || res.Usage.CostUSD < 0.0414 || res.Usage.CostUSD > 0.0416 {
		t.Fatalf("cost %+v", res.Usage)
	}
}

func TestCodexAuthError(t *testing.T) {
	evs, res := parse(t, Codex{}, "codex-auth-error.jsonl", "some-new-model")
	if !res.IsError || !strings.Contains(res.Error, "401") {
		t.Fatalf("result %+v", res)
	}
	if types(evs) != "lifecycle error error error" { // reconnect noise dropped
		t.Fatalf("events %s", types(evs))
	}
	if _, known := ComputeCost("some-new-model", Usage{InputTokens: 10}); known {
		t.Fatal("unknown model priced")
	}
}

func spec(scopes ...string) agentdef.Resolved {
	r := agentdef.Resolved{Harness: "claude-code", Model: "opus-5.5", Instructions: "Be careful.", HarnessOptions: map[string]string{}}
	for i := 0; i+1 < len(scopes); i += 2 {
		r.Scopes = append(r.Scopes, agentdef.ResolvedScope{Scope: scopes[i], Mode: agentdef.Mode(scopes[i+1])})
	}
	return r
}

var ws = Workspace{Dir: "/workspace", MCPURL: "http://api:8000/mcp", Token: "la_tok", TokenEnv: "LAZYAGENTS_TOKEN"}

func flag(argv []string, name string) string {
	if i := slices.Index(argv, name); i >= 0 && i+1 < len(argv) {
		return argv[i+1]
	}
	return ""
}

func TestClaudeInvocation(t *testing.T) {
	inv, err := ClaudeCode{}.Invocation(ws, Turn{Spec: spec("repo:write", "denied", "tests:run", "allowed"), Prompt: "go", SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	a := inv.Argv
	if a[0] != "claude" || a[1] != "-p" || a[2] != "go" || flag(a, "--model") != "claude-opus-5-5" || flag(a, "--resume") != "s1" ||
		flag(a, "--output-format") != "stream-json" || !slices.Contains(a, "--verbose") || flag(a, "--permission-prompts") != "none" {
		t.Fatalf("argv %v", a)
	}
	if !strings.Contains(flag(a, "--allowedTools"), "Bash") || !strings.Contains(flag(a, "--disallowedTools"), "Bash(git push:*)") {
		t.Fatalf("tool rules %v", a)
	}
	if !strings.Contains(string(inv.Files[".lazyagents/mcp.json"]), `"Authorization": "Bearer la_tok"`) {
		t.Fatalf("mcp config %s", inv.Files[".lazyagents/mcp.json"])
	}
	// No shell scope: no Bash.
	inv, _ = ClaudeCode{}.Invocation(ws, Turn{Spec: spec(), Prompt: "go"})
	if strings.Contains(flag(inv.Argv, "--allowedTools"), "Bash") || slices.Contains(inv.Argv, "--resume") {
		t.Fatalf("argv %v", inv.Argv)
	}
}

func TestCodexInvocation(t *testing.T) {
	s := spec("repo:write", "allowed")
	s.Model = "gpt-5-codex"
	inv, _ := Codex{}.Invocation(ws, Turn{Spec: s, Prompt: "go"})
	a := strings.Join(inv.Argv, " ")
	for _, want := range []string{"codex exec --json", `mcp_servers.lazyagents.url="http://api:8000/mcp"`, `bearer_token_env_var="LAZYAGENTS_TOKEN"`,
		`approval_policy="never"`, `mcp_servers.lazyagents.default_tools_approval_mode="approve"`, `mcp_servers.lazyagents.required=true`, `network={enabled=true}`, `filesystem={":workspace_roots"={".git"="write"}}`, "-m gpt-5-codex"} {
		if !strings.Contains(a, want) {
			t.Errorf("missing %q in %s", want, a)
		}
	}
	if inv.Argv[len(inv.Argv)-1] != "go" || inv.Env["LAZYAGENTS_TOKEN"] != "la_tok" {
		t.Fatalf("argv %v", inv.Argv)
	}
	inv, _ = Codex{}.Invocation(ws, Turn{Spec: spec(), Prompt: "next", SessionID: "th1"})
	n := len(inv.Argv)
	if inv.Argv[4] != "resume" || inv.Argv[n-2] != "th1" || inv.Argv[n-1] != "next" || strings.Contains(strings.Join(inv.Argv, " "), "network_access") {
		t.Fatalf("resume argv %v", inv.Argv)
	}
}

func TestCodexGitPermissions(t *testing.T) {
	for _, session := range []string{"", "existing-thread"} {
		for _, mode := range []string{"allowed", "denied", "requires_approval"} {
			t.Run(session+"/"+mode, func(t *testing.T) {
				inv, err := (Codex{}).Invocation(ws, Turn{Spec: spec("repo:write", mode), SessionID: session, Prompt: "go"})
				if err != nil {
					t.Fatal(err)
				}
				args := strings.Join(inv.Argv, " ")
				grant := strings.Contains(args, `filesystem={":workspace_roots"={".git"="write"}}`)
				if grant != (mode == "allowed") {
					t.Fatalf("unexpected Git permission: %s", args)
				}
				if grant && (strings.Contains(args, "sandbox_mode=") || strings.Contains(args, "sandbox_workspace_write.")) {
					t.Fatalf("legacy sandbox overrides permission profile: %s", args)
				}
			})
		}
	}
}

func TestAgentsMD(t *testing.T) {
	s := spec("db:migrate", "requires_approval")
	md := string(AgentsMD("# Repo rules\n\nUse gofmt.\n", s, []string{".lazyagents/context/adr-postgres.md"}, "PAY-214"))
	if !strings.HasPrefix(md, sectionStart) || !strings.Contains(md, "# Repo rules") || !strings.Contains(md, "`db:migrate`: requires approval") ||
		!strings.Contains(md, ".lazyagents/context/adr-postgres.md") {
		t.Fatalf("AGENTS.md:\n%s", md)
	}
	for _, want := range []string{"verify every acceptance criterion", "implement it yourself or delegate", "Commit and push", "status=done"} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing completion instruction %q", want)
		}
	}
	// Re-rendering replaces our section and keeps the repo's.
	again := string(AgentsMD(md, spec(), nil, "PAY-215"))
	if strings.Count(again, sectionStart) != 1 || !strings.Contains(again, "PAY-215") || strings.Contains(again, "PAY-214") || !strings.Contains(again, "Use gofmt.") {
		t.Fatalf("re-render:\n%s", again)
	}
}

func TestPlanningInvocationsAreReadOnly(t *testing.T) {
	s := spec("repo:write", "denied", "tests:run", "denied")
	s.HarnessOptions = map[string]string{"readonly": "true", "sandbox": "read-only"}
	claude, err := (ClaudeCode{}).Invocation(Workspace{MCPURL: "http://localhost/mcp", Token: "fake", TokenEnv: "LA_TOKEN"}, Turn{Spec: s, Prompt: "Plan"})
	if err != nil {
		t.Fatal(err)
	}
	if m := flag(claude.Argv, "--permission-mode"); m == "plan" || m == "acceptEdits" {
		t.Fatalf("planning permission mode %q; plan mode blocks propose_tickets", m)
	}
	if !slices.Contains(strings.Split(flag(claude.Argv, "--allowedTools"), ","), "mcp__"+lazyagentsServer) {
		t.Fatal("planning must allow the lazyagents MCP tools")
	}
	allowed := strings.Split(flag(claude.Argv, "--allowedTools"), ",")
	for _, tool := range []string{"Edit", "Write", "Bash", "Agent"} {
		if slices.Contains(allowed, tool) {
			t.Fatalf("planning allowed %s", tool)
		}
	}
	denied := strings.Split(flag(claude.Argv, "--disallowedTools"), ",")
	for _, tool := range []string{"Edit", "Write", "Bash", "Agent"} {
		if !slices.Contains(denied, tool) {
			t.Fatalf("planning did not deny %s", tool)
		}
	}
	codex, err := (Codex{}).Invocation(Workspace{MCPURL: "http://localhost/mcp", Token: "fake", TokenEnv: "LA_TOKEN"}, Turn{Spec: s, Prompt: "Plan"})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(codex.Argv, " ")
	if !strings.Contains(args, `sandbox_mode="read-only"`) || strings.Contains(args, "network_access=true") {
		t.Fatalf("unsafe Codex invocation: %s", args)
	}
}

func TestSkillsDeliveredPerHarness(t *testing.T) {
	skills := []agentdef.Skill{
		{Name: "go-errors", Description: `Use when "handling" errors in Go`, Body: "Wrap with %w."},
		{Name: "api-design", Description: "Use when adding endpoints", Body: "Contract first."},
	}
	s := spec()
	s.Skills = skills

	inv, err := ClaudeCode{}.Invocation(ws, Turn{Spec: s, Prompt: "go"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(inv.Files[".claude/skills/go-errors/SKILL.md"])
	if got != "---\nname: \"go-errors\"\ndescription: \"Use when \\\"handling\\\" errors in Go\"\n---\n\nWrap with %w.\n" {
		t.Fatalf("SKILL.md:\n%s", got)
	}
	if ex := string(inv.Files[SkillsExcludeFile]); ex != "/.claude/skills/api-design/\n/.claude/skills/go-errors/\n" {
		t.Fatalf("exclude list %q", ex)
	}
	// Native skills aren't repeated in AGENTS.md.
	if md := string(AgentsMD("", s, nil, "T-1")); strings.Contains(md, "## Skills") {
		t.Fatalf("claude AGENTS.md lists skills:\n%s", md)
	}

	cs := s
	cs.Harness = agentdef.HarnessCodex
	inv, _ = Codex{}.Invocation(ws, Turn{Spec: cs, Prompt: "go"})
	if _, ok := inv.Files[".agents/skills/go-errors/SKILL.md"]; !ok {
		t.Fatalf("codex files %v", inv.Files)
	}
	md := string(AgentsMD("", cs, nil, "T-1"))
	if strings.Contains(md, "## Skills") {
		t.Fatalf("codex AGENTS.md:\n%s", md)
	}

	inv, _ = ClaudeCode{}.Invocation(ws, Turn{Spec: spec(), Prompt: "go"})
	if _, ok := inv.Files[SkillsExcludeFile]; ok {
		t.Fatal("no skills, no exclude file")
	}
}

func TestAgentsMDEpicStacksOnOnePR(t *testing.T) {
	s := spec("repo:write", "allowed")
	s.Repository = &agentdef.Repository{DefaultBranch: "main", EpicBranch: "epic/pay-1234abcd", EpicName: "Ledger"}
	md := string(AgentsMD("", s, nil, "PAY-1"))
	for _, want := range []string{
		"one pull request from `epic/pay-1234abcd` into `main`",
		"git push origin origin/main:refs/heads/epic/pay-1234abcd",
		"gh pr create --draft --head epic/pay-1234abcd --base main",
		"gh pr create --base epic/pay-1234abcd",
		"gh pr merge --squash",
		"never merge the epic's PR",
		"`set_pr`",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("epic section missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "(add `--draft` if anything is unfinished)") {
		t.Fatal("epic tickets must not get the standalone PR steps")
	}
}

// Local tests are the verification: no generated instructions wait on GitHub checks.
func TestAgentsMDDoesNotWaitForPRChecks(t *testing.T) {
	epic := spec("repo:write", "allowed")
	epic.Repository = &agentdef.Repository{DefaultBranch: "main", EpicBranch: "epic/pay-1234abcd", EpicName: "Ledger"}
	for name, md := range map[string]string{
		"standalone": string(AgentsMD("", spec("repo:write", "allowed"), nil, "PAY-1")),
		"epic":       string(AgentsMD("", epic, nil, "PAY-1")),
		"delegated":  string(SubagentAgentsMD("", spec("repo:write", "allowed"), nil, "PAY-1")),
	} {
		if strings.Contains(md, "gh pr checks") || strings.Contains(md, "gh run view") {
			t.Fatalf("%s AGENTS.md waits on GitHub checks:\n%s", name, md)
		}
		if !strings.Contains(md, "never wait for or poll GitHub PR checks") {
			t.Fatalf("%s AGENTS.md lacks the no-wait rule:\n%s", name, md)
		}
	}
}

func TestAgentsMDGitHubSection(t *testing.T) {
	allowed := string(AgentsMD("", spec("repo:write", "allowed"), nil, "PAY-1"))
	for _, want := range []string{"`gh` CLI", "git push -u origin HEAD", "gh pr create", "`set_pr`"} {
		if !strings.Contains(allowed, want) {
			t.Fatalf("allowed repo:write missing %q:\n%s", want, allowed)
		}
	}
	if gated := string(AgentsMD("", spec("repo:write", "requires_approval"), nil, "PAY-1")); !strings.Contains(gated, "request_approval for it") {
		t.Fatalf("gated section should ask for approval:\n%s", gated)
	}
	if denied := string(AgentsMD("", spec("repo:write", "denied"), nil, "PAY-1")); strings.Contains(denied, "gh pr create") {
		t.Fatal("denied repo:write must not be told to open PRs")
	}
	ro := spec("repo:write", "allowed")
	ro.HarnessOptions = map[string]string{"readonly": "true"}
	if strings.Contains(string(AgentsMD("", ro, nil, "PAY-1")), "gh pr create") {
		t.Fatal("planning runs must not be told to open PRs")
	}
	if sub := string(SubagentAgentsMD("", spec("repo:write", "allowed"), nil, "PAY-1")); strings.Contains(sub, "gh pr create") {
		t.Fatal("sub agents leave PRs to the main agent")
	}
}

func TestChatAgentsMD(t *testing.T) {
	spec := agentdef.Resolved{Harness: agentdef.HarnessCodex, Instructions: "Agent persona", Skills: []agentdef.Skill{{Name: "context", Description: "Read context"}}}
	got := string(ChatAgentsMD(spec, []string{".lazyagents/context/memory.md"}, "PAY"))
	for _, want := range []string{"project PAY", "conversation with a person", "Answer directly", "search_knowledge", "read_page", "read_artifact", "get_ticket", "get_backlinks", "Do not call claim_ticket, complete_run or propose_tickets", "Agent persona", ".lazyagents/context/memory.md", "context"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, bad := range []string{"claim_ticket first", "## Git", "git push", "delegate`", "## Delegation", "Keep notes worth remembering", "write_note"} {
		if strings.Contains(got, bad) {
			t.Errorf("ticket workflow %q in %s", bad, got)
		}
	}
}

func TestCodexMCPPolicyOnResumeAndPlanning(t *testing.T) {
	for _, session := range []string{"", "thread-existing"} {
		t.Run(session, func(t *testing.T) {
			s := spec()
			s.HarnessOptions = map[string]string{"sandbox": "read-only", "readonly": "true"}
			s.MCPServers = []agentdef.MCPServer{{Name: "external", URL: "https://example.com/mcp"}}
			inv, err := (Codex{}).Invocation(ws, Turn{Spec: s, Prompt: "continue", SessionID: session})
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Join(inv.Argv, " ")
			for _, required := range []string{`approval_policy="never"`, `sandbox_mode="read-only"`, `mcp_servers.lazyagents.default_tools_approval_mode="approve"`, `mcp_servers.lazyagents.required=true`} {
				if !strings.Contains(args, required) {
					t.Errorf("missing %s in %s", required, args)
				}
			}
			if !strings.Contains(args, `mcp_servers.external.default_tools_approval_mode="approve"`) || strings.Contains(args, "network_access=true") {
				t.Fatalf("configured MCP approval missing or shell network enabled: %s", args)
			}
		})
	}
}

func TestEffortOption(t *testing.T) {
	// Unset: Claude settings carry no modelSettings and Codex no reasoning override.
	inv, _ := ClaudeCode{}.Invocation(ws, Turn{Spec: spec(), Prompt: "go"})
	if strings.Contains(string(inv.Files[".lazyagents/claude-settings.json"]), "modelSettings") {
		t.Fatalf("unset effort leaked into settings: %s", inv.Files[".lazyagents/claude-settings.json"])
	}
	inv, _ = Codex{}.Invocation(ws, Turn{Spec: spec(), Prompt: "go"})
	if strings.Contains(strings.Join(inv.Argv, " "), "model_reasoning_effort") {
		t.Fatalf("unset effort leaked into argv: %v", inv.Argv)
	}

	s := spec()
	s.HarnessOptions["effort"] = "xhigh"
	inv, err := ClaudeCode{}.Invocation(ws, Turn{Spec: s, Prompt: "go"})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ModelSettings map[string]struct {
			EffortLevel string `json:"effortLevel"`
		} `json:"modelSettings"`
		Hooks map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(inv.Files[".lazyagents/claude-settings.json"], &doc); err != nil {
		t.Fatal(err)
	}
	if doc.ModelSettings["claude-opus-5-5"].EffortLevel != "xhigh" || doc.Hooks["PreToolUse"] == nil {
		t.Fatalf("settings %s", inv.Files[".lazyagents/claude-settings.json"])
	}

	for in, want := range map[string]string{"low": "low", "medium": "medium", "high": "high", "xhigh": "high", "max": "high"} {
		s.HarnessOptions["effort"] = in
		inv, _ := Codex{}.Invocation(ws, Turn{Spec: s, Prompt: "go"})
		if !strings.Contains(strings.Join(inv.Argv, " "), `-c model_reasoning_effort="`+want+`"`) {
			t.Errorf("effort %s: argv %v", in, inv.Argv)
		}
	}
	s.HarnessOptions = map[string]string{"effort": "high"}
	if err := validateOpenCodeOptions(s.HarnessOptions); err != nil {
		t.Fatalf("opencode: %v", err)
	}
}

func TestCodexDependencyCachePermissions(t *testing.T) {
	workspace := ws
	workspace.DependencyCacheDir = "/runner/cache"
	for _, scope := range []string{"repo:read", "repo:write", "tests:run"} {
		inv, err := (Codex{}).Invocation(workspace, Turn{Spec: spec(scope, "allowed"), Prompt: "go"})
		if err != nil {
			t.Fatal(err)
		}
		hasCache := strings.Contains(strings.Join(inv.Argv, " "), "--add-dir /runner/cache")
		if hasCache != (scope != "repo:read") {
			t.Fatalf("%s: %v", scope, inv.Argv)
		}
	}
}

func TestDirectExecutionPolicyAndSelectiveContextInstructions(t *testing.T) {
	s := spec()
	s.Instructions = "Standing role routine: hand implementation to an implementer."
	md := string(AgentsMD("", s, []string{".lazyagents/context/one.md"}, "PAY-214"))
	for _, want := range []string{"Work directly", "optional", "skip whoami", "Read only the files relevant", "Your own diff review"} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q", want)
		}
	}
	s.ReviewRequired = true
	md = string(AgentsMD("", s, nil, "PAY-214"))
	if !strings.Contains(md, "Independent review is required") {
		t.Fatal("explicit review policy was lost")
	}
}

func TestAgentsMDTestsSection(t *testing.T) {
	want := "## Tests\n\nRun the narrowest tests that cover your change"
	for name, md := range map[string]string{
		"managed":   string(AgentsMD("", spec(), nil, "PAY-1")),
		"delegated": string(SubagentAgentsMD("", spec(), nil, "PAY-1")),
	} {
		if !strings.Contains(md, want) || !strings.Contains(md, "don't start your own database") {
			t.Fatalf("%s AGENTS.md lacks the Tests section:\n%s", name, md)
		}
	}
	if md := string(AgentsMD("", spec(), nil, "PAY-1")); !strings.Contains(md, "skip get_ticket") || !strings.Contains(md, "check_criterion with ns") {
		t.Fatal("managed AGENTS.md lacks the claim/criteria guidance")
	}
	if strings.Contains(string(ChatAgentsMD(spec(), nil, "PAY")), "## Tests") {
		t.Fatal("chat AGENTS.md must not carry the Tests section")
	}
}
