package harness

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

const ocToken = "la_TESTTOKENSECRET9999"

func ocWS() Workspace {
	return Workspace{OpenCodeStateDir: "/work/.lazyagents/opencode/main", Dir: "/work/space", MCPURL: "http://mcp.test/mcp", Token: ocToken, TokenEnv: "LAZYAGENTS_TOKEN"}
}

func ocSpec(model string, opts map[string]string, scopes ...agentdef.ResolvedScope) agentdef.Resolved {
	return agentdef.Resolved{Harness: HarnessOpenCode, Model: model, HarnessOptions: opts, Scopes: scopes}
}

func ocScope(s string, m agentdef.Mode) agentdef.ResolvedScope {
	return agentdef.ResolvedScope{Scope: s, Mode: m}
}

func ocOwned(string) bool { return true }

type ocCfg struct {
	Providers   map[string]map[string]any                   `json:"providers"`
	MCP         struct{ Servers map[string]map[string]any } `json:"mcp"`
	Permissions []policyStatement                           `json:"permissions"`
	Policies    []policyStatement                           `json:"policies"`
}

func ocConfig(t *testing.T, inv Invocation) ocCfg {
	t.Helper()
	var c ocCfg
	if err := json.Unmarshal(inv.StateFiles[opencodeConfigFile], &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// effective evaluates last-match-wins; unmatched is allowed (2.2.1 policy).
func effective(ps []policyStatement, action, resource string) string {
	out := "allow"
	for _, p := range ps {
		if (p.Action == action || p.Action == "*") && globMatch(p.Resource, resource) {
			out = p.Effect
		}
	}
	return out
}

func globMatch(pat, s string) bool {
	if pat == "*" || pat == s {
		return true
	}
	if p, ok := strings.CutSuffix(pat, "*"); ok {
		return strings.HasPrefix(s, p)
	}
	return false
}

// Criterion 1: fresh and resumed argv/env.
func TestOpenCodeInvocationFreshAndResumed(t *testing.T) {
	h := OpenCode{ConfigIsolated: true, SessionOwned: ocOwned}
	spec := ocSpec("openai/gpt-5#high", nil, ocScope("tests:run", agentdef.Allowed))
	inv, err := h.Invocation(ocWS(), Turn{Spec: spec, Prompt: "do the thing"})
	if err != nil {
		t.Fatal(err)
	}
	want := "opencode run --standalone --format json --model openai/gpt-5#high do the thing"
	if got := strings.Join(inv.Argv, " "); got != want {
		t.Fatalf("fresh argv\n got %s\nwant %s", got, want)
	}
	if inv.Env["PWD"] != "/work/space" || !strings.HasPrefix(inv.Env["HOME"], "/work/.lazyagents/opencode/") ||
		!strings.HasPrefix(inv.Env["XDG_DATA_HOME"], "/work/.lazyagents/opencode/") || inv.Env[opencodeTokenEnv] != ocToken {
		t.Fatalf("env %v", inv.Env)
	}
	inv, err = h.Invocation(ocWS(), Turn{Spec: spec, Prompt: "continue", SessionID: "ses_abc123"})
	if err != nil {
		t.Fatal(err)
	}
	want = "opencode run --standalone --format json --model openai/gpt-5#high --session ses_abc123 continue"
	if got := strings.Join(inv.Argv, " "); got != want {
		t.Fatalf("resumed argv\n got %s\nwant %s", got, want)
	}
	for _, a := range inv.Argv {
		if a == "--continue" || a == "--fork" || a == "--auto" {
			t.Fatalf("forbidden flag %s", a)
		}
	}
}

func TestOpenCodeModelAndCredentialMapping(t *testing.T) {
	for model, want := range map[string][2]string{
		"anthropic/claude-sonnet-5-5":  {"harness:anthropic", "ANTHROPIC_API_KEY"},
		"openai/gpt-5#high":            {"harness:openai", "OPENAI_API_KEY"},
		"anthropic/claude-opus-5-5#hi": {"harness:anthropic", "ANTHROPIC_API_KEY"},
	} {
		secret, env, err := OpenCodeCredentialEnv(model)
		if err != nil || secret != want[0] || env != want[1] {
			t.Fatalf("%s: %s %s %v", model, secret, env, err)
		}
	}
	for _, bad := range []string{"", "opus-5.5", "gpt-5", "google/gemini", "openai/", "/gpt", "openai/gpt 5", "Openai/gpt-5"} {
		if _, _, err := OpenCodeCredentialEnv(bad); err == nil {
			t.Fatalf("model %q accepted", bad)
		}
		if _, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: ocSpec(bad, nil), Prompt: "x"}); err == nil {
			t.Fatalf("invocation accepted model %q", bad)
		}
	}
	if CheckOpenCodeVersion("2.2.1\n") != nil || CheckOpenCodeVersion("v2.2.1") != nil || CheckOpenCodeVersion("opencode v2.2.1\n") != nil {
		t.Fatal("pinned version rejected")
	}
	if CheckOpenCodeVersion("2.2.3") == nil || CheckOpenCodeVersion("opencode v2.2.10") == nil || CheckOpenCodeVersion("opencode v2.2.1-rc1") == nil || CheckOpenCodeVersion("") == nil {
		t.Fatal("unpinned version accepted")
	}
}

