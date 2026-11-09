package chatmodels

import (
	"bytes"
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
