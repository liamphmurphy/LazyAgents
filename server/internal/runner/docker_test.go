package runner_test

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// TestDockerBackend runs the approval round trip in real containers. It
// needs Docker and the agent-base image, so it's opt-in:
//
//	RUNNER_DOCKER_TEST=1 go test ./internal/runner -run Docker
//
// The fake harness replaces `claude` in a throwaway image, and no API keys
// are injected, so no real model is ever called.
func TestDockerBackend(t *testing.T) {
	if os.Getenv("RUNNER_DOCKER_TEST") == "" {
		t.Skip("set RUNNER_DOCKER_TEST=1 (needs Docker and vesper/agent-base)")
	}
	const image = "vesper/agent-base:runner-test"
	docker, err := exec.LookPath("docker") // before setup narrows PATH
	if err != nil {
		t.Skip("docker not installed")
	}
	e := setupWith(t, func(t *testing.T, dir, bin, base string) (runner.Backend, string) {
		df := "FROM vesper/agent-base:latest\nUSER root\nCOPY fake-claude /usr/local/bin/claude\nRUN rm -f /usr/local/bin/codex\nUSER node\n"
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(df), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(docker, "build", "-q", "-t", image, dir).CombinedOutput(); err != nil {
			t.Fatalf("build test image: %v\n%s", err, out)
		}
		u, _ := url.Parse(base)
		d := &runner.Docker{Socket: dockerSocket(), Image: image}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := d.Ping(ctx); err != nil {
			t.Fatal(err)
		}
		// Containers reach the test server on the host.
		return d, "http://host.docker.internal:" + u.Port() + "/mcp"
	})
	e.dispatch("PAY-214")
	e.until("waiting for approval", e.state("PAY-214=waiting"))
	run := e.run("PAY-214")
	if run.HarnessSessionID == nil || run.ToolCalls != 3 {
		t.Fatalf("after turn 1: tools=%d", run.ToolCalls)
	}
	tk, _ := tracker.Get(e.ctx, e.q, e.ws, "PAY-214")
	aps, _ := e.q.PendingApprovalsForTicket(e.ctx, tk.ID)
	if _, err := e.runs.Decide(e.ctx, e.ws, e.me.ID, aps[0].ID, true, "go"); err != nil {
		t.Fatal(err)
	}
	e.until("completed", e.state("PAY-214=completed"))
	// The run's volume is gone.
	out, _ := exec.Command(docker, "volume", "ls", "-q", "--filter", "name=lazyagents-run-"+run.ID.String()).CombinedOutput()
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("volume left behind: %s", out)
	}
}

// HasWorkspace is how a runner tells its own runs from the other backend's.
func TestDockerHasWorkspace(t *testing.T) {
	if os.Getenv("RUNNER_DOCKER_TEST") == "" {
		t.Skip("set RUNNER_DOCKER_TEST=1 (needs Docker)")
	}
	ctx := context.Background()
	d := &runner.Docker{Socket: dockerSocket()}
	id := "has-workspace-test"
	if has, err := d.HasWorkspace(ctx, id); err != nil || has {
		t.Fatalf("before: %v %v", has, err)
	}
	if err := d.EnsureWorkspace(ctx, id); err != nil {
		t.Fatal(err)
	}
	defer d.RemoveWorkspace(ctx, id)
	if has, err := d.HasWorkspace(ctx, id); err != nil || !has {
		t.Fatalf("after ensure: %v %v", has, err)
	}
	if err := d.RemoveWorkspace(ctx, id); err != nil {
		t.Fatal(err)
	}
	if has, err := d.HasWorkspace(ctx, id); err != nil || has {
		t.Fatalf("after remove: %v %v", has, err)
	}
}

func dockerSocket() string {
	if h := os.Getenv("DOCKER_HOST"); strings.HasPrefix(h, "unix://") {
		return strings.TrimPrefix(h, "unix://")
	}
	home := os.Getenv("HOME")
	for _, p := range []string{home + "/.orbstack/run/docker.sock", home + "/.docker/run/docker.sock", "/var/run/docker.sock"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/var/run/docker.sock"
}
