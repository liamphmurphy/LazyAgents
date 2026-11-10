package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Turns install tools with Nix, without root: the agent image ships Nix and
// points it at a daemon (NIX_REMOTE=daemon). One nix-daemon container per
// runner owns the store, which lives in a volume every turn mounts, so a
// package downloaded once is there for every later turn and run. Turns stay
// uid 1000 with no capabilities; the store is root-owned, so they can only
// add to it through the daemon. The daemon has Docker's default
// capabilities (it switches to its build users) and sits on the default
// bridge, not the agents network: it needs the binary cache, not the API.

const (
	nixMountPath = "/nix"
	nixDaemonBin = "/nix/var/nix/profiles/default/bin/nix-daemon"
)

func (d *Docker) nixDaemonName() string { return "lazyagents-nix-daemon-" + d.stack() }

// nixMount is the shared store as a turn mount, or nil when Nix is off or
// its daemon failed CheckNixDaemon: without the store a turn's nix commands
// fail at once ("cannot connect to socket") instead of hanging on a daemon
// that never answers.
func (d *Docker) nixMount() map[string]any {
	if d.NixVolume == "" || d.nixUnhealthy.Load() {
		return nil
	}
	return map[string]any{"Type": "volume", "Source": d.NixVolume, "Target": nixMountPath}
}

// EnsureNixDaemon starts the runner's nix-daemon unless it's already running.
// The first container to mount an empty store volume copies the image's /nix
// into it, so the daemon starts from the Nix the image ships.
func (d *Docker) EnsureNixDaemon(ctx context.Context) error {
	if d.NixVolume == "" {
		return nil
	}
	name := d.nixDaemonName()
	var info struct {
		State  struct{ Running bool }
		Config struct{ Env []string }
	}
	_, err := d.do(ctx, "GET", "/containers/"+url.PathEscape(name)+"/json", nil, "", &info)
	switch {
	case err == nil && !slices.Contains(info.Config.Env, nixDaemonEnv):
		// Created before the daemon was pinned to the local store: it serves
		// every request by connecting to itself and never answers. Replace it.
		if _, err := d.do(ctx, "DELETE", "/containers/"+url.PathEscape(name)+"?force=1", nil, "", nil); err != nil && !isStatus(err, 404) {
			return fmt.Errorf("replace nix-daemon: %w", err)
		}
		if err := d.createNixDaemon(ctx, name); err != nil {
			return err
		}
	case err == nil && info.State.Running:
		return nil
	case err == nil: // stopped (a host reboot): start it again below
	case isStatus(err, 404):
		if err := d.createNixDaemon(ctx, name); err != nil {
			return err
		}
	default:
		return fmt.Errorf("inspect nix-daemon: %w", err)
	}
	if _, err := d.do(ctx, "POST", "/containers/"+url.PathEscape(name)+"/start", nil, "", nil); err != nil && !isStatus(err, 304) {
		return fmt.Errorf("start nix-daemon: %w", err)
	}
	return nil
}

// nixDaemonEnv pins the daemon to the store on disk. The agent image sets
// NIX_REMOTE=daemon for turns, and a daemon that inherits it opens the
// "daemon" store for each worker: a connection to its own socket, recursively,
// so every store query hangs and the store is never initialized.
const nixDaemonEnv = "NIX_REMOTE=local"

func (d *Docker) createNixDaemon(ctx context.Context, name string) error {
	// No run label: Cleanup removes leftover turns, and this isn't one.
	body := map[string]any{
		"Image": d.Image, "Cmd": []string{nixDaemonBin}, "User": "0", "Env": []string{nixDaemonEnv},
		"Labels": map[string]string{stackLabelKey: d.stack()},
		"HostConfig": map[string]any{
			"Mounts":        []map[string]any{d.nixMount()},
			"Init":          true,
			"SecurityOpt":   []string{"no-new-privileges"},
			"RestartPolicy": map[string]any{"Name": "unless-stopped"},
		},
	}
	if _, err := d.do(ctx, "POST", "/containers/create?name="+url.QueryEscape(name), jsonBody(body), "application/json", nil); err != nil && !isStatus(err, 409) {
		return fmt.Errorf("create nix-daemon: %w", err)
	}
	return nil
}

// NixStatusFile is where CheckNixDaemon reports, in the proxy sockets
// directory: turns can read it by name (the directory isn't listable), so an
// agent can diagnose the daemon without access to the host's Docker.
const NixStatusFile = "nix-status.json"

// nixProbe asks the daemon for a store path's info as the turn user would:
// it opens the store database, which a daemon that only handshakes doesn't.
const nixProbe = `timeout 20 nix path-info "$(readlink -f /nix/var/nix/profiles/default)"`

