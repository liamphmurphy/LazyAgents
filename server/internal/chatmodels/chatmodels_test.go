package chatmodels

import (
	"context"
	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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
	if err != nil || calls != len(chatHarnesses) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	for _, h := range list {
		if !h.Available || len(h.Models) != 1 {
			t.Fatalf("unexpected harness: %+v", h)
		}
	}
}

func TestResolveAcceptsRestatedCurrentModel(t *testing.T) {
	catalog := Catalog{Discover: func(_ context.Context, harness, _ string) ([]Model, error) {
		return []Model{{ID: "opus", Default: true}, {ID: "sonnet"}}, nil
	}}
	cc := agentdef.HarnessClaudeCode
	cur := Selection{Harness: cc, Model: "opus-5.5"}
	sel, err := catalog.Resolve(context.Background(), nil, uuid.New(), cur, Selection{Harness: cc, Model: "opus-5.5"})
	if err != nil || sel != cur {
		t.Fatalf("sel=%+v err=%v", sel, err)
	}
	if _, err := catalog.Resolve(context.Background(), nil, uuid.New(), cur, Selection{Harness: cc, Model: "haiku-5.5"}); err == nil {
		t.Fatal("expected an unlisted, changed model to be rejected")
	}
	if sel, err := catalog.Resolve(context.Background(), nil, uuid.New(), cur, Selection{Harness: cc, Model: "sonnet"}); err != nil || sel.Model != "sonnet" {
		t.Fatalf("sel=%+v err=%v", sel, err)
	}
}

func TestResolveEffort(t *testing.T) {
	catalog := Catalog{Discover: func(_ context.Context, harness, _ string) ([]Model, error) {
		if harness == agentdef.HarnessCodex {
			return []Model{{ID: "gpt", Default: true, Efforts: []string{"low", "medium", "high"}}}, nil
		}
		return []Model{{ID: "opus", Default: true, Efforts: agentdef.Efforts}, {ID: "haiku-4", Efforts: []string{}}}, nil
	}}
	ctx, ws := context.Background(), uuid.New()
	cc := agentdef.HarnessClaudeCode
	cur := Selection{Harness: cc, Model: "opus"}
	for _, tc := range []struct {
		name     string
		cur, req Selection
		want     string
		bad      bool
	}{
		{"set", cur, Selection{Effort: "max"}, "max", false},
		{"kept when omitted", Selection{Harness: cc, Model: "opus", Effort: "xhigh"}, Selection{Model: "opus"}, "xhigh", false},
		{"cleared by default", Selection{Harness: cc, Model: "opus", Effort: "xhigh"}, Selection{Effort: EffortDefault}, "", false},
		{"dropped when the new model lacks it", Selection{Harness: cc, Model: "opus", Effort: "max"}, Selection{Harness: agentdef.HarnessCodex}, "", false},
		{"kept across harnesses when offered", Selection{Harness: cc, Model: "opus", Effort: "high"}, Selection{Harness: agentdef.HarnessCodex}, "high", false},
		{"alias takes the harness levels", Selection{Harness: cc, Model: "opus-5.5"}, Selection{Effort: "xhigh"}, "xhigh", false},
		{"unoffered", cur, Selection{Model: "haiku-4", Effort: "low"}, "", true},
		{"unknown", cur, Selection{Effort: "ultra"}, "", true},
	} {
		sel, err := catalog.Resolve(ctx, nil, ws, tc.cur, tc.req)
		if tc.bad != (err != nil) || (!tc.bad && sel.Effort != tc.want) {
			t.Errorf("%s: sel=%+v err=%v", tc.name, sel, err)
		}
	}
}

func TestKnownEffortsDropsUltra(t *testing.T) {
	if got := knownEfforts([]string{"max", "ultra", "low"}); !slices.Equal(got, []string{"low", "max"}) {
		t.Fatalf("got %v", got)
	}
}

func TestParseAgyModels(t *testing.T) {
	got, err := ParseAgyModels(strings.NewReader("Available models:\n\ngemini-4-carbon-medium\tGemini 4 Carbon (Medium)\n* gemini-3.8-flash-medium  Fast\n  gemini-4-hydrogen-high\n- gemini-4-hydrogen-high duplicate\nnote: these change\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 ||
		got[0].ID != "gemini-4-carbon-medium" || got[0].Label != "Gemini 4 Carbon (Medium)" || got[0].Default ||
		got[1].ID != "gemini-3.8-flash-medium" || !got[1].Default || got[1].Label != "Fast" ||
		got[2].ID != "gemini-4-hydrogen-high" || got[2].Default {
		t.Fatalf("models = %+v", got)
	}
	if _, err := ParseAgyModels(strings.NewReader("ERROR: authentication required\n")); err == nil {
		t.Fatal("a listing with no model slugs must be an error, not an empty catalog")
	}
}

func TestAntigravityIsAChatHarnessWithNativePlanLogin(t *testing.T) {
	if !slices.Contains(chatHarnesses, agentdef.HarnessAntigravity) || KeySecret[agentdef.HarnessAntigravity] != "harness:google" || labels[agentdef.HarnessAntigravity] != "Antigravity" {
		t.Fatal("Antigravity must be registered in the catalog")
	}
	if PlanLogin(agentdef.HarnessAntigravity, "AIzaSyExample") {
		t.Fatal("legacy Gemini API key must not count as an Antigravity plan login")
	}
	valid := `{"v":1,"files":{".config/antigravity/plugin-secrets.json":"{\"token\":\"ok\"}"}}`
	if !PlanLogin(agentdef.HarnessAntigravity, valid) {
		t.Fatal("native Antigravity login bundle must count as a plan login")
	}
}
