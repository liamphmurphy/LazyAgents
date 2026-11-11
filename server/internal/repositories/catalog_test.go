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
	if err != nil || len(entries) != 102 || requests.Load() != 2 {
		t.Fatalf("entries %d requests %d err %v", len(entries), requests.Load(), err)
	}
	if _, err = svc.Search(ctx, owner, "", false); err != nil || requests.Load() != 2 {
		t.Fatal("cache miss", err)
	}
	if _, err = svc.Resolve(ctx, owner, "app"); err == nil || !strings.Contains(err.Error(), "acme/app, other/app") {
		t.Fatal("ambiguous name", err)
	}
	if url, err := svc.Resolve(ctx, owner, "repo-99"); err != nil || url != "https://github.com/acme/repo-99.git" {
		t.Fatal(url, err)
	}
	if _, err = svc.Search(ctx, owner, "app", true); err != nil || requests.Load() != 4 {
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
