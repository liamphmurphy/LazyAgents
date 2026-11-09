package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
)

// maxCodexAuth matches the 16 KB limit on a stored secret.
const maxCodexAuth = 16 << 10

// Docker runs each process as a container from the agent image, with the
// run's named volume at /workspace (§6.5). It talks to the Engine API over
// the socket directly, so the runner image needs no docker CLI.
//
// Mounting the socket gives the runner root-equivalent access to the host.
// v1 targets a single laptop, where that's documented and accepted.
type Docker struct {
	Socket  string // /var/run/docker.sock
	Image   string // vesper/agent-base:latest
	Network string // where api:8000 resolves; "" = default bridge
	// MemoryMB and PidsLimit cap each container; 0 leaves that limit off.
	MemoryMB  int64
	PidsLimit int64
	// SaveCodexAuth stores a ChatGPT login a turn refreshed. scope is what
	// SecretEnv put in CodexAuthScopeEnv.
	SaveCodexAuth func(ctx context.Context, scope, auth string) error
	client        *http.Client
}

const (
	dockerAPI     = "http://docker/v1.43"
	workspacePath = "/workspace"
	homePath      = "/home/node"
	codexAuthPath = ".codex/auth.json"
	labelKey      = "dev.lazyagents.run"
)

func (d *Docker) Name() string { return "docker" }

func (d *Docker) http() *http.Client {
	if d.client == nil {
		d.client = &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", d.Socket)
			},
		}}
	}
	return d.client
}

func (d *Docker) do(ctx context.Context, method, path string, body io.Reader, ctype string, out any) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, dockerAPI+path, body)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := d.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker %s %s: %w", method, path, err)
	}
	if res.StatusCode >= 300 {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		var msg struct{ Message string }
		_ = json.Unmarshal(b, &msg)
		if msg.Message == "" {
			msg.Message = strings.TrimSpace(string(b))
		}
		return res, &dockerError{Status: res.StatusCode, Msg: msg.Message}
	}
	if out != nil {
		defer res.Body.Close()
		return res, json.NewDecoder(res.Body).Decode(out)
	}
	return res, nil
}

type dockerError struct {
	Status int
	Msg    string
}

func (e *dockerError) Error() string { return fmt.Sprintf("docker: %d %s", e.Status, e.Msg) }

