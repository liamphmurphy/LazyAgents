package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// A turn's Docker proxy gives the agent a Docker API of its own, so tests can
// start whatever they need (docker run, Testcontainers in any language)
// without the agent ever holding the runner's socket, which is root on the
// host. Each turn gets a unix socket the runner serves; the proxy forwards a
// fixed set of Engine calls and rewrites every container it creates into a
// sandbox: no host mounts, no privileges or added capabilities, no host
// namespaces, resource limits, and ports published only on the turn's own
// network gateway rather than on every host interface. Everything it creates
// carries the turn's label and is removed when the turn ends.

const (
	proxyLabelKey = "dev.lazyagents.proxy"
	// proxyMountPath is where turns see the sockets directory.
	proxyMountPath = "/run/lazyagents-docker"
	// maxProxyContainers bounds how many containers one turn may have.
	maxProxyContainers = 20
	maxProxyBody       = 1 << 20
)

// ProxyBackend gives a turn its own sandboxed Docker API.
type ProxyBackend interface {
	// StartDockerProxy serves a Docker API for one turn. The turn joins
	// Network and gets Env; Stop removes everything the turn created and is
	// safe to call more than once.
	// workspaceID names the run's checkout, which containers may bind from.
	StartDockerProxy(ctx context.Context, runID, workspaceID, key string) (*TurnDocker, error)
}

// TurnDocker is a turn's Docker API.
type TurnDocker struct {
	Network string
	Env     map[string]string
	Stop    func()
}

// errProxyOff means this runner has no sockets directory shared with turns.
var errProxyOff = errors.New("this runner has no Docker proxy (set RUNNER_DOCKER_PROXY_DIR and RUNNER_DOCKER_PROXY_VOLUME)")

func proxyNetworkName(key string) string { return "lazyagents-dkr-" + key }

// PrepareDockerProxy makes the sockets directory: traversable but not
// listable by turns, so a turn can open only the socket whose name it was given.
func (d *Docker) PrepareDockerProxy() error {
	if d.ProxyDir == "" || d.ProxyVolume == "" {
		return nil
	}
	if err := os.MkdirAll(d.ProxyDir, 0o711); err != nil {
		return fmt.Errorf("docker proxy: %w", err)
	}
	return os.Chmod(d.ProxyDir, 0o711)
}

func (d *Docker) StartDockerProxy(ctx context.Context, runID, workspaceID, key string) (*TurnDocker, error) {
	if d.ProxyDir == "" || d.ProxyVolume == "" {
		return nil, errProxyOff
	}
	ws, err := d.workspaceMount(workspaceID)
	if err != nil {
		return nil, err
	}
	p := &turnProxy{d: d, runID: runID, key: key, network: proxyNetworkName(key), workspace: ws}
	// Not internal: what tests start may need the internet (a cloud emulator
	// fetching its runtime), and published ports need a bridge with a gateway.
	_, err = d.do(ctx, "POST", "/networks/create", jsonBody(map[string]any{
		"Name": p.network, "Labels": p.labels(nil),
	}), "application/json", nil)
	if err != nil && !isStatus(err, 409) { // 409: left by a turn that died; reuse it
		return nil, fmt.Errorf("create the turn's docker network: %w", err)
	}
	var info struct {
		IPAM struct{ Config []struct{ Gateway string } }
	}
	if _, err := d.do(ctx, "GET", "/networks/"+url.PathEscape(p.network), nil, "", &info); err != nil {
		p.cleanup()
		return nil, fmt.Errorf("inspect the turn's docker network: %w", err)
	}
	for _, c := range info.IPAM.Config {
		if ip := net.ParseIP(c.Gateway); ip != nil && ip.To4() != nil {
			p.gateway = c.Gateway
			break
		}
	}
	if p.gateway == "" {
		p.cleanup()
		return nil, errors.New("the turn's docker network has no IPv4 gateway")
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		p.cleanup()
		return nil, err
	}
	name := hex.EncodeToString(token) + ".sock"
	sock := filepath.Join(d.ProxyDir, name)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		p.cleanup()
		return nil, fmt.Errorf("docker proxy socket: %w", err)
	}
	// Turns run as uid 1000; the unlistable directory is what keeps one turn
	// from finding another's socket.
	if err := os.Chmod(sock, 0o666); err != nil {
		_ = ln.Close()
		p.cleanup()
		return nil, err
	}
	srv := &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = srv.Close()
			_ = os.Remove(sock)
			p.cleanup()
		})
	}
	return &TurnDocker{
		Network: p.network,
		Env: map[string]string{
			"DOCKER_HOST": "unix://" + proxyMountPath + "/" + name,
			// Ryuk reaps containers by mounting the Docker socket, which the
			// sandbox refuses; the proxy removes them when the turn ends.
			"TESTCONTAINERS_RYUK_DISABLED": "true",
			// Published ports listen on the turn network's gateway only.
			"TESTCONTAINERS_HOST_OVERRIDE": p.gateway,
			// Builds go through the classic builder (see build): BuildKit's
			// session and gRPC streams can't be inspected.
			"DOCKER_BUILDKIT": "0",
		},
		Stop: stop,
	}, nil
}

