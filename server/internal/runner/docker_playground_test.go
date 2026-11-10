package runner

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/playground"
)

// playgroundEngine is enough of the Engine API to start a playground: the
// runner's own container, networks, and containers that run and log.
type playgroundEngine struct {
	mu        sync.Mutex
	calls     []string
	created   map[string]map[string]any // by name
	ids       map[string]string         // id → name
	logs      map[string]string         // name → Docker's timestamped output
	exitCode  map[string]int            // name → exited with this; absent: running
	connected []string
}

func (f *playgroundEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/v1.43")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+p)
	host, _ := os.Hostname()
	idOf := func(prefix, suffix string) string {
		return strings.TrimSuffix(strings.TrimPrefix(p, prefix), suffix)
	}
	switch {
	case p == "/containers/"+host+"/json":
		_, _ = io.WriteString(w, `{"Id":"runner-self"}`)
	case p == "/networks/create":
		_, _ = io.WriteString(w, `{}`)
	case strings.HasSuffix(p, "/connect"):
		var body struct{ Container string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.connected = append(f.connected, idOf("/networks/", "/connect")+"<-"+body.Container)
		_, _ = io.WriteString(w, `{}`)
	case p == "/containers/json":
		var list []map[string]any
		for id, name := range f.ids {
			list = append(list, map[string]any{"Id": id, "Labels": f.created[name]["Labels"]})
		}
		slices.SortFunc(list, func(a, b map[string]any) int { return strings.Compare(a["Id"].(string), b["Id"].(string)) })
		_ = json.NewEncoder(w).Encode(list)
	case p == "/containers/create":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		name := r.URL.Query().Get("name")
		f.created[name] = body
		id := fmt.Sprintf("c%d", len(f.ids)+1)
		f.ids[id] = name
		fmt.Fprintf(w, `{"Id":%q}`, id)
	case strings.HasSuffix(p, "/start"):
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(p, "/wait"):
		fmt.Fprintf(w, `{"StatusCode":%d}`, f.exitCode[f.ids[idOf("/containers/", "/wait")]])
	case strings.HasSuffix(p, "/logs"):
		for _, line := range strings.SplitAfter(f.logs[f.ids[idOf("/containers/", "/logs")]], "\n") {
			if line == "" {
				continue
			}
			h := make([]byte, 8)
			h[0] = 1
			binary.BigEndian.PutUint32(h[4:], uint32(len(line)))
			_, _ = w.Write(append(h, line...))
		}
	case strings.HasPrefix(p, "/containers/") && strings.HasSuffix(p, "/json"):
		name := f.ids[idOf("/containers/", "/json")]
		status := "running"
		if _, done := f.exitCode[name]; done {
			status = "exited"
		}
		fmt.Fprintf(w, `{"State":{"Status":%q,"ExitCode":%d}}`, status, f.exitCode[name])
	case r.Method == http.MethodDelete || strings.HasSuffix(p, "/disconnect"):
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected "+p, http.StatusNotImplemented)
	}
}

func startPlaygroundEngine(t *testing.T) (*playgroundEngine, *Docker) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	f := &playgroundEngine{created: map[string]map[string]any{}, ids: map[string]string{}, logs: map[string]string{}, exitCode: map[string]int{}}
	srv := &http.Server{Handler: f}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return f, &Docker{Socket: sock, Stack: "test", Image: "vesper/agent-base:latest", NixVolume: "lazyagents-nix", MemoryMB: 512}
}

