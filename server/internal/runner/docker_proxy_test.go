package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// proxyEngine fakes the Engine behind a turn's Docker proxy: it remembers the
// labels of what it creates and every request that reached it.
type proxyEngine struct {
	mu       sync.Mutex
	reached  []string                  // "METHOD path" that got past the proxy
	creates  []map[string]any          // container create bodies
	labels   map[string]map[string]any // container, network or volume id → labels
	deleted  []string
	networks map[string]bool
}

func (f *proxyEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := apiVersion.ReplaceAllString(r.URL.Path, "")
	f.reached = append(f.reached, r.Method+" "+p)
	parts := strings.Split(strings.Trim(p, "/"), "/")
	listed := func(kind string) []string {
		filters := map[string][]string{}
		_ = json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters)
		var ids []string
		for id, l := range f.labels {
			if !strings.HasPrefix(id, kind) {
				continue
			}
			match := true
			for _, want := range filters["label"] {
				k, v, _ := strings.Cut(want, "=")
				if got, ok := l[k]; !ok || (v != "" && got != v) {
					match = false
				}
			}
			if match {
				ids = append(ids, id)
			}
		}
		return ids
	}
	switch {
	case r.Method == "POST" && p == "/networks/create":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.labels["net-"+body["Name"].(string)] = asMap(body["Labels"])
		w.WriteHeader(201)
		_, _ = io.WriteString(w, `{"Id":"n1"}`)
	case r.Method == "GET" && len(parts) == 2 && parts[0] == "networks":
		l, ok := f.labels["net-"+parts[1]]
		if !ok {
			http.Error(w, `{"message":"no such network"}`, 404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Labels": l, "IPAM": map[string]any{"Config": []any{map[string]any{"Subnet": "172.30.0.0/16", "Gateway": "172.30.0.1"}}}})
	case r.Method == "POST" && p == "/containers/create":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.creates = append(f.creates, body)
		id := fmt.Sprintf("ctr-%d", len(f.creates))
		f.labels[id] = asMap(body["Labels"])
		w.WriteHeader(201)
		_, _ = fmt.Fprintf(w, `{"Id":%q}`, id)
	case r.Method == "GET" && p == "/containers/json":
		var out []map[string]any
		for _, id := range listed("ctr-") {
			out = append(out, map[string]any{"Id": id, "ID": id})
		}
		_ = json.NewEncoder(w).Encode(out)
	case r.Method == "GET" && p == "/networks":
		var out []map[string]any
		for _, id := range listed("net-") {
			out = append(out, map[string]any{"ID": id})
		}
		_ = json.NewEncoder(w).Encode(out)
	case r.Method == "GET" && p == "/volumes":
		var out []map[string]any
		for _, id := range listed("vol-") {
			out = append(out, map[string]any{"Name": id})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Volumes": out})
	case r.Method == "POST" && p == "/volumes/create":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.labels["vol-"+body["Name"].(string)] = asMap(body["Labels"])
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(body)
	case r.Method == "GET" && len(parts) == 3 && parts[0] == "containers" && parts[2] == "json":
		l, ok := f.labels[parts[1]]
		if !ok {
			http.Error(w, `{"message":"no such container"}`, 404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": parts[1], "Config": map[string]any{"Labels": l}})
	case r.Method == "GET" && len(parts) == 2 && parts[0] == "volumes":
		l, ok := f.labels["vol-"+strings.TrimPrefix(parts[1], "vol-")]
		if !ok {
			http.Error(w, `{"message":"no such volume"}`, 404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Labels": l})
	case r.Method == "DELETE":
		f.deleted = append(f.deleted, p)
		w.WriteHeader(204)
	default:
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{}`)
	}
}

func (f *proxyEngine) reachedEngine(call string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.reached {
		if c == call {
			return true
		}
	}
	return false
}

// startProxyEngine returns a Docker backend talking to f, with a proxy
// sockets directory, and a started proxy for one turn.
func startProxyEngine(t *testing.T, f *proxyEngine) (*Docker, *TurnDocker) {
	t.Helper()
	f.labels, f.networks = map[string]map[string]any{}, map[string]bool{}
	dir := t.TempDir()
	sock := filepath.Join(dir, "engine.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: f}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	d := &Docker{Socket: sock, Stack: "test", MemoryMB: 512, PidsLimit: 256,
		ProxyDir: filepath.Join(dir, "proxy"), ProxyVolume: "lazyagents-docker-proxy"}
	if err := d.PrepareDockerProxy(); err != nil {
		t.Fatal(err)
	}
	td, err := d.StartDockerProxy(context.Background(), "run-1", "run-1-2")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(td.Stop)
	return d, td
}

// turnClient talks to the proxy the way a client inside the turn would, over
// the socket named in DOCKER_HOST.
func turnClient(d *Docker, td *TurnDocker) *http.Client {
	name := strings.TrimPrefix(td.Env["DOCKER_HOST"], "unix://"+proxyMountPath+"/")
	sock := filepath.Join(d.ProxyDir, name)
	return &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}
}

func call(t *testing.T, c *http.Client, method, path string, body any) (int, string) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, "http://docker/v1.47"+path, r)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(out)
}

func TestDockerProxyTurnEnv(t *testing.T) {
	d, td := startProxyEngine(t, &proxyEngine{})
	if td.Network != "lazyagents-dkr-run-1-2" {
		t.Fatalf("network = %q", td.Network)
	}
	if td.Env["TESTCONTAINERS_HOST_OVERRIDE"] != "172.30.0.1" || td.Env["TESTCONTAINERS_RYUK_DISABLED"] != "true" {
		t.Fatalf("env = %v", td.Env)
	}
	if !strings.HasPrefix(td.Env["DOCKER_HOST"], "unix://"+proxyMountPath+"/") {
		t.Fatalf("DOCKER_HOST = %q", td.Env["DOCKER_HOST"])
	}
	info, err := os.Stat(d.ProxyDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o711 {
		t.Fatalf("sockets directory mode = %v, want 0711 so turns can't list it", info.Mode().Perm())
	}
}

func TestDockerProxySandboxesContainers(t *testing.T) {
	f := &proxyEngine{}
	d, td := startProxyEngine(t, f)
	c := turnClient(d, td)
	code, out := call(t, c, "POST", "/containers/create", map[string]any{
		"Image": "postgres:17-alpine", "Labels": map[string]any{"org.testcontainers": "true"},
		"ExposedPorts": map[string]any{"5432/tcp": map[string]any{}},
		"HostConfig": map[string]any{
			"PortBindings":    map[string]any{"5432/tcp": []any{map[string]any{"HostIp": "0.0.0.0", "HostPort": ""}}},
			"PublishAllPorts": true, "Memory": float64(64 << 30), "LogConfig": map[string]any{"Type": "syslog"},
		},
	})
	if code != 201 {
		t.Fatalf("create: %d %s", code, out)
	}
	got := f.creates[0]
	host := asMap(got["HostConfig"])
	bindings := asList(asMap(host["PortBindings"])["5432/tcp"])
	if len(bindings) != 1 || asMap(bindings[0])["HostIp"] != "172.30.0.1" {
		t.Fatalf("ports must publish on the turn's gateway only: %v", host["PortBindings"])
	}
	if host["NetworkMode"] != td.Network {
		t.Fatalf("NetworkMode = %v", host["NetworkMode"])
	}
	if host["Memory"] != float64(512<<20) || host["PidsLimit"] != float64(256) {
		t.Fatalf("limits = %v / %v", host["Memory"], host["PidsLimit"])
	}
	if _, ok := host["LogConfig"]; ok {
		t.Fatal("LogConfig must not pass through")
	}
	if fmt.Sprint(host["SecurityOpt"]) != "[no-new-privileges]" {
		t.Fatalf("SecurityOpt = %v", host["SecurityOpt"])
	}
	labels := asMap(got["Labels"])
	if labels[proxyLabelKey] != "run-1-2" || labels[labelKey] != "run-1" || labels["org.testcontainers"] != "true" {
		t.Fatalf("labels = %v", labels)
	}
}

func TestDockerProxyRefusesEscapes(t *testing.T) {
	f := &proxyEngine{}
	d, td := startProxyEngine(t, f)
	c := turnClient(d, td)
	f.labels["vol-theirs"] = map[string]any{proxyLabelKey: "run-9-1"}
	for name, host := range map[string]map[string]any{
		"privileged":          {"Privileged": true},
		"added capability":    {"CapAdd": []any{"SYS_ADMIN"}},
		"host path bind":      {"Binds": []any{"/:/host"}},
		"docker socket bind":  {"Binds": []any{"/var/run/docker.sock:/var/run/docker.sock"}},
		"bind mount":          {"Mounts": []any{map[string]any{"Type": "bind", "Source": "/etc", "Target": "/x"}}},
		"another turn volume": {"Mounts": []any{map[string]any{"Type": "volume", "Source": "vol-theirs", "Target": "/x"}}},
		"run checkout volume": {"Binds": []any{"lazyagents-run-abc:/workspace"}},
		"host network":        {"NetworkMode": "host"},
		"stack network":       {"NetworkMode": "lazyagents_default"},
		"another container":   {"NetworkMode": "container:lazyagents-turn-x"},
		"host pid":            {"PidMode": "host"},
		"devices":             {"Devices": []any{map[string]any{"PathOnHost": "/dev/kmsg"}}},
		"volumes from":        {"VolumesFrom": []any{"lazyagents-turn-x"}},
		"unconfined seccomp":  {"SecurityOpt": []any{"seccomp=unconfined"}},
		"runtime":             {"Runtime": "runc-privileged"},
	} {
		t.Run(name, func(t *testing.T) {
			code, out := call(t, c, "POST", "/containers/create", map[string]any{"Image": "alpine", "HostConfig": host})
			if code != 403 || !strings.Contains(out, "vesper:") {
				t.Fatalf("want 403 from the proxy, got %d %s", code, out)
			}
		})
	}
	if len(f.creates) != 0 {
		t.Fatalf("refused creates reached the engine: %v", f.creates)
	}
	code, _ := call(t, c, "POST", "/volumes/create", map[string]any{"Name": "v", "DriverOpts": map[string]any{"o": "bind", "device": "/"}})
	if code != 403 {
		t.Fatalf("volume with driver options: %d", code)
	}
	code, _ = call(t, c, "POST", "/build", nil)
	if code != 403 {
		t.Fatalf("build: %d", code)
	}
	code, _ = call(t, c, "POST", "/containers/x/update", map[string]any{"Memory": 0})
	if code != 403 {
		t.Fatalf("update: %d", code)
	}
}

func TestDockerProxyScopesToTheTurn(t *testing.T) {
	f := &proxyEngine{}
	d, td := startProxyEngine(t, f)
	c := turnClient(d, td)
	f.labels["ctr-other"] = map[string]any{proxyLabelKey: "run-9-1"}
	f.labels["lazyagents-turn-x"] = map[string]any{labelKey: "run-9"}
	for _, id := range []string{"ctr-other", "lazyagents-turn-x"} {
		code, _ := call(t, c, "POST", "/containers/"+id+"/exec", map[string]any{"Cmd": []any{"sh"}})
		if code != 404 {
			t.Fatalf("exec into %s: %d, want 404", id, code)
		}
		code, _ = call(t, c, "DELETE", "/containers/"+id, nil)
		if code != 404 {
			t.Fatalf("delete %s: %d, want 404", id, code)
		}
	}
	f.labels["ctr-other-tc"] = map[string]any{proxyLabelKey: "run-9-1", "org.testcontainers": "true"}
	code, out := call(t, c, "POST", "/containers/create", map[string]any{"Image": "redis:7", "Labels": map[string]any{"org.testcontainers": "true"}})
	if code != 201 {
		t.Fatalf("create: %d %s", code, out)
	}
	var created struct{ Id string }
	_ = json.Unmarshal([]byte(out), &created)
	if code, out := call(t, c, "POST", "/containers/"+created.Id+"/exec", map[string]any{"Cmd": []any{"redis-cli", "ping"}, "Privileged": true}); code != 403 {
		t.Fatalf("privileged exec: %d %s", code, out)
	}
	if code, out := call(t, c, "POST", "/containers/"+created.Id+"/start", nil); code != 200 || !f.reachedEngine("POST /containers/"+created.Id+"/start") {
		t.Fatalf("start own container: %d %s", code, out)
	}
	_, out = call(t, c, "GET", "/containers/json?all=1&filters="+url.QueryEscape(`{"label":["org.testcontainers=true"]}`), nil)
	var list []struct{ Id string }
	_ = json.Unmarshal([]byte(out), &list)
	if len(list) != 1 || list[0].Id != created.Id {
		t.Fatalf("list shows only the turn's containers: %s", out)
	}
}

func TestDockerProxyStopRemovesWhatTheTurnMade(t *testing.T) {
	f := &proxyEngine{}
	d, td := startProxyEngine(t, f)
	c := turnClient(d, td)
	if code, out := call(t, c, "POST", "/containers/create", map[string]any{"Image": "redis:7"}); code != 201 {
		t.Fatalf("create: %d %s", code, out)
	}
	if code, out := call(t, c, "POST", "/volumes/create", map[string]any{"Name": "data"}); code != 201 {
		t.Fatalf("volume: %d %s", code, out)
	}
	f.labels["ctr-other"] = map[string]any{proxyLabelKey: "run-9-1"}
	td.Stop()
	td.Stop() // safe twice
	f.mu.Lock()
	deleted := strings.Join(f.deleted, " ")
	f.mu.Unlock()
	for _, want := range []string{"/containers/ctr-1", "/networks/net-" + td.Network, "/volumes/vol-data"} {
		if !strings.Contains(deleted, want) {
			t.Fatalf("Stop didn't remove %s: %s", want, deleted)
		}
	}
	if strings.Contains(deleted, "ctr-other") {
		t.Fatalf("Stop removed another turn's container: %s", deleted)
	}
	entries, _ := os.ReadDir(d.ProxyDir)
	if len(entries) != 0 {
		t.Fatalf("socket left behind: %v", entries)
	}
}

func TestDockerStartMountsProxyReadOnly(t *testing.T) {
	f, d := startFakeEngine(t)
	d.ProxyDir, d.ProxyVolume = t.TempDir(), "lazyagents-docker-proxy"
	if _, err := d.Start(context.Background(), "run-1", ExecSpec{Name: "t", Argv: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	for _, m := range asList(asMap(f.created["HostConfig"])["Mounts"]) {
		mount := asMap(m)
		if mount["Target"] == proxyMountPath {
			if mount["ReadOnly"] != true || mount["Source"] != "lazyagents-docker-proxy" {
				t.Fatalf("proxy mount = %v", mount)
			}
			return
		}
	}
	t.Fatal("turn doesn't mount the proxy sockets directory")
}

func TestEnsureNixDaemon(t *testing.T) {
	f := &proxyEngine{}
	d, _ := startProxyEngine(t, f)
	d.Image, d.NixVolume = "vesper/agent-base:latest", "lazyagents-nix"
	if err := d.EnsureNixDaemon(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.creates) != 1 {
		t.Fatalf("creates = %d", len(f.creates))
	}
	body := f.creates[0]
	host := asMap(body["HostConfig"])
	mount := asMap(asList(host["Mounts"])[0])
	if mount["Source"] != "lazyagents-nix" || mount["Target"] != "/nix" || fmt.Sprint(body["Cmd"]) != "["+nixDaemonBin+"]" {
		t.Fatalf("daemon = %v", body)
	}
	if _, ok := asMap(body["Labels"])[labelKey]; ok {
		t.Fatal("the daemon must not carry a run label, or startup cleanup removes it")
	}
	if host["NetworkMode"] != nil {
		t.Fatalf("the daemon must stay off the agents network: %v", host["NetworkMode"])
	}
	if !f.reachedEngine("POST /containers/lazyagents-nix-daemon-test/start") {
		t.Fatal("daemon not started")
	}
}

func TestDockerStartMountsNixStore(t *testing.T) {
	f, d := startFakeEngine(t)
	d.NixVolume = "lazyagents-nix"
	if _, err := d.Start(context.Background(), "run-1", ExecSpec{Name: "t", Argv: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	for _, m := range asList(asMap(f.created["HostConfig"])["Mounts"]) {
		if mount := asMap(m); mount["Target"] == "/nix" && mount["Source"] == "lazyagents-nix" {
			return
		}
	}
	t.Fatal("turn doesn't mount the shared Nix store")
}