func isStatus(err error, code int) bool {
	var de *dockerError
	return errors.As(err, &de) && de.Status == code
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

func volumeName(runID string) string { return "lazyagents-run-" + runID }

// homeVolumeName is the run's home directory. Each turn is a new container, so
// the harness sessions a later turn resumes have to outlive the container.
func homeVolumeName(runID string) string { return volumeName(runID) + "-home" }

func (d *Docker) WorkspaceDir(string) string { return workspacePath }

// Ping checks the socket and that the image exists.
func (d *Docker) Ping(ctx context.Context) error {
	if _, err := d.do(ctx, "GET", "/_ping", nil, "", nil); err != nil {
		return err
	}
	if _, err := d.do(ctx, "GET", "/images/"+url.PathEscape(d.Image)+"/json", nil, "", nil); err != nil {
		if isStatus(err, 404) {
			return fmt.Errorf("image %s not found: build it with `docker compose build agent-base`", d.Image)
		}
		return err
	}
	return nil
}

func (d *Docker) EnsureWorkspace(ctx context.Context, runID string) error {
	for _, name := range []string{volumeName(runID), homeVolumeName(runID)} {
		_, err := d.do(ctx, "POST", "/volumes/create", jsonBody(map[string]any{
			"Name": name, "Labels": map[string]string{labelKey: runID},
		}), "application/json", nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Docker) HasWorkspace(ctx context.Context, runID string) (bool, error) {
	res, err := d.do(ctx, "GET", "/volumes/"+volumeName(runID), nil, "", nil)
	if isStatus(err, 404) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	res.Body.Close()
	return true, nil
}

func (d *Docker) RemoveWorkspace(ctx context.Context, runID string) error {
	for _, name := range []string{volumeName(runID), homeVolumeName(runID)} {
		_, err := d.do(ctx, "DELETE", "/volumes/"+name+"?force=1", nil, "", nil)
		if err != nil && !isStatus(err, 404) {
			return err
		}
	}
	return nil
}

// Cleanup removes turn containers left by a runner that died; their turns
// are re-run (the harness resumes its session).
func (d *Docker) Cleanup(ctx context.Context) error {
	filters, _ := json.Marshal(map[string][]string{"label": {labelKey}})
	var list []struct{ ID string }
	if _, err := d.do(ctx, "GET", "/containers/json?all=1&filters="+url.QueryEscape(string(filters)), nil, "", &list); err != nil {
		return err
	}
	for _, c := range list {
		_, _ = d.do(ctx, "DELETE", "/containers/"+c.ID+"?force=1", nil, "", nil)
	}
	return nil
}

func tarFiles(files map[string][]byte) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	dirs := map[string]bool{}
	for _, n := range names {
		if strings.Contains(n, "..") || strings.HasPrefix(n, "/") {
			return nil, fmt.Errorf("runner: bad file path %q", n)
		}
		// Parent directories first, owned by the agent user (uid 1000).
		parts := strings.Split(n, "/")
		for i := 1; i < len(parts); i++ {
			dir := strings.Join(parts[:i], "/") + "/"
			if !dirs[dir] {
				dirs[dir] = true
				if err := tw.WriteHeader(&tar.Header{Name: dir, Mode: 0o755, Typeflag: tar.TypeDir, Uid: 1000, Gid: 1000, ModTime: time.Now()}); err != nil {
					return nil, err
				}
			}
		}
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: 0o600, Size: int64(len(files[n])), Uid: 1000, Gid: 1000, ModTime: time.Now()}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(files[n]); err != nil {
			return nil, err
		}
	}
	return &buf, tw.Close()
}

// hostConfig is the sandbox every turn runs in: no capabilities, no privilege
// escalation through setuid binaries, and bounded resources.
func (d *Docker) hostConfig(mounts []map[string]any) map[string]any {
	host := map[string]any{
		"Mounts":      mounts,
		"NetworkMode": d.Network,
		"Init":        true, // forward SIGINT to the harness, reap children
		"CapDrop":     []string{"ALL"},
		"SecurityOpt": []string{"no-new-privileges"},
		// A runner on the host is reached by this name, on Linux too.
		"ExtraHosts": []string{"host.docker.internal:host-gateway"},
	}
	if d.MemoryMB > 0 {
		host["Memory"] = d.MemoryMB << 20
	}
	if d.PidsLimit > 0 {
		host["PidsLimit"] = d.PidsLimit
	}
	return host
}

// codexAuthTar is the archive that puts a ChatGPT login at ~/.codex/auth.json.
func codexAuthTar(auth string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	dir, _ := path.Split(codexAuthPath)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: dir, Mode: 0o700, Uid: 1000, Gid: 1000, ModTime: time.Now()}); err != nil {
		return nil, err
	}
	if err := tw.WriteHeader(&tar.Header{Name: codexAuthPath, Mode: 0o600, Size: int64(len(auth)), Uid: 1000, Gid: 1000, ModTime: time.Now()}); err != nil {
		return nil, err
	}
	if _, err := io.WriteString(tw, auth); err != nil {
		return nil, err
	}
	return &buf, tw.Close()
}

