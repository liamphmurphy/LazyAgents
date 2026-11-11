package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/repositories"
	"github.com/liamphmurphy/vesper/server/internal/runs"
)

type presetToken struct{}

func (presetToken) Reveal(context.Context, uuid.UUID, *uuid.UUID, string) (string, error) {
	return "fixture-token", nil
}

type presetTransport func(*http.Request) (*http.Response, error)

func (f presetTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRepositoryPresetsUseConnectionWithoutSavingSources(t *testing.T) {
	e := newChatEnvWith(t, func(rs *runs.Service) {
		rs.RepositoryCatalog = &repositories.Service{Secrets: presetToken{}, Client: &http.Client{Transport: presetTransport(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Authorization") != "Bearer fixture-token" || r.URL.Host != "api.github.com" {
				t.Fatal("wrong GitHub connection")
			}
			body := ""
			switch r.URL.Path {
			case "/user":
				body = `{"login":"connected-user"}`
			case "/user/orgs":
				body = `[{"login":"team"}]`
			case "/user/repos":
				body = `[]`
			default:
				t.Fatalf("unexpected GitHub path %s", r.URL.Path)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}}
	})
	out := e.want("GET", "/repositories/presets", nil, 200)
	owners := out["owners"].([]any)
	if len(owners) != 2 || owners[0].(map[string]any)["login"] != "connected-user" || owners[0].(map[string]any)["organization"] != false || owners[1].(map[string]any)["login"] != "team" {
		t.Fatal(out)
	}
	var count int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM repository_sources WHERE workspace_id=$1 AND user_id=$2`, e.me.WorkspaceID, e.me.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("preset discovery saved sources", count, err)
	}
}
