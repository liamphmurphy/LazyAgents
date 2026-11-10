// Package playground is a chat's running app, for the person to click
// through: the processes an agent declares, run from the chat's workspace
// with fresh services next to them, and served to the browser on a port of
// the runner's own by the gateway in this package. The runner starts the
// containers (runner.Docker); this package holds what to run and how a
// request reaches it.
package playground

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/testenv"
)

// Spec is what a playground runs, as the agent declares it.
type Spec struct {
	// Services start empty next to the processes, reachable by name. When the
	// agent declares none, the repository's test environment's are used.
	Services []agentdef.Service `json:"services,omitempty"`
	// Setup runs once services are up and before the processes, in order,
	// from the workspace root: migrations, seed data, dependency installs.
	Setup []string `json:"setup,omitempty"`
	// Processes are the app's long-running parts, one container each.
	Processes []Process `json:"processes"`
	// Routes send request paths to processes, longest prefix first. Without
	// any, everything goes to the first process.
	Routes []Route `json:"routes,omitempty"`
	// Env is set in setup and every process, after what services expose.
	Env map[string]string `json:"env,omitempty"`
	// OpenAPI is a workspace-relative path to the app's OpenAPI document,
	// which the API tab reads when the running app serves none.
	OpenAPI string `json:"openapi,omitempty"`
	// OpenAPIInferred marks a document the persona wrote from the code rather
	// than one the repository keeps; OpenAPICommit is the commit it read.
	OpenAPIInferred bool   `json:"openapi_inferred,omitempty"`
	OpenAPICommit   string `json:"openapi_commit,omitempty"`
	// NixPackages are installed for setup and the processes, like a
	// repository's test environment's (installables such as
	// "nixpkgs#protobuf"). Left out, the test environment's are used.
	NixPackages []string `json:"nix_packages,omitempty"`
}

// Process is one long-running part of the app.
type Process struct {
	Name string `json:"name"`
	// Command runs under bash -c. With a Port it must listen on 0.0.0.0:Port.
	Command string `json:"command"`
	// Dir is where Command runs, relative to the workspace root; it's
	// created if it doesn't exist.
	Dir string `json:"dir,omitempty"`
	// Port is where it serves HTTP. 0 is a background process (a worker, a
	// job runner): nothing routes to it, and it only has to keep running.
	Port int               `json:"port,omitempty"`
	Env  map[string]string `json:"env,omitempty"`
}

// Background reports a process that serves nothing.
func (p Process) Background() bool { return p.Port == 0 }

// Route sends requests under Path to a process.
type Route struct {
	Path    string `json:"path"`
	Process string `json:"process"`
}

const (
	MaxProcesses = 4
	MaxSetup     = 10
	MaxRoutes    = 10
	maxCommand   = 2000
	maxEnvValue  = 4096
	// ReservedPrefix is where the gateway serves the playground's own pages
	// (its OpenAPI document and logs); the app can't route it.
	ReservedPrefix = "/__vesper"
)

