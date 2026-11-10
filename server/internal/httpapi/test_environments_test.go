package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/seed"
)

// A person replaces a repository's test environment by URL, sees it under its
// key, can't save one the runner would refuse, and forgets it.
func TestTestEnvironmentsCRUD(t *testing.T) {
	ctx := context.Background()
	s, h := newServer(t)
	if _, err := seed.Run(ctx, s.Pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	const base = "/api/v1/runner/test-environments"
	put := `{"repo":"git@github.com:Acme/App.git","recipe":{"services":[{"name":"redis","image":"redis:7"}],"nix_packages":["nixpkgs#protobuf"]}}`
	if rec := sendJSON(h, "PUT", base, put); rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	for name, body := range map[string]string{
		"shell in a package": `{"repo":"github.com/acme/app","recipe":{"nix_packages":["nixpkgs#x; rm -rf /"]}}`,
		"bad service name":   `{"repo":"github.com/acme/app","recipe":{"services":[{"name":"Bad Name","image":"redis"}]}}`,
		"no repo":            `{"repo":"","recipe":{}}`,
	} {
		if rec := sendJSON(h, "PUT", base, body); rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	rec := sendJSON(h, "GET", base, "")
	var list struct {
		Environments []struct {
			Repo     string `json:"repo"`
			EditedAt string `json:"edited_at"`
			Recipe   struct {
				NixPackages []string `json:"nix_packages"`
			} `json:"recipe"`
		} `json:"environments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Environments) != 1 {
		t.Fatalf("list: %v %s", err, rec.Body)
	}
	if e := list.Environments[0]; e.Repo != "github.com/acme/app" || e.EditedAt == "" || strings.Join(e.Recipe.NixPackages, ",") != "nixpkgs#protobuf" {
		t.Fatalf("environment = %+v", e)
	}
	if rec := sendJSON(h, "DELETE", base+"?repo=https://github.com/acme/app", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := sendJSON(h, "DELETE", base+"?repo=github.com/acme/app", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d", rec.Code)
	}
}