func (d *Docker) Start(ctx context.Context, runID string, spec ExecSpec) (Process, error) {
	if spec.Env == nil {
		spec.Env = map[string]string{}
	}
	workspace := path.Join(workspacePath, spec.Dir)
	for k, v := range dependencyEnv(d.DependencyCacheDir(), workspace) {
		if _, exists := spec.Env[k]; !exists {
			spec.Env[k] = v
		}
	}
	mounts := []map[string]any{{"Type": "volume", "Source": volumeName(runID), "Target": workspacePath}}
	// A persistent volume is shared by all runs and deliberately has no run
	// label, so workspace cleanup never removes downloaded dependencies.
	mounts = append(mounts, map[string]any{"Type": "volume", "Source": homeVolumeName(runID), "Target": homePath})
	mounts = append(mounts, map[string]any{"Type": "volume", "Source": "lazyagents-dependency-cache", "Target": "/cache"})
	env := make([]string, 0, len(spec.Env))
	for k, v := range spec.Env {
		if k == CodexAuthEnv || k == CodexAuthScopeEnv {
			continue // delivered as a file below, never through the environment
		}
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	dir := workspacePath
	if spec.Dir != "" {
		if strings.Contains(spec.Dir, "..") || strings.HasPrefix(spec.Dir, "/") {
			return nil, fmt.Errorf("runner: bad directory %q", spec.Dir)
		}
		dir = path.Join(workspacePath, spec.Dir)
	}
	body := map[string]any{
		"Image": d.Image, "Cmd": spec.Argv, "Env": env, "WorkingDir": dir,
		"Labels":     map[string]string{labelKey: runID},
		"HostConfig": d.hostConfig(mounts),
	}
	var created struct{ ID string }
	name := url.QueryEscape(spec.Name)
	if _, err := d.do(ctx, "POST", "/containers/create?name="+name, jsonBody(body), "application/json", &created); err != nil {
		if isStatus(err, 409) { // a stale container with this name
			_, _ = d.do(ctx, "DELETE", "/containers/"+name+"?force=1", nil, "", nil)
			_, err = d.do(ctx, "POST", "/containers/create?name="+name, jsonBody(body), "application/json", &created)
		}
		if err != nil {
			return nil, err
		}
	}
	p := &dockerProc{d: d, id: created.ID, codexAuth: spec.Env[CodexAuthEnv], codexScope: spec.Env[CodexAuthScopeEnv]}
	if p.codexAuth != "" {
		tb, err := codexAuthTar(p.codexAuth)
		if err != nil {
			p.remove()
			return nil, err
		}
		if _, err := d.do(ctx, "PUT", "/containers/"+created.ID+"/archive?path="+url.QueryEscape(homePath), tb, "application/x-tar", nil); err != nil {
			p.remove()
			return nil, err
		}
	}
	if len(spec.Files) > 0 {
		tb, err := tarFiles(spec.Files)
		if err != nil {
			p.remove()
			return nil, err
		}
		if _, err := d.do(ctx, "PUT", "/containers/"+created.ID+"/archive?path="+url.QueryEscape(dir), tb, "application/x-tar", nil); err != nil {
			p.remove()
			return nil, err
		}
	}
	// Attach before start so no output is lost.
	logs, err := d.do(context.Background(), "POST", "/containers/"+created.ID+"/attach?stream=1&stdout=1&stderr=1", nil, "", nil)
	if err != nil {
		p.remove()
		return nil, err
	}
	if _, err := d.do(ctx, "POST", "/containers/"+created.ID+"/start", nil, "", nil); err != nil {
		logs.Body.Close()
		p.remove()
		return nil, err
	}
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	p.out, p.err = outR, errR
	go func() {
		defer logs.Body.Close()
		e := demux(logs.Body, outW, errW)
		outW.CloseWithError(e)
		errW.CloseWithError(e)
	}()
	return p, nil
}

// demux splits Docker's multiplexed attach stream (8-byte frame headers).
func demux(r io.Reader, stdout, stderr io.Writer) error {
	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(r, hdr); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		n := int64(binary.BigEndian.Uint32(hdr[4:]))
		w := stdout
		if hdr[0] == 2 {
			w = stderr
		}
		if _, err := io.CopyN(w, r, n); err != nil {
			return err
		}
	}
}

