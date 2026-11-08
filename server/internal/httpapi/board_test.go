package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func TestBoardAndEpicCriteriaHTTP(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	q := store.New(pool)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: &artifacts.Service{Pool: pool}}); err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: pool}
	s := &Server{Pool: pool, Q: q, Tracker: tr, Runs: &runs.Service{Pool: pool, Tracker: tr}}
	handler := s.Handler()
	call := func(method, path, body string, want int, dst any) {
		t.Helper()
		r := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
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
	u, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, err := q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: u.WorkspaceID, Key: "PAY"})
	if err != nil {
		t.Fatal(err)
	}
	var epicID string
	if err = pool.QueryRow(ctx, `INSERT INTO epics(workspace_id,project_id,name) VALUES($1,$2,'Board test') RETURNING id`, u.WorkspaceID, p.ID).Scan(&epicID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO tickets(workspace_id,project_id,number,key,title,status,priority,epic_id,estimate_points,sort_order) SELECT $1,$2,10000+n,'BOARD-'||n,'Card '||n,CASE WHEN n<=510 THEN 'todo' WHEN n<=565 THEN 'done' ELSE 'canceled' END,CASE WHEN n=2 THEN 'urgent' ELSE 'none' END,$3,2,n FROM generate_series(1,620) n`, u.WorkspaceID, p.ID, epicID)
	if err != nil {
		t.Fatal(err)
	}
	path := "/projects/PAY/board?epic=" + epicID
	var board api.Board
	call("GET", path, "", 200, &board)
	if len(board.Columns) != 5 || board.Columns[1].Count != 510 || len(board.Columns[1].Cards) != 510 || board.Columns[1].Cards[0].Key != "BOARD-2" || board.Columns[1].Points != 1020 || board.Columns[4].Count != 55 || len(board.Columns[4].Cards) != 50 || !board.Columns[4].Truncated {
		t.Fatalf("board columns: %+v", board.Columns)
	}
	if board.Epic == nil || board.Epic.Epic.TicketsTotal != 565 || board.Epic.Epic.PointsTotal != 1130 || board.Epic.Epic.TicketsOpen != 510 {
		t.Fatalf("epic: %+v", board.Epic)
	}
	call("GET", path+"&include_canceled=true", "", 200, &board)
	if len(board.Columns) != 6 || board.Columns[5].Count != 55 || len(board.Columns[5].Cards) != 50 || board.Columns[5].Points != 110 {
		t.Fatalf("canceled: %+v", board.Columns)
	}
	call("GET", path+"&assignee=missing", "", 200, &board)
	if board.Columns[1].Count != 0 {
		t.Fatal("assignee filter")
	}
	call("GET", "/projects/NOPE/board", "", 404, nil)
	projects, err := q.ListProjects(ctx, u.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range projects {
		if other.ID != p.ID {
			call("GET", "/projects/"+other.Key+"/board?epic="+epicID, "", 404, nil)
			break
		}
	}
	call("GET", "/projects/PAY/board?epic=00000000-0000-0000-0000-000000000001", "", 404, nil)

	// A canceled blocker remains unmet even though it is hidden by default.
	_, err = pool.Exec(ctx, `INSERT INTO ticket_dependencies(ticket_id,blocker_id) SELECT a.id,b.id FROM tickets a,tickets b WHERE a.workspace_id=$1 AND b.workspace_id=$1 AND a.key='BOARD-2' AND b.key='BOARD-566'`, u.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	call("GET", path, "", 200, &board)
	if board.Columns[1].Cards[0].UnmetDependencies != 1 {
		t.Fatal("canceled blocker was satisfied")
	}
	var ticket api.Ticket
	call("GET", "/tickets/BOARD-1", "", 200, &ticket)
	revision := ticket.Revision
	listener, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Release()
	if _, err = listener.Exec(ctx, "LISTEN lazyagents_changes"); err != nil {
		t.Fatal(err)
	}
	call("PATCH", "/tickets/BOARD-1", fmt.Sprintf(`{"revision":%d}`, revision), 200, &ticket)
	if ticket.Revision != revision {
		t.Fatal("no-op bumped revision")
	}
	call("PATCH", "/tickets/BOARD-1", fmt.Sprintf(`{"revision":%d,"estimate_points":3}`, revision), 200, &ticket)
	notifyCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	found := false
	for !found {
		n, err := listener.Conn().WaitForNotification(notifyCtx)
		if err != nil {
			t.Fatal(err)
		}
		var event struct {
			Type string   `json:"type"`
			IDs  []string `json:"ids"`
		}
		if err = json.Unmarshal([]byte(n.Payload), &event); err != nil {
			t.Fatal(err)
		}
		for _, id := range event.IDs {
			if event.Type == "ticket.updated" && id == ticket.Id.String() {
				found = true
			}
		}
	}
	if ticket.Revision != revision+1 || ticket.EstimatePoints == nil || *ticket.EstimatePoints != 3 {
		t.Fatalf("estimate patch: %+v", ticket)
	}
	call("PATCH", "/tickets/BOARD-1", fmt.Sprintf(`{"revision":%d}`, revision), 409, nil)
	call("PATCH", "/tickets/BOARD-1", `{"estimate_points":4}`, 422, nil)
	call("PATCH", "/tickets/BOARD-1", `{"status":"invalid"}`, 422, nil)
	call("POST", "/tickets/bulk", `{"keys":["BOARD-1"],"patch":{"revision":1}}`, 422, nil)

	atlas, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: u.WorkspaceID, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	row, err := tracker.Get(ctx, q, u.WorkspaceID, "BOARD-1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: u.WorkspaceID, TicketID: &row.ID, AgentID: atlas.ID, Mode: "managed", State: "queued", ResolvedSpec: json.RawMessage(`{}`), Harness: "codex", Model: "fake", Purpose: "ticket"})
	if err != nil {
		t.Fatal(err)
	}
	call("PATCH", "/tickets/BOARD-1", `{"status":"canceled"}`, 409, nil)

	var c1, c2 api.EpicCriterion
	ep := "/epics/" + epicID
	call("POST", ep+"/criteria", `{"text":"First"}`, 201, &c1)
	call("POST", ep+"/criteria", `{"text":"Second"}`, 201, &c2)
	cp := "/epic-criteria/" + c2.Id.String()
	call("PATCH", cp, `{"position":1,"text":"Edited","done":true}`, 200, &c2)
	if c2.Position != 1 || !c2.Done || c2.CheckedBy == nil || c2.CheckedBy.Id == nil || *c2.CheckedBy.Id != u.ID || c2.CheckedAt == nil {
		t.Fatalf("criterion: %+v", c2)
	}
	call("PATCH", cp, `{"position":0}`, 422, nil)
	call("PATCH", cp, `{"text":" "}`, 422, nil)
	call("PATCH", cp, fmt.Sprintf(`{"text":%q}`, strings.Repeat("x", 1001)), 422, nil)
	call("PATCH", ep, `{"status":"done"}`, 409, nil)
	call("GET", path, "", 200, &board)
	if len(board.Epic.DoneBlockers.OpenTickets) != 510 || len(board.Epic.DoneBlockers.UncheckedCriteria) != 1 {
		t.Fatalf("blockers: %+v", board.Epic.DoneBlockers)
	}
	c2 = api.EpicCriterion{}
	call("PATCH", cp, `{"done":false}`, 200, &c2)
	if c2.CheckedBy != nil || c2.CheckedAt != nil {
		t.Fatal("uncheck attribution")
	}
	call("DELETE", cp, "", 204, nil)
	call("DELETE", cp, "", 404, nil)
	// An empty epic can close and must reject criterion changes until reopened.
	var emptyID string
	if err = pool.QueryRow(ctx, `INSERT INTO epics(workspace_id,project_id,name) VALUES($1,$2,'Empty') RETURNING id`, u.WorkspaceID, p.ID).Scan(&emptyID); err != nil {
		t.Fatal(err)
	}
	empty := "/epics/" + emptyID
	var e api.Epic
	call("PATCH", empty, `{"status":"done"}`, 200, &e)
	if e.Status != api.EpicStatusDone {
		t.Fatal(e.Status)
	}
	call("POST", empty+"/criteria", `{"text":"x"}`, 409, nil)
	call("PATCH", empty, `{"status":"open"}`, 200, &e)
	var c3 api.EpicCriterion
	call("POST", empty+"/criteria", `{"text":"x"}`, 201, &c3)
	call("PATCH", "/epic-criteria/"+c3.Id.String(), `{"done":true}`, 200, nil)
	call("PATCH", empty, `{"status":"done"}`, 200, nil)
	call("PATCH", "/epic-criteria/"+c3.Id.String(), `{"done":false}`, 409, nil)
	call("DELETE", "/epic-criteria/"+c3.Id.String(), "", 409, nil)
	call("PATCH", empty, `{"status":"open"}`, 200, nil)
	call("DELETE", "/epic-criteria/"+c3.Id.String(), "", 204, nil)
}
