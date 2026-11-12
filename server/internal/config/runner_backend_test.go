package config

import "testing"

// Only the hardened Docker backend runs real harnesses. The fake backend runs
// on the host with no isolation, so it's accepted only with a stand-in for
// every harness; "local" (real CLIs on the host) is gone.
func TestRunnerBackendIsDockerOrFake(t *testing.T) {
	for _, tc := range []struct {
		backend, claude, codex string
		ok                     bool
	}{
		{"docker", "", "", true},
		{"fake", "/fake/claude", "/fake/codex", true},
		{"fake", "/fake/claude", "", false},
		{"fake", "", "/fake/codex", false},
		{"fake", "", "", false},
		{"local", "", "", false},
		{"local", "/fake/claude", "/fake/codex", false},
	} {
		t.Setenv("RUNNER_BACKEND", tc.backend)
		t.Setenv("RUNNER_CLAUDE_BIN", tc.claude)
		t.Setenv("RUNNER_CODEX_BIN", tc.codex)
		_, err := Load()
		if tc.ok && err != nil {
			t.Errorf("%s claude=%q codex=%q: %v", tc.backend, tc.claude, tc.codex, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s claude=%q codex=%q: accepted", tc.backend, tc.claude, tc.codex)
		}
	}
}
