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
