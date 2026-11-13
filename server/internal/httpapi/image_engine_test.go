package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/runs"
)

func TestImageEngineSettingAndPersonaOverride(t *testing.T) {
	e := newChatEnvServer(t, func(rs *runs.Service) {
		rs.ModelDiscovery = func(_ context.Context, h, _ string) ([]chatmodels.Model, error) {
			if h == agentdef.HarnessCodex {
				return nil, context.DeadlineExceeded // Codex isn't connected
			}
			return []chatmodels.Model{{ID: h + "-main", Default: true}}, nil
		}
	}, func(*Server) {})
	got := e.want("GET", "/image-engine", nil, http.StatusOK)
	if got["engine"] != "auto" {
		t.Fatalf("default engine = %v", got["engine"])
	}
	byEngine := map[string]map[string]any{}
	for _, o := range got["options"].([]any) {
		m := o.(map[string]any)
		byEngine[m["engine"].(string)] = m
	}
	if byEngine["codex"]["available"] != false || byEngine["codex"]["reason"] == nil || byEngine["antigravity"]["available"] != true || byEngine["antigravity"]["label"] != "Antigravity" {
		t.Fatalf("options = %v", got["options"])
	}
	if got = e.want("PUT", "/image-engine", map[string]any{"engine": "antigravity"}, http.StatusOK); got["engine"] != "antigravity" {
		t.Fatalf("saved = %v", got)
	}
	if got = e.want("GET", "/image-engine", nil, http.StatusOK); got["engine"] != "antigravity" {
		t.Fatalf("read back = %v", got)
	}
	e.invalid("PUT", "/image-engine", map[string]any{"engine": "dalle"}, "engine", "")

	p := e.want("POST", "/personas", map[string]any{"name": "Artist", "capabilities": map[string]any{"code": false, "images": true, "image_engine": "codex"}}, http.StatusCreated)
	if p["capabilities"].(map[string]any)["image_engine"] != "codex" {
		t.Fatalf("created = %v", p["capabilities"])
	}
	id := p["id"].(string)
	p = e.want("PATCH", "/personas/"+id, map[string]any{"revision": p["revision"], "capabilities": map[string]any{"code": false, "images": true, "image_engine": ""}}, http.StatusOK)
	if p["capabilities"].(map[string]any)["image_engine"] != "" {
		t.Fatalf("cleared = %v", p["capabilities"])
	}
	e.invalid("PATCH", "/personas/"+id, map[string]any{"revision": p["revision"], "capabilities": map[string]any{"code": false, "images": true, "image_engine": "dalle"}}, "capabilities.image_engine", "")
}