type turnProxy struct {
	d       *Docker
	runID   string
	key     string
	network string
	gateway string
	// workspace is the run's checkout as a mount (its volume, or its host
	// folder): what binds under /workspace are rewritten to.
	workspace map[string]any
	fwd       *httputil.ReverseProxy
	once      sync.Once
	// created serializes container creation so the per-turn cap holds.
	created sync.Mutex
}

// labels marks what the turn creates, on top of any labels the caller set.
func (p *turnProxy) labels(own map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range own {
		out[k] = v
	}
	for k, v := range p.d.labels(p.runID) {
		out[k] = v
	}
	out[proxyLabelKey] = p.key
	return out
}

func (p *turnProxy) filter() string { return proxyLabelKey + "=" + p.key }

// cleanup removes everything the turn created: containers, then the networks
// and volumes they used.
func (p *turnProxy) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	filters, _ := json.Marshal(map[string][]string{"label": {p.filter()}})
	q := "filters=" + url.QueryEscape(string(filters))
	var containers []struct{ ID string }
	if _, err := p.d.do(ctx, "GET", "/containers/json?all=1&"+q, nil, "", &containers); err == nil {
		for _, c := range containers {
			_, _ = p.d.do(ctx, "DELETE", "/containers/"+c.ID+"?force=1&v=1", nil, "", nil)
		}
	}
	var networks []struct{ ID string }
	if _, err := p.d.do(ctx, "GET", "/networks?"+q, nil, "", &networks); err == nil {
		for _, n := range networks {
			_, _ = p.d.do(ctx, "DELETE", "/networks/"+n.ID, nil, "", nil)
		}
	}
	var volumes struct{ Volumes []struct{ Name string } }
	if _, err := p.d.do(ctx, "GET", "/volumes?"+q, nil, "", &volumes); err == nil {
		for _, v := range volumes.Volumes {
			_, _ = p.d.do(ctx, "DELETE", "/volumes/"+url.PathEscape(v.Name)+"?force=1", nil, "", nil)
		}
	}
}

var apiVersion = regexp.MustCompile(`^/v[0-9]+(\.[0-9]+)?`)

// proxyError answers the way the Engine does, so clients show the message.
func proxyError(w http.ResponseWriter, code int, format string, args ...any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "vesper: " + fmt.Sprintf(format, args...)})
}

