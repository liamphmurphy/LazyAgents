package agentdef

import (
	"reflect"
	"testing"
)

func base() Spec {
	s := Spec{
		Harness: HarnessClaudeCode, Model: "opus-5.5", Instructions: "You own backend changes.\n",
		Scopes: map[string]Mode{
			"repo:write": Allowed, "tests:run": Allowed, "db:migrate": RequiresApproval, "deploy:staging": Denied,
		},
		Budget:         Budget{MaxRuntime: "2h", MaxTurns: 20},
		MCPServers:     []MCPServer{{Name: "context7", URL: "https://mcp.context7.com/mcp"}},
		HarnessOptions: map[string]map[string]string{"claude-code": {"permission_mode": "acceptEdits"}, "codex": {"sandbox": "workspace-write"}},
	}
	if err := s.Normalize(); err != nil {
		panic(err)
	}
	return s
}

func scopes(r Resolved) map[string]Mode {
	m := map[string]Mode{}
	for _, s := range r.Scopes {
		m[s.Scope] = s.Mode
	}
	return m
}

func TestResolveWithoutOverrideIsTheAgent(t *testing.T) {
	r := Resolve(base(), nil)
	if r.Harness != "claude-code" || r.Model != "opus-5.5" || len(r.Provenance) != 0 {
		t.Fatalf("resolved = %+v", r)
	}
	if r.HarnessOptions["permission_mode"] != "acceptEdits" || r.HarnessOptions["sandbox"] != "" {
		t.Fatalf("only the resolved harness's options should apply: %v", r.HarnessOptions)
	}
}

func TestProjectsCanOnlyTighten(t *testing.T) {
	o := &Override{Scopes: map[string]Mode{
		"repo:write":     Denied,  // tighten: applies
		"db:migrate":     Allowed, // loosen: ignored
		"deploy:staging": Allowed, // loosen: ignored
		"tests:run":      RequiresApproval,
		"secrets:read":   Allowed, // the agent never granted it: stays denied
	}}
	if err := o.Normalize(); err != nil {
		t.Fatal(err)
	}
	r := Resolve(base(), o)
	want := map[string]Mode{
		"repo:write": Denied, "db:migrate": RequiresApproval, "deploy:staging": Denied,
		"tests:run": RequiresApproval, "secrets:read": Denied,
	}
	if got := scopes(r); !reflect.DeepEqual(got, want) {
		t.Fatalf("scopes = %v\nwant     %v", got, want)
	}
	for _, s := range r.Scopes {
		wantSrc := FromProject
		if s.Scope == "db:migrate" || s.Scope == "deploy:staging" {
			wantSrc = FromAgent // the project's looser value didn't take
		}
		if s.Source != wantSrc {
			t.Errorf("%s source = %s, want %s", s.Scope, s.Source, wantSrc)
		}
	}
}

func TestWildcardDenialCoversMatchingScopes(t *testing.T) {
	s := base()
	s.Scopes["prod:read"] = Allowed
	s.Scopes["prod:deploy"] = RequiresApproval
	o := &Override{Scopes: map[string]Mode{"prod:*": Denied}}
	r := Resolve(s, o)
	for _, sc := range []string{"prod:read", "prod:deploy", "prod:anything-later"} {
		if m := r.Mode(sc); m != Denied {
			t.Errorf("%s = %s, want denied", sc, m)
		}
	}
	if r.Mode("repo:write") != Allowed {
		t.Error("wildcard leaked onto another prefix")
	}
	// An agent-level wildcard can't be loosened by a project's exact scope.
	s2 := base()
	s2.Scopes["prod:*"] = Denied
	r2 := Resolve(s2, &Override{Scopes: map[string]Mode{"prod:read": Allowed}})
	if r2.Mode("prod:read") != Denied {
		t.Error("project loosened an agent wildcard")
	}
}

