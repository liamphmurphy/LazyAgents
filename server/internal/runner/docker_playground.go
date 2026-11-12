package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/playground"
)

// A playground runs on a network of its own: its services, a one-shot setup
// container, and a container per process, all from the chat's workspace and
// in the turn's sandbox. The runner joins that network itself, because its
// gateway is what the browser talks to; nothing is published on the host.
// Unlike a turn's services, the network isn't internal: an app's setup
// installs dependencies, and the app may call out.

const (
	playgroundLabelKey  = "dev.lazyagents.playground"
	playgroundSourceKey = "dev.lazyagents.playground.source"
	playgroundSetup     = "setup"
	playgroundTools     = "tools"
	// PlaygroundHost is the runner's name on each playground network.
	PlaygroundHost = "playground"
	// playgroundProfile holds the Nix packages a playground installs, in the
	// workspace (git-excluded) so every one of its containers sees them.
	playgroundProfile = workspacePath + "/.lazyagents/playground-profile"
)

var (
	// PlaygroundSetupTimeout bounds a playground's setup commands together.
	PlaygroundSetupTimeout = 15 * time.Minute
	// PlaygroundReadyTimeout bounds how long a process may take to listen on
	// its port (a first `go run` compiles).
	PlaygroundReadyTimeout = 5 * time.Minute
	// PlaygroundBackgroundGrace is how long a background process must keep
	// running after start before the playground counts as up.
	PlaygroundBackgroundGrace = 3 * time.Second
	// PlaygroundWarmUpTimeout bounds loading the first page once a process
	// listens; one slower than this starts anyway.
	PlaygroundWarmUpTimeout = 2 * time.Minute
	playgroundPoll          = 500 * time.Millisecond
)

// PlaygroundRun is a playground as the backend starts it.
type PlaygroundRun struct {
	ID    uuid.UUID
	RunID string // whose workspace it runs
	Spec  playground.Spec
}

// PlaygroundError is a playground that couldn't start, with the end of the
// output that says why.
type PlaygroundError struct {
	Msg string
	Log string
}

func (e *PlaygroundError) Error() string { return e.Msg }

// PlaygroundBackend runs playgrounds (the Docker backend).
type PlaygroundBackend interface {
	// StartPlayground starts everything fresh, replacing what was running,
	// and returns where each process listens as the runner reaches it.
	StartPlayground(ctx context.Context, pg PlaygroundRun, step func(string)) (map[string]string, error)
	// StopPlayground removes everything; safe to call more than once.
	StopPlayground(ctx context.Context, id uuid.UUID) error
	// RunningPlaygrounds lists the playgrounds with containers here.
	RunningPlaygrounds(ctx context.Context) ([]uuid.UUID, error)
	// StackName is the install whose containers this backend owns.
	StackName() string
	// PlaygroundNetwork is the network a turn joins to reach a playground.
	PlaygroundNetwork(id uuid.UUID) string
	// PullMissing pulls an image unless it's here already, and says whether
	// it pulled.
	PullMissing(ctx context.Context, image string) (bool, error)
	playground.Backend
}

func (d *Docker) PullMissing(ctx context.Context, image string) (bool, error) {
	_, err := d.do(ctx, "GET", "/images/"+url.PathEscape(image)+"/json", nil, "", nil)
	if err == nil {
		return false, nil
	}
	if !isStatus(err, 404) {
		return false, err
	}
	return true, d.pull(ctx, image)
}

func (d *Docker) StackName() string { return d.stack() }

func (d *Docker) PlaygroundNetwork(id uuid.UUID) string { return playgroundNetwork(id) }

func playgroundKey(id uuid.UUID) string {
	h := strings.ReplaceAll(id.String(), "-", "")
	return h[len(h)-12:] // the random end of a v7 UUID
}

// Names stay within a 63-character DNS label: the runner reaches each
// process by its container name.
func playgroundNetwork(id uuid.UUID) string { return "lazyagents-play-" + playgroundKey(id) }

func playgroundContainer(id uuid.UUID, source string) string {
	return playgroundNetwork(id) + "-" + source
}

func (d *Docker) playgroundLabels(pg PlaygroundRun, source string) map[string]string {
	l := d.labels(pg.RunID)
	l[playgroundLabelKey] = pg.ID.String()
	if source != "" {
		l[playgroundSourceKey] = source
	}
	return l
}

