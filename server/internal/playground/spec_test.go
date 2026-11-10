package playground

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

func vesperSpec() Spec {
	return Spec{
		Services: []agentdef.Service{{Name: "postgres", Image: "postgres:17-alpine", Expose: map[string]string{"DATABASE_URL": "postgres://postgres@postgres/postgres"}}},
		Setup:    []string{"cd server && go run ./cmd/vesper migrate"},
		Processes: []Process{
			{Name: "api", Command: "go run ./cmd/vesper serve", Dir: "server", Port: 8000},
			{Name: "web", Command: "npm run dev -- -H 0.0.0.0", Dir: "web", Port: 3000, Env: map[string]string{"API_INTERNAL_URL": "http://api:8000"}},
		},
		Routes:  []Route{{Path: "/api/", Process: "api"}, {Path: "/events", Process: "api"}, {Path: "/", Process: "web"}},
		Env:     map[string]string{"DATABASE_URL": "postgres://override"},
		OpenAPI: "api/openapi.yaml",
	}
}

func TestRouteSendsEachPathToTheLongestMatchingProcess(t *testing.T) {
	s := vesperSpec()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"/":                  "web",
		"/chat/123":          "web",
		"/api":               "api",
		"/api/v1/runs":       "api",
		"/apis":              "web", // a prefix of the name, not a directory above it
		"/events":            "api",
		"/events/stream":     "api",
		"/eventsource-shim":  "web",
		"/_next/static/a.js": "web",
	} {
		got, ok := s.Route(path)
		if !ok || got.Name != want {
			t.Errorf("Route(%q) = %q, want %q", path, got.Name, want)
		}
	}
}

func TestValidateDefaultsToTheFirstProcess(t *testing.T) {
	s := Spec{Processes: []Process{{Name: "app", Command: "npm start", Port: 3000}}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if p, ok := s.Route("/anything"); !ok || p.Name != "app" {
		t.Fatalf("Route = %q, want app", p.Name)
	}
}

func TestValidateRefusesWhatCouldEscapeOrCollide(t *testing.T) {
	for name, mutate := range map[string]func(*Spec){
		"no processes":                func(s *Spec) { s.Processes = nil },
		"dir outside the workspace":   func(s *Spec) { s.Processes[0].Dir = "../etc" },
		"absolute dir":                func(s *Spec) { s.Processes[0].Dir = "/etc" },
		"route to a portless process": func(s *Spec) { s.Processes[0].Port = 0 },
		"process named a service":     func(s *Spec) { s.Processes[0].Name = "postgres" },
		"reserved name":               func(s *Spec) { s.Processes[0].Name = "setup" },
		"route to the gateway":        func(s *Spec) { s.Routes = append(s.Routes, Route{Path: "/__vesper/logs", Process: "api"}) },
		"route to a service":          func(s *Spec) { s.Routes[0].Process = "postgres" },
		"dirty route":                 func(s *Spec) { s.Routes[0].Path = "/api/../admin" },
		"runner env":                  func(s *Spec) { s.Env["LAZYAGENTS_RUN_ID"] = "x" },
		"openapi outside":             func(s *Spec) { s.OpenAPI = "../secrets.json" },
		"openapi not a spec":          func(s *Spec) { s.OpenAPI = "README.md" },
		"empty setup":                 func(s *Spec) { s.Setup = []string{" "} },
		"long command":                func(s *Spec) { s.Processes[0].Command = strings.Repeat("x", 2001) },
	} {
		s := vesperSpec()
		mutate(&s)
		var inv *InvalidError
		if err := s.Validate(); !errors.As(err, &inv) {
			t.Errorf("%s: Validate = %v, want an InvalidError", name, err)
		}
	}
}

func TestProcessEnvLayersSpecOverServicesAndProcessOverSpec(t *testing.T) {
	s := vesperSpec()
	web := s.Processes[1]
	env := s.ProcessEnv(&web)
	if env["DATABASE_URL"] != "postgres://override" || env["API_INTERNAL_URL"] != "http://api:8000" {
		t.Fatalf("env = %v", env)
	}
}

func TestBackgroundProcessesServeNothing(t *testing.T) {
	s := Spec{Processes: []Process{{Name: "runner", Command: "vesper runner"}, {Name: "web", Command: "npm start", Port: 3000}}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	// The catch-all goes to the first process that serves, not the first one.
	if p, ok := s.Route("/"); !ok || p.Name != "web" {
		t.Fatalf("Route(/) = %q", p.Name)
	}
	for name, spec := range map[string]Spec{
		"route to a worker": {Processes: []Process{{Name: "runner", Command: "x"}, {Name: "web", Command: "y", Port: 1}}, Routes: []Route{{Path: "/jobs", Process: "runner"}}},
		"nothing serves":    {Processes: []Process{{Name: "runner", Command: "x"}}},
		"negative port":     {Processes: []Process{{Name: "web", Command: "y", Port: -1}}},
	} {
		var inv *InvalidError
		if err := spec.Validate(); !errors.As(err, &inv) {
			t.Errorf("%s: Validate = %v, want an InvalidError", name, err)
		}
	}
}

func TestValidateChecksNixPackagesAndTheInferredCommit(t *testing.T) {
	ok := Spec{Processes: []Process{{Name: "web", Command: "y", Port: 1}}, NixPackages: []string{"nixpkgs#protobuf"},
		OpenAPI: ".lazyagents/playground/openapi.json", OpenAPIInferred: true, OpenAPICommit: "9f17116"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Spec){
		"shell in a package":    func(s *Spec) { s.NixPackages = []string{"nixpkgs#jq; rm -rf /"} },
		"commit isn't a hash":   func(s *Spec) { s.OpenAPICommit = "HEAD" },
		"inferred without path": func(s *Spec) { s.OpenAPI = "" },
	} {
		s := ok
		s.NixPackages = slices.Clone(ok.NixPackages)
		mutate(&s)
		var inv *InvalidError
		if err := s.Validate(); !errors.As(err, &inv) {
			t.Errorf("%s: Validate = %v, want an InvalidError", name, err)
		}
	}
}
