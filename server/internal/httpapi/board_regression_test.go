package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestBoardErrorContractsAndWorkspaceIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	tr := &tracker.Service{Pool: pool}
	h := (&Server{Pool: pool, Q: q, Tracker: tr, Runs: &runs.Service{Pool: pool, Tracker: tr}}).Handler()
	call := func(method, path, body string, want int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: %d want%d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	for _, tc := range []struct{ body, field string }{{`{"estimate_points":4}`, "estimate_points"}, {`{"status":"invalid"}`, "status"}} {
		var e api.Error
		if err := json.Unmarshal(call("PATCH", "/tickets/PAY-214", tc.body, 422), &e); err != nil {
			t.Fatal(err)
		}
		if e.Field == nil || *e.Field != tc.field {
			t.Fatalf("wrong invalid field: %+v", e)
		}
	}
	u, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, err := q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: u.WorkspaceID, Key: "PAY"})
	if err != nil {
		t.Fatal(err)
	}
	var epic uuid.UUID
	if err = pool.QueryRow(ctx, `INSERT INTO epics(workspace_id,project_id,name) VALUES($1,$2,'Gate regression') RETURNING id`, u.WorkspaceID, p.ID).Scan(&epic); err != nil {
		t.Fatal(err)
	}
	criterion, err := tr.CreateEpicCriterion(ctx, u.WorkspaceID, epic, "must check")
	if err != nil {
		t.Fatal(err)
	}
	ep := "/epics/" + epic.String()
	// No child exists: an unchecked criterion alone blocks closing.
	msg := string(call("PATCH", ep, `{"status":"done"}`, 409))
	if !strings.Contains(msg, "criteria [1]") {
		t.Fatalf("missing unchecked position: %s", msg)
	}
	yes := true
	if _, err = tr.PatchEpicCriterion(ctx, u.WorkspaceID, tracker.UserActor(u.ID), criterion.ID, nil, &yes, nil); err != nil {
		t.Fatal(err)
	}
	child, err := tr.Create(ctx, u.WorkspaceID, tracker.UserActor(u.ID), tracker.NewTicket{ProjectKey: "PAY", Title: "Only open blocker", EpicID: &epic})
	if err != nil {
		t.Fatal(err)
	}
	// All criteria checked: an open child alone blocks closing and is named.
	msg = string(call("PATCH", ep, `{"status":"done"}`, 409))
	if !strings.Contains(msg, child.Key) {
		t.Fatalf("missing child key: %s", msg)
	}
	call("PATCH", "/tickets/"+child.Key, `{"status":"canceled"}`, 200)
	call("PATCH", ep, `{"status":"done"}`, 200)
	// A foreign workspace must be invisible on reads and all mutation endpoints.
	var foreignWS, foreignProject, foreignEpic, foreignCriterion uuid.UUID
	if err = pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('Foreign','foreign') RETURNING id`).Scan(&foreignWS); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO projects(workspace_id,key,name,color) VALUES($1,'FOREIGN','Foreign','#123456') RETURNING id`, foreignWS).Scan(&foreignProject); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO epics(workspace_id,project_id,name) VALUES($1,$2,'Foreign epic') RETURNING id`, foreignWS, foreignProject).Scan(&foreignEpic); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO epic_criteria(epic_id,position,text) VALUES($1,1,'Foreign criterion') RETURNING id`, foreignEpic).Scan(&foreignCriterion); err != nil {
		t.Fatal(err)
	}
	foreignEP := "/epics/" + foreignEpic.String()
	foreignCP := "/epic-criteria/" + foreignCriterion.String()
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/projects/FOREIGN/board", ""},
		{"GET", "/projects/PAY/board?epic=" + foreignEpic.String(), ""},
		{"PATCH", foreignEP, `{"status":"done","requires_mockup":true}`},
		{"GET", foreignEP + "/criteria", ""},
		{"POST", foreignEP + "/criteria", `{"text":"intrusion"}`},
		{"PATCH", foreignCP, `{"text":"intrusion","done":true}`},
		{"DELETE", foreignCP, ""},
	} {
		call(tc.method, tc.path, tc.body, 404)
	}
	var status, text string
	var requires, done bool
	var count int
	if err = pool.QueryRow(ctx, `SELECT e.status,e.requires_mockup,c.text,c.done,(SELECT count(*) FROM epic_criteria WHERE epic_id=e.id) FROM epics e JOIN epic_criteria c ON c.epic_id=e.id WHERE c.id=$1`, foreignCriterion).Scan(&status, &requires, &text, &done, &count); err != nil {
		t.Fatal(err)
	}
	if status != "open" || requires || text != "Foreign criterion" || done || count != 1 {
		t.Fatal(fmt.Sprintf("foreign state changed: %s %v %s %v %d", status, requires, text, done, count))
	}
}
