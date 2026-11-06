// Package runner executes managed runs (PLAN.md §6.4–6.6): it takes
// dispatched runs off the queue, prepares a workspace that persists between
// turns, runs each turn as one headless harness invocation, streams its
// events into run_events, and decides what happens when the process exits.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// ExecSpec is one process in a run's workspace.
type ExecSpec struct {
	Name string            // container/process name, for logs and cleanup
	Argv []string          // argv[0] is the program
	Env  map[string]string // only these are passed (plus PATH/HOME on local)
	// Dir is the working directory, relative to the workspace root ("" = the
	// root). A delegated run's git worktree is one.
	Dir        string
	StateFiles map[string][]byte // OpenCode config staged in backend-owned external state
	Files      map[string][]byte // written relative to the working directory before start
}

// Process is a running ExecSpec.
type Process interface {
	Stdout() io.Reader // the harness event stream
	Stderr() io.Reader
	Interrupt() error // SIGINT: let the harness end its turn
	Kill() error
	Wait() (exitCode int, err error)
}

// Backend runs processes in a run's persistent workspace (§6.5).
type Backend interface {
	Name() string // docker | local
	// WorkspaceDir is the checkout's path as processes see it.
	WorkspaceDir(runID string) string
	// EnsureWorkspace creates the run's workspace if it doesn't exist.
	EnsureWorkspace(ctx context.Context, runID string) error
	RemoveWorkspace(ctx context.Context, runID string) error
	Start(ctx context.Context, runID string, spec ExecSpec) (Process, error)
	// Cleanup removes processes a previous runner left behind.
	Cleanup(ctx context.Context) error
}

// ------------------------------------------------------------ local

// Local runs harnesses as host processes in a directory per run. It's for
// development on a laptop where the CLIs are installed and logged in: there
// is no isolation, and the host environment (HOME, logins) is inherited.
type Local struct {
	Dir string // root for run workspaces
	// Bins maps a harness program to the binary to run, e.g.
	// {"claude": "/path/to/fake"}. It applies to the harness wherever it
	// appears in argv (turns are wrapped in `sh -c … "$@"`).
	Bins map[string]string
	// OnlyBins refuses to run a harness program that isn't in Bins, so a
	// test can never fall through to a real, logged-in CLI on PATH.
	OnlyBins bool
	// OpenCodeIsolated attests an OS boundary preventing granted tools from
	// modifying protected config/state or workspace ancestors, including races.
	// A same-user local process alone is insufficient. Default is off.
	OpenCodeIsolated bool
	// ExtraHostEnv names host variables OpenCode may inherit besides the
	// minimal allowlist. It is a test hook (the fake CLI reads its script
	// mode from the environment); production never sets it.
	ExtraHostEnv []string
}

// ConfigIsolator is implemented by backends that can attest OpenCode's config
// is isolated. A backend without it never runs OpenCode (fail closed).
type ConfigIsolator interface {
	OpenCodeConfigIsolated() bool
	OpenCodeStateDir(runID, dir string) string
}

func (l *Local) OpenCodeConfigIsolated() bool { return l.OpenCodeIsolated }