// Criterion 5: unsupported resume.
func TestOpenCodeResumeRejected(t *testing.T) {
	spec := ocSpec("openai/gpt-5", nil)
	cases := map[string]struct {
		h  OpenCode
		id string
	}{
		"no ownership check": {OpenCode{ConfigIsolated: true}, "ses_1"},
		"missing or foreign": {OpenCode{ConfigIsolated: true, SessionOwned: func(string) bool { return false }}, "ses_1"},
		"flag-shaped id":     {OpenCode{ConfigIsolated: true, SessionOwned: ocOwned}, "--fork"},
		"whitespace id":      {OpenCode{ConfigIsolated: true, SessionOwned: ocOwned}, "a b"},
		"overlong id":        {OpenCode{ConfigIsolated: true, SessionOwned: ocOwned}, strings.Repeat("a", 200)},
	}
	for name, c := range cases {
		if _, err := c.h.Invocation(ocWS(), Turn{Spec: spec, Prompt: "x", SessionID: c.id}); err == nil {
			t.Errorf("%s: resume accepted", name)
		}
	}
}

// Criterion 2: config, skills, MCP placement and credential hygiene.
func TestOpenCodeFilesAndSecrets(t *testing.T) {
	spec := ocSpec("openai/gpt-5", nil, ocScope("repo:write", agentdef.Allowed))
	spec.Skills = []agentdef.Skill{{Name: "go-testing", Description: "d", Body: "body"}}
	spec.MCPServers = []agentdef.MCPServer{{Name: "docs", URL: "https://docs.example/mcp"}}
	inv, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: spec, Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".opencode/skills/go-testing/SKILL.md", ".lazyagents/skills/go-testing/SKILL.md", SkillsExcludeFile} {
		if _, ok := inv.Files[p]; !ok {
			t.Errorf("missing file %s", p)
		}
	}
	if ex := string(inv.Files[SkillsExcludeFile]); !strings.Contains(ex, "/opencode.json") || !strings.Contains(ex, "/.opencode/skills/") {
		t.Errorf("exclude list %q", ex)
	}
	c := ocConfig(t, inv)
	srv := c.MCP.Servers["lazyagents"]
	if srv["url"] != "http://mcp.test/mcp" || srv["type"] != "remote" || srv["oauth"] != false {
		t.Fatalf("lazyagents mcp %v", srv)
	}
	if c.MCP.Servers["docs"]["url"] != "https://docs.example/mcp" {
		t.Fatalf("external mcp %v", c.MCP.Servers)
	}
	if c.Providers["openai"]["env"].([]any)[0] != "OPENAI_API_KEY" {
		t.Fatalf("providers %v", c.Providers)
	}
	// Token only reaches the process through the environment placeholder.
	for name, body := range inv.Files {
		if strings.Contains(string(body), ocToken) {
			t.Errorf("token written to %s", name)
		}
	}
	if !strings.Contains(string(inv.StateFiles["opencode.json"]), "{env:LAZYAGENTS_RUN_TOKEN}") {
		t.Error("config lacks token placeholder")
	}
	if strings.Contains(strings.Join(inv.Argv, " "), ocToken) {
		t.Error("token in argv")
	}
	for k, v := range inv.Env {
		if v == ocToken && k != opencodeTokenEnv {
			t.Errorf("token leaked into %s", k)
		}
	}
	// Provider keys are injected by the runner, never rendered here.
	if _, ok := inv.Env["OPENAI_API_KEY"]; ok {
		t.Error("adapter injected provider key")
	}
	// The generated index in AGENTS.md points at files that exist.
	md := string(AgentsMD("", spec, nil, "T-1"))
	if !strings.Contains(md, ".lazyagents/skills/go-testing/SKILL.md") {
		t.Error("skills index missing")
	}
	if _, ok := inv.Files[".lazyagents/skills/go-testing/SKILL.md"]; !ok {
		t.Error("indexed skill not written")
	}
	// Policy: deny-then-allow ordering from the contract.
	for _, tc := range []struct{ action, res, want string }{
		{"integration.use", "mcp:lazyagents", "allow"}, {"integration.use", "mcp:docs", "allow"},
		{"integration.use", "mcp:other", "deny"}, {"integration.use", "plugin:x", "deny"},
		{"integration.use", "skill:unknown", "deny"}, {"integration.use", "skill:go-testing", "allow"},
		{"provider.use", "openai", "allow"}, {"provider.use", "anthropic", "deny"},
		{"tool.use", "lazyagents_get_ticket", "allow"}, {"tool.use", "webfetch", "deny"},
	} {
		if got := effective(c.Policies, tc.action, tc.res); got != tc.want {
			t.Errorf("%s %s = %s, want %s", tc.action, tc.res, got, tc.want)
		}
	}
	for _, p := range c.Policies {
		if p.Action == "mcp.use" || p.Action == "skill.use" || p.Action == "plugin.use" || p.Action == "integration.connect" {
			t.Errorf("unsupported action %s rendered", p.Action)
		}
	}
}