// nixDiagnostics is what the status file records when the probe fails.
const nixDiagnostics = `id; echo; grep -E '^(Cap|NoNewPrivs|Seccomp)' /proc/1/status; echo; ` +
	`ls -ld /nix/store /nix/var/nix/db; ls -la /nix/var/nix/db; echo; ` +
	`for p in /proc/[0-9]*; do ls -l $p/fd 2>/dev/null | grep -q big-lock && echo "big-lock held by $(basename $p): $(tr '\0' ' ' < $p/cmdline)"; done; ` +
	`for p in /proc/[0-9]*; do echo "$(basename $p) $(awk '{print $3}' $p/stat 2>/dev/null) $(tr '\0' ' ' < $p/cmdline 2>/dev/null)"; done | head -40`

// NixStatus is the content of NixStatusFile.
type NixStatus struct {
	Healthy     bool      `json:"healthy"`
	CheckedAt   time.Time `json:"checked_at"`
	Restarted   bool      `json:"restarted"` // the first probe failed and the daemon was restarted
	Probe       string    `json:"probe,omitempty"`
	Diagnostics string    `json:"diagnostics,omitempty"`
	DaemonLog   string    `json:"daemon_log,omitempty"`
}

// CheckNixDaemon probes the daemon through its socket, restarting it once if
// the probe fails, and records the outcome in NixStatusFile. An unhealthy
// daemon keeps the store away from turns (see nixMount).
func (d *Docker) CheckNixDaemon(ctx context.Context) (NixStatus, error) {
	st := NixStatus{CheckedAt: time.Now().UTC()}
	if d.NixVolume == "" {
		return st, nil
	}
	name := d.nixDaemonName()
	out, code, err := d.execIn(ctx, name, "1000", nixProbe)
	if err != nil || code != 0 {
		st.Restarted = true
		if _, rerr := d.do(ctx, "POST", "/containers/"+url.PathEscape(name)+"/restart?t=5", nil, "", nil); rerr == nil {
			time.Sleep(nixRestartSettle)
			out, code, err = d.execIn(ctx, name, "1000", nixProbe)
		}
	}
	st.Healthy = err == nil && code == 0
	st.Probe = lastLines(strings.TrimSpace(fmt.Sprintf("exit %d %v\n%s", code, errString(err), out)), 20)
	if !st.Healthy {
		st.Diagnostics, _, _ = d.execIn(ctx, name, "0", nixDiagnostics)
		st.Diagnostics = lastLines(st.Diagnostics, 80)
		st.DaemonLog = d.logTail(name)
	}
	d.nixUnhealthy.Store(!st.Healthy)
	if d.ProxyDir != "" {
		b, _ := json.MarshalIndent(st, "", "  ")
		if werr := os.WriteFile(filepath.Join(d.ProxyDir, NixStatusFile), b, 0o644); werr != nil {
			return st, werr
		}
	}
	if !st.Healthy {
		return st, fmt.Errorf("nix-daemon doesn't answer store queries (%s); turns run without Nix", firstLine(st.Probe))
	}
	return st, nil
}

// nixRestartSettle gives a restarted daemon time to open its socket.
var nixRestartSettle = 3 * time.Second

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// execIn runs a shell command in a running container and returns its output
// and exit code; ctx bounds the whole exec.
func (d *Docker) execIn(ctx context.Context, container, user, script string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var created struct{ ID string }
	if _, err := d.do(ctx, "POST", "/containers/"+url.PathEscape(container)+"/exec", jsonBody(map[string]any{
		"Cmd": []string{"sh", "-c", script}, "User": user, "Env": []string{"NIX_REMOTE=daemon"},
		"AttachStdout": true, "AttachStderr": true,
	}), "application/json", &created); err != nil {
		return "", -1, err
	}
	res, err := d.do(ctx, "POST", "/exec/"+created.ID+"/start", jsonBody(map[string]any{"Detach": false, "Tty": false}), "application/json", nil)
	if err != nil {
		return "", -1, err
	}
	var out bytes.Buffer
	derr := demux(io.LimitReader(res.Body, 256<<10), &out, &out)
	res.Body.Close()
	if derr != nil {
		return out.String(), -1, derr
	}
	var info struct {
		ExitCode int
		Running  bool
	}
	if _, err := d.do(ctx, "GET", "/exec/"+created.ID+"/json", nil, "", &info); err != nil {
		return out.String(), -1, err
	}
	if info.Running {
		return out.String(), -1, fmt.Errorf("still running when its output ended")
	}
	return out.String(), info.ExitCode, nil
}