// selfID is the runner's own container, which joins each playground's
// network so the gateway can reach the processes.
func (d *Docker) selfID(ctx context.Context) (string, error) {
	d.selfMu.Lock()
	defer d.selfMu.Unlock()
	if d.self != "" {
		return d.self, nil
	}
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}
	var info struct{ ID string }
	if _, err := d.do(ctx, "GET", "/containers/"+url.PathEscape(host)+"/json", nil, "", &info); err != nil {
		return "", fmt.Errorf("playgrounds need the runner to run in Docker, as the compose files run it (no container %q here: %w)", host, err)
	}
	d.self = info.ID
	return d.self, nil
}

func (d *Docker) StartPlayground(ctx context.Context, pg PlaygroundRun, step func(string)) (map[string]string, error) {
	if err := d.StopPlayground(ctx, pg.ID); err != nil {
		return nil, fmt.Errorf("remove the previous playground: %w", err)
	}
	self, err := d.selfID(ctx)
	if err != nil {
		return nil, err
	}
	network := playgroundNetwork(pg.ID)
	_, err = d.do(ctx, "POST", "/networks/create", jsonBody(map[string]any{
		"Name": network, "Labels": d.playgroundLabels(pg, ""),
	}), "application/json", nil)
	if err != nil && !isStatus(err, 409) {
		return nil, fmt.Errorf("create the playground network: %w", err)
	}
	// As "playground" there: a turn on the network reaches the gateway, and
	// with it the app as the person sees it, routes included.
	join := map[string]any{"Container": self, "EndpointConfig": map[string]any{"Aliases": []string{PlaygroundHost}}}
	if _, err := d.do(ctx, "POST", "/networks/"+url.PathEscape(network)+"/connect", jsonBody(join), "application/json", nil); err != nil && !isStatus(err, 403) && !isStatus(err, 409) {
		return nil, fmt.Errorf("join the playground network: %w", err)
	}

	spec := pg.Spec
	if len(spec.NixPackages) > 0 && d.nixMount() == nil {
		return nil, &PlaygroundError{Msg: "the spec asks for Nix packages, but this runner has no Nix (RUNNER_NIX_VOLUME)"}
	}
	// Services don't depend on each other or on the tools, so they all start
	// at once, alongside the tools install. Setup waits for all of them: it
	// may migrate a database or call a tool.
	var starts []func(context.Context) error
	for _, s := range spec.Services {
		starts = append(starts, func(ctx context.Context) error { return d.startPlaygroundService(ctx, pg, network, s) })
	}
	if len(spec.NixPackages) > 0 {
		starts = append(starts, func(ctx context.Context) error { return d.installPlaygroundTools(ctx, pg, network) })
	}
	if len(starts) > 0 {
		step(startingStep(spec))
		if err := firstError(ctx, starts...); err != nil {
			return nil, err
		}
	}

	if len(spec.Setup) > 0 {
		step("Running setup")
		body, err := d.playgroundBody(pg, network, playgroundSetup, setupScript(spec.Setup), spec.Directory, spec.ProcessEnv(nil))
		if err != nil {
			return nil, err
		}
		id, err := d.createAndStart(ctx, playgroundContainer(pg.ID, playgroundSetup), body, "")
		if err != nil {
			return nil, fmt.Errorf("start setup: %w", err)
		}
		if err := d.waitExit(ctx, id, PlaygroundSetupTimeout); err != nil {
			return nil, &PlaygroundError{Msg: "setup " + err.Error(), Log: d.playgroundLogTail(id)}
		}
	}

	addrs := map[string]string{}
	ids := map[string]string{}
	for _, p := range spec.Processes {
		step("Starting " + p.Name)
		body, err := d.playgroundBody(pg, network, p.Name, p.Command, path.Join(spec.Directory, p.Dir), spec.ProcessEnv(&p))
		if err != nil {
			return nil, err
		}
		id, err := d.createAndStart(ctx, playgroundContainer(pg.ID, p.Name), body, "")
		if err != nil {
			return nil, fmt.Errorf("start %s: %w", p.Name, err)
		}
		ids[p.Name] = id
		if !p.Background() {
			addrs[p.Name] = net.JoinHostPort(playgroundContainer(pg.ID, p.Name), strconv.Itoa(p.Port))
		}
	}
	for _, p := range spec.Processes {
		if p.Background() {
			continue
		}
		step(fmt.Sprintf("Waiting for %s on :%d", p.Name, p.Port))
		if err := d.waitListening(ctx, ids[p.Name], addrs[p.Name]); err != nil {
			return nil, &PlaygroundError{Msg: fmt.Sprintf("%s %v", p.Name, err), Log: d.playgroundLogTail(ids[p.Name])}
		}
	}
	// A background process serves nothing to wait for; one that dies on
	// start (a bad flag, a missing binary) has done so by the time the served
	// ones listen, or within the grace after.
	for _, p := range spec.Processes {
		if !p.Background() {
			continue
		}
		step("Checking " + p.Name)
		if err := d.stillRunning(ctx, ids[p.Name], PlaygroundBackgroundGrace); err != nil {
			return nil, &PlaygroundError{Msg: fmt.Sprintf("%s %v", p.Name, err), Log: d.playgroundLogTail(ids[p.Name])}
		}
	}
	// A dev server (next dev, vite) listens at once but builds a page on its
	// first request: load it here, so the wait shows as a step and the
	// person's first page is quick.
	if p, ok := spec.Route("/"); ok && !p.Background() {
		step("Loading " + p.Name + "'s first page")
		d.warmUp(ctx, addrs[p.Name])
	}
	return addrs, nil
}

