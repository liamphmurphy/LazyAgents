package httpapi

import (
	"context"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
)

func TestOnboardingCompleteIsRecordedOnce(t *testing.T) {
	e := newChatEnv(t)
	me := e.want("GET", "/me", nil, http.StatusOK)
	if me["onboarded_at"] != nil || me["harness_connected"] != true {
		t.Fatalf("fresh profile %v: want no onboarded_at and harness_connected (local CLI logins)", me)
	}
	first := e.want("POST", "/me/onboarding/complete", nil, http.StatusOK)["onboarded_at"]
	if first == nil {
		t.Fatal("onboarded_at not set")
	}
	// Running setup again from Settings keeps the first completion.
	if again := e.want("POST", "/me/onboarding/complete", nil, http.StatusOK)["onboarded_at"]; again != first {
		t.Fatalf("onboarded_at moved from %v to %v", first, again)
	}
	boot := e.want("GET", "/bootstrap", nil, http.StatusOK)
	if boot["me"].(map[string]any)["onboarded_at"] != first {
		t.Fatalf("bootstrap me %v", boot["me"])
	}
	// Teammates never see it.
	for _, u := range boot["users"].([]any) {
		if _, ok := u.(map[string]any)["onboarded_at"]; ok {
			t.Fatalf("teammate %v carries onboarded_at", u)
		}
	}
}

// With API-key runs (the Docker backend), Verify stores the pasted key as the
// harness secret and discovers models with it.
func TestVerifyHarnessStoresKeyAndDiscovers(t *testing.T) {
	var seen []string
	var srv *Server
	e := newChatEnvServer(t, func(rs *runs.Service) {
		rs.RequireHarnessKey = true
		rs.ModelDiscovery = func(_ context.Context, h, key string) ([]chatmodels.Model, error) {
			seen = append(seen, h+"="+key)
			if key == "sk-ant-api03-revoked" {
				return nil, context.DeadlineExceeded
			}
			return []chatmodels.Model{{ID: "opus", Label: "Opus", Default: true}, {ID: "sonnet", Label: "Sonnet"}, {ID: "haiku", Label: "Haiku"}}, nil
		}
	}, func(s *Server) { srv = s })
	if me := e.want("GET", "/me", nil, http.StatusOK); me["harness_connected"] != false {
		t.Fatalf("no key yet, got %v", me["harness_connected"])
	}
	key := map[string]any{"kind": "api_key", "credential": "sk-ant-api03-good"}
	// Secrets off: explained, nothing stored.
	if out := e.want("POST", "/harness/claude-code/verify", key, http.StatusConflict); out["message"] == nil {
		t.Fatalf("secrets off: %v", out)
	}
	box := &secrets.Service{Q: e.q, Box: testBox(t)}
	e.runs.Secrets = box
	srv.Secrets = box

	e.invalid("POST", "/harness/claude-code/verify", map[string]any{"kind": "subscription", "credential": "sk-ant-api03-good"}, "credential", "sk-ant-oat")
	e.invalid("POST", "/harness/claude-code/verify", map[string]any{"kind": "api_key", "credential": "sk-ant-oat01-x"}, "credential", "sk-ant-")
	e.invalid("POST", "/harness/codex/verify", map[string]any{"kind": "subscription", "credential": "sk-proj-1"}, "credential", "auth.json")
	e.invalid("POST", "/harness/codex/verify", map[string]any{"kind": "api_key", "credential": "nope"}, "credential", "sk-")
	if len(seen) != 0 {
		t.Fatalf("a malformed credential reached discovery: %v", seen)
	}

	out := e.want("POST", "/harness/claude-code/verify", key, http.StatusOK)
	if out["ok"] != true || len(out["models"].([]any)) != 3 || out["error"] != nil || out["credential"] != nil {
		t.Fatalf("verify: %v", out)
	}
	if got, err := box.Reveal(context.Background(), e.me.WorkspaceID, nil, "harness:anthropic"); err != nil || got != "sk-ant-api03-good" {
		t.Fatalf("stored %q, %v", got, err)
	}
	if seen[len(seen)-1] != "claude-code=sk-ant-api03-good" {
		t.Fatalf("discovery saw %v", seen)
	}
	if me := e.want("GET", "/me", nil, http.StatusOK); me["harness_connected"] != true {
		t.Fatalf("after verify, harness_connected = %v", me["harness_connected"])
	}

	// A key the provider refuses: not ok, a public reason, never the key.
	out = e.want("POST", "/harness/claude-code/verify", map[string]any{"kind": "api_key", "credential": "sk-ant-api03-revoked"}, http.StatusOK)
	if out["ok"] != false || out["error"] == nil || len(out["models"].([]any)) != 0 {
		t.Fatalf("refused key: %v", out)
	}
	// Codex has no key: checking without a credential reports why.
	out = e.want("POST", "/harness/codex/verify", nil, http.StatusOK)
	if out["ok"] != false || out["error"] != "no harness:openai secret" {
		t.Fatalf("codex without key: %v", out)
	}
}

// With the host's CLI logins (the local backend), Verify asks the CLI over its
// control protocol: here the repository's fake-harness standing in for both.
func TestVerifyHarnessAgainstFakeHarness(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-harness")
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/liamphmurphy/vesper/server/cmd/fake-harness").CombinedOutput(); err != nil {
		t.Fatalf("build fake-harness: %v\n%s", err, out)
	}
	t.Setenv("RUNNER_CLAUDE_BIN", bin)
	t.Setenv("RUNNER_CODEX_BIN", bin)
	e := newChatEnvWith(t, func(rs *runs.Service) { rs.ModelDiscovery = nil })
	for harness, model := range map[string]string{"claude-code": "sonnet", "codex": "gpt-5-codex"} {
		out := e.want("POST", "/harness/"+harness+"/verify", nil, http.StatusOK)
		models, _ := out["models"].([]any)
		if out["ok"] != true || len(models) != 1 || models[0].(map[string]any)["id"] != model {
			t.Fatalf("%s: %v", harness, out)
		}
	}
	t.Setenv("RUNNER_CLAUDE_BIN", filepath.Join(dir, "missing"))
	if out := e.want("POST", "/harness/claude-code/verify", nil, http.StatusOK); out["ok"] != false || out["error"] == nil {
		t.Fatalf("missing CLI: %v", out)
	}
}