var (
	nameRe    = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}$`)
	envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	commitRe  = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
	// Names other containers on the playground network already answer to.
	reservedNames = []string{"setup", "tools", "gateway", "playground", "localhost"}
)

// InvalidError is a spec that fails Validate.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return "playground: " + e.Msg }

func invalid(format string, args ...any) error {
	return &InvalidError{fmt.Sprintf(format, args...)}
}

// Validate checks a spec and fills its defaults (the catch-all route).
func (s *Spec) Validate() error {
	if err := agentdef.ValidateServices(s.Services); err != nil {
		return &InvalidError{err.Error()}
	}
	names := map[string]string{}
	for _, svc := range s.Services {
		names[svc.Name] = "service"
	}
	if len(s.Processes) == 0 || len(s.Processes) > MaxProcesses {
		return invalid("declare 1 to %d processes", MaxProcesses)
	}
	for i := range s.Processes {
		p := &s.Processes[i]
		p.Name, p.Command, p.Dir = strings.TrimSpace(p.Name), strings.TrimSpace(p.Command), strings.TrimSpace(p.Dir)
		if !nameRe.MatchString(p.Name) || slices.Contains(reservedNames, p.Name) {
			return invalid("process name %q must be lowercase letters, digits, - or _, and not %s", p.Name, strings.Join(reservedNames, ", "))
		}
		if kind, ok := names[p.Name]; ok {
			return invalid("process %q has the same name as a %s", p.Name, kind)
		}
		names[p.Name] = "process"
		if p.Command == "" || len(p.Command) > maxCommand {
			return invalid("process %q needs a command of at most %d characters", p.Name, maxCommand)
		}
		if p.Port < 0 || p.Port > 65535 {
			return invalid("process %q: port %d isn't a port", p.Name, p.Port)
		}
		if !p.Background() {
			names[p.Name] = "served"
		}
		if p.Dir != "" && !relativePath(p.Dir) {
			return invalid("process %q: dir %q must be a path inside the workspace", p.Name, p.Dir)
		}
		if err := validEnv("process "+p.Name, p.Env); err != nil {
			return err
		}
	}
	if len(s.Setup) > MaxSetup {
		return invalid("at most %d setup commands", MaxSetup)
	}
	for i, c := range s.Setup {
		if c = strings.TrimSpace(c); c == "" || len(c) > maxCommand {
			return invalid("setup command %d must be 1 to %d characters", i+1, maxCommand)
		}
		s.Setup[i] = c
	}
	if err := validEnv("env", s.Env); err != nil {
		return err
	}
	if len(s.Routes) > MaxRoutes {
		return invalid("at most %d routes", MaxRoutes)
	}
	seen := map[string]bool{}
	for i := range s.Routes {
		r := &s.Routes[i]
		raw := strings.TrimSpace(r.Path)
		if r.Path = raw; len(raw) > 1 {
			r.Path = strings.TrimSuffix(raw, "/") // "/api/" routes like "/api"
		}
		if !strings.HasPrefix(r.Path, "/") || path.Clean(r.Path) != r.Path {
			return invalid("route %q must be a clean path starting with /", raw)
		}
		if under(r.Path, ReservedPrefix) {
			return invalid("route %q: %s is the playground's own", r.Path, ReservedPrefix)
		}
		switch names[r.Process] {
		case "served":
		case "process":
			return invalid("route %q goes to %q, which has no port", r.Path, r.Process)
		default:
			return invalid("route %q goes to %q, which isn't a process", r.Path, r.Process)
		}
		if seen[r.Path] {
			return invalid("two routes for %q", r.Path)
		}
		seen[r.Path] = true
	}
	served := slices.IndexFunc(s.Processes, func(p Process) bool { return !p.Background() })
	if served < 0 {
		return invalid("at least one process needs a port to serve")
	}
	if !seen["/"] {
		s.Routes = append(s.Routes, Route{Path: "/", Process: s.Processes[served].Name})
	}
	if s.OpenAPI = strings.TrimSpace(s.OpenAPI); s.OpenAPI != "" {
		ext := path.Ext(s.OpenAPI)
		if !relativePath(s.OpenAPI) || (ext != ".yaml" && ext != ".yml" && ext != ".json") {
			return invalid("openapi %q must be a .yaml, .yml or .json file inside the workspace", s.OpenAPI)
		}
	}
	if s.OpenAPICommit = strings.TrimSpace(s.OpenAPICommit); s.OpenAPICommit != "" && !commitRe.MatchString(s.OpenAPICommit) {
		return invalid("openapi_commit %q must be a commit hash", s.OpenAPICommit)
	}
	if (s.OpenAPIInferred || s.OpenAPICommit != "") && s.OpenAPI == "" {
		return invalid("an inferred document needs its openapi path")
	}
	if err := (&testenv.Recipe{NixPackages: s.NixPackages}).Validate(); err != nil {
		return &InvalidError{strings.TrimPrefix(err.Error(), "test environment: ")}
	}
	return nil
}

func validEnv(field string, env map[string]string) error {
	for k, v := range env {
		if !envNameRe.MatchString(k) {
			return invalid("%s: %q isn't an environment variable name", field, k)
		}
		if strings.HasPrefix(strings.ToUpper(k), "LAZYAGENTS_") {
			return invalid("%s: %s is reserved for the runner", field, k)
		}
		if len(v) > maxEnvValue {
			return invalid("%s: %s is longer than %d characters", field, k, maxEnvValue)
		}
	}
	return nil
}

// relativePath reports a clean path that stays inside the workspace.
func relativePath(p string) bool {
	return p != "" && !strings.HasPrefix(p, "/") && path.Clean(p) == p && p != "." && p != ".." && !strings.HasPrefix(p, "../")
}

// under reports whether p is prefix or a path below it.
func under(p, prefix string) bool {
	if prefix == "/" {
		return true
	}
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}

// Route returns the process serving a request path: the longest route that
// is the path or a directory above it. Validate guarantees one for "/".
func (s *Spec) Route(p string) (Process, bool) {
	best := -1
	for i, r := range s.Routes {
		if under(p, r.Path) && (best < 0 || len(r.Path) > len(s.Routes[best].Path)) {
			best = i
		}
	}
	if best < 0 {
		return Process{}, false
	}
	for _, proc := range s.Processes {
		if proc.Name == s.Routes[best].Process {
			return proc, true
		}
	}
	return Process{}, false
}

// ProcessEnv is what a setup command or process sees: what services expose,
// then the spec's env, then the process's own.
func (s *Spec) ProcessEnv(p *Process) map[string]string {
	env := map[string]string{}
	for _, svc := range s.Services {
		for k, v := range svc.Expose {
			env[k] = v
		}
	}
	for k, v := range s.Env {
		env[k] = v
	}
	if p != nil {
		for k, v := range p.Env {
			env[k] = v
		}
	}
	return env
}