// warmUp requests a process's root page once. Whatever it answers, even an
// error, the playground is up: the person sees the same answer.
func (d *Docker) warmUp(ctx context.Context, addr string) {
	ctx, cancel := context.WithTimeout(ctx, PlaygroundWarmUpTimeout)
	defer cancel()
	dial := d.dialPlayground
	if dial == nil {
		dial = func(ctx context.Context, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		}
	}
	client := &http.Client{
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) { return dial(ctx, addr) }},
		// Follow a redirect within the app (to a sign-in page, say), which is
		// built on its own request, but nowhere else.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Host != addr || len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/", nil)
	if err != nil {
		return
	}
	res, err := client.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 8<<20))
	res.Body.Close()
}

// startPlaygroundService starts a service and waits until it's ready.
func (d *Docker) startPlaygroundService(ctx context.Context, pg PlaygroundRun, network string, s agentdef.Service) error {
	body := d.serviceBody(pg.RunID, network, s)
	body["Labels"] = d.playgroundLabels(pg, s.Name)
	id, err := d.createAndStart(ctx, playgroundContainer(pg.ID, s.Name), body, s.Image)
	if err != nil {
		return &PlaygroundError{Msg: fmt.Sprintf("service %s didn't start: %v", s.Name, err), Log: d.logTail(id)}
	}
	if err := d.waitReady(ctx, id, s); err != nil {
		return &PlaygroundError{Msg: fmt.Sprintf("service %s isn't ready", s.Name), Log: err.Error()}
	}
	return nil
}

// installPlaygroundTools installs the spec's Nix packages into the
// workspace's playground profile.
func (d *Docker) installPlaygroundTools(ctx context.Context, pg PlaygroundRun, network string) error {
	pkgs := pg.Spec.NixPackages
	body, err := d.playgroundBody(pg, network, playgroundTools, nixInstallScript(pkgs), "", nil)
	if err != nil {
		return err
	}
	id, err := d.createAndStart(ctx, playgroundContainer(pg.ID, playgroundTools), body, "")
	if err != nil {
		return fmt.Errorf("start the tools install: %w", err)
	}
	if err := d.waitExit(ctx, id, PlaygroundSetupTimeout); err != nil {
		return &PlaygroundError{Msg: "installing " + strings.Join(pkgs, ", ") + " " + err.Error(), Log: d.playgroundLogTail(id)}
	}
	return nil
}

// nixInstallScript installs pkgs into the playground profile, unless the
// last install there was of the same packages and the profile still holds
// them: a restart would otherwise evaluate nixpkgs again for nothing. The
// manifest check follows the profile into the store, so packages Nix has
// since collected are installed again.
func nixInstallScript(pkgs []string) string {
	want := shellQuote(strings.Join(pkgs, " "))
	stamp := shellQuote(playgroundProfile + ".packages")
	profile := shellQuote(playgroundProfile)
	return "if [ -e " + profile + "/manifest.json ] && [ \"$(cat " + stamp + " 2>/dev/null)\" = " + want + " ]; then\n" +
		"  echo 'tools already installed'\n  exit 0\nfi\n" +
		"nix profile install --profile " + profile + " " + strings.Join(quoteAll(pkgs), " ") +
		" && printf '%s\\n' " + want + " > " + stamp + "\n"
}