func TestScalarsAppendsAndBudget(t *testing.T) {
	o := &Override{
		Harness: "codex", InstructionsAppend: "Follow ADR-012 for partitioning.",
		MCPServersAdd: []MCPServer{{Name: "postgres-staging", URL: "https://pg.internal/mcp"}, {Name: "context7", URL: "https://evil.example/mcp"}},
		Budget:        Budget{MaxRuntime: "3h", MaxTurns: 10}, // runtime looser (ignored), turns tighter (applies)
		Setup:         []string{"make deps"},
	}
	if err := o.Normalize(); err != nil {
		t.Fatal(err)
	}
	r := Resolve(base(), o)
	if r.Harness != "codex" || r.HarnessOptions["sandbox"] != "workspace-write" || r.HarnessOptions["permission_mode"] != "" {
		t.Fatalf("harness swap: %s %v", r.Harness, r.HarnessOptions)
	}
	if r.Instructions != "You own backend changes.\n\nFollow ADR-012 for partitioning.\n" {
		t.Fatalf("instructions = %q", r.Instructions)
	}
	if len(r.MCPServers) != 2 || r.MCPServers[0].URL != "https://mcp.context7.com/mcp" || r.MCPServers[1].Name != "postgres-staging" {
		t.Fatalf("a project may add servers but not redefine the agent's: %+v", r.MCPServers)
	}
	if r.Budget != (Budget{MaxRuntime: "2h", MaxTurns: 10}) {
		t.Fatalf("budget = %+v", r.Budget)
	}
	for _, f := range []string{"harness", "instructions", "mcp_servers", "budget", "setup"} {
		if r.Provenance[f] != FromProject {
			t.Errorf("provenance[%s] = %q", f, r.Provenance[f])
		}
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]Spec{
		"harness":  {Harness: "aider", Model: "x"},
		"model":    {Harness: "codex"},
		"scope":    {Model: "x", Scopes: map[string]Mode{"Repo Write": Allowed}},
		"mode":     {Model: "x", Scopes: map[string]Mode{"repo:write": "sometimes"}},
		"budget":   {Model: "x", Budget: Budget{MaxRuntime: "two hours"}},
		"reserved": {Model: "x", MCPServers: []MCPServer{{Name: "lazyagents", URL: "http://x"}}},
		"url":      {Model: "x", MCPServers: []MCPServer{{Name: "files", URL: "file:///etc"}}},
	}
	for name, s := range cases {
		if err := s.Normalize(); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	ok := Spec{Model: "opus-5.5"}
	if err := ok.Normalize(); err != nil || ok.Harness != HarnessClaudeCode || ok.MaxConcurrency != 1 {
		t.Fatalf("defaults: %+v %v", ok, err)
	}
}

func TestEnforcement(t *testing.T) {
	cases := []struct {
		scope   string
		mode    Mode
		harness string
		want    Enforcement
	}{
		{"db:migrate", RequiresApproval, HarnessClaudeCode, ByBroker},
		{"repo:write", Allowed, HarnessCodex, ByCredential},
		{"tests:run", Allowed, HarnessClaudeCode, ByHarness},
		{"tests:run", Allowed, HarnessCodex, ByNothing},
		{"tests:run", RequiresApproval, HarnessClaudeCode, ByHarness},
		{"tests:run", RequiresApproval, HarnessCodex, ByNothing},
		{"deploy:staging", Denied, HarnessClaudeCode, ByCredential},
	}
	for _, c := range cases {
		if got, _ := EnforcementFor(c.scope, c.mode, c.harness); got != c.want {
			t.Errorf("%s %s on %s = %s, want %s", c.scope, c.mode, c.harness, got, c.want)
		}
	}
}

func TestChanges(t *testing.T) {
	o := Override{Harness: "codex", Scopes: map[string]Mode{"prod:*": Denied}, InstructionsAppend: "Follow ADR-012.\nMore."}
	want := []string{"harness → codex", "+ instructions: “Follow ADR-012.”", "prod:* → denied"}
	if got := o.Changes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Changes = %q", got)
	}
}

func TestSkillValidation(t *testing.T) {
	ok := Skill{Name: "go-errors", Description: "  Use when\nhandling errors ", Body: "  body \n"}
	s := base()
	s.Skills = []Skill{ok}
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	if got := s.Skills[0]; got.Description != "Use when handling errors" || got.Body != "body" {
		t.Fatalf("not normalized: %+v", got)
	}
	if r := Resolve(s, nil); len(r.Skills) != 1 || r.Skills[0].Name != "go-errors" {
		t.Fatalf("resolved skills %+v", r.Skills)
	}
	for name, sk := range map[string][]Skill{
		"bad name":    {{Name: "Go Errors", Description: "d", Body: "b"}},
		"no desc":     {{Name: "a", Body: "b"}},
		"no body":     {{Name: "a", Description: "d"}},
		"duplicate":   {ok, ok},
		"huge body":   {{Name: "a", Description: "d", Body: string(make([]byte, MaxSkillBodyBytes+1))}},
		"path escape": {{Name: "../x", Description: "d", Body: "b"}},
	} {
		s := base()
		s.Skills = sk
		if s.Normalize() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
