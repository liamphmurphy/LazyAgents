package runner_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// hostFolderLocal is the fake backend posing as one with host folders, so
// the runner's choice can be watched without Docker.
type hostFolderLocal struct {
	*runner.Fake
	dir   string
	calls atomic.Int32
}

func (h *hostFolderLocal) HostWorkspacesDir() string { return h.dir }

func (h *hostFolderLocal) EnsureHostWorkspace(ctx context.Context, runID string) (string, error) {
	h.calls.Add(1)
	return h.WorkspaceDir(runID), h.EnsureWorkspace(ctx, runID)
}

// A run gets a host folder only once the person turns it on, and the folder
// is recorded on the run for the UI.
func TestRunnerUsesHostFoldersWhenTurnedOn(t *testing.T) {
	e := setup(t)
	backend := &hostFolderLocal{Fake: e.runner.Backend.(*runner.Fake), dir: "/Users/me/vesper/workspaces"}
	e.runner.Backend = backend
	t.Setenv("FAKE_HARNESS_MODE", "crash") // a failed run keeps its workspace

	e.dispatch("PAY-224")
	e.until("failed", e.state("PAY-224=failed"))
	if r := e.run("PAY-224"); r.WorkspaceHostPath != nil || backend.calls.Load() != 0 {
		t.Fatalf("off: host path %v, calls %d", r.WorkspaceHostPath, backend.calls.Load())
	}

	if err := e.q.SetHostFolderWorkspaces(e.ctx, store.SetHostFolderWorkspacesParams{ID: e.ws, HostFolderWorkspaces: true}); err != nil {
		t.Fatal(err)
	}
	e.dispatch("PAY-224")
	e.until("failed again", func() bool { r := e.run("PAY-224"); return r.State == "failed" && r.WorkspaceHostPath != nil })
	r := e.run("PAY-224")
	if want := backend.WorkspaceDir(r.ID.String()); *r.WorkspaceHostPath != want || backend.calls.Load() != 1 {
		t.Fatalf("on: host path %q want %q, calls %d", *r.WorkspaceHostPath, want, backend.calls.Load())
	}
	paths, err := e.q.RunWorkspaceHostPaths(e.ctx, []uuid.UUID{r.ID})
	if err != nil || len(paths) != 1 || paths[0].Path != *r.WorkspaceHostPath {
		t.Fatalf("paths %+v %v", paths, err)
	}
}
