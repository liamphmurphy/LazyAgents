package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func newServer(t *testing.T) (*Server, http.Handler) {
	pool := testdb.New(t)
	s := &Server{AnonymousAPI: true, Pool: pool, Q: store.New(pool), Tracker: &tracker.Service{Pool: pool}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return s, s.Handler()
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// With no login, every request acts as the workspace's first user.
func TestRequestsActAsLocalUser(t *testing.T) {
	s, h := newServer(t)

	if rec := get(h, "/api/v1/bootstrap"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("empty database: code = %d, want 503", rec.Code)
	}

	if _, err := seed.Run(context.Background(), s.Pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/api/v1/bootstrap")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Me struct{ Name string } `json:"me"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Me.Name != "Maya Chen" {
		t.Fatalf("me = %q, want Maya Chen", body.Me.Name)
	}
}

func TestPlainSeedCreatesLocalUser(t *testing.T) {
	s, h := newServer(t)
	if _, err := seed.Run(context.Background(), s.Pool, seed.Options{UserName: "Liam Murphy"}); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/api/v1/bootstrap")
	if rec.Code != http.StatusOK || !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("code = %d", rec.Code)
	}
	var body struct {
		Me struct{ Name, Initials string } `json:"me"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Me.Name != "Liam Murphy" || body.Me.Initials != "LM" {
		t.Fatalf("me = %+v", body.Me)
	}
}

func TestFreshBootHasOnlyTheOwnerAndFullyEnabledAssistant(t *testing.T) {
	s, h := newServer(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, s.Pool, seed.Options{}); err != nil {
		t.Fatal(err)
	}
	var boot struct {
		Users, Agents, Projects, Epics, Labels []json.RawMessage
	}
	rec := get(h, "/api/v1/bootstrap")
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap: %d %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &boot); err != nil {
		t.Fatal(err)
	}
	if len(boot.Users) != 1 || len(boot.Agents)+len(boot.Projects)+len(boot.Epics)+len(boot.Labels) != 0 {
		t.Fatalf("fresh boot contains demo data: %s", rec.Body)
	}
	me, err := s.Q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Q.DefaultPersona(ctx, store.DefaultPersonaParams{WorkspaceID: me.WorkspaceID, UserID: me.ID})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Assistant" || !p.CanCode || !p.CanGenerateImages || !p.CanCreatePersonas || !p.CanCreateSvgs || !p.CanCreateWorkflows || !p.CanCreateConnectors {
		t.Fatalf("initial Assistant permissions: %+v", p)
	}
	// Restarting must preserve permission choices on an existing Assistant.
	if _, err := s.Pool.Exec(ctx, "UPDATE personas SET can_code=false WHERE id=$1", p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Run(ctx, s.Pool, seed.Options{}); err != nil {
		t.Fatal(err)
	}
	p, err = s.Q.DefaultPersona(ctx, store.DefaultPersonaParams{WorkspaceID: me.WorkspaceID, UserID: me.ID})
	if err != nil || p.CanCode {
		t.Fatalf("restart changed permissions: %+v %v", p, err)
	}
}
