package config

import "testing"

func TestModelPriceConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name, raw string
		valid     bool
	}{
		{"complete", `{"test":{"input_per_million":2,"cached_input_per_million":0.2,"output_per_million":10}}`, true},
		{"negative", `{"test":{"input_per_million":-2,"cached_input_per_million":0,"output_per_million":10}}`, false},
		{"incomplete", `{"test":{"input_per_million":2}}`, false},
		{"null", `{"test":{"input_per_million":null,"cached_input_per_million":0,"output_per_million":10}}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RUNNER_MODEL_PRICES", tt.raw)
			c, err := Load()
			if tt.valid {
				if err != nil || c.Runner.ModelPrices["test"].Input != 2 {
					t.Fatalf("valid prices rejected: %v", err)
				}
			} else if err == nil {
				t.Fatal("invalid prices accepted")
			}
		})
	}
}

func TestHostWorkspacesMustBeAbsolute(t *testing.T) {
	t.Setenv("RUNNER_HOST_WORKSPACES_DIR", "workspaces")
	if _, err := Load(); err == nil {
		t.Fatal("relative RUNNER_HOST_WORKSPACES_DIR accepted")
	}
	t.Setenv("RUNNER_HOST_WORKSPACES_DIR", "/Users/me/vesper/workspaces")
	if c, err := Load(); err != nil || c.Runner.HostWorkspaces != "/Users/me/vesper/workspaces" {
		t.Fatalf("absolute path: %q %v", c.Runner.HostWorkspaces, err)
	}
}
