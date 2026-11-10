package testenv

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

// CandidateGlobs are the files Infer reads, relative to the checkout root.
var CandidateGlobs = []string{
	".github/workflows/*.yml", ".github/workflows/*.yaml",
	"docker-compose*.yml", "docker-compose*.yaml", "compose*.yml", "compose*.yaml",
}

// Infer reads what a repository already declares for its tests. GitHub
// Actions service containers are the best evidence, since CI runs the same
// tests: their images, environment and health checks become services, and job
// variables that reach them through localhost (TEST_DATABASE_URL=...@localhost:5432)
// are rewritten to the service's name. Without CI services, compose files
// contribute well-known backing services only (databases, queues, caches,
// object stores), not the application's own containers. files maps a path
// to its content.
func Infer(files map[string][]byte) Recipe {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var services []agentdef.Service
	expose := map[string]map[string]string{} // service → variables
	for _, p := range paths {
		if strings.HasPrefix(p, ".github/workflows/") {
			services, expose = fromWorkflow(files[p], services, expose)
		}
	}
	if len(services) == 0 {
		for _, p := range paths {
			if !strings.HasPrefix(p, ".github/") {
				services = fromCompose(files[p], services)
			}
		}
	}
	if len(services) > agentdef.MaxServices {
		services = services[:agentdef.MaxServices]
	}
	for i := range services {
		if vars := expose[services[i].Name]; len(vars) > 0 {
			services[i].Expose = vars
		}
	}
	r := Recipe{Services: services}
	if r.Validate() != nil {
		return Recipe{} // never hand the runner something it would refuse
	}
	return r
}

var serviceNameRe = regexp.MustCompile(`[^a-z0-9_-]+`)

func serviceName(key string) string {
	return strings.Trim(serviceNameRe.ReplaceAllString(strings.ToLower(key), "-"), "-")
}

// literal reports whether a workflow value is usable outside GitHub: no
// expressions, which only Actions can evaluate.
func literal(v string) bool { return v != "" && !strings.Contains(v, "${{") }

