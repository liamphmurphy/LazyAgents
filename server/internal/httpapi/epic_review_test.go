package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func TestCreateEpicReviewEndpoint(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	tr := &tracker.Service{Pool: pool}
	h := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: &runs.Service{Pool: pool, Tracker: tr}}).Handler()
	call := func(method, path string, want int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, "/api/v1"+path, strings.NewReader("")))
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	u, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var epic uuid.UUID
	if err = pool.QueryRow(ctx, `INSERT INTO epics(workspace_id,project_id,name) SELECT $1,id,'Ledger' FROM projects WHERE workspace_id=$1 AND key='PAY' RETURNING id`, u.WorkspaceID).Scan(&epic); err != nil {
		t.Fatal(err)
	}
	if _, err = tr.Create(ctx, u.WorkspaceID, tracker.UserActor(u.ID), tracker.NewTicket{ProjectKey: "PAY", Title: "open work", EpicID: &epic}); err != nil {
		t.Fatal(err)
	}
	path := "/epics/" + epic.String() + "/review"
	var first, second api.WorkTicket
	if err := json.Unmarshal(call("POST", path, 200), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(call("POST", path, 200), &second); err != nil {
		t.Fatal(err)
	}
	if first.Key == "" || first.Key != second.Key || first.Id != second.Id {
		t.Fatalf("not idempotent: %+v %+v", first, second)
	}
	var tk api.Ticket
	if err := json.Unmarshal(call("GET", "/tickets/"+first.Key, 200), &tk); err != nil {
		t.Fatal(err)
	}
	if !tk.EpicReview {
		t.Fatalf("epic_review not set: %+v", tk)
	}
	var board api.Board
	if err := json.Unmarshal(call("GET", "/projects/PAY/board?epic="+epic.String(), 200), &board); err != nil {
		t.Fatal(err)
	}
	if board.Epic == nil || board.Epic.Review == nil || board.Epic.Review.Key != first.Key {
		t.Fatalf("board review: %+v", board.Epic)
	}
	call("POST", "/epics/"+uuid.NewString()+"/review", 404)
}
