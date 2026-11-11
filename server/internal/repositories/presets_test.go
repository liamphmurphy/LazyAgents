package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func presetResponse(status int, value any) *http.Response {
	raw, _ := json.Marshal(value)
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(raw)))}
}
func TestPresetsIncludeAuthenticatedAccountAndPaginatedOrganizations(t *testing.T) {
	pages := map[string]int{}
	svc := Service{Secrets: tokenValue("preset-token"), Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer preset-token" {
			t.Fatal("wrong forge or credential")
		}
		pages[r.URL.Path]++
		switch r.URL.Path {
		case "/user":
			return presetResponse(200, map[string]string{"login": "Alice"}), nil
		case "/user/orgs":
			rows := []map[string]string{}
			if r.URL.Query().Get("page") == "1" {
				for i := 0; i < 100; i++ {
					rows = append(rows, map[string]string{"login": fmt.Sprintf("org-%d", i)})
				}
			} else {
				rows = append(rows, map[string]string{"login": "team"}, map[string]string{"login": "ORG-0"})
			}
			return presetResponse(200, rows), nil
		case "/user/repos":
			if r.URL.Query().Get("visibility") != "all" || r.URL.Query().Get("affiliation") != "owner,collaborator,organization_member" {
				t.Fatal("token-accessible repositories not requested")
			}
			rows := []map[string]any{}
			if r.URL.Query().Get("page") == "1" {
				for i := 0; i < 100; i++ {
					rows = append(rows, map[string]any{"owner": map[string]string{"login": "org-0", "type": "Organization"}})
				}
			} else {
				rows = append(rows, map[string]any{"owner": map[string]string{"login": "private-team", "type": "Organization"}}, map[string]any{"owner": map[string]string{"login": "other-person", "type": "User"}})
			}
			return presetResponse(200, rows), nil
		default:
			t.Fatalf("unexpected request %s", r.URL)
			return nil, nil
		}
	})}}
	out, err := svc.Presets(t.Context(), Owner{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Owners) != 103 || out.Warning != "" || out.Owners[0].Login != "alice" || out.Owners[0].Organization {
		t.Fatalf("presets: %+v", out)
	}
	if pages["/user"] != 1 || pages["/user/orgs"] != 2 || pages["/user/repos"] != 2 {
		t.Fatal("pagination", pages)
	}
	seen := map[string]bool{}
	for _, p := range out.Owners {
		if seen[p.Login] {
			t.Fatalf("duplicate owner %s", p.Login)
		}
		seen[p.Login] = true
	}
	if !seen["private-team"] || seen["other-person"] {
		t.Fatalf("incorrect org filtering: %+v", out)
	}
}

func TestPresetsPreserveAccountAndManualFallbackWhenOrgDiscoveryFails(t *testing.T) {
	for _, status := range []int{200, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			svc := Service{Secrets: tokenValue("preset-token"), Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/user":
					return presetResponse(200, map[string]string{"login": "alice"}), nil
				case "/user/orgs":
					return presetResponse(status, []any{}), nil
				case "/user/repos":
					return presetResponse(200, []any{map[string]any{"owner": map[string]string{"login": "fine-grained-org", "type": "Organization"}}}), nil
				default:
					t.Fatalf("unexpected request %s", r.URL)
					return nil, nil
				}
			})}}
			out, err := svc.Presets(t.Context(), Owner{})
			if err != nil || len(out.Owners) != 2 || out.Owners[0].Login != "alice" || out.Owners[1].Login != "fine-grained-org" {
				t.Fatal(out, err)
			}
			if (out.Warning != "") != (status == 403) {
				t.Fatal("missing partial-discovery notice", out)
			}
		})
	}
}
func TestPresetsRequireConnectionAndValidateAccount(t *testing.T) {
	if _, err := (&Service{}).Presets(context.Background(), Owner{}); err == nil {
		t.Fatal("missing GitHub connection accepted")
	}
	for _, login := range []string{"", "../escape", "alice/*"} {
		svc := Service{Secrets: tokenValue("test"), Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/user" {
				t.Fatal("continued after invalid account")
			}
			return presetResponse(200, map[string]string{"login": login}), nil
		})}}
		if _, err := svc.Presets(t.Context(), Owner{}); err == nil {
			t.Fatalf("invalid account %q accepted", login)
		}
	}
}