// A process that dies before listening fails the start with its own output,
// after the setup ran; every container is sandboxed like a turn but on the
// playground's network only, and without the run's home (harness logins).
func TestStartPlaygroundSandboxesEveryContainerAndReportsAProcessThatDies(t *testing.T) {
	f, d := startPlaygroundEngine(t)
	id := uuid.MustParse("019a6f00-0000-7000-8000-1234567890ab")
	web := playgroundContainer(id, "web")
	f.exitCode[web] = 1
	f.logs[web] = "2026-11-10T17:42:04.000000000Z Error: Cannot find module 'next'\n"
	spec := playground.Spec{
		Services:  []agentdef.Service{{Name: "postgres", Image: "postgres:17-alpine"}},
		Setup:     []string{"npm ci"},
		Processes: []playground.Process{{Name: "web", Command: "npm run dev", Dir: "web", Port: 3000}},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	var steps []string
	_, err := d.StartPlayground(context.Background(), PlaygroundRun{ID: id, RunID: "run-1", Spec: spec}, func(s string) { steps = append(steps, s) })
	var perr *PlaygroundError
	if !errors.As(err, &perr) || !strings.Contains(perr.Msg, "web exited with code 1") || !strings.Contains(perr.Log, "Cannot find module 'next'") {
		t.Fatalf("StartPlayground = %v (%+v), want web's failure with its output", err, perr)
	}
	if want := []string{"Starting postgres", "Running setup", "Starting web", "Waiting for web on :3000"}; !slices.Equal(steps, want) {
		t.Errorf("steps = %q, want %q", steps, want)
	}
	network := playgroundNetwork(id)
	if !slices.Contains(f.connected, network+"<-runner-self") {
		t.Errorf("the runner didn't join %s: %v", network, f.connected)
	}
	for _, name := range []string{playgroundContainer(id, "setup"), web} {
		body := f.created[name]
		host := body["HostConfig"].(map[string]any)
		if host["NetworkMode"] != network || host["ExtraHosts"] != nil || fmt.Sprint(host["CapDrop"]) != "[ALL]" {
			t.Errorf("%s host config %v", name, host)
		}
		for _, m := range host["Mounts"].([]any) {
			if target := m.(map[string]any)["Target"]; target == homePath {
				t.Errorf("%s mounts the run's home", name)
			}
		}
		labels := body["Labels"].(map[string]any)
		if labels[playgroundLabelKey] != id.String() || labels[labelKey] != "run-1" {
			t.Errorf("%s labels %v", name, labels)
		}
	}
	// A login shell would read /etc/profile, which resets PATH and loses Go.
	if cmd := f.created[web]["Cmd"].([]any); len(cmd) != 3 || cmd[0] != "bash" || cmd[1] != "-c" || cmd[2] != processScript("web", "npm run dev") {
		t.Errorf("web runs %v, want bash -c with the image's PATH", cmd)
	}
	// Docker would create a missing WorkingDir as root; the root always exists.
	if wd := f.created[web]["WorkingDir"]; wd != workspacePath {
		t.Errorf("web starts in %v, want the workspace root", wd)
	}
	if svc := f.created[playgroundContainer(id, "postgres")]["HostConfig"].(map[string]any); svc["NetworkMode"] != network {
		t.Errorf("postgres network %v", svc["NetworkMode"])
	}
}

func TestStartPlaygroundReportsAFailedSetup(t *testing.T) {
	f, d := startPlaygroundEngine(t)
	id := uuid.New()
	setup := playgroundContainer(id, "setup")
	f.exitCode[setup] = 2
	f.logs[setup] = "2026-11-10T17:42:01Z migrate: relation \"runs\" already exists\n"
	spec := playground.Spec{Setup: []string{"make migrate"}, Processes: []playground.Process{{Name: "api", Command: "go run .", Port: 8000}}}
	_ = spec.Validate()
	_, err := d.StartPlayground(context.Background(), PlaygroundRun{ID: id, RunID: "run-1", Spec: spec}, func(string) {})
	var perr *PlaygroundError
	if !errors.As(err, &perr) || perr.Msg != "setup exited with code 2" || !strings.Contains(perr.Log, "already exists") {
		t.Fatalf("StartPlayground = %v", err)
	}
	if _, started := f.created[playgroundContainer(id, "api")]; started {
		t.Error("a process started after setup failed")
	}
}

func TestPlaygroundLogsMergeContainersInTimeOrder(t *testing.T) {
	f, d := startPlaygroundEngine(t)
	id := uuid.New()
	run := PlaygroundRun{ID: id, RunID: "run-1"}
	for _, c := range []struct{ source, logs string }{
		{"api", "2026-11-10T17:44:19.100Z PUT /api/v1/runner/test-environments 500\n2026-11-10T17:44:19.300Z error: null value\n"},
		{"postgres", "2026-11-10T17:44:19.200Z ERROR: not-null constraint\n"},
	} {
		name := playgroundContainer(id, c.source)
		f.created[name] = map[string]any{"Labels": d.playgroundLabels(run, c.source)}
		f.ids["id-"+c.source] = name
		f.logs[name] = c.logs
	}
	var got []string
	err := d.Logs(context.Background(), id, 10, false, func(l playground.LogLine) error {
		got = append(got, l.Source+": "+l.Text)
		return nil
	})
	want := []string{"api: PUT /api/v1/runner/test-environments 500", "postgres: ERROR: not-null constraint", "api: error: null value"}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("Logs = %q, %v; want %q", got, err, want)
	}
	got = nil
	_ = d.Logs(context.Background(), id, 1, false, func(l playground.LogLine) error { got = append(got, l.Text); return nil })
	if !slices.Equal(got, []string{"error: null value"}) {
		t.Errorf("tail 1 = %q, want only the newest line", got)
	}
}

// Each setup command starts from the workspace root, and one that fails
// stops setup, even mid-way through an && chain. The script runs under bash
// against a stand-in workspace, as the setup container would run it.
func TestPlaygroundSetupCommandsStartFromTheRootAndStopOnFailure(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"web", "server"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	run := func(cmds ...string) error {
		script := strings.ReplaceAll(setupScript(cmds), "cd "+workspacePath+" ", "cd "+root+" ")
		cmd := exec.Command("bash", "-c", script)
		cmd.Dir = filepath.Join(root, "web") // anywhere but the root
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, out)
		}
		return nil
	}
	if err := run("cd web && touch from-web", "cd server && touch from-server", "touch at-root"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"web/from-web", "server/from-server", "at-root"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	if err := run("cd missing && touch never", "touch after-failure"); err == nil {
		t.Error("setup succeeded though a command failed")
	}
	if _, err := os.Stat(filepath.Join(root, "after-failure")); err == nil {
		t.Error("setup went on after a failed command")
	}
}

// Docker creates a missing WorkingDir as root before the process's user runs,
// leaving a directory nothing in the playground can write to. A process's dir
// is made by its own shell instead, as the agent user.
func TestPlaygroundProcessMakesItsOwnDirectory(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command("bash", "-c", processScript("dist/it's new", "pwd > ../../where && touch here"))
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "dist", "it's new", "here")); err != nil {
		t.Fatalf("the process didn't run in its new directory: %v", err)
	}
	if got := processScript("", "npm run dev"); got != "npm run dev" {
		t.Errorf("a process at the root runs %q, want its command as given", got)
	}
}