func TestOpenCodeTicketModeRejectedWithoutIsolatedConfig(t *testing.T) {
	spec := ocSpec("anthropic/claude-sonnet-5-5", nil)
	_, err := (OpenCode{}).Invocation(ocWS(), Turn{Spec: spec, Prompt: "p"})
	if err == nil || !strings.Contains(err.Error(), "isolated managed config") {
		t.Fatalf("ticket-mode run accepted without isolated config: %v", err)
	}
	if _, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: spec, Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
}

// Criterion 4: read-only enforcement or rejection; ticket scope handling.
func TestOpenCodeReadonlyPolicyOrRejection(t *testing.T) {
	spec := ocSpec("anthropic/claude-sonnet-5-5", map[string]string{"readonly": "true"})
	if _, err := (OpenCode{}).Invocation(ocWS(), Turn{Spec: spec, Prompt: "p"}); err == nil {
		t.Fatal("readonly accepted without isolated config")
	}
	inv, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: spec, Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	c := ocConfig(t, inv)
	for _, tool := range []string{"bash", "edit", "write", "task", "webfetch"} {
		if effective(c.Policies, "tool.use", tool) != "deny" || effective(c.Permissions, tool, "*") != "deny" {
			t.Errorf("%s not denied in readonly", tool)
		}
	}
	for _, tool := range []string{"read", "grep", "glob", "lazyagents_search_knowledge"} {
		if effective(c.Policies, "tool.use", tool) != "allow" {
			t.Errorf("%s denied in readonly", tool)
		}
	}
	if effective(c.Policies, "provider.use", "openai") != "deny" || effective(c.Policies, "provider.use", "anthropic") != "allow" {
		t.Error("provider policy")
	}
	withMCP := spec
	withMCP.MCPServers = []agentdef.MCPServer{{Name: "docs", URL: "https://x.test/mcp"}}
	if _, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: withMCP, Prompt: "p"}); err == nil {
		t.Fatal("external MCP accepted in readonly")
	}
}

