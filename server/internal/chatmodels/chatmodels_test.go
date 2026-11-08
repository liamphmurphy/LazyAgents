package chatmodels

import (
	"context"
	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLiveDiscovery(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("x-api-key") != "secret" {
			t.Error("missing authentication")
		}
		if r.URL.Query().Get("after_id") == "" {
			w.Write([]byte(`{"data":[{"id":"claude-new","display_name":"New model"}],"has_more":true,"last_id":"claude-new"}`))
		} else {
			w.Write([]byte(`{"data":[{"id":"claude-other"}],"has_more":false}`))
		}
	}))
	defer srv.Close()
	models, err := discoverHTTP(context.Background(), srv.Client(), srv.URL, agentdef.HarnessClaudeCode, "secret")
	if err != nil || len(models) != 2 || calls != 2 || !models[0].Default || models[0].Label != "New model" {
		t.Fatalf("models=%+v calls=%d err=%v", models, calls, err)
	}
}

func TestDiscoveryFailureDoesNotExposeProviderBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`secret provider details`))
	}))
	defer srv.Close()
	_, err := discoverHTTP(context.Background(), srv.Client(), srv.URL, agentdef.HarnessCodex, "secret")
	if err == nil || err.Error() != "provider model discovery failed (401)" {
		t.Fatal(err)
	}
}

func TestLocalCatalogWithoutAppSecretUsesDiscovery(t *testing.T) {
	calls := 0
	catalog := Catalog{Secrets: &secrets.Service{}, Discover: func(_ context.Context, harness, key string) ([]Model, error) {
		calls++
		if key != "" {
			t.Fatal("expected discovery to resolve local credentials")
		}
		return []Model{{ID: harness + "-live", Default: true}}, nil
	}}
	list, err := catalog.List(context.Background(), nil, uuid.New())
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	for _, h := range list {
		if h.Harness == agentdef.HarnessOpenCode {
			// Never reached via local login: unattested isolation, no discovery.
			if h.Available || len(h.Models) != 0 {
				t.Fatalf("opencode %+v", h)
			}
			continue
		}
		if !h.Available || len(h.Models) != 1 {
			t.Fatalf("unexpected harness: %+v", h)
		}
	}
}

func TestDiscoveryEndpointSeamIsOptIn(t *testing.T) {
	t.Setenv(DiscoveryURLEnv, "")
	if got := discoveryEndpoint("openai", "https://api.openai.com/v1/models"); got != "https://api.openai.com/v1/models" {
		t.Fatalf("unset seam changed the endpoint: %s", got)
	}
	if got := discoveryEndpoint("anthropic", "https://api.anthropic.com/v1/models?limit=1000"); got != "https://api.anthropic.com/v1/models?limit=1000" {
		t.Fatalf("unset seam changed the endpoint: %s", got)
	}
	t.Setenv(DiscoveryURLEnv, "http://127.0.0.1:9/")
	if got := discoveryEndpoint("anthropic", "x"); got != "http://127.0.0.1:9/anthropic/v1/models?limit=1000" {
		t.Fatal(got)
	}
	if got := discoveryEndpoint("openai", "x"); got != "http://127.0.0.1:9/openai/v1/models" {
		t.Fatal(got)
	}
}

func TestDiscoverUsesSeamEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/models" || r.Header.Get("x-api-key") != "dummy" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.Write([]byte(`{"data":[{"id":"claude-sonnet-5-5"}],"has_more":false}`))
	}))
	defer srv.Close()
	t.Setenv(DiscoveryURLEnv, srv.URL)
	models, err := Discover(context.Background(), agentdef.HarnessClaudeCode, "dummy")
	if err != nil || len(models) != 1 || models[0].ID != "claude-sonnet-5-5" {
		t.Fatalf("models=%+v err=%v", models, err)
	}
}
