package playground

import (
	"errors"
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
		"no processes":              func(s *Spec) { s.Processes = nil },
		"dir outside the workspace": func(s *Spec) { s.Processes[0].Dir = "../etc" },
		"absolute dir":              func(s *Spec) { s.Processes[0].Dir = "/etc" },
		"no port":                   func(s *Spec) { s.Processes[0].Port = 0 },
		"process named a service":   func(s *Spec) { s.Processes[0].Name = "postgres" },
		"reserved name":             func(s *Spec) { s.Processes[0].Name = "setup" },
		"route to the gateway":      func(s *Spec) { s.Routes = append(s.Routes, Route{Path: "/__vesper/logs", Process: "api"}) },
		"route to a service":        func(s *Spec) { s.Routes[0].Process = "postgres" },
		"dirty route":               func(s *Spec) { s.Routes[0].Path = "/api/../admin" },
		"runner env":                func(s *Spec) { s.Env["LAZYAGENTS_RUN_ID"] = "x" },
		"openapi outside":           func(s *Spec) { s.OpenAPI = "../secrets.json" },
		"openapi not a spec":        func(s *Spec) { s.OpenAPI = "README.md" },
		"empty setup":               func(s *Spec) { s.Setup = []string{" "} },
		"long command":              func(s *Spec) { s.Processes[0].Command = strings.Repeat("x", 2001) },
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