func TestOpenCodeTicketScopes(t *testing.T) {
	h := OpenCode{ConfigIsolated: true}
	run := func(scopes ...agentdef.ResolvedScope) (ocCfg, error) {
		inv, err := h.Invocation(ocWS(), Turn{Spec: ocSpec("openai/gpt-5", nil, scopes...), Prompt: "p"})
		if err != nil {
			return ocCfg{}, err
		}
		return ocConfig(t, inv), nil
	}
	c, err := run(ocScope("repo:write", agentdef.Allowed), ocScope("tests:run", agentdef.Allowed))
	if err != nil || effective(c.Permissions, "bash", "git push origin") != "allow" || effective(c.Permissions, "edit", "*") != "allow" {
		t.Fatalf("writer: %v", err)
	}
	c, err = run(ocScope("repo:write", agentdef.Denied), ocScope("tests:run", agentdef.Allowed))
	if err != nil || effective(c.Permissions, "bash", "git push origin") != "deny" || effective(c.Permissions, "bash", "gh pr create --title x") != "deny" || effective(c.Permissions, "bash", "go test") != "allow" {
		t.Fatalf("no push: %v %+v", err, c.Permissions)
	}
	c, err = run(ocScope("repo:write", agentdef.Denied), ocScope("tests:run", agentdef.Denied), ocScope("repo:read", agentdef.Denied))
	if err != nil || effective(c.Permissions, "bash", "ls") != "deny" || effective(c.Policies, "tool.use", "bash") != "deny" {
		t.Fatalf("no shell: %v", err)
	}
	// Brokered approvals run server-side; non-brokered ones can't be enforced.
	if _, err = run(ocScope("db:migrate", agentdef.RequiresApproval)); err != nil {
		t.Fatalf("brokered approval rejected: %v", err)
	}
	for _, s := range []string{"repo:write", "tests:run", "custom:thing"} {
		if _, err = run(ocScope(s, agentdef.RequiresApproval)); err == nil {
			t.Errorf("approval for %s accepted without a policy bridge", s)
		}
	}
}

func TestOpenCodeRejectsUnsupportedOptionsAndPolicy(t *testing.T) {
	for _, k := range []string{"mcp.use", "skill.use", "plugin.use", "integration.connect", "permission_mode", "sandbox", "auto", "bogus"} {
		if _, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: ocSpec("openai/gpt-5", map[string]string{k: "true"}), Prompt: "p"}); err == nil {
			t.Errorf("option %s accepted", k)
		}
	}
	for _, a := range []string{"mcp.use", "skill.use", "plugin.use", "integration.connect"} {
		if validatePolicy([]policyStatement{{a, "x", "allow"}}) == nil {
			t.Errorf("policy action %s accepted", a)
		}
	}
	for _, p := range []string{"", "  ", "-p evil"} {
		if _, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: ocSpec("openai/gpt-5", nil), Prompt: p}); err == nil {
			t.Errorf("prompt %q accepted", p)
		}
	}
	bad := ocSpec("openai/gpt-5", nil)
	bad.MCPServers = []agentdef.MCPServer{{Name: "lazyagents", URL: "https://x.test"}}
	if _, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: bad, Prompt: "p"}); err == nil {
		t.Error("shadowing the reserved MCP server accepted")
	}
	bad = ocSpec("openai/gpt-5", nil)
	bad.Skills = []agentdef.Skill{{Name: "../evil"}}
	if _, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: bad, Prompt: "p"}); err == nil {
		t.Error("path-traversing skill accepted")
	}
	if _, err := (OpenCode{ConfigIsolated: true}).Invocation(Workspace{Dir: "rel", MCPURL: "http://x/mcp"}, Turn{Spec: ocSpec("openai/gpt-5", nil), Prompt: "p"}); err == nil {
		t.Error("relative workspace accepted")
	}
}

