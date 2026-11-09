// Package config loads process configuration from the environment.
package config

import (
	"encoding/json"
	"fmt"
	"github.com/liamphmurphy/vesper/server/internal/harness"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr        string // HTTP listen address, e.g. ":8000"
	DatabaseURL string
	S3          S3
	Seed        bool
	Version     string

	// The local user created when SEED=false (first boot only). There is no
	// login for now; every request acts as this user.
	LocalUserName  string
	LocalUserEmail string

	// SecretsKey encrypts stored secrets (base64, 32 bytes). Empty = secrets disabled.
	SecretsKey string
	// PublicURL is how agents and browsers reach the app (Caddy in compose).
	PublicURL string

	Runner Runner
	// PRSyncInterval is how often open PRs are polled (0 disables).
	PRSyncInterval int // seconds
	// MemoryConsolidateInterval is how often personal memories are
	// consolidated (duplicates merged; 0 disables).
	MemoryConsolidateInterval int // seconds
}

// Runner configures `vesper runner` (docs/dev/plan.md §6.5).
type Runner struct {
	ModelPrices   map[string]harness.Price
	Backend       string // docker | local
	MaxConcurrent int
	MCPURL        string // how turns reach the API's /mcp
	Image         string // docker
	Network       string // docker
	DockerSocket  string // docker
	MemoryMB      int64  // docker: per-container memory limit, 0 = none
	PidsLimit     int64  // docker: per-container process limit, 0 = none
	DataDir       string // local: run workspaces
	ClaudeBin     string // local: path to `claude`
	CodexBin      string // local: path to `codex`
	// ChatIdleTTL is how long an idle Personal Assistant chat keeps its run
	// and workspace (RUNNER_CHAT_IDLE_TTL_S).
	ChatIdleTTL time.Duration
	// MaxHold and MaxHoldAttempts bound how long a run waits at a provider
	// limit (RUNNER_MAX_HOLD_S, RUNNER_MAX_HOLD_ATTEMPTS).
	MaxHold         time.Duration
	MaxHoldAttempts int
}

type S3 struct {
	Endpoint     string // e.g. http://seaweedfs:8333
	Region       string
	AccessKey    string
	SecretKey    string
	Buckets      []string
	UsePathStyle bool
	// ArtifactBucket holds uploaded artifact files (one of Buckets).
	ArtifactBucket string
}