// startingStep names what a playground starts before its setup.
func startingStep(spec playground.Spec) string {
	var names []string
	for _, s := range spec.Services {
		names = append(names, s.Name)
	}
	switch {
	case len(names) > 0 && len(spec.NixPackages) > 0:
		return "Starting " + joinAnd(names) + ", installing tools"
	case len(names) > 0:
		return "Starting " + joinAnd(names)
	default:
		return "Installing tools"
	}
}

func joinAnd(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// firstError runs tasks at once and returns the first error any of them
// returned, cancelling the rest as soon as one fails.
func firstError(ctx context.Context, tasks ...func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg    sync.WaitGroup
		once  sync.Once
		first error
	)
	for _, task := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := task(ctx); err != nil {
				once.Do(func() {
					first = err
					cancel()
				})
			}
		}()
	}
	wg.Wait()
	return first
}

// stillRunning fails if a container exits within grace.
func (d *Docker) stillRunning(ctx context.Context, id string, grace time.Duration) error {
	deadline := time.Now().Add(grace)
	for {
		var info struct {
			State struct {
				Status   string
				ExitCode int
			}
		}
		if _, err := d.do(ctx, "GET", "/containers/"+id+"/json", nil, "", &info); err != nil {
			return err
		}
		if info.State.Status == "exited" || info.State.Status == "dead" {
			return fmt.Errorf("exited with code %d", info.State.ExitCode)
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(playgroundPoll):
		}
	}
}

// processScript runs command in dir, which it creates as the container's
// user, with the playground's Nix tools first on PATH.
func processScript(dir, command string) string {
	script := "export PATH=" + playgroundProfile + "/bin:\"$PATH\"\n"
	if dir != "" {
		script += "mkdir -p -- " + shellQuote(dir) + " && cd -- " + shellQuote(dir) + " || exit 1\n"
	}
	return script + command
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = shellQuote(s)
	}
	return out
}

// setupScript runs each setup command from the workspace root, as the spec
// promises, in a subshell of its own: a cd in one doesn't move the next, and
// a command whose status is non-zero stops setup even when it's an && chain,
// which set -e alone lets through.
func setupScript(cmds []string) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, c := range cmds {
		b.WriteString("(cd " + workspacePath + " && " + c + "\n)\n")
	}
	return b.String()
}

// playgroundBody is a setup or process container: the agent image in the
// turn's sandbox, with the workspace and shared caches but not the run's home
// (it holds the harness logins), on the playground network only.
func (d *Docker) playgroundBody(pg PlaygroundRun, network, source, command, dir string, env map[string]string) (map[string]any, error) {
	checkout, err := d.workspaceMount(pg.RunID)
	if err != nil {
		return nil, err
	}
	mounts := []map[string]any{checkout, {"Type": "volume", "Source": "lazyagents-dependency-cache", "Target": "/cache"}}
	if m := d.nixMount(); m != nil {
		mounts = append(mounts, m)
	}
	host := d.hostConfig(mounts)
	host["NetworkMode"] = network
	delete(host, "ExtraHosts")
	// Docker would create a missing WorkingDir as root, which nothing in the
	// sandbox can write to afterwards: start at the root and let the agent's
	// user make it (processScript).
	command = processScript(dir, command)
	all := dependencyEnv(d.DependencyCacheDir(), workspacePath)
	for k, v := range env {
		all[k] = v
	}
	list := make([]string, 0, len(all))
	for k, v := range all {
		list = append(list, k+"="+v)
	}
	sort.Strings(list)
	return map[string]any{
		// Not a login shell: /etc/profile resets PATH, dropping the image's
		// Go and Nix profile directories.
		"Image": d.Image, "Cmd": []string{"bash", "-c", command}, "Env": list, "WorkingDir": workspacePath,
		"Labels": d.playgroundLabels(pg, source), "HostConfig": host,
		"NetworkingConfig": map[string]any{"EndpointsConfig": map[string]any{
			network: map[string]any{"Aliases": []string{source}},
		}},
	}, nil
}

