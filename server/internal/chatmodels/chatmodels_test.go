package chatmodels

import (
	"context"
	"github.com/google/uuid"
	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/secrets"
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
	if err != nil || calls != len(agentdef.Harnesses) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	for _, h := range list {
		if !h.Available || len(h.Models) != 1 {
			t.Fatalf("unexpected harness: %+v", h)
		}
	}
}