func (p *turnProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := apiVersion.ReplaceAllString(r.URL.Path, "")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	get := r.Method == http.MethodGet || r.Method == http.MethodHead
	post := r.Method == http.MethodPost
	switch {
	case get && slices.Contains([]string{"/_ping", "/version", "/info", "/images/json"}, path):
		p.forward(w, r)
	case post && path == "/images/create":
		p.pullImage(w, r)
	case post && path == "/auth":
		p.forward(w, r)
	case post && path == "/build":
		p.build(w, r)
	case r.Method == http.MethodDelete && len(parts) >= 2 && parts[0] == "images":
		p.removeImage(w, r, strings.Join(parts[1:], "/")) // names may contain slashes
	case get && len(parts) >= 3 && parts[0] == "images" && (parts[len(parts)-1] == "json" || parts[len(parts)-1] == "history"):
		p.forward(w, r) // /images/{name}/json; names may contain slashes
	case get && len(parts) >= 3 && parts[0] == "distribution" && parts[len(parts)-1] == "json":
		p.forward(w, r)

	case post && path == "/containers/create":
		p.createContainer(w, r)
	case get && path == "/containers/json":
		p.forwardFiltered(w, r)
	case len(parts) >= 2 && parts[0] == "containers":
		p.container(w, r, parts[1], strings.Join(parts[2:], "/"))
	case len(parts) == 3 && parts[0] == "exec":
		p.exec(w, r, parts[1], parts[2])

	case post && path == "/networks/create":
		p.createNetwork(w, r)
	case get && path == "/networks":
		p.forwardFiltered(w, r)
	case len(parts) >= 2 && parts[0] == "networks":
		p.networkCall(w, r, parts[1], strings.Join(parts[2:], "/"))

	case post && path == "/volumes/create":
		p.createVolume(w, r)
	case get && path == "/volumes":
		p.forwardFiltered(w, r)
	case len(parts) == 2 && parts[0] == "volumes" && (get || r.Method == http.MethodDelete):
		if !p.owns(r.Context(), "/volumes/"+url.PathEscape(parts[1]), func(v map[string]any) map[string]any { return asMap(v["Labels"]) }) {
			proxyError(w, http.StatusNotFound, "no such volume in this turn: %s", parts[1])
			return
		}
		p.forward(w, r)

	default:
		proxyError(w, http.StatusForbidden, "%s %s isn't available to agents (BuildKit sessions, swarm, plugins and system calls are host-wide)", r.Method, path)
	}
}

// forward passes the request to the Engine unchanged. Attach and exec
// upgrade the connection, which the reverse proxy carries through.
func (p *turnProxy) forward(w http.ResponseWriter, r *http.Request) {
	p.once.Do(func() {
		p.fwd = &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL.Scheme, pr.Out.URL.Host = "http", "docker"
				pr.Out.Host = "docker"
			},
			Transport:     p.d.http().Transport,
			FlushInterval: -1, // logs, pulls and waits stream
			ModifyResponse: func(res *http.Response) error {
				// Clients pick BuildKit when the daemon offers it; this one
				// only accepts classic builds.
				if res.Header.Get("Builder-Version") != "" {
					res.Header.Set("Builder-Version", "1")
				}
				return nil
			},
		}
	})
	p.fwd.ServeHTTP(w, r)
}

// forwardFiltered lists only what this turn created.
func (p *turnProxy) forwardFiltered(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filters, err := withLabel(q.Get("filters"), p.filter())
	if err != nil {
		proxyError(w, http.StatusBadRequest, "filters: %v", err)
		return
	}
	q.Set("filters", filters)
	r.URL.RawQuery = q.Encode()
	p.forward(w, r)
}

// withLabel adds a label filter to an Engine filters argument, in either of
// the two shapes clients send.
func withLabel(raw, label string) (string, error) {
	filters := map[string][]string{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &filters); err != nil {
			legacy := map[string]map[string]bool{}
			if json.Unmarshal([]byte(raw), &legacy) != nil {
				return "", err
			}
			for k, set := range legacy {
				for v, on := range set {
					if on {
						filters[k] = append(filters[k], v)
					}
				}
			}
		}
	}
	filters["label"] = append(filters["label"], label)
	b, _ := json.Marshal(filters)
	return string(b), nil
}

// owns reports whether the object at path carries this turn's label; labels
// picks the label map out of the inspect response.
func (p *turnProxy) owns(ctx context.Context, path string, labels func(map[string]any) map[string]any) bool {
	var info map[string]any
	if _, err := p.d.do(ctx, "GET", path, nil, "", &info); err != nil {
		return false
	}
	v, _ := labels(info)[proxyLabelKey].(string)
	return v == p.key
}