type dockerProc struct {
	d   *Docker
	id  string
	out io.Reader
	err io.Reader
	// The ChatGPT login this process was given, and where it is stored.
	codexAuth, codexScope string
}

func (p *dockerProc) Stdout() io.Reader { return p.out }
func (p *dockerProc) Stderr() io.Reader { return p.err }

func (p *dockerProc) signal(sig string) error {
	_, err := p.d.do(context.Background(), "POST", "/containers/"+p.id+"/kill?signal="+sig, nil, "", nil)
	if isStatus(err, 409) { // not running any more
		return nil
	}
	return err
}

func (p *dockerProc) Interrupt() error { return p.signal("SIGINT") }
func (p *dockerProc) Kill() error      { return p.signal("SIGKILL") }

func (p *dockerProc) remove() {
	_, _ = p.d.do(context.Background(), "DELETE", "/containers/"+p.id+"?force=1", nil, "", nil)
}

// waitRetries is how many times Wait re-attaches to a container after the
// long-poll fails at the transport (the socket dropped, e.g. across a Docker
// VM sleep). Removing the container then would kill a turn that is still fine.
const waitRetries = 3

var waitBackoff = time.Second

func (p *dockerProc) Wait() (int, error) {
	defer p.remove()
	var res struct{ StatusCode int }
	var err error
	for attempt := 0; ; attempt++ {
		_, err = p.d.do(context.Background(), "POST", "/containers/"+p.id+"/wait", nil, "", &res)
		var de *dockerError
		if err == nil || errors.As(err, &de) || attempt >= waitRetries {
			break // done, or the engine answered with an error retrying won't change
		}
		slog.Warn("docker wait failed; retrying", "container", p.id, "err", err)
		time.Sleep(waitBackoff << attempt)
	}
	if err != nil {
		return -1, err
	}
	// Even a failed turn may have rotated the Codex login: keep it first.
	p.keepRefreshedCodexAuth()
	if res.StatusCode != 0 && p.oomKilled() {
		return res.StatusCode, fmt.Errorf("the container ran out of memory (exit %d); raise RUNNER_MEMORY_MB or reduce what the run builds at once", res.StatusCode)
	}
	return res.StatusCode, nil
}

// oomKilled reports whether the kernel killed the container for its memory
// limit, which otherwise looks like any other SIGKILL (exit 137).
func (p *dockerProc) oomKilled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var info struct{ State struct{ OOMKilled bool } }
	if _, err := p.d.do(ctx, "GET", "/containers/"+p.id+"/json", nil, "", &info); err != nil {
		return false
	}
	return info.State.OOMKilled
}

// keepRefreshedCodexAuth reads the only file that ever comes back out of a
// container. Codex rotates its refresh token, so losing a refreshed login
// would break the stored one. The file is agent-writable, so only a newer
// login for the same account is accepted (chatmodels.CodexAuthRefreshed).
func (p *dockerProc) keepRefreshedCodexAuth() {
	if p.codexAuth == "" || p.d.SaveCodexAuth == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := p.d.do(ctx, "GET", "/containers/"+p.id+"/archive?path="+url.QueryEscape(path.Join(homePath, codexAuthPath)), nil, "", nil)
	if err != nil {
		return
	}
	defer res.Body.Close()
	tr := tar.NewReader(res.Body)
	if _, err := tr.Next(); err != nil {
		return
	}
	next, err := io.ReadAll(io.LimitReader(tr, maxCodexAuth+1))
	if err != nil || len(next) > maxCodexAuth || !chatmodels.CodexAuthRefreshed(p.codexAuth, string(next)) {
		return
	}
	if err := p.d.SaveCodexAuth(ctx, p.codexScope, string(next)); err != nil {
		slog.Warn("save refreshed codex login", "err", err)
	}
}
