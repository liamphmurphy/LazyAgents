package chatmodels

import (
	"bytes"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

func TestClaudeControlModelsWithoutInference(t *testing.T) {
	var input bytes.Buffer
	output := `{"type":"control_response","response":{"subtype":"success","request_id":"models","response":{"models":[{"value":"default","displayName":"Recommended"},{"value":"new-live-model","displayName":"New model"}]}}}`
	models, err := localModels(agentdef.HarnessClaudeCode, &input, strings.NewReader(output))
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
	models, err := localModels(agentdef.HarnessCodex, &input, strings.NewReader(output))
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
		_, err := localModels(agentdef.HarnessCodex, &input, strings.NewReader(output))
		if err == nil || strings.Contains(err.Error(), "private details") {
			t.Fatalf("err=%v", err)
		}
	}
}