func (p *turnProxy) ownsContainer(ctx context.Context, id string) bool {
	return p.owns(ctx, "/containers/"+url.PathEscape(id)+"/json", func(c map[string]any) map[string]any {
		return asMap(asMap(c["Config"])["Labels"])
	})
}

func (p *turnProxy) ownsNetwork(ctx context.Context, id string) bool {
	return p.owns(ctx, "/networks/"+url.PathEscape(id), func(n map[string]any) map[string]any { return asMap(n["Labels"]) })
}

func (p *turnProxy) ownsVolume(ctx context.Context, name string) bool {
	return p.owns(ctx, "/volumes/"+url.PathEscape(name), func(v map[string]any) map[string]any { return asMap(v["Labels"]) })
}

// containerActions are the calls a turn may make on its own containers.
// update is missing on purpose: it would lift the resource limits.
var containerActions = map[string][]string{
	"json": {"GET"}, "top": {"GET"}, "logs": {"GET"}, "stats": {"GET"}, "changes": {"GET"},
	"start": {"POST"}, "stop": {"POST"}, "restart": {"POST"}, "kill": {"POST"}, "wait": {"POST"},
	"pause": {"POST"}, "unpause": {"POST"}, "resize": {"POST"}, "attach": {"POST"}, "rename": {"POST"},
	"exec": {"POST"}, "archive": {"GET", "HEAD", "PUT"}, "": {"DELETE"},
}

func (p *turnProxy) container(w http.ResponseWriter, r *http.Request, id, action string) {
	if !slices.Contains(containerActions[action], r.Method) {
		proxyError(w, http.StatusForbidden, "%s on a container isn't available to agents", strings.TrimSpace(r.Method+" "+action))
		return
	}
	if !p.ownsContainer(r.Context(), id) {
		proxyError(w, http.StatusNotFound, "no such container in this turn: %s", id)
		return
	}
	if action == "exec" {
		var body map[string]any
		if !readBody(w, r, &body) {
			return
		}
		if on, _ := body["Privileged"].(bool); on {
			proxyError(w, http.StatusForbidden, "privileged exec isn't allowed")
			return
		}
		setBody(r, body)
	}
	p.forward(w, r)
}

func (p *turnProxy) exec(w http.ResponseWriter, r *http.Request, id, action string) {
	allowed := (action == "json" && r.Method == http.MethodGet) || (r.Method == http.MethodPost && (action == "start" || action == "resize"))
	if !allowed {
		proxyError(w, http.StatusForbidden, "%s %s on an exec isn't available to agents", r.Method, action)
		return
	}
	var info struct{ ContainerID string }
	if _, err := p.d.do(r.Context(), "GET", "/exec/"+url.PathEscape(id)+"/json", nil, "", &info); err != nil || !p.ownsContainer(r.Context(), info.ContainerID) {
		proxyError(w, http.StatusNotFound, "no such exec in this turn: %s", id)
		return
	}
	p.forward(w, r)
}

func (p *turnProxy) createContainer(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !readBody(w, r, &body) {
		return
	}
	p.created.Lock()
	defer p.created.Unlock()
	ctx := r.Context()
	filters, _ := json.Marshal(map[string][]string{"label": {p.filter()}})
	var existing []struct{ ID string }
	if _, err := p.d.do(ctx, "GET", "/containers/json?all=1&filters="+url.QueryEscape(string(filters)), nil, "", &existing); err != nil {
		proxyError(w, http.StatusBadGateway, "count this turn's containers: %v", err)
		return
	}
	if len(existing) >= maxProxyContainers {
		proxyError(w, http.StatusTooManyRequests, "a turn may have at most %d containers; remove some first", maxProxyContainers)
		return
	}
	sandboxed, err := p.sandbox(body, func(name string) bool { return p.ownsVolume(ctx, name) }, func(name string) bool { return p.ownsNetwork(ctx, name) })
	if err != nil {
		proxyError(w, http.StatusForbidden, "%v", err)
		return
	}
	setBody(r, sandboxed)
	p.forward(w, r)
}

