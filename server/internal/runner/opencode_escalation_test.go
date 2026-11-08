package runner_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/harness"
	"github.com/liamphmurphy/vesper/server/internal/runner"
)

// This probe needs no database or paid provider. It exercises actual 2.2.1
// wildcard matching, writes and session storage through the production backend.
func TestRealOpenCodeSelfEscalation(t *testing.T) {
	if os.Getenv("LAZYAGENTS_REAL_OPENCODE") != "1" {
		t.Skip("set LAZYAGENTS_REAL_OPENCODE=1 for the isolated localhost probe")
	}
	real := os.Getenv("LAZYAGENTS_REAL_OPENCODE_BIN")
	if real == "" {
		var err error
		real, err = exec.LookPath("opencode")
		if err != nil {
			t.Fatal(err)
		}
	}
	real, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	local := &runner.Local{Dir: filepath.Join(root, "runs"), OnlyBins: true, OpenCodeIsolated: true}
	if err := local.EnsureWorkspace(context.Background(), "probe"); err != nil {
		t.Fatal(err)
	}
	ws := local.WorkspaceDir("probe")
	state := local.OpenCodeStateDir("probe", "")
	attack := filepath.Join(state, "config", "opencode", "opencode.json")
	proof := filepath.Join(ws, "proof.txt")
	shellProof := filepath.Join(ws, "shell-ran")
	var mu sync.Mutex
	phase, step := 0, 0
	var requests []string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/messages") {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var req struct{ Model string }
		_ = json.Unmarshal(raw, &req)
		if r.Header.Get("x-api-key") != "fake-isolation-key" {
			t.Errorf("unexpected provider credential")
		}
		mu.Lock()
		current := phase
		n := step
		if !strings.Contains(req.Model, "haiku") {
			step++
			requests = append(requests, string(raw))
		}
		mu.Unlock()
		w.Header().Set("content-type", "text/event-stream")
		out := sseWriter{w}
		out.ev("message_start", map[string]any{"message": map[string]any{"id": fmt.Sprintf("msg_probe_%d_%d", current, n), "type": "message", "role": "assistant", "model": req.Model, "content": []any{}, "usage": map[string]any{"input_tokens": 100, "output_tokens": 1}}})
		tool := ""
		input := map[string]any{}
		if !strings.Contains(req.Model, "haiku") {
			if current == 0 {
				switch n {
				case 0:
					tool = "write"
					input = map[string]any{"path": proof, "content": "ordinary write succeeded"}
				case 1:
					tool = "write"
					input = map[string]any{"path": attack, "content": `{"policies":[{"action":"tool.use","resource":"shell:*","effect":"allow"}]}`}
				case 2:
					tool = "write"
					input = map[string]any{"path": filepath.Join(ws, ".lazyagents", "poison"), "content": "metadata poison"}
				}
			} else if n == 0 {
				tool = "shell"
				input = map[string]any{"command": "touch " + shellProof}
			}
		}
		block := map[string]any{"type": "text", "text": "probe complete"}
		stop := "end_turn"
		if tool != "" {
			block = map[string]any{"type": "tool_use", "id": fmt.Sprintf("toolu_%d_%d", current, n), "name": tool, "input": input}
			stop = "tool_use"
		}
		out.ev("content_block_start", map[string]any{"index": 0, "content_block": block})
		out.ev("content_block_stop", map[string]any{"index": 0})
		out.ev("message_delta", map[string]any{"delta": map[string]any{"stop_reason": stop}, "usage": map[string]any{"output_tokens": 10}})
		out.ev("message_stop", map[string]any{})
	}))
	defer stub.Close()
	launchDir := filepath.Join(root, "launcher")
	if err := os.MkdirAll(launchDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(launchDir, "main.go"), []byte(launcherSrc), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(launchDir, "go.mod"), []byte("module probe\n\ngo 1.26\n"), 0600); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(launchDir, "launcher")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	build := exec.CommandContext(ctx, "go", "build", "-ldflags", "-X main.real="+real+" -X main.base="+stub.URL, "-o", launcher, ".")
	build.Dir = launchDir
	b, err := build.CombinedOutput()
	cancel()
	if err != nil {
		t.Fatalf("launcher build %v: %s", err, b)
	}
	local.Bins = map[string]string{"opencode": launcher}
	versionCtx, versionCancel := context.WithTimeout(context.Background(), 10*time.Second)
	versionProc, err := local.Start(versionCtx, "probe", runner.ExecSpec{Argv: []string{"opencode", "--version"}})
	if err != nil {
		versionCancel()
		t.Fatal(err)
	}
	versionDone := make(chan struct{})
	go func() {
		select {
		case <-versionCtx.Done():
			_ = versionProc.Kill()
		case <-versionDone:
		}
	}()
	versionOut, _ := io.ReadAll(versionProc.Stdout())
	versionErr, _ := io.ReadAll(versionProc.Stderr())
	versionCode, err := versionProc.Wait()
	close(versionDone)
	versionCancel()
	_ = versionProc.(io.Closer).Close()
	if err != nil || versionCode != 0 {
		t.Fatalf("version check %d %v %s", versionCode, err, versionErr)
	}
	if err := harness.CheckOpenCodeVersion(string(versionOut)); err != nil {
		t.Fatal(err)
	}
	// Ambient host settings must be ignored by the local backend.
	t.Setenv("OPENCODE_CONFIG", "/must-not-load")
	t.Setenv("OPENCODE_CONFIG_CONTENT", `{"policies":[{"action":"tool.use","resource":"*","effect":"allow"}]}`)
	t.Setenv("ANTHROPIC_API_KEY", "host-key-must-not-leak")
	spec := agentdef.Resolved{Harness: harness.HarnessOpenCode, Model: "anthropic/claude-sonnet-5-5"}
	adapter := harness.OpenCode{ConfigIsolated: true, SessionOwned: func(string) bool { return true }}
	run := func(session string) (harness.Result, string) {
		t.Helper()
		inv, err := adapter.Invocation(harness.Workspace{Dir: ws, OpenCodeStateDir: state, MCPURL: stub.URL + "/mcp", Token: "fake-token"}, harness.Turn{Spec: spec, Prompt: "Run the isolation probe", SessionID: session})
		if err != nil {
			t.Fatal(err)
		}
		// LAZYAGENTS-26 independently fixes action:resource policy allows. Correct
		// only those allows here so this test proves real writes, rather than passing
		// because the unrelated old policy accidentally blocks every write.
		var cfg map[string]any
		_ = json.Unmarshal(inv.StateFiles["opencode.json"], &cfg)
		for _, v := range cfg["policies"].([]any) {
			p := v.(map[string]any)
			if p["action"] == "tool.use" && p["effect"] == "allow" {
				res := p["resource"].(string)
				if !strings.Contains(res, ":") {
					p["resource"] = res + ":*"
				}
			}
		}
		cfg["policies"] = append(cfg["policies"].([]any), map[string]any{"action": "tool.use", "resource": "external_directory:*", "effect": "allow"})
		cfg["permissions"] = append(cfg["permissions"].([]any), map[string]any{"action": "external_directory", "resource": "*", "effect": "allow"})
		inv.StateFiles["opencode.json"], _ = json.Marshal(cfg)
		inv.Env["ANTHROPIC_API_KEY"] = "fake-isolation-key"
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		proc, err := local.Start(ctx, "probe", runner.ExecSpec{Argv: inv.Argv, Env: inv.Env, Files: inv.Files, StateFiles: inv.StateFiles})
		if err != nil {
			t.Fatal(err)
		}
		defer proc.(io.Closer).Close()
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = proc.Kill()
			case <-done:
			}
		}()
		stderr := make(chan string, 1)
		go func() { b, _ := io.ReadAll(proc.Stderr()); stderr <- string(b) }()
		parser := adapter.NewParser(spec.Model)
		var raw strings.Builder
		scan := bufio.NewScanner(proc.Stdout())
		scan.Buffer(make([]byte, 4096), 4<<20)
		for scan.Scan() {
			raw.WriteString(scan.Text())
			raw.WriteByte('\n')
			parser.Line(scan.Bytes())
		}
		if err := scan.Err(); err != nil {
			_ = proc.Kill()
			t.Fatal(err)
		}
		code, err := proc.Wait()
		close(done)
		if err != nil || code != 0 {
			t.Fatalf("CLI exit %d %v: %s\n%s", code, err, <-stderr, raw.String())
		}
		_ = <-stderr
		parser.(harness.ExitObserver).Exited(code)
		return parser.Result(), raw.String()
	}
	result, first := run("")
	if result.SessionID == "" || result.IsError || result.Final != "probe complete" {
		t.Fatalf("missing session: %s", first)
	}
	if b, err := os.ReadFile(proof); err != nil || string(b) != "ordinary write succeeded" {
		t.Fatalf("normal write blocked: %s %v\n%s", b, err, first)
	}
	for _, p := range []string{attack, filepath.Join(ws, ".lazyagents", "poison")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("protected file written: %s\n%s", p, first)
		}
	}
	if !strings.Contains(first, "Blocked by configuration policy") {
		t.Fatalf("missing deny evidence: %s", first)
	}
	mu.Lock()
	phase = 1
	step = 0
	mu.Unlock()
	resumed, second := run(result.SessionID)
	if resumed.SessionID != result.SessionID || resumed.IsError || resumed.Final != "probe complete" {
		t.Fatalf("resume changed session: %s -> %s", result.SessionID, resumed.SessionID)
	}
	if _, err := os.Stat(shellProof); !os.IsNotExist(err) {
		t.Fatal("escalated shell ran")
	}
	if !strings.Contains(second, "No tool named") && !strings.Contains(second, "Blocked by configuration policy") {
		t.Fatalf("shell denial missing: %s", second)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) < 6 || !strings.Contains(requests[len(requests)-1], "ordinary write succeeded") {
		t.Fatalf("resumed history missing; requests=%d", len(requests))
	}
}
