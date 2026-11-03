package harness_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/harness"
	"github.com/liamphmurphy/lazyagents/server/internal/runner"
)

// Criterion 5: the runner's process handling, driven with the adapter's real
// argv/env and a stub `opencode` that spawns a child (as --standalone's
// private server would). Killing the process must take the child with it.
func TestOpenCodeCancelKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	pidFile := filepath.Join(root, "child.pid")
	stub := filepath.Join(root, "opencode-stub")
	script := "#!/bin/sh\nsleep 300 &\necho $! > " + pidFile + "\necho '{\"type\":\"session\",\"session_id\":\"ses_stub\"}'\nwait\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	l := &runner.Local{Dir: filepath.Join(root, "runs")}
	if err := l.EnsureWorkspace(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	ws := harness.Workspace{Dir: l.WorkspaceDir("r1"), MCPURL: "http://mcp.test/mcp", Token: "la_TESTTOKEN12345"}
	spec := agentdef.Resolved{Harness: harness.HarnessOpenCode, Model: "openai/gpt-5"}
	inv, err := harness.OpenCode{ConfigIsolated: true}.Invocation(ws, harness.Turn{Spec: spec, Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}
	argv := append([]string{stub}, inv.Argv[1:]...)
	proc, err := l.Start(context.Background(), "r1", runner.ExecSpec{Name: "r1", Argv: argv, Env: inv.Env, Files: inv.Files})
	if err != nil {
		t.Fatal(err)
	}
	// The stream is live and parsed before cancellation.
	buf := make([]byte, 128)
	n, _ := proc.Stdout().Read(buf)
	p := harness.OpenCode{ConfigIsolated: true}.NewParser(spec.Model)
	p.Line([]byte(strings.TrimSpace(string(buf[:n]))))
	if p.Result().SessionID != "ses_stub" {
		t.Fatalf("session not read from live stream: %q", buf[:n])
	}
	var childPID int
	for i := 0; i < 100 && childPID == 0; i++ {
		if b, err := os.ReadFile(pidFile); err == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		} else {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if childPID == 0 {
		t.Fatal("stub child never started")
	}
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	code, _ := proc.Wait()
	if code != 128+int(syscall.SIGKILL) {
		t.Fatalf("exit code %d", code)
	}
	_, _ = io.Copy(io.Discard, proc.Stdout())
	// A killed turn is a failed turn, never a successful reply.
	p.(interface{ Exited(int) }).Exited(code)
	if r := p.Result(); !r.IsError || r.Final != "" {
		t.Fatalf("killed turn result %+v", r)
	}
	for i := 0; i < 100; i++ {
		if syscall.Kill(childPID, 0) != nil {
			return // child gone
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child process %d survived", childPID)
}