// sandbox rewrites a container create request so the container can't reach
// past itself: HostConfig is rebuilt from fields that only shape the
// container, and anything that would reach the host is refused with a reason
// rather than dropped, so a test fails with a message instead of oddly.
func (p *turnProxy) sandbox(body map[string]any, ownVolume, ownNetwork func(string) bool) (map[string]any, error) {
	in := asMap(body["HostConfig"])
	refuse := func(field, why string) error { return fmt.Errorf("HostConfig.%s isn't allowed: %s", field, why) }
	if on, _ := in["Privileged"].(bool); on {
		return nil, refuse("Privileged", "a privileged container controls the host")
	}
	for _, field := range []string{"CapAdd", "Devices", "DeviceRequests", "DeviceCgroupRules", "VolumesFrom", "Links"} {
		if list, _ := in[field].([]any); len(list) > 0 {
			return nil, refuse(field, "it reaches past the container")
		}
	}
	for _, field := range []string{"CgroupParent", "Runtime", "Cgroup"} {
		if s, _ := in[field].(string); s != "" {
			return nil, refuse(field, "it changes how the host runs the container")
		}
	}
	for _, field := range []string{"PidMode", "IpcMode", "UTSMode", "UsernsMode", "CgroupnsMode"} {
		if s, _ := in[field].(string); s == "host" || strings.HasPrefix(s, "container:") {
			return nil, refuse(field, "it shares another namespace")
		}
	}
	for _, opt := range asList(in["SecurityOpt"]) {
		if s, _ := opt.(string); s != "no-new-privileges" && s != "no-new-privileges:true" {
			return nil, refuse("SecurityOpt", "only no-new-privileges is accepted")
		}
	}

	host := map[string]any{}
	for _, field := range []string{"Tmpfs", "ShmSize", "Init", "AutoRemove", "Ulimits", "ExtraHosts", "ReadonlyRootfs",
		"Dns", "DnsOptions", "DnsSearch", "Sysctls", "NanoCpus", "CpuShares", "CpuQuota", "CpuPeriod", "OomScoreAdj"} {
		if v, ok := in[field]; ok && v != nil {
			host[field] = v
		}
	}
	host["SecurityOpt"] = []string{"no-new-privileges"}
	restart := asMap(in["RestartPolicy"])
	if name, _ := restart["Name"].(string); name == "on-failure" {
		host["RestartPolicy"] = restart
	} else {
		host["RestartPolicy"] = map[string]any{"Name": "no"}
	}
	if limit := p.d.MemoryMB << 20; limit > 0 {
		if m, _ := in["Memory"].(float64); m > 0 && int64(m) < limit {
			host["Memory"] = int64(m)
		} else {
			host["Memory"] = limit
		}
	}
	if p.d.PidsLimit > 0 {
		host["PidsLimit"] = p.d.PidsLimit
	}

	var binds []string
	var mounts []any
	for _, b := range asList(in["Binds"]) {
		spec, _ := b.(string)
		parts := strings.Split(spec, ":")
		src := parts[0]
		if strings.HasPrefix(src, "/") && len(parts) >= 2 {
			m, err := p.workspaceBind(src, parts[1], len(parts) > 2 && slices.Contains(strings.Split(parts[2], ","), "ro"))
			if err != nil {
				return nil, refuse("Binds", fmt.Sprintf("%q: %v", spec, err))
			}
			mounts = append(mounts, m)
			continue
		}
		if src == "" || strings.HasPrefix(src, ".") || !ownVolume(src) {
			return nil, refuse("Binds", fmt.Sprintf("%q: only this run's checkout (under /workspace) and volumes this turn created can be mounted", spec))
		}
		binds = append(binds, spec)
	}
	if len(binds) > 0 {
		host["Binds"] = binds
	}
	for _, m := range asList(in["Mounts"]) {
		mount := asMap(m)
		switch t, _ := mount["Type"].(string); t {
		case "tmpfs":
		case "volume":
			src, _ := mount["Source"].(string)
			if src != "" && !ownVolume(src) {
				return nil, refuse("Mounts", fmt.Sprintf("volume %q wasn't created by this turn", src))
			}
			if opts := asMap(mount["VolumeOptions"]); len(asMap(opts["DriverConfig"])) > 0 {
				return nil, refuse("Mounts", "volume driver options can mount host paths")
			}
		case "bind":
			src, _ := mount["Source"].(string)
			dst, _ := mount["Target"].(string)
			ro, _ := mount["ReadOnly"].(bool)
			translated, err := p.workspaceBind(src, dst, ro)
			if err != nil {
				return nil, refuse("Mounts", fmt.Sprintf("bind %q: %v", src, err))
			}
			mount = translated
		default:
			return nil, refuse("Mounts", fmt.Sprintf("%q mounts reach the host; use a volume or tmpfs", t))
		}
		mounts = append(mounts, mount)
	}
	if len(mounts) > 0 {
		host["Mounts"] = mounts
	}

	// Networking: the turn's network unless the caller picked another network
	// the turn created; never the host's, another container's or the stack's.
	network := p.network
	switch mode, _ := in["NetworkMode"].(string); {
	case mode == "" || mode == "default" || mode == "bridge" || mode == p.network:
	case mode == "none":
		network = "none"
	case mode == "host" || strings.HasPrefix(mode, "container:"):
		return nil, refuse("NetworkMode", "it shares the host's or another container's network")
	case ownNetwork(mode):
		network = mode
	default:
		return nil, refuse("NetworkMode", fmt.Sprintf("network %q wasn't created by this turn", mode))
	}
	host["NetworkMode"] = network
	out := map[string]any{}
	for k, v := range body {
		out[k] = v
	}
	if nc := asMap(body["NetworkingConfig"]); nc != nil {
		endpoints := map[string]any{}
		for name, cfg := range asMap(nc["EndpointsConfig"]) {
			switch {
			case name == "" || name == "default" || name == "bridge":
				// The docker CLI keys its endpoint by the NetworkMode it sent,
				// "default" for a plain docker run: that's the turn's network.
				name = network
			case name == network || name == p.network || ownNetwork(name):
			default:
				return nil, refuse("NetworkingConfig", fmt.Sprintf("network %q wasn't created by this turn", name))
			}
			endpoints[name] = cfg
		}
		out["NetworkingConfig"] = map[string]any{"EndpointsConfig": endpoints}
	}

	// Published ports listen on the turn network's gateway only: the turn can
	// reach them, the person's network can't.
	bindings := map[string]any{}
	for port, list := range asMap(in["PortBindings"]) {
		var rebound []any
		for _, b := range asList(list) {
			hostPort, _ := asMap(b)["HostPort"].(string)
			rebound = append(rebound, map[string]any{"HostIp": p.gateway, "HostPort": hostPort})
		}
		if len(rebound) == 0 {
			rebound = []any{map[string]any{"HostIp": p.gateway, "HostPort": ""}}
		}
		bindings[port] = rebound
	}
	if all, _ := in["PublishAllPorts"].(bool); all {
		for port := range asMap(body["ExposedPorts"]) {
			if _, ok := bindings[port]; !ok {
				bindings[port] = []any{map[string]any{"HostIp": p.gateway, "HostPort": ""}}
			}
		}
	}
	if len(bindings) > 0 && network != "none" {
		host["PortBindings"] = bindings
	}
	out["HostConfig"] = host
	out["Labels"] = p.labels(asMap(body["Labels"]))
	return out, nil
}

