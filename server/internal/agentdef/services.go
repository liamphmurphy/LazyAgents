package agentdef

import (
	"path"
	"regexp"
	"strings"
)

// Service is a container a run's turns can reach by Name, such as the
// database a repository's tests need. The runner starts it next to each turn
// on a network of that turn's own, sandboxed like the turn (no-new-privileges,
// resource limits, no host mounts or published ports) but with Docker's
// default capabilities, and removes it when the turn ends: nothing persists
// between turns.
//
// Services are declared on a project by a person and frozen into the run's
// resolved definition at dispatch, so an agent can't change what it gets.
type Service struct {
	Name  string `json:"name"`  // the hostname turns use; lowercase letters, digits, - or _
	Image string `json:"image"` // pulled if missing
	// Command replaces the image's default command.
	Command []string `json:"command,omitempty"`
	// User runs the container as this user; empty keeps the image's.
	User string            `json:"user,omitempty"`
	Env  map[string]string `json:"env,omitempty"`
	// Tmpfs are writable scratch directories (data directories, sockets).
	Tmpfs []string `json:"tmpfs,omitempty"`
	// Healthcheck is a Docker healthcheck test, e.g. ["CMD", "pg_isready"].
	// Turns start once it passes; without one, once the container runs.
	Healthcheck []string `json:"healthcheck,omitempty"`
	// Expose sets environment variables in each turn, typically how to reach
	// the service: {"TEST_DATABASE_URL": "postgres://...@postgres:5432/..."}.
	Expose map[string]string `json:"expose,omitempty"`
}

// MaxServices bounds how many services a run starts next to each turn.
const MaxServices = 5

var (
	envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	imageRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]*$`)
)

// reservedEnvPrefixes are variables the runner sets for every turn; a service
// can't replace the turn's token or run identity.
var reservedEnvPrefixes = []string{"LAZYAGENTS_", "ANTHROPIC_", "OPENAI_", "CLAUDE_", "CODEX_", "GEMINI_", "AGY_", "GH_", "GITHUB_", "GIT_"}

func validServices(field string, services []Service) error {
	if len(services) > MaxServices {
		return invalid(field, "at most %d services", MaxServices)
	}
	seen := map[string]bool{}
	exposed := map[string]string{}
	for i := range services {
		s := &services[i]
		s.Name, s.Image, s.User = strings.TrimSpace(s.Name), strings.TrimSpace(s.Image), strings.TrimSpace(s.User)
		if !nameRe.MatchString(s.Name) {
			return invalid(field, "service name %q must be lowercase letters, digits, - or _", s.Name)
		}
		if seen[s.Name] {
			return invalid(field, "duplicate service %q", s.Name)
		}
		seen[s.Name] = true
		if !imageRe.MatchString(s.Image) {
			return invalid(field, "service %q needs an image like postgres:17-alpine", s.Name)
		}
		for k := range s.Env {
			if !envNameRe.MatchString(k) {
				return invalid(field, "service %q: %q isn't an environment variable name", s.Name, k)
			}
		}
		for _, p := range s.Tmpfs {
			if !strings.HasPrefix(p, "/") || path.Clean(p) != p || p == "/" {
				return invalid(field, "service %q: tmpfs %q must be an absolute directory", s.Name, p)
			}
		}
		for k := range s.Expose {
			if !envNameRe.MatchString(k) {
				return invalid(field, "service %q: %q isn't an environment variable name", s.Name, k)
			}
			for _, p := range reservedEnvPrefixes {
				if strings.HasPrefix(strings.ToUpper(k), p) {
					return invalid(field, "service %q: %s is reserved for the runner", s.Name, k)
				}
			}
			if other, ok := exposed[k]; ok {
				return invalid(field, "services %q and %q both set %s", other, s.Name, k)
			}
			exposed[k] = s.Name
		}
	}
	return nil
}

// ValidateServices checks services declared outside a project override, such
// as a repository's learned test environment, by the same rules.
func ValidateServices(services []Service) error { return validServices("services", services) }
