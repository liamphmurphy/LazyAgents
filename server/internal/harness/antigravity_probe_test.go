package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

// TestAgyProbe drives the real `agy` CLI without spending anything, as the wiki
// pattern "Verify harness adapters against the real CLI safely" asks: env -i, a
// temp HOME, a fake API key and every proxy pointed at a dead port, so the turn
// fails at the network and nothing reaches Google. It checks the parts of the
// contract docs/contracts/antigravity.md marks unverified that can be checked
// without a model: the stream's shape, that our settings file loads, and that a
// failure reaches the parser as an error.
//
// Opt-in: AGY_PROBE=1 AGY_BIN=/path/to/agy. AGY_RECORD=<dir> also writes what
// agy printed to <dir>/antigravity-error.jsonl, to replace the synthetic fixture.
func TestAgyProbe(t *testing.T) {
	bin := os.Getenv("AGY_BIN")
	if os.Getenv("AGY_PROBE") != "1" || bin == "" {
		t.Skip("set AGY_PROBE=1 and AGY_BIN to probe the real agy")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("no agy at %s", bin)
	}
	home := t.TempDir()
	ws := t.TempDir()
	// The settings our launcher writes for a read-only chat: the CLI must accept them.
	inv, err := (Antigravity{}).Invocation(Workspace{Dir: ws, MCPURL: "http://127.0.0.1:9/mcp", Token: "probe", TokenEnv: "LAZYAGENTS_TOKEN", ContainerSandbox: true},
		Turn{Spec: probeSpec(), Prompt: "Reply with the single word ok."})
	if err != nil {
		t.Fatal(err)
	}
	var cfg agyConfig
	if err := json.Unmarshal(inv.Files[agyConfigPath], &cfg); err != nil {
		t.Fatal(err)
	}
	write := func(rel string, v any) {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(v)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".gemini/antigravity-cli/settings.json", cfg.Settings)
	write(".gemini/config/mcp_config.json", map[string]any{"mcpServers": cfg.MCPServers})

	env := []string{"HOME=" + home, "PATH=/usr/bin:/bin", "GEMINI_API_KEY=AIzaProbeNotARealKey", "NO_COLOR=1",
		"HTTPS_PROXY=http://127.0.0.1:9", "HTTP_PROXY=http://127.0.0.1:9", "ALL_PROXY=http://127.0.0.1:9", "NO_PROXY="}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput(); err != nil {
		t.Fatalf("agy --version: %v\n%s", err, out)
	} else {
		t.Logf("agy --version: %s", strings.TrimSpace(string(out)))
	}

	cmd := exec.CommandContext(ctx, bin, "-p", "Reply with the single word ok.", "--output-format", "stream-json")
	cmd.Dir, cmd.Env = ws, env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	t.Logf("exit: %v\nstderr: %s", runErr, stderr.String())
	if dir := os.Getenv("AGY_RECORD"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "antigravity-error.jsonl"), stdout.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	p := (Antigravity{}).NewParser("")
	events := map[string]int{}
	sc := bufio.NewScanner(&stdout)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		var l struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil || l.Event == "" {
			t.Errorf("stdout line without an event field: %q", sc.Text())
			continue
		}
		events[l.Event]++
		p.Line(sc.Bytes())
	}
	t.Logf("events seen: %v", events)
	if events["init"] != 1 || events["result"] != 1 {
		t.Errorf("want one init and one result, got %v (the docs say each appears once)", events)
	}
	if res := p.Result(); !res.IsError {
		t.Errorf("a turn with no network must reach the parser as a failure: %+v", res)
	}
}

func probeSpec() (s agentdef.Resolved) {
	s.Harness, s.Model = "antigravity", ""
	s.HarnessOptions = map[string]string{"readonly": "true"}
	return s
}