// workspaceBind turns a bind of a path in the turn's checkout (compose's
// ./server, resolved by the client to /workspace/server) into a mount of the
// run's own checkout at that subpath. Anything else on the host is refused.
func (p *turnProxy) workspaceBind(src, dst string, readOnly bool) (map[string]any, error) {
	clean := path.Clean(src)
	if clean != workspacePath && !strings.HasPrefix(clean, workspacePath+"/") {
		return nil, errors.New("only this run's checkout (under /workspace) can be bind-mounted, never other host paths")
	}
	if dst == "" || !strings.HasPrefix(dst, "/") {
		return nil, errors.New("the target must be an absolute path")
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(clean, workspacePath), "/")
	mount := map[string]any{"Target": dst, "ReadOnly": readOnly}
	switch p.workspace["Type"] {
	case "volume":
		// The Engine resolves a subpath inside the volume and refuses one that
		// escapes it through a symlink.
		mount["Type"], mount["Source"] = "volume", p.workspace["Source"]
		if rel != "" {
			mount["VolumeOptions"] = map[string]any{"Subpath": rel}
		}
	case "bind":
		// A host folder: a bind follows symlinks on the host, so resolve the
		// path here (the runner sees the folder at the same path) and keep it
		// inside the run's folder.
		folder, _ := p.workspace["Source"].(string)
		real, err := filepath.EvalSymlinks(filepath.Join(folder, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("resolve it in the checkout: %w", err)
		}
		root, err := filepath.EvalSymlinks(folder)
		if err != nil {
			return nil, err
		}
		if real != root && !strings.HasPrefix(real, root+string(filepath.Separator)) {
			return nil, errors.New("it leads outside the run's checkout")
		}
		mount["Type"], mount["Source"] = "bind", real
	default:
		return nil, errors.New("this run has no checkout to mount")
	}
	return mount, nil
}

// imageReplaceable reports whether a pull or build may (re)write ref: a new
// name; an image a turn made; or, for a pull, one that came from a registry
// (re-pulling it changes nothing a registry couldn't). Locally built images,
// such as the agent image every turn runs in, are never replaced.
// removeImage lets a turn clean up (docker compose down --rmi local) images
// its run built, and nothing else: other runs' builds, pulled images and the
// agent image are shared by every run on the daemon.
func (p *turnProxy) removeImage(w http.ResponseWriter, r *http.Request, ref string) {
	var info struct {
		Config struct{ Labels map[string]string }
	}
	_, err := p.d.do(r.Context(), "GET", "/images/"+url.PathEscape(ref)+"/json", nil, "", &info)
	if err != nil {
		proxyError(w, http.StatusNotFound, "no such image built by this run: %s", ref)
		return
	}
	if l := info.Config.Labels; l[proxyLabelKey] == "" || l[labelKey] != p.runID || l[stackLabelKey] != p.d.stack() {
		proxyError(w, http.StatusForbidden, "%s wasn't built by this run; only images a run builds can be removed by it", ref)
		return
	}
	p.forward(w, r)
}

func (p *turnProxy) imageReplaceable(ctx context.Context, ref string, pull bool) bool {
	var info struct {
		RepoDigests []string
		Config      struct{ Labels map[string]string }
	}
	_, err := p.d.do(ctx, "GET", "/images/"+url.PathEscape(ref)+"/json", nil, "", &info)
	switch {
	case isStatus(err, 404):
		return true
	case err != nil:
		return false
	case info.Config.Labels[proxyLabelKey] != "" && info.Config.Labels[stackLabelKey] == p.d.stack():
		return true
	}
	return pull && len(info.RepoDigests) > 0
}

func (p *turnProxy) pullImage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ref := q.Get("fromImage")
	if tag := q.Get("tag"); tag != "" && !strings.Contains(ref, "@") {
		ref += ":" + tag
	}
	if q.Get("fromSrc") != "" || q.Get("repo") != "" {
		proxyError(w, http.StatusForbidden, "image import isn't available to agents; pull from a registry")
		return
	}
	if !p.imageReplaceable(r.Context(), ref, true) {
		proxyError(w, http.StatusForbidden, "%s is a locally built image; pulling would replace it for every run", ref)
		return
	}
	p.forward(w, r)
}