type workflow struct {
	Env  map[string]any `yaml:"env"`
	Jobs map[string]struct {
		Env      map[string]any `yaml:"env"`
		Services map[string]struct {
			Image   string         `yaml:"image"`
			Env     map[string]any `yaml:"env"`
			Ports   []any          `yaml:"ports"`
			Options string         `yaml:"options"`
		} `yaml:"services"`
		Steps []struct {
			Env map[string]any `yaml:"env"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func fromWorkflow(b []byte, services []agentdef.Service, expose map[string]map[string]string) ([]agentdef.Service, map[string]map[string]string) {
	var wf workflow
	if yaml.Unmarshal(b, &wf) != nil {
		return services, expose
	}
	jobs := make([]string, 0, len(wf.Jobs))
	for name := range wf.Jobs {
		jobs = append(jobs, name)
	}
	sort.Strings(jobs)
	for _, jobName := range jobs {
		job := wf.Jobs[jobName]
		if len(job.Services) == 0 {
			continue
		}
		// host port → service name and container port, for rewriting URLs.
		ports := map[string][2]string{}
		names := make([]string, 0, len(job.Services))
		for name := range job.Services {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, key := range names {
			svc := job.Services[key]
			name := serviceName(key)
			if name == "" || !literal(svc.Image) {
				continue
			}
			// Jobs often repeat one service (unit and e2e both want Postgres):
			// the same image is the same service, and both jobs' variables
			// reach it. A different image under a taken name is skipped.
			if existing := findService(services, name); existing != nil {
				if existing.Image != svc.Image {
					continue
				}
			} else {
				s := agentdef.Service{Name: name, Image: svc.Image, Env: stringMap(svc.Env)}
				if hc := healthCmd(svc.Options); hc != "" {
					s.Healthcheck = []string{"CMD-SHELL", hc}
				}
				services = append(services, s)
			}
			for _, p := range svc.Ports {
				host, container := portPair(fmt.Sprint(p))
				if host != "" {
					ports[host] = [2]string{name, container}
				}
			}
		}
		vars := map[string]string{}
		for k, v := range stringMap(wf.Env) {
			vars[k] = v
		}
		for k, v := range stringMap(job.Env) {
			vars[k] = v
		}
		for _, step := range job.Steps {
			for k, v := range stringMap(step.Env) {
				if _, ok := vars[k]; !ok {
					vars[k] = v
				}
			}
		}
		for k, v := range vars {
			if svc, rewritten := throughService(v, ports); svc != "" {
				if expose[svc] == nil {
					expose[svc] = map[string]string{}
				}
				expose[svc][k] = rewritten
			}
		}
	}
	return services, expose
}

func findService(services []agentdef.Service, name string) *agentdef.Service {
	for i := range services {
		if services[i].Name == name {
			return &services[i]
		}
	}
	return nil
}

func hasService(services []agentdef.Service, name string) bool {
	return findService(services, name) != nil
}

// stringMap keeps the literal scalar values of a YAML map.
func stringMap(m map[string]any) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		switch v.(type) {
		case string, int, float64, bool:
			if s := fmt.Sprint(v); literal(s) {
				out[k] = s
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// portPair splits "5432:5432", "127.0.0.1:6379:6379/tcp" or "5432" into the
// host and container ports.
func portPair(p string) (host, container string) {
	p, _, _ = strings.Cut(p, "/")
	parts := strings.Split(p, ":")
	switch len(parts) {
	case 1:
		return parts[0], parts[0]
	case 2:
		return parts[0], parts[1]
	case 3:
		return parts[1], parts[2]
	}
	return "", ""
}

var localURLRe = regexp.MustCompile(`(localhost|127\.0\.0\.1):([0-9]+)`)

// throughService rewrites a value that reaches a service through a published
// localhost port to reach it by name, as a turn does.
func throughService(v string, ports map[string][2]string) (service, rewritten string) {
	m := localURLRe.FindStringSubmatch(v)
	if m == nil {
		return "", ""
	}
	target, ok := ports[m[2]]
	if !ok {
		return "", ""
	}
	return target[0], strings.Replace(v, m[0], net.JoinHostPort(target[0], target[1]), 1)
}

// healthCmd pulls --health-cmd out of a service's docker create options.
func healthCmd(options string) string {
	args := shellFields(options)
	for i, a := range args {
		if a == "--health-cmd" && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, "--health-cmd="); ok {
			return v
		}
	}
	return ""
}

// shellFields splits on spaces, honoring single and double quotes.
func shellFields(s string) []string {
	var out []string
	var cur strings.Builder
	in, quote := false, rune(0)
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '"' || r == '\''):
			quote, in = r, true
		case quote == 0 && (r == ' ' || r == '\n' || r == '\t'):
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if in {
		out = append(out, cur.String())
	}
	return out
}

// backingImages are the images compose inference trusts to be a test
// dependency rather than part of the application.
var backingImages = []string{
	"postgres", "postgis/postgis", "mysql", "mariadb", "redis", "valkey/valkey", "mongo", "rabbitmq",
	"nats", "memcached", "elasticsearch", "docker.elastic.co/elasticsearch/elasticsearch",
	"opensearchproject/opensearch", "minio/minio", "localstack/localstack", "redpandadata/redpanda",
	"bitnami/kafka", "apache/kafka", "clickhouse/clickhouse-server", "cassandra", "mcr.microsoft.com/mssql/server",
}

func fromCompose(b []byte, services []agentdef.Service) []agentdef.Service {
	var file struct {
		Services map[string]struct {
			Image       string    `yaml:"image"`
			Build       any       `yaml:"build"`
			Environment yaml.Node `yaml:"environment"`
			Healthcheck struct {
				Test any `yaml:"test"`
			} `yaml:"healthcheck"`
		} `yaml:"services"`
	}
	if yaml.Unmarshal(b, &file) != nil {
		return services
	}
	names := make([]string, 0, len(file.Services))
	for n := range file.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, key := range names {
		svc := file.Services[key]
		name := serviceName(key)
		repo, _, _ := strings.Cut(svc.Image, ":")
		repo = strings.TrimPrefix(repo, "docker.io/library/")
		if svc.Build != nil || name == "" || hasService(services, name) || !isBacking(repo) || strings.Contains(svc.Image, "${") {
			continue
		}
		s := agentdef.Service{Name: name, Image: svc.Image, Env: composeEnv(svc.Environment)}
		switch t := svc.Healthcheck.Test.(type) {
		case string:
			s.Healthcheck = []string{"CMD-SHELL", t}
		case []any:
			for _, v := range t {
				s.Healthcheck = append(s.Healthcheck, fmt.Sprint(v))
			}
		}
		services = append(services, s)
	}
	return services
}

func isBacking(repo string) bool {
	for _, b := range backingImages {
		if repo == b {
			return true
		}
	}
	return false
}

// composeEnv reads environment in its map or KEY=value list form, keeping
// literal values only (no ${VAR} interpolation).
func composeEnv(n yaml.Node) map[string]string {
	out := map[string]string{}
	switch n.Kind {
	case yaml.MappingNode:
		var m map[string]any
		_ = n.Decode(&m)
		return stringMap(m)
	case yaml.SequenceNode:
		var list []string
		_ = n.Decode(&list)
		for _, kv := range list {
			if k, v, ok := strings.Cut(kv, "="); ok && !strings.Contains(v, "${") {
				out[k] = v
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