// ------------------------------------------------------------- parser

func ocRun(t *testing.T, file string, exit int) ([]Event, Result) {
	t.Helper()
	f, err := os.Open("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := (OpenCode{}).NewParser("openai/gpt-5").(*opencodeParser)
	var evs []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		evs = append(evs, p.Line(sc.Bytes())...)
	}
	evs = append(evs, p.Line([]byte("stderr noise"))...)
	if exit >= 0 {
		p.Exited(exit)
	}
	return evs, p.Result()
}

// Criterion 3: success, with duplicate terminal summary and unknown events.
func TestOpenCodeTurn(t *testing.T) {
	evs, res := ocRun(t, "opencode-turn.jsonl", 0)
	if got, want := types(evs), "lifecycle message tool_call:lazyagents_get_ticket tool_result:lazyagents_get_ticket message usage"; got != want {
		t.Fatalf("events\n got %s\nwant %s", got, want)
	}
	if res.IsError || res.Final != "Sanitized answer." || res.SessionID != "ses_synthetic01" || res.UsageLimited {
		t.Fatalf("result %+v", res)
	}
	u := res.Usage
	if u.InputTokens != 120 || u.OutputTokens != 30 || u.CachedTokens != 20 || u.ToolCalls != 1 || u.CostSource != "unknown" || u.CostUSD != 0 {
		t.Fatalf("usage %+v", u)
	}
	pl := evs[len(evs)-1].Payload
	if _, has := pl["cost_usd"]; has || pl["cost_known"] != false || pl["cost_source"] != "unknown" {
		t.Fatalf("unknown cost payload %v", pl)
	}
}

func TestOpenCodeExplicitZeroCost(t *testing.T) {
	evs, res := ocRun(t, "opencode-zero-cost.jsonl", 0)
	pl := evs[len(evs)-1].Payload
	if res.Usage.CostSource != "harness" || pl["cost_known"] != true || pl["cost_usd"] != float64(0) {
		t.Fatalf("%+v %v", res.Usage, pl)
	}
	if _, has := pl["cached_tokens"]; has {
		t.Fatal("missing counter reported as zero")
	}
}

func TestOpenCodeToolErrorIsNotProviderFailure(t *testing.T) {
	evs, res := ocRun(t, "opencode-tool-error.jsonl", 0)
	if res.IsError || res.Final == "" {
		t.Fatalf("result %+v", res)
	}
	var out any
	for _, e := range evs {
		if e.Type == "tool_result" {
			if e.Payload["is_error"] != true {
				t.Fatal("tool error not flagged")
			}
			out = e.Payload["output"]
		}
	}
	if s, _ := out.(string); strings.Contains(s, "la_SECRET") || strings.Contains(s, "Bearer") {
		t.Fatalf("tool output not redacted: %v", out)
	}
}

func TestOpenCodeFailuresNeverFabricateSuccess(t *testing.T) {
	cases := []struct {
		name, file string
		exit       int
		limited    bool
		errHas     string
	}{
		{"auth error", "opencode-auth-error.jsonl", 1, false, "authentication"},
		{"auth error exit 0", "opencode-auth-error.jsonl", 0, false, "authentication"},
		{"quota", "opencode-quota-error.jsonl", 1, true, "quota"},
		{"malformed", "opencode-malformed.jsonl", 0, false, "malformed"},
		{"truncated stream", "opencode-truncated.jsonl", 0, false, "without completion"},
		{"stream ended, no exit status", "opencode-turn.jsonl", -1, false, "without completion"},
		{"nonzero exit after completion", "opencode-turn.jsonl", 2, false, "code 2"},
		{"killed", "opencode-turn.jsonl", 137, false, "code 137"},
	}
	for _, c := range cases {
		evs, res := ocRun(t, c.file, c.exit)
		if !res.IsError || res.Final != "" || res.UsageLimited != c.limited || !strings.Contains(strings.ToLower(res.Error), c.errHas) {
			t.Errorf("%s: %+v", c.name, res)
		}
		for _, e := range evs {
			if e.Type == "usage" && res.IsError && c.file != "opencode-turn.jsonl" && c.file != "opencode-malformed.jsonl" {
				t.Errorf("%s: usage emitted on failure", c.name)
			}
		}
	}
}