// build runs a classic (non-BuildKit) build: its RUN steps are containers on
// the turn's network with the daemon's defaults (no privileges, no mounts).
// BuildKit is refused because its session and gRPC streams carry requests,
// host networking among them, that the proxy can't read.
func (p *turnProxy) build(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if v := q.Get("version"); v != "" && v != "1" {
		proxyError(w, http.StatusForbidden, "BuildKit isn't available to agents; build with DOCKER_BUILDKIT=0")
		return
	}
	switch mode := q.Get("networkmode"); mode {
	case "", "default", "bridge", p.network:
	case "none":
	default:
		proxyError(w, http.StatusForbidden, "builds run on the turn's network, not %q", mode)
		return
	}
	for _, tag := range q["t"] {
		if !p.imageReplaceable(r.Context(), tag, false) {
			proxyError(w, http.StatusForbidden, "tag %s names an image this turn didn't build; pick another tag", tag)
			return
		}
	}
	labels := map[string]any{}
	if raw := q.Get("labels"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &labels)
	}
	b, _ := json.Marshal(p.labels(labels))
	q.Set("labels", string(b))
	q.Set("version", "1")
	if q.Get("networkmode") != "none" {
		q.Set("networkmode", p.network)
	}
	q.Del("extrahosts")
	r.URL.RawQuery = q.Encode()
	p.forward(w, r)
}