// opencodeHostEnv is the only host environment OpenCode inherits locally:
// no provider keys, no HOME or XDG roots (the adapter supplies isolated ones).
// Run-injected secrets arrive through ExecSpec.Env.
func opencodeHostEnv(extra ...string) []string {
	var out []string
	for _, k := range append([]string{"PATH", "LANG", "LC_ALL", "LC_CTYPE", "TERM", "TMPDIR", "TZ", "USER", "LOGNAME", "SHELL",
		"SSL_CERT_FILE", "SSL_CERT_DIR", "NIX_SSL_CERT_FILE"}, extra...) {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// harnessPrograms are the CLIs adapters invoke.
var harnessPrograms = map[string]bool{"claude": true, "codex": true, "opencode": true}

func (l *Local) Name() string { return "local" }

func (l *Local) WorkspaceDir(runID string) string {
	p := filepath.Join(l.Dir, "runs", runID, "workspace")
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p // OpenCode refuses non-absolute workspace paths.
}

func (l *Local) EnsureWorkspace(_ context.Context, runID string) error {
	return os.MkdirAll(l.WorkspaceDir(runID), 0o755)
}

func (l *Local) RemoveWorkspace(_ context.Context, runID string) error {
	return os.RemoveAll(filepath.Join(l.Dir, "runs", runID))
}

func (l *Local) Cleanup(context.Context) error { return nil } // processes die with the runner

// subDir joins a workspace-relative directory to root, refusing escapes.
func subDir(root, rel string) (string, error) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	if p != filepath.Clean(root) && !strings.HasPrefix(p, filepath.Clean(root)+string(filepath.Separator)) {
		return "", fmt.Errorf("runner: directory %q escapes the workspace", rel)
	}
	return p, nil
}

func writeFiles(root string, files map[string][]byte) error {
	if len(files) == 0 {
		return nil
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	for rel, data := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if !strings.HasPrefix(p, filepath.Clean(root)+string(filepath.Separator)) {
			return fmt.Errorf("runner: file %q escapes the workspace", rel)
		}
		if err := r.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			return err
		}
		if err := r.WriteFile(rel, data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (l *Local) Start(ctx context.Context, runID string, spec ExecSpec) (Process, error) {
	dir := l.WorkspaceDir(runID)
	if spec.Dir != "" {
		var err error
		if dir, err = subDir(dir, spec.Dir); err != nil {
			return nil, err
		}
	}
	if isOpenCode(spec.Argv) {
		var err error
		spec, err = l.prepareOpenCode(runID, dir, spec)
		if err != nil {
			return nil, err
		}
	}
	if err := writeFiles(dir, spec.Files); err != nil {
		return nil, err
	}
	argv := append([]string{}, spec.Argv...)
	program := ""
	for i, a := range argv {
		if !harnessPrograms[a] {
			continue
		}
		program = a
		b, ok := l.Bins[a]
		if !ok && l.OnlyBins {
			return nil, fmt.Errorf("runner: %q has no configured binary (OnlyBins)", a)
		}
		if ok {
			argv[i] = b
		}
		break // the first match is the program; later ones are arguments
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if program == "opencode" {
		cmd.Env = opencodeHostEnv(l.ExtraHostEnv...)
	}
	// Desktop-launched local runners may omit Homebrew from PATH. Preserve
	// configured tool precedence while making installed tools such as gh visible.
	// OnlyBins tests intentionally isolate PATH from real logged-in tools.
	if runtime.GOOS == "darwin" && !l.OnlyBins && program != "opencode" {
		cmd.Env = append(cmd.Env, "PATH="+os.Getenv("PATH")+":/opt/homebrew/bin:/usr/local/bin")
	}
	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // signal the whole tree
	// Our own pipes, not StdoutPipe: cmd.Wait closes those as soon as the
	// process exits, which can drop the stream's last lines (the result).
	outR, outW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = outW, errW
	err = cmd.Start()
	outW.Close() // the child holds its copies; EOF arrives when it exits
	errW.Close()
	if err != nil {
		outR.Close()
		errR.Close()
		return nil, err
	}
	return &localProc{cmd: cmd, out: outR, err: errR}, nil
}

type localProc struct {
	cmd *exec.Cmd
	out *os.File
	err *os.File
}

// Close releases the read ends once the stream has been drained.
func (p *localProc) Close() error {
	p.out.Close()
	return p.err.Close()
}

func (p *localProc) Stdout() io.Reader { return p.out }
func (p *localProc) Stderr() io.Reader { return p.err }
func (p *localProc) Interrupt() error  { return syscall.Kill(-p.cmd.Process.Pid, syscall.SIGINT) }
func (p *localProc) Kill() error       { return syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL) }

func (p *localProc) Wait() (int, error) {
	err := p.cmd.Wait()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), nil
		}
		return ee.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}