func TestOpenCodeRedactsSecretsInErrors(t *testing.T) {
	evs, res := ocRun(t, "opencode-auth-error.jsonl", 1)
	all := res.Error
	for _, e := range evs {
		b, _ := json.Marshal(e.Payload)
		all += string(b)
	}
	if strings.Contains(all, "sk-ant") || strings.Contains(all, ocToken) {
		t.Fatalf("secret in output: %s", all)
	}
	if !strings.Contains(redactSecrets("Authorization: Bearer abc.def"), "[redacted]") || strings.Contains(redactSecrets("Bearer abc.def"), "abc") {
		t.Fatal("bearer not redacted")
	}
}

func TestOpenCodeOversizedLineFails(t *testing.T) {
	p := (OpenCode{}).NewParser("openai/gpt-5").(*opencodeParser)
	p.Line([]byte(`{"type":"text","text":"` + strings.Repeat("a", opencodeMaxLine) + `"}`))
	p.Line([]byte(`{"type":"completed"}`))
	p.Exited(0)
	if r := p.Result(); !r.IsError || r.Final != "" {
		t.Fatalf("%+v", r)
	}
}

// The adapter is registered, but For hands out the fail-closed zero value: it
// rejects every run until the runner attests isolated config.
func TestOpenCodeRegisteredFailClosed(t *testing.T) {
	h, err := For(HarnessOpenCode)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Invocation(ocWS(), Turn{Spec: ocSpec("openai/gpt-5", nil), Prompt: "p"})
	if err == nil || !strings.Contains(err.Error(), "isolated") {
		t.Fatalf("zero-value adapter must reject runs for missing isolation, got %v", err)
	}
	c := agentdef.Capabilities[HarnessOpenCode]
	if c.ToolGating || c.PermissionPrompt || c.NativeCost {
		t.Fatalf("capabilities advertised before enforcement gates pass: %+v", c)
	}
}

// Real 2.2.1 captures (no model call): the error envelope nests type/message
// under "error" and carries sessionID in camel case.
func TestOpenCodeRealErrorEnvelopes(t *testing.T) {
	evs, res := ocRun(t, "opencode-real-auth-error.jsonl", 1)
	if !res.IsError || res.UsageLimited || res.SessionID != "ses_e5cebef69ffe3F8OaMgPKkU2wR" ||
		!strings.Contains(res.Error, "provider.auth") || !strings.Contains(res.Error, "authentication") {
		t.Fatalf("%+v", res)
	}
	if len(evs) == 0 || evs[0].Type != "lifecycle" {
		t.Fatalf("session start not emitted: %+v", evs)
	}
	_, res = ocRun(t, "opencode-real-no-route.jsonl", 1)
	if !res.IsError || !strings.Contains(res.Error, "Model unavailable") {
		t.Fatalf("%+v", res)
	}
}

