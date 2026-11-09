package agentdef

import (
	"strings"
	"testing"
)

func pg() Service {
	return Service{
		Name: "postgres", Image: "postgres:17-alpine", User: "postgres",
		Env:         map[string]string{"POSTGRES_PASSWORD": "pw", "PGDATA": "/var/lib/postgresql/data/pgdata"},
		Tmpfs:       []string{"/var/lib/postgresql/data", "/var/run/postgresql"},
		Healthcheck: []string{"CMD", "pg_isready", "-U", "postgres"},
		Expose:      map[string]string{"TEST_DATABASE_URL": "postgres://postgres:pw@postgres:5432/postgres?sslmode=disable"},
	}
}

func TestServicesValidate(t *testing.T) {
	ok := Override{Services: []Service{pg()}}
	if err := ok.Normalize(); err != nil {
		t.Fatalf("valid service refused: %v", err)
	}
	for name, mutate := range map[string]func(*Service){
		"bad name":        func(s *Service) { s.Name = "Postgres DB" },
		"no image":        func(s *Service) { s.Image = "" },
		"image flags":     func(s *Service) { s.Image = "--privileged postgres" },
		"relative tmpfs":  func(s *Service) { s.Tmpfs = []string{"data"} },
		"root tmpfs":      func(s *Service) { s.Tmpfs = []string{"/"} },
		"token override":  func(s *Service) { s.Expose = map[string]string{"LAZYAGENTS_TOKEN": "x"} },
		"git credentials": func(s *Service) { s.Expose = map[string]string{"gh_token": "x"} },
		"bad env name":    func(s *Service) { s.Env = map[string]string{"A-B": "x"} },
	} {
		s := pg()
		mutate(&s)
		o := Override{Services: []Service{s}}
		if err := o.Normalize(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	dup := Override{Services: []Service{pg(), pg()}}
	if err := dup.Normalize(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("duplicate services: %v", err)
	}
	second := pg()
	second.Name = "pg2"
	clash := Override{Services: []Service{pg(), second}}
	if err := clash.Normalize(); err == nil || !strings.Contains(err.Error(), "both set") {
		t.Errorf("two services exposing one variable: %v", err)
	}
}

func TestServicesResolveFromProject(t *testing.T) {
	s := Spec{Harness: HarnessClaudeCode, Model: "m"}
	r := Resolve(s, &Override{Services: []Service{pg()}})
	if len(r.Services) != 1 || r.Services[0].Name != "postgres" || r.Provenance["services"] != FromProject {
		t.Fatalf("resolved services %+v provenance %v", r.Services, r.Provenance)
	}
	if !strings.Contains(strings.Join(Override{Services: []Service{pg()}}.Changes(), "\n"), "+ service postgres (postgres:17-alpine)") {
		t.Fatal("override summary doesn't mention the service")
	}
}