// createAndStart creates a container by name, replacing a stale one and
// pulling a missing image, and starts it. It returns the ID of whatever it
// created, even when starting failed, so its logs can be read.
func (d *Docker) createAndStart(ctx context.Context, name string, body map[string]any, image string) (string, error) {
	var created struct{ ID string }
	create := func() error {
		_, err := d.do(ctx, "POST", "/containers/create?name="+url.QueryEscape(name), jsonBody(body), "application/json", &created)
		return err
	}
	err := create()
	if isStatus(err, 409) {
		_, _ = d.do(ctx, "DELETE", "/containers/"+url.PathEscape(name)+"?force=1&v=1", nil, "", nil)
		err = create()
	}
	if isStatus(err, 404) && image != "" {
		if err = d.pull(ctx, image); err == nil {
			err = create()
		}
	}
	if err != nil {
		return "", err
	}
	_, err = d.do(ctx, "POST", "/containers/"+created.ID+"/start", nil, "", nil)
	return created.ID, err
}

// waitExit waits for a one-shot container and fails unless it exits 0.
func (d *Docker) waitExit(ctx context.Context, id string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var res struct{ StatusCode int }
	if _, err := d.do(ctx, "POST", "/containers/"+id+"/wait", nil, "", &res); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			_, _ = d.do(context.Background(), "POST", "/containers/"+id+"/kill", nil, "", nil)
			return fmt.Errorf("took longer than %s", timeout)
		}
		return err
	}
	if res.StatusCode != 0 {
		return fmt.Errorf("exited with code %d", res.StatusCode)
	}
	return nil
}

// waitListening waits until a process accepts connections on its port, and
// fails if it exits first.
func (d *Docker) waitListening(ctx context.Context, id, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, PlaygroundReadyTimeout)
	defer cancel()
	for {
		dial := d.dialPlayground
		if dial == nil {
			dial = func(ctx context.Context, addr string) (net.Conn, error) {
				return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", addr)
			}
		}
		conn, err := dial(ctx, addr)
		if err == nil {
			conn.Close()
			return nil
		}
		var info struct {
			State struct {
				Status   string
				ExitCode int
			}
		}
		if _, err := d.do(ctx, "GET", "/containers/"+id+"/json", nil, "", &info); err == nil &&
			(info.State.Status == "exited" || info.State.Status == "dead") {
			return fmt.Errorf("exited with code %d before listening on its port", info.State.ExitCode)
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				_, port, _ := net.SplitHostPort(addr)
				return fmt.Errorf("wasn't listening on 0.0.0.0:%s after %s", port, PlaygroundReadyTimeout)
			}
			return ctx.Err()
		case <-time.After(playgroundPoll):
		}
	}
}

// playgroundLogTail is more of a failed setup or process's output than a
// service's: build errors run long.
func (d *Docker) playgroundLogTail(id string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := d.do(ctx, "GET", "/containers/"+id+"/logs?stdout=1&stderr=1&tail=60", nil, "", nil)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var out strings.Builder
	_ = demux(io.LimitReader(res.Body, 64<<10), &out, &out)
	return strings.TrimSpace(out.String())
}

type playgroundContainerInfo struct {
	ID     string
	Labels map[string]string
}

func (d *Docker) playgroundContainers(ctx context.Context, label string) ([]playgroundContainerInfo, error) {
	filters, _ := json.Marshal(map[string][]string{"label": {label, stackLabelKey + "=" + d.stack()}})
	var list []playgroundContainerInfo
	_, err := d.do(ctx, "GET", "/containers/json?all=1&filters="+url.QueryEscape(string(filters)), nil, "", &list)
	return list, err
}

func (d *Docker) StopPlayground(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	list, err := d.playgroundContainers(ctx, playgroundLabelKey+"="+id.String())
	if err != nil {
		return err
	}
	for _, c := range list {
		// v=1: a database image's anonymous data volume goes with it.
		if _, err := d.do(ctx, "DELETE", "/containers/"+c.ID+"?force=1&v=1", nil, "", nil); err != nil && !isStatus(err, 404) {
			return err
		}
	}
	network := playgroundNetwork(id)
	if self, err := d.selfID(ctx); err == nil {
		_, _ = d.do(ctx, "POST", "/networks/"+url.PathEscape(network)+"/disconnect", jsonBody(map[string]any{"Container": self, "Force": true}), "application/json", nil)
	}
	if _, err := d.do(ctx, "DELETE", "/networks/"+url.PathEscape(network), nil, "", nil); err != nil && !isStatus(err, 404) {
		return err
	}
	return nil
}

