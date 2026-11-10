package runner

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// checkoutMount is the /workspace mount of the last container created.
func (f *fakeEngine) checkoutMount(t *testing.T) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	mounts, _ := f.created["HostConfig"].(map[string]any)["Mounts"].([]any)
	for _, m := range mounts {
		if m := m.(map[string]any); m["Target"] == workspacePath {
			return m
		}
	}
	t.Fatalf("no %s mount in %v", workspacePath, mounts)
	return nil
}

func (f *fakeEngine) volumeNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var names []string
	for n := range f.volumes {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

func startTurn(t *testing.T, d *Docker, runID string) {
	t.Helper()
	if _, err := d.Start(context.Background(), runID, ExecSpec{Name: "t", Argv: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
}

// With host folders off, a checkout is a named volume, exactly as before.
func TestDockerWorkspaceIsAVolumeByDefault(t *testing.T) {
	f, d := startFakeEngine(t)
	ctx := context.Background()
	if err := d.EnsureWorkspace(ctx, "run-1"); err != nil {
		t.Fatal(err)
	}
	if got := f.volumeNames(); !slices.Equal(got, []string{"lazyagents-run-run-1", "lazyagents-run-run-1-home"}) {
		t.Fatalf("volumes %v", got)
	}
	startTurn(t, d, "run-1")
	if m := f.checkoutMount(t); m["Type"] != "volume" || m["Source"] != "lazyagents-run-run-1" {
		t.Fatalf("mount %v", m)
	}
	if _, err := d.EnsureHostWorkspace(ctx, "run-2"); err == nil {
		t.Fatal("EnsureHostWorkspace without RUNNER_HOST_WORKSPACES_DIR should fail")
	}
}

// A host-folder checkout is bind-mounted from its folder for its whole life,
// and removing the workspace deletes the folder.
func TestDockerHostWorkspaceIsBindMounted(t *testing.T) {
	f, d := startFakeEngine(t)
	d.HostWorkspaces = filepath.Join(t.TempDir(), "workspaces")
	ctx := context.Background()
	folder, err := d.EnsureHostWorkspace(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(d.HostWorkspaces, "run-1"); folder != want {
		t.Fatalf("folder %q, want %q", folder, want)
	}
	if st, err := os.Stat(folder); err != nil || !st.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	// Only the home directory is a volume.
	if got := f.volumeNames(); !slices.Equal(got, []string{"lazyagents-run-run-1-home"}) {
		t.Fatalf("volumes %v", got)
	}
	// Later setups call EnsureWorkspace; the checkout stays in the folder.
	if err := d.EnsureWorkspace(ctx, "run-1"); err != nil {
		t.Fatal(err)
	}
	if got := f.volumeNames(); !slices.Equal(got, []string{"lazyagents-run-run-1-home"}) {
		t.Fatalf("volumes after EnsureWorkspace %v", got)
	}
	startTurn(t, d, "run-1")
	if m := f.checkoutMount(t); m["Type"] != "bind" || m["Source"] != folder {
		t.Fatalf("mount %v", m)
	}
	if has, err := d.HasWorkspace(ctx, "run-1"); err != nil || !has {
		t.Fatalf("HasWorkspace %v %v", has, err)
	}
	if err := d.RemoveWorkspace(ctx, "run-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("folder left behind: %v", err)
	}
	if has, err := d.HasWorkspace(ctx, "run-1"); err != nil || has {
		t.Fatalf("HasWorkspace after remove %v %v", has, err)
	}
	if got := f.volumeNames(); len(got) != 0 {
		t.Fatalf("volumes left behind %v", got)
	}
}

// Turning host folders on never moves a workspace that already exists: its
// next turn would get an empty checkout.
func TestDockerHostWorkspaceKeepsAnExistingVolume(t *testing.T) {
	f, d := startFakeEngine(t)
	ctx := context.Background()
	if err := d.EnsureWorkspace(ctx, "run-1"); err != nil {
		t.Fatal(err)
	}
	d.HostWorkspaces = t.TempDir()
	folder, err := d.EnsureHostWorkspace(ctx, "run-1")
	if err != nil || folder != "" {
		t.Fatalf("folder %q err %v, want the volume kept", folder, err)
	}
	if _, err := os.Stat(filepath.Join(d.HostWorkspaces, "run-1")); !os.IsNotExist(err) {
		t.Fatalf("folder created for a volume workspace: %v", err)
	}
	startTurn(t, d, "run-1")
	if m := f.checkoutMount(t); m["Type"] != "volume" {
		t.Fatalf("mount %v", m)
	}
}

// The run ID becomes a folder that RemoveWorkspace deletes, so it can never
// name the root or anything outside it.
func TestDockerHostWorkspaceRefusesBadIDs(t *testing.T) {
	_, d := startFakeEngine(t)
	d.HostWorkspaces = t.TempDir()
	keep := filepath.Join(d.HostWorkspaces, "other-run")
	if err := os.Mkdir(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, id := range []string{"", ".", "..", "../x", "a/b", `a\b`} {
		if _, err := d.EnsureHostWorkspace(ctx, id); err == nil || !strings.Contains(err.Error(), "bad workspace id") {
			t.Errorf("EnsureHostWorkspace(%q) err %v", id, err)
		}
		if err := d.RemoveWorkspace(ctx, id); err == nil {
			t.Errorf("RemoveWorkspace(%q) accepted", id)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("a bad ID removed another run's folder: %v", err)
	}
}
