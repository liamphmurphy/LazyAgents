// Package config loads process configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr        string // HTTP listen address, e.g. ":8000"
	DatabaseURL string
	S3          S3
	Seed        bool
	Version     string
}

type S3 struct {
	Endpoint     string // e.g. http://seaweedfs:8333
	Region       string
	AccessKey    string
	SecretKey    string
	Buckets      []string
	UsePathStyle bool
}

// Load reads configuration from environment variables, applying local-dev defaults.
func Load() (Config, error) {
	c := Config{
		Addr:        env("API_ADDR", ":8000"),
		DatabaseURL: env("DATABASE_URL", "postgres://lazyagents:lazyagents@localhost:5432/lazyagents?sslmode=disable"),
		S3: S3{
			Endpoint:     env("S3_ENDPOINT", "http://localhost:8333"),
			Region:       env("S3_REGION", "us-east-1"),
			AccessKey:    env("S3_ACCESS_KEY", "lazyagents"),
			SecretKey:    env("S3_SECRET_KEY", "lazyagents-secret"),
			Buckets:      splitList(env("S3_BUCKETS", "artifacts,run-logs")),
			UsePathStyle: true,
		},
		Version: env("LAZYAGENTS_VERSION", "dev"),
	}
	var err error
	if c.Seed, err = boolEnv("SEED", false); err != nil {
		return c, err
	}
	return c, nil
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
