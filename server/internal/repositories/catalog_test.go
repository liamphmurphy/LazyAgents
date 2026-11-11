package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

type tokenValue string

func (t tokenValue) Reveal(context.Context, uuid.UUID, *uuid.UUID, string) (string, error) {
	return string(t), nil
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCanonicalRepositoryIdentity(t *testing.T) {
	for _, raw := range []string{"Acme/App", "https://github.com/Acme/App/", "git@GitHub.com:Acme/App.git", "ssh://git@github.com/acme/app"} {
		got, err := Canonical(raw)
		if err != nil || got != "https://github.com/acme/app.git" {
			t.Fatalf("%s: %q %v", raw, got, err)
		}
	}
	for _, raw := range []string{"file:///tmp/x", "../repo", "acme/..", "https://evil.example/acme/repo", "https://github.com@evil.example/a/b", "https://token@github.com/acme/repo", "-u/x", "acme/a;id", "acme/a/b", "acme/a\nb"} {
		if u, err := Canonical(raw); err == nil {
			t.Fatalf("accepted %q as %s", raw, u)
		}
	}
	if got := Directory("https://github.com/acme/app.git"); got != ".lazyagents/repos/acme/app" {
		t.Fatal(got)
	}
}
func TestOrganizationDiscoveryPaginationRefreshAndAmbiguity(t *testing.T) {
	pool := testdb.New(t)
	ctx := t.Context()
	if _, err := seed.Run(ctx, pool, seed.Options{}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	user, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{WorkspaceID: user.WorkspaceID, UserID: user.ID}
	var requests atomic.Int32
	deny := false
	svc := Service{Q: q, Secrets: tokenValue("test-token"), Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("wrong forge or authorization")
		}
		if r.URL.Path == "/users/acme" && !deny {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"type":"Organization"}`))}, nil
		}
		if deny {
			return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("{}"))}, nil
		}
		rows := []map[string]string{}
		if r.URL.Query().Get("page") == "1" {
			for i := 0; i < 100; i++ {
				rows = append(rows, map[string]string{"full_name": fmt.Sprintf("acme/repo-%d", i)})
			}
		} else {
			rows = append(rows, map[string]string{"full_name": "acme/app"})
		}
		body, _ := json.Marshal(rows)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}}
	if err := svc.Save(ctx, owner, "organization", "Acme", false); err != nil {
		t.Fatal(err)
	}
	if err := svc.Save(ctx, owner, "repository", "other/app", false); err != nil {
		t.Fatal(err)
	}
	entries, err := svc.Search(ctx, owner, "", false)
	if err != nil || len(entries) != 102 || requests.Load() != 3 {
		t.Fatalf("entries %d requests %d err %v", len(entries), requests.Load(), err)
	}
	if _, err = svc.Search(ctx, owner, "", false); err != nil || requests.Load() != 3 {
		t.Fatal("cache miss", err)
	}
	if _, err = svc.Resolve(ctx, owner, "app"); err == nil || !strings.Contains(err.Error(), "acme/app, other/app") {
		t.Fatal("ambiguous name", err)
	}
	if url, err := svc.Resolve(ctx, owner, "repo-99"); err != nil || url != "https://github.com/acme/repo-99.git" {
		t.Fatal(url, err)
	}
	if _, err = svc.Search(ctx, owner, "app", true); err != nil || requests.Load() != 6 {
		t.Fatal("refresh", requests.Load(), err)
	}
	deny = true
	if _, err = svc.Search(ctx, owner, "", true); err == nil {
		t.Fatal("permission failure was hidden")
	}
	// Explicit repositories never require saving or an organization lookup.
	if url, err := svc.Resolve(ctx, owner, "unlisted/private"); err != nil || url != "https://github.com/unlisted/private.git" {
		t.Fatal(url, err)
	}
	if _, err = svc.Resolve(ctx, owner, "missing"); err == nil {
		t.Fatal("unknown name resolved")
	}
	if err = svc.Save(ctx, owner, "repository", "other/app", true); err != nil {
		t.Fatal(err)
	}
	legacy := "https://git.example.com/acme/old"
	if err := q.AddRepositorySource(ctx, store.AddRepositorySourceParams{WorkspaceID: owner.WorkspaceID, UserID: owner.UserID, Kind: "repository", Name: legacy}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Save(ctx, owner, "repository", legacy, true); err != nil {
		t.Fatal("remove legacy source", err)
	}
	sources, err := svc.Sources(ctx, owner)
	if err != nil || len(sources) != 1 {
		t.Fatal(sources, err)
	}
}

func TestOwnerWildcardNormalization(t *testing.T) {
	for _, raw := range []string{"liamphmurphy", "liamphmurphy/*", "https://github.com/liamphmurphy/*", "https://github.com/LIAMPHMURPHY/"} {
		name, err := Organization(raw)
		if err != nil || name != "liamphmurphy" {
			t.Errorf("%q: name=%q err=%v", raw, name, err)
		}
	}
	for _, raw := range []string{"*", "https://github.com/*/repo", "liamphmurphy/app*", "liamphmurphy/*/src", "https://evil.example/liamphmurphy/*", "https://token@github.com/liamphmurphy/*"} {
		if _, err := Organization(raw); err == nil {
			t.Errorf("accepted unsupported owner wildcard %q", raw)
		}
	}
}

func TestPersonalOwnerWildcardIncludesPrivateRepositories(t *testing.T) {
	pool := testdb.New(t)
	ctx := t.Context()
	if _, err := seed.Run(ctx, pool, seed.Options{}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	user, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{WorkspaceID: user.WorkspaceID, UserID: user.ID}
	var requests atomic.Int32
	fresh, denied := false, false
	svc := Service{Q: q, Secrets: tokenValue("test-token"), Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		response := func(value any) *http.Response {
			raw, _ := json.Marshal(value)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}
		}
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("wrong host or missing token")
		}
		if r.URL.Path == "/users/alice" {
			return response(map[string]string{"type": "User"}), nil
		}
		names := []string{}
		page := r.URL.Query().Get("page")
		switch r.URL.Path {
		case "/users/alice/repos":
			if page == "1" {
				for i := 0; i < 100; i++ {
					names = append(names, fmt.Sprintf("alice/public-%d", i))
				}
			} else {
				names = append(names, "alice/shared")
			}
		case "/user/repos":
			if r.URL.Query().Get("affiliation") != "owner,collaborator" || r.URL.Query().Get("visibility") != "all" {
				t.Fatal("private repositories were not requested")
			}
			if denied {
				return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			}
			if page == "1" {
				names = append(names, "alice/private", "alice/public-0")
				for i := 0; i < 98; i++ {
					names = append(names, fmt.Sprintf("someone-else/repo-%d", i))
				}
			} else {
				names = append(names, "alice/private-page2")
				if fresh {
					names = append(names, "alice/new-private")
				}
			}
		default:
			t.Fatalf("unexpected GitHub request %s", r.URL)
		}
		rows := []map[string]string{}
		for _, name := range names {
			rows = append(rows, map[string]string{"full_name": name})
		}
		return response(rows), nil
	})}}
	for _, source := range []struct{ kind, name string }{{"repository", "https://github.com/alice/*"}, {"organization", "alice/*"}} {
		if err := svc.Save(ctx, owner, source.kind, source.name, false); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := svc.Sources(ctx, owner)
	if err != nil || len(sources) != 1 || sources[0].Kind != "organization" || sources[0].Name != "alice" {
		t.Fatal(sources, err)
	}
	entries, err := svc.Search(ctx, owner, "", false)
	if err != nil || len(entries) != 103 || requests.Load() != 5 {
		t.Fatalf("entries=%d calls=%d err=%v", len(entries), requests.Load(), err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name, "alice/") {
			t.Fatalf("included another owner's repository: %s", entry.Name)
		}
	}
	if url, err := svc.Resolve(ctx, owner, "private-page2"); err != nil || url != "https://github.com/alice/private-page2.git" {
		t.Fatal(url, err)
	}
	if requests.Load() != 5 {
		t.Fatal("owner catalogue was not cached")
	}
	fresh = true
	entries, err = svc.Search(ctx, owner, "new-private", true)
	if err != nil || len(entries) != 1 || requests.Load() != 10 {
		t.Fatal("refresh", entries, requests.Load(), err)
	}
	denied = true
	if _, err := svc.Search(ctx, owner, "", true); err == nil {
		t.Fatal("silently returned only public repositories after private enumeration failed")
	}
	if err := svc.Save(ctx, owner, "repository", "https://github.com/alice/*", true); err != nil {
		t.Fatal(err)
	}
	sources, err = svc.Sources(ctx, owner)
	if err != nil || len(sources) != 0 {
		t.Fatal("wildcard was not removed", sources, err)
	}
}