// Load reads configuration from environment variables, applying local-dev defaults.
func Load() (Config, error) {
	c := Config{
		Addr:        env("API_ADDR", ":8000"),
		DatabaseURL: env("DATABASE_URL", "postgres://lazyagents:lazyagents@localhost:5432/lazyagents?sslmode=disable"),
		S3: S3{
			Endpoint:       env("S3_ENDPOINT", "http://localhost:8333"),
			Region:         env("S3_REGION", "us-east-1"),
			AccessKey:      env("S3_ACCESS_KEY", "lazyagents"),
			SecretKey:      env("S3_SECRET_KEY", "lazyagents-secret"),
			Buckets:        splitList(env("S3_BUCKETS", "artifacts,run-logs")),
			UsePathStyle:   true,
			ArtifactBucket: env("S3_ARTIFACT_BUCKET", "artifacts"),
		},
		Version:        env("LAZYAGENTS_VERSION", "dev"),
		LocalUserName:  env("LOCAL_USER_NAME", "Local user"),
		LocalUserEmail: env("LOCAL_USER_EMAIL", "me@localhost"),
		SecretsKey:     env("SECRETS_KEY", ""),
		PublicURL:      env("PUBLIC_URL", "http://localhost:8080"),
	}
	var err error
	if c.Seed, err = boolEnv("SEED", false); err != nil {
		return c, err
	}
	r := Runner{
		Backend:      env("RUNNER_BACKEND", "docker"),
		Image:        env("RUNNER_IMAGE", "vesper/agent-base:latest"),
		Network:      env("RUNNER_NETWORK", "lazyagents_default"),
		DockerSocket: env("DOCKER_SOCKET", "/var/run/docker.sock"),
		DataDir:      env("RUNNER_DATA_DIR", ".lazyagents/runs"),
		ClaudeBin:    env("RUNNER_CLAUDE_BIN", ""),
		CodexBin:     env("RUNNER_CODEX_BIN", ""),
	}
	if r.Backend != "docker" && r.Backend != "local" {
		return c, fmt.Errorf("config: RUNNER_BACKEND=%q must be docker or local", r.Backend)
	}
	defMCP := "http://api:8000/mcp"
	if r.Backend == "local" {
		defMCP = "http://localhost:8000/mcp"
	}
	r.MCPURL = env("RUNNER_MCP_URL", defMCP)
	memory, err := intEnv("RUNNER_MEMORY_MB", 8192)
	if err != nil {
		return c, err
	}
	pids, err := intEnv("RUNNER_PIDS_LIMIT", 4096)
	if err != nil {
		return c, err
	}
	if memory < 0 || pids < 0 {
		return c, fmt.Errorf("config: RUNNER_MEMORY_MB and RUNNER_PIDS_LIMIT can't be negative")
	}
	r.MemoryMB, r.PidsLimit = int64(memory), int64(pids)
	if r.MaxConcurrent, err = intEnv("RUNNER_MAX_CONCURRENT", 2); err != nil {
		return c, err
	}
	if r.MaxConcurrent < 1 || r.MaxConcurrent > 32 {
		return c, fmt.Errorf("config: RUNNER_MAX_CONCURRENT must be 1–32")
	}
	if raw := os.Getenv("RUNNER_MODEL_PRICES"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &r.ModelPrices); err != nil {
			return c, fmt.Errorf("config: RUNNER_MODEL_PRICES must be a JSON object of model token prices")
		}
		var entries map[string]map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return c, fmt.Errorf("config: invalid model price entries")
		}
		for model, price := range r.ModelPrices {
			for _, key := range []string{"input_per_million", "cached_input_per_million", "output_per_million"} {
				if value, ok := entries[model][key]; !ok || string(value) == "null" {
					return c, fmt.Errorf("config: model %s is missing %s", model, key)
				}
			}
			for _, v := range []float64{price.Input, price.CachedInput, price.Output} {
				if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
					return c, fmt.Errorf("config: invalid token price for %s", model)
				}
			}
		}
	}
	ttl, err := intEnv("RUNNER_CHAT_IDLE_TTL_S", 4*60*60)
	if err != nil {
		return c, err
	}
	if ttl < 60 {
		return c, fmt.Errorf("config: RUNNER_CHAT_IDLE_TTL_S must be at least 60")
	}
	r.ChatIdleTTL = time.Duration(ttl) * time.Second
	hold, err := intEnv("RUNNER_MAX_HOLD_S", 24*60*60)
	if err != nil {
		return c, err
	}
	if hold < 60 {
		return c, fmt.Errorf("config: RUNNER_MAX_HOLD_S must be at least 60")
	}
	r.MaxHold = time.Duration(hold) * time.Second
	if r.MaxHoldAttempts, err = intEnv("RUNNER_MAX_HOLD_ATTEMPTS", 12); err != nil {
		return c, err
	}
	if r.MaxHoldAttempts < 1 {
		return c, fmt.Errorf("config: RUNNER_MAX_HOLD_ATTEMPTS must be at least 1")
	}
	c.Runner = r
	if c.PRSyncInterval, err = intEnv("PR_SYNC_INTERVAL_S", 60); err != nil {
		return c, err
	}
	if c.MemoryConsolidateInterval, err = intEnv("MEMORY_CONSOLIDATE_INTERVAL_S", 6*60*60); err != nil {
		return c, err
	}
	return c, nil
}

func intEnv(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def, fmt.Errorf("config: %s=%q is not a number", key, v)
	}
	return n, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func boolEnv(key string, def bool) (bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def, fmt.Errorf("config: %s=%q is not a boolean", key, v)
	}
	return b, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