func TestOpenCodeComputedAndUnknownCost(t *testing.T) {
	run := func(prices map[string]Price, usage string) Result {
		p := (OpenCode{Prices: prices}).NewParser("openai/gpt-5#high")
		p.Line([]byte(`{"type":"text","text":"ok"}`))
		p.Line([]byte(`{"type":"completed","usage":` + usage + `}`))
		p.(ExitObserver).Exited(0)
		return p.Result()
	}
	exact := map[string]Price{"openai/gpt-5#high": {Input: 2, CachedInput: 1, Output: 10}}
	r := run(exact, `{"input_tokens":1000000,"output_tokens":100000,"cached_tokens":500000}`)
	if r.IsError || r.Usage.CostSource != "computed" || r.Usage.CostUSD != 0.5*2+0.5*1+1 {
		t.Fatalf("%+v", r.Usage)
	}
	// Another provider's or a variant-less entry never prices this model.
	for name, prices := range map[string]map[string]Price{
		"none":       nil,
		"no variant": {"openai/gpt-5": {Input: 2, CachedInput: 1, Output: 10}},
		"bare model": {"gpt-5": {Input: 2, CachedInput: 1, Output: 10}},
	} {
		if r := run(prices, `{"input_tokens":10,"output_tokens":10}`); r.Usage.CostSource != "unknown" || r.Usage.CostUSD != 0 {
			t.Errorf("%s: %+v", name, r.Usage)
		}
	}
	// Missing counters are unavailable, not zero.
	if r := run(exact, `{"input_tokens":10}`); r.Usage.CostSource != "unknown" {
		t.Errorf("missing output counter priced: %+v", r.Usage)
	}
	if r := run(exact, `{"input_tokens":10,"output_tokens":10}`); r.Usage.CostSource != "unknown" {
		t.Errorf("missing cached counter priced although cached input is cheaper: %+v", r.Usage)
	}
	// Native cost, even zero, wins over computed.
	if r := run(exact, `{"input_tokens":10,"output_tokens":10,"cost":0}`); r.Usage.CostSource != "harness" {
		t.Errorf("%+v", r.Usage)
	}
}

func TestOpenCodeExternalStateRequiredAndProtectedRulesOrdered(t *testing.T) {
	for _, state := range []string{"", "relative", "/work/space", "/work/space/state"} {
		ws := ocWS()
		ws.OpenCodeStateDir = state
		if _, err := (OpenCode{ConfigIsolated: true}).Invocation(ws, Turn{Spec: ocSpec("openai/gpt-5", nil), Prompt: "work"}); err == nil {
			t.Fatalf("unsafe state accepted: %q", state)
		}
	}
	inv, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: ocSpec("openai/gpt-5", nil), Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := inv.Files["opencode.json"]; ok {
		t.Fatal("config staged in checkout")
	}
	if inv.Env["OPENCODE_CONFIG"] != "/work/.lazyagents/opencode/main/opencode.json" {
		t.Fatal(inv.Env)
	}
	cfg := ocConfig(t, inv)
	for _, resource := range []string{"write:*/.lazyagents/*", "edit:*/.lazyagents/*", "write:.lazyagents/*", "edit:.lazyagents/*"} {
		found := -1
		lastAllow := -1
		for i, p := range cfg.Policies {
			if p.Action == "tool.use" && p.Effect == "allow" {
				lastAllow = i
			}
			if p.Action == "tool.use" && p.Resource == resource && p.Effect == "deny" {
				found = i
			}
		}
		if found <= lastAllow {
			t.Fatalf("protected rule missing or before allow: %s", resource)
		}
	}
}

func TestOpenCodeWebToolsFollowWebReadScope(t *testing.T) {
	for mode, want := range map[agentdef.Mode]string{agentdef.Allowed: "allow", agentdef.Denied: "deny"} {
		spec := ocSpec("anthropic/claude-sonnet-5-5", nil, ocScope("web:read", mode))
		inv, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: spec, Prompt: "p"})
		if err != nil {
			t.Fatal(err)
		}
		c := ocConfig(t, inv)
		for _, tool := range []string{"webfetch", "websearch"} {
			if effective(c.Policies, "tool.use", tool) != want || effective(c.Permissions, tool, "*") != want {
				t.Errorf("web:read=%s: %s want %s", mode, tool, want)
			}
		}
	}
	spec := ocSpec("anthropic/claude-sonnet-5-5", nil, ocScope("web:read", agentdef.RequiresApproval))
	if _, err := (OpenCode{ConfigIsolated: true}).Invocation(ocWS(), Turn{Spec: spec, Prompt: "p"}); err == nil {
		t.Error("opencode accepted web:read requires_approval it can't enforce")
	}
}
