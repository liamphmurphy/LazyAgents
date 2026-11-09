package runner

import (
	"archive/tar"
	"context"
	"io"
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestHostConfigIsHardened(t *testing.T) {
	host := (&Docker{Network: "bridge", MemoryMB: 512, PidsLimit: 100}).hostConfig(nil)
	if caps, _ := host["CapDrop"].([]string); !slices.Equal(caps, []string{"ALL"}) {
		t.Fatalf("CapDrop = %v", host["CapDrop"])
	}
	if opts, _ := host["SecurityOpt"].([]string); !slices.Contains(opts, "no-new-privileges") {
		t.Fatalf("SecurityOpt = %v", host["SecurityOpt"])
	}
	if host["Memory"] != int64(512<<20) || host["PidsLimit"] != int64(100) {
		t.Fatalf("limits = %v %v", host["Memory"], host["PidsLimit"])
	}
	if _, ok := (&Docker{}).hostConfig(nil)["Memory"]; ok {
		t.Fatal("0 should leave the memory limit off")
	}
}

func TestCodexAuthTarIsOwnedByTheAgentUser(t *testing.T) {
	r, err := codexAuthTar(`{"x":1}`)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(r)
	if h, err := tr.Next(); err != nil || h.Typeflag != tar.TypeDir || h.Uid != 1000 {
		t.Fatalf("dir header %+v %v", h, err)
	}
	h, err := tr.Next()
	if err != nil || h.Name != ".codex/auth.json" || h.Uid != 1000 || h.Mode != 0o600 {
		t.Fatalf("file header %+v %v", h, err)
	}
	if b, _ := io.ReadAll(tr); string(b) != `{"x":1}` {
		t.Fatalf("body %s", b)
	}
}

type fakeRevealer map[string]string

func (f fakeRevealer) Reveal(_ context.Context, _ uuid.UUID, _ *uuid.UUID, name string) (string, error) {
	return f[name], nil
}

func TestSecretEnvDeliversPlanLogins(t *testing.T) {
	const login = `{"auth_mode":"chatgpt","tokens":{"access_token":"tok-access","refresh_token":"tok-refresh","id_token":"tok-id","account_id":"a"}}`
	plan := []SecretDecision{{Name: "harness:anthropic", Injected: true, Env: []string{"ANTHROPIC_API_KEY"}}, {Name: "harness:openai", Injected: true, Env: []string{"CODEX_API_KEY", "OPENAI_API_KEY"}}}
	ws, project := uuid.New(), uuid.New()
	env, err := SecretEnv(context.Background(), fakeRevealer{"harness:anthropic": "sk-ant-oat01-x", "harness:openai": login}, ws, &project, plan)
	if err != nil {
		t.Fatal(err)
	}
	if env[ClaudeOAuthEnv] != "sk-ant-oat01-x" || env["ANTHROPIC_API_KEY"] != "" {
		t.Fatalf("claude env %v", env)
	}
	if env[CodexAuthEnv] != login || env["OPENAI_API_KEY"] != "" || env["CODEX_API_KEY"] != "" {
		t.Fatalf("codex env %v", env)
	}
	gotWS, gotProject, err := ParseCodexAuthScope(env[CodexAuthScopeEnv])
	if err != nil || gotWS != ws || *gotProject != project {
		t.Fatalf("scope %v %v %v", gotWS, gotProject, err)
	}
	if got := redact("leaked tok-refresh here", sensitiveEnv(env)); got != "leaked [redacted] here" {
		t.Fatalf("redact = %q", got)
	}
	keys, err := SecretEnv(context.Background(), fakeRevealer{"harness:anthropic": "sk-ant-api03-x", "harness:openai": "sk-proj-x"}, ws, nil, plan)
	if err != nil || keys["ANTHROPIC_API_KEY"] != "sk-ant-api03-x" || keys["OPENAI_API_KEY"] != "sk-proj-x" || keys[ClaudeOAuthEnv] != "" {
		t.Fatalf("api keys %v %v", keys, err)
	}
}