func (d *Docker) RunningPlaygrounds(ctx context.Context) ([]uuid.UUID, error) {
	list, err := d.playgroundContainers(ctx, playgroundLabelKey)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, c := range list {
		if id, err := uuid.Parse(c.Labels[playgroundLabelKey]); err == nil && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// ReadFile reads a file from the workspace a running playground uses.
func (d *Docker) ReadFile(ctx context.Context, id uuid.UUID, rel string) ([]byte, error) {
	list, err := d.playgroundContainers(ctx, playgroundLabelKey+"="+id.String())
	if err != nil || len(list) == 0 {
		return nil, errors.Join(errors.New("the playground isn't running"), err)
	}
	runID := list[0].Labels[labelKey]
	rc, _, err := d.OpenWorkspaceFile(ctx, runID, rel, playground.MaxSpecBytes)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, playground.MaxSpecBytes+1))
}

// Logs merges every playground container's output: the last tail lines
// across them in time order, then, with follow, lines as they're written.
// Docker prefixes each line with its RFC 3339 time (timestamps=1).
func (d *Docker) Logs(ctx context.Context, id uuid.UUID, tail int, follow bool, emit func(playground.LogLine) error) error {
	list, err := d.playgroundContainers(ctx, playgroundLabelKey+"="+id.String())
	if err != nil {
		return err
	}
	var history []playground.LogLine
	last := map[string]time.Time{}
	for _, c := range list {
		lines, err := d.containerLogs(ctx, c, fmt.Sprintf("tail=%d", tail), nil)
		if err != nil {
			continue // removed meanwhile
		}
		history = append(history, lines...)
		if n := len(lines); n > 0 {
			last[c.ID] = lines[n-1].Time
		}
	}
	sort.SliceStable(history, func(i, j int) bool { return history[i].Time.Before(history[j].Time) })
	if len(history) > tail {
		history = history[len(history)-tail:]
	}
	for _, l := range history {
		if err := emit(l); err != nil {
			return err
		}
	}
	if !follow {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	lines := make(chan playground.LogLine, 64)
	var wg sync.WaitGroup
	for _, c := range list {
		after := last[c.ID]
		since := "since=" + strconv.FormatInt(after.Unix(), 10)
		if after.IsZero() {
			since = "tail=0"
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.containerLogs(ctx, c, since+"&follow=1", func(l playground.LogLine) {
				if !l.Time.After(after) { // since= is whole seconds: skip what was sent
					return
				}
				select {
				case lines <- l:
				case <-ctx.Done():
				}
			})
		}()
	}
	go func() { wg.Wait(); close(lines) }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case l, ok := <-lines:
			if !ok {
				return nil // every container stopped
			}
			if err := emit(l); err != nil {
				return err
			}
		}
	}
}

// containerLogs reads a container's log lines; with each, it calls fn as
// they arrive instead of collecting them.
func (d *Docker) containerLogs(ctx context.Context, c playgroundContainerInfo, query string, each func(playground.LogLine)) ([]playground.LogLine, error) {
	res, err := d.do(ctx, "GET", "/containers/"+c.ID+"/logs?stdout=1&stderr=1&timestamps=1&"+query, nil, "", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	source := c.Labels[playgroundSourceKey]
	var (
		out     []playground.LogLine
		mu      sync.Mutex
		readers sync.WaitGroup
	)
	stream := func(name string) *io.PipeWriter {
		pr, pw := io.Pipe()
		readers.Add(1)
		go func() {
			defer readers.Done()
			sc := bufio.NewScanner(pr)
			sc.Buffer(make([]byte, 64<<10), 1<<20)
			for sc.Scan() {
				l := parseLogLine(source, name, sc.Text())
				if each != nil {
					each(l)
					continue
				}
				mu.Lock()
				out = append(out, l)
				mu.Unlock()
			}
			_, _ = io.Copy(io.Discard, pr) // a line too long for the scanner
		}()
		return pw
	}
	stdout, stderr := stream("stdout"), stream("stderr")
	err = demux(res.Body, stdout, stderr)
	stdout.Close()
	stderr.Close()
	readers.Wait()
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, err
}

func parseLogLine(source, stream, raw string) playground.LogLine {
	l := playground.LogLine{Source: source, Stream: stream, Text: raw}
	if ts, text, ok := strings.Cut(raw, " "); ok {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			l.Time, l.Text = t, text
		}
	}
	return l
}