func (p *turnProxy) createNetwork(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !readBody(w, r, &body) {
		return
	}
	if driver, _ := body["Driver"].(string); driver != "" && driver != "bridge" {
		proxyError(w, http.StatusForbidden, "only bridge networks are available to agents")
		return
	}
	// Driver options and custom address ranges could claim host interface
	// names or routes; Docker picks both.
	out := map[string]any{"Name": body["Name"], "Driver": "bridge", "Labels": p.labels(asMap(body["Labels"]))}
	for _, field := range []string{"Internal", "Attachable", "CheckDuplicate", "EnableIPv6"} {
		if v, ok := body[field]; ok {
			out[field] = v
		}
	}
	setBody(r, out)
	p.forward(w, r)
}

func (p *turnProxy) networkCall(w http.ResponseWriter, r *http.Request, id, action string) {
	ctx := r.Context()
	ok := (action == "" && (r.Method == http.MethodGet || r.Method == http.MethodDelete)) ||
		(r.Method == http.MethodPost && (action == "connect" || action == "disconnect"))
	if !ok {
		proxyError(w, http.StatusForbidden, "%s %s on a network isn't available to agents", r.Method, action)
		return
	}
	if !p.ownsNetwork(ctx, id) {
		proxyError(w, http.StatusNotFound, "no such network in this turn: %s", id)
		return
	}
	if action == "connect" || action == "disconnect" {
		var body map[string]any
		if !readBody(w, r, &body) {
			return
		}
		container, _ := body["Container"].(string)
		if !p.ownsContainer(ctx, container) {
			proxyError(w, http.StatusNotFound, "no such container in this turn: %s", container)
			return
		}
		setBody(r, body)
	}
	p.forward(w, r)
}

func (p *turnProxy) createVolume(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !readBody(w, r, &body) {
		return
	}
	if driver, _ := body["Driver"].(string); driver != "" && driver != "local" {
		proxyError(w, http.StatusForbidden, "only local volumes are available to agents")
		return
	}
	if len(asMap(body["DriverOpts"])) > 0 {
		proxyError(w, http.StatusForbidden, "volume driver options can mount host paths")
		return
	}
	setBody(r, map[string]any{"Name": body["Name"], "Driver": "local", "Labels": p.labels(asMap(body["Labels"]))})
	p.forward(w, r)
}

func readBody(w http.ResponseWriter, r *http.Request, v any) bool {
	b, err := io.ReadAll(io.LimitReader(r.Body, maxProxyBody+1))
	if err != nil {
		proxyError(w, http.StatusBadRequest, "read request: %v", err)
		return false
	}
	if len(b) > maxProxyBody {
		proxyError(w, http.StatusRequestEntityTooLarge, "request body over %d bytes", maxProxyBody)
		return false
	}
	if len(bytes.TrimSpace(b)) == 0 {
		b = []byte("{}")
	}
	if err := json.Unmarshal(b, v); err != nil {
		proxyError(w, http.StatusBadRequest, "request body: %v", err)
		return false
	}
	return true
}

func setBody(r *http.Request, v any) {
	b, _ := json.Marshal(v)
	r.Body = io.NopCloser(bytes.NewReader(b))
	r.ContentLength = int64(len(b))
	r.Header.Set("Content-Length", fmt.Sprint(len(b)))
	r.Header.Set("Content-Type", "application/json")
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}
