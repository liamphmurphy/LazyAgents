package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func TestTicketCriteriaDraftHTTP(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	q := store.New(pool)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: pool}
	s := &Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: &runs.Service{Pool: pool, Tracker: tr}}
	handler := s.Handler()
	call := func(method, path, body string, want int, dst any) {
		t.Helper()
		r := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(r, req)
		if r.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, r.Code, want, r.Body.String())
		}
		if dst != nil {
			if err := json.Unmarshal(r.Body.Bytes(), dst); err != nil {
				t.Fatal(err)
			}
		}
	}
	var tk api.Ticket
	call("POST", "/tickets", `{"project_key":"PAY","title":"modal","criteria":["one","two"]}`, 201, &tk)
	if len(tk.Criteria) != 2 {
		t.Fatalf("criteria: %+v", tk.Criteria)
	}
	c0, c1 := tk.Criteria[0], tk.Criteria[1]
	exp := func(cs ...api.Criterion) string {
		b, _ := json.Marshal(cs)
		return string(b)
	}
	path := "/tickets/" + tk.Key
	body := func(rev int, expected, items string) string {
		b, _ := json.Marshal(map[string]any{"revision": rev, "title": "renamed",
			"criteria": map[string]json.RawMessage{"expected": json.RawMessage(expected), "items": json.RawMessage(items)}})
		return string(b)
	}
	items := `[{"id":"` + c1.Id.String() + `","text":"two","done":true},{"text":"new","done":false}]`

	// Stale revision: 409 on revision, nothing applied.
	var e api.Error
	call("PATCH", path, body(tk.Revision+3, exp(c0, c1), items), 409, &e)
	if e.Field == nil || *e.Field != "revision" {
		t.Fatalf("stale revision error: %+v", e)
	}
	// Snapshot mismatch: 409 on criteria.
	stale := c0
	stale.Text = "outdated"
	call("PATCH", path, body(tk.Revision, exp(stale, c1), items), 409, &e)
	if e.Field == nil || *e.Field != "criteria" {
		t.Fatalf("snapshot error: %+v", e)
	}
	// Invalid text: 422 criteria.
	call("PATCH", path, body(tk.Revision, exp(c0, c1), `[{"text":" ","done":false}]`), 422, &e)
	if e.Field == nil || *e.Field != "criteria" {
		t.Fatalf("text error: %+v", e)
	}
	// Missing revision: 422 revision.
	call("PATCH", path, `{"criteria":{"expected":[],"items":[]}}`, 422, &e)
	if e.Field == nil || *e.Field != "revision" {
		t.Fatalf("revision error: %+v", e)
	}
	var cur api.Ticket
	call("GET", path, "", 200, &cur)
	if cur.Title != "modal" || len(cur.Criteria) != 2 || cur.Criteria[0].Text != "one" {
		t.Fatalf("changed after rejections: %+v", cur)
	}

	// Success.
	var out api.Ticket
	call("PATCH", path, body(tk.Revision, exp(c0, c1), items), 200, &out)
	if out.Title != "renamed" || len(out.Criteria) != 2 || out.Criteria[0].Id != c1.Id || !out.Criteria[0].Done ||
		out.Criteria[0].Position != 1 || out.Criteria[1].Text != "new" || out.Criteria[1].Position != 2 {
		t.Fatalf("saved: %+v", out)
	}

	// Bulk with criteria: 422.
	call("POST", "/tickets/bulk", `{"keys":["`+tk.Key+`"],"patch":{"criteria":{"expected":[],"items":[]}}}`, 422, &e)
	if e.Field == nil || *e.Field != "criteria" {
		t.Fatalf("bulk error: %+v", e)
	}
}
