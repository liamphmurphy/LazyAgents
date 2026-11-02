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

	// The local user created when SEED=false (first boot only). There is no
	// login for now; every request acts as this user.
	LocalUserName  string
	LocalUserEmail string

	// SecretsKey encrypts stored secrets (base64, 32 bytes). Empty = secrets disabled.
	SecretsKey string
	// PublicURL is how agents and browsers reach the app (Caddy in compose).
	PublicURL string
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
