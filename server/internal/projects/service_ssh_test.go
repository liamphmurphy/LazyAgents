package projects

import (
	"context"
	"errors"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

func TestRepoURLValidation(t *testing.T) {
	ok := []string{"git@github.com:liamphmurphy/LazyAgents.git", "git@github.com:a/b", "ssh://git@github.com/a/b.git", "  git@github.com:a/b.git/ ", ""}
	bad := []string{"https://github.com/a/b", "http://github.com/a/b.git", "git@gitlab.com:a/b.git", "git@github.com:a/b.git?x=1", "git@github.com:a/..", "git@github.com:a", "file:///tmp/x", "git@evil.com:a/b", "git@github.com:a/b/c", "ssh://git@github.com:22/a/b", "ext::sh -c touch /tmp/x", "-oProxyCommand=x"}
	for _, u := range ok {
		if err := normalize(&Config{Name: "N", Color: "#aabbcc", RepoURL: u}); err != nil {
			t.Errorf("%q rejected: %v", u, err)
		}
	}
	for _, u := range bad {
		if err := normalize(&Config{Name: "N", Color: "#aabbcc", RepoURL: u}); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
}

func TestUpdateKeepsUnchangedLegacyHTTPSRepoURL(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	ws, err := q.CreateWorkspace(ctx, store.CreateWorkspaceParams{Name: "W", Slug: "w"})
	if err != nil {
		t.Fatal(err)
	}
	legacy := "https://github.com/a/b"
	created, err := q.CreateProject(ctx, store.CreateProjectParams{WorkspaceID: ws.ID, Key: "LEG", Name: "Legacy", Color: "#112233", RepoUrl: &legacy})
	if err != nil {
		t.Fatal(err)
	}
	svc := Service{Pool: pool}
	p, err := svc.Update(ctx, ws.ID, "LEG", Config{Name: "Renamed", Color: "#112233", RepoURL: legacy, DefaultBranch: "main", Revision: created.Revision})
	if err != nil {
		t.Fatalf("unchanged legacy URL rejected: %v", err)
	}
	if p.Name != "Renamed" || p.RepoUrl == nil || *p.RepoUrl != legacy {
		t.Fatalf("got %+v", p)
	}
	_, err = svc.Update(ctx, ws.ID, "LEG", Config{Name: "Renamed", Color: "#112233", RepoURL: "https://github.com/a/c", DefaultBranch: "main", Revision: p.Revision})
	var ae *apperr.ValidationError
	if err == nil || !errors.As(err, &ae) || ae.Field != "repo_url" {
		t.Fatalf("changed HTTPS URL: err = %v", err)
	}
	if _, err := svc.Update(ctx, ws.ID, "LEG", Config{Name: "Renamed", Color: "#112233", RepoURL: "git@github.com:a/c.git", DefaultBranch: "main", Revision: p.Revision}); err != nil {
		t.Fatalf("SSH change rejected: %v", err)
	}
}
