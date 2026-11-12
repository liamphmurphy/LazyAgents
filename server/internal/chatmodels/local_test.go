package chatmodels

import (
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

func TestClaudeControlModelsWithoutInference(t *testing.T) {
	var input bytes.Buffer
	output := `{"type":"control_response","response":{"subtype":"success","request_id":"models","response":{"models":[{"value":"default","displayName":"Recommended"},{"value":"new-live-model","displayName":"New model"}]}}}`
	models, err := ListOverStdio(agentdef.HarnessClaudeCode, &input, strings.NewReader(output))
	if err != nil || len(models) != 2 || !models[0].Default || models[1].ID != "new-live-model" {
		t.Fatalf("models=%+v err=%v", models, err)
	}
	if strings.Contains(input.String(), `"user"`) || strings.Count(input.String(), "\n") != 1 || !strings.Contains(input.String(), `"subtype":"initialize"`) {
		t.Fatalf("unexpected requests: %s", input.String())
	}
}

// Claude Code names its aliases without a version ("Fable") and puts the
// version the alias resolves to at the start of the description.
func TestClaudeModelLabelsCarryTheVersion(t *testing.T) {
	// Trimmed from Claude Code 2.1.322's initialize response.
	output := `{"type":"control_response","response":{"subtype":"success","request_id":"models","response":{"models":[` +
		`{"value":"default","displayName":"Default (recommended)","description":"Use the default model (currently Opus 5.5) · $4/$20 per Mtok"},` +
		`{"value":"opus","displayName":"Opus","description":"Opus 5.5 · Best for everyday, complex tasks · $4/$20 per Mtok"},` +
		`{"value":"fable","displayName":"Fable","description":"Fable 5.2 · Most capable for your hardest and longest-running tasks · $8/$40 per Mtok"},` +
		`{"value":"sonnet","displayName":"Sonnet","description":"Efficient for routine tasks"},` +
		`{"value":"haiku","displayName":"Haiku"}]}}}`
	models, err := ListOverStdio(agentdef.HarnessClaudeCode, &bytes.Buffer{}, strings.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, m := range models {
		got[m.ID] = m.Label
	}
	want := map[string]string{"default": "Default (recommended)", "opus": "Opus 5.5", "fable": "Fable 5.2", "sonnet": "Sonnet", "haiku": "Haiku"}
	if !maps.Equal(got, want) {
		t.Fatalf("labels=%v want %v", got, want)
	}
}

func TestCodexModelsPaginationAndDefaults(t *testing.T) {
	var input bytes.Buffer
	output := `{"id":1,"result":{}}
{"method":"notification"}
{"id":2,"result":{"data":[{"model":"new-codex","displayName":"New Codex","isDefault":true},{"model":"hidden-model","hidden":true}],"nextCursor":"next"}}
{"id":3,"result":{"data":[{"model":"other-codex"}],"nextCursor":null}}`
	models, err := ListOverStdio(agentdef.HarnessCodex, &input, strings.NewReader(output))
	if err != nil || len(models) != 2 || !models[0].Default || models[1].Label != "other-codex" {
		t.Fatalf("models=%+v err=%v", models, err)
	}
	if !strings.Contains(input.String(), `"cursor":"next"`) || strings.Contains(input.String(), "thread/start") || strings.Contains(input.String(), "turn/start") {
		t.Fatalf("unexpected requests: %s", input.String())
	}
}

func TestLocalDiscoveryFailsClosed(t *testing.T) {
	for _, output := range []string{`{"id":1,"error":{"message":"private details"}}`, `{"id":1,"result":{}}`, `broken json`} {
		var input bytes.Buffer
		_, err := ListOverStdio(agentdef.HarnessCodex, &input, strings.NewReader(output))
		if err == nil || strings.Contains(err.Error(), "private details") {
			t.Fatalf("err=%v", err)
		}
	}
}

func TestLocalModelsReportEfforts(t *testing.T) {
	claude := `{"type":"control_response","response":{"subtype":"success","request_id":"models","response":{"models":[` +
		`{"value":"opus","displayName":"Opus","supportsEffort":true,"supportedEffortLevels":["low","medium","high","xhigh","max"]},` +
		`{"value":"haiku-4","displayName":"Haiku 4"}]}}}`
	models, err := ListOverStdio(agentdef.HarnessClaudeCode, &bytes.Buffer{}, strings.NewReader(claude))
	if err != nil || len(models) != 2 || !slices.Equal(models[0].Efforts, agentdef.Efforts) || len(models[1].Efforts) != 0 {
		t.Fatalf("claude models=%+v err=%v", models, err)
	}
	codex := `{"id":1,"result":{}}
{"id":2,"result":{"data":[{"model":"gpt","displayName":"GPT","isDefault":true,"defaultReasoningEffort":"medium",` +
		`"supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"medium"},{"reasoningEffort":"max"},{"reasoningEffort":"ultra"}]}]}}`
	models, err = ListOverStdio(agentdef.HarnessCodex, &bytes.Buffer{}, strings.NewReader(codex))
	if err != nil || len(models) != 1 || !slices.Equal(models[0].Efforts, []string{"low", "medium", "max"}) || models[0].DefaultEffort != "medium" {
		t.Fatalf("codex models=%+v err=%v", models, err)
	}
}

// The API only ever asks a stand-in harness for models on the host; with no
// RUNNER_*_BIN it must not fall through to a real, logged-in CLI on PATH.
func TestDiscoverLocalRefusesWithoutFakeBinary(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n: >\"$0.ran\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("RUNNER_CLAUDE_BIN", "")
	t.Setenv("RUNNER_CODEX_BIN", "")
	for _, h := range []string{agentdef.HarnessClaudeCode, agentdef.HarnessCodex} {
		if _, err := DiscoverLocal(context.Background(), h, ""); err == nil {
			t.Errorf("%s: discovered without a fake binary", h)
		}
	}
	for _, name := range []string{"claude", "codex"} {
		if _, err := os.Stat(filepath.Join(dir, name+".ran")); err == nil {
			t.Errorf("%s on PATH ran", name)
		}
	}
}
