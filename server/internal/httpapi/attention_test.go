package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/auth"
	"github.com/liamphmurphy/lazyagents/server/internal/realtime"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

func TestAttentionAndCrossTabApproval(t *testing.T) {
	pool := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: &artifacts.Service{Pool: pool}}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	u, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: pool}
	svc := &runs.Service{Pool: pool, Tracker: tr}
	h := realtime.New()
	done := make(chan struct{})
	go func() { defer close(done); h.Run(ctx, pool) }()
	defer func() { cancel(); <-done }()
	srv := &Server{Pool: pool, Q: q, Tracker: tr, Runs: svc, Events: h}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	atlas, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: u.WorkspaceID, Handle: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := agents.IssueToken(ctx, q, atlas.ID, "unmanaged", "test", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := agents.ResolveToken(ctx, q, tok)
	if err != nil {
		t.Fatal(err)
	}
	run, err := svc.Claim(ctx, caller, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	// Managed resumption must queue a turn. Nothing starts a real harness here.
	if _, err = pool.Exec(ctx, "UPDATE runs SET mode='managed' WHERE id=$1", run.ID); err != nil {
		t.Fatal(err)
	}
	outcome, err := svc.RequestApproval(ctx, caller, "PAY-214", "db:migrate", "Test migration", map[string]any{"sql": "SELECT 1"}, "Read only test")
	if err != nil {
		t.Fatal(err)
	}
	ap := outcome.Approval
	// Ensure the actual query scans unions, read state, and seeded suggestions.
	authctx := auth.WithUser(ctx, u)
	inbox, err := srv.ListInbox(authctx, api.ListInboxRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	items := inbox.(api.ListInbox200JSONResponse).Items
	id := "approval:" + ap.ID.String()
	found := false
	for _, item := range items {
		if item.Id == id {
			found = true
			if item.Read || item.Resolved {
				t.Fatal("new approval should be unread and pending")
			}
		}
	}
	if !found {
		t.Fatal("missing approval")
	}
	if _, err = srv.ReadInboxItem(authctx, api.ReadInboxItemRequestObject{Body: &api.ReadInboxItemJSONRequestBody{Id: id}}); err != nil {
		t.Fatal(err)
	}
	inbox, err = srv.ListInbox(authctx, api.ListInboxRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inbox.(api.ListInbox200JSONResponse).Items {
		if item.Id == id && !item.Read {
			t.Fatal("read state did not persist")
		}
	}

	// Human mentions are matched by @email-local-part or @full name, with
	// self-comments excluded and read state isolated per user.
	tk, err := tracker.Get(ctx, q, u.WorkspaceID, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	var commentID string
	err = pool.QueryRow(ctx, `INSERT INTO comments(workspace_id,ticket_id,author_type,author_id,body_md)
 VALUES($1,$2,'agent',$3,$4) RETURNING id::text`, u.WorkspaceID, tk.ID, atlas.ID, "Please review, @"+strings.Split(u.Email, "@")[0]).Scan(&commentID)
	if err != nil {
		t.Fatal(err)
	}
	inbox, err = srv.ListInbox(authctx, api.ListInboxRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, i := range inbox.(api.ListInbox200JSONResponse).Items {
		if i.Id == "mention:"+commentID {
			found = true
		}
	}
	if !found {
		t.Fatal("human mention missing")
	}
	users, err := q.ListUsers(ctx, u.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range users {
		if other.ID == u.ID {
			continue
		}
		otherItems, err := q.ListInbox(ctx, store.ListInboxParams{UserID: other.ID, WorkspaceID: u.WorkspaceID})
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range otherItems {
			if i.ID == id && i.Read {
				t.Fatal("read state leaked between users")
			}
		}
		break
	}
	needs := api.ListTicketsParamsViewNeedsHuman
	tickets, err := srv.ListTickets(authctx, api.ListTicketsRequestObject{Params: api.ListTicketsParams{View: &needs}})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, tk := range tickets.(api.ListTickets200JSONResponse).Tickets {
		if tk.Key == "PAY-214" {
			found = true
		}
	}
	if !found {
		t.Fatal("needs human view missed pending approval")
	}
	// Open two browser-equivalent SSE connections before approving in one tab.
	streams := []chan string{}
	for range 2 {
		streamctx, stop := context.WithCancel(ctx)
		defer stop()
		req, _ := http.NewRequestWithContext(streamctx, "GET", ts.URL+"/events", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		ch := make(chan string, 64)
		streams = append(streams, ch)
		go func() {
			defer close(ch)
			scan := bufio.NewScanner(resp.Body)
			for scan.Scan() {
				line := scan.Text()
				if strings.HasPrefix(line, "data: ") {
					ch <- strings.TrimPrefix(line, "data: ")
				}
			}
		}()
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("stream did not open")
		}
	}
	start := time.Now()
	resp, err := http.Post(ts.URL+"/api/v1/approvals/"+ap.ID.String()+"/approve", "application/json", strings.NewReader(`{"note":"Proceed"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("approval status %d", resp.StatusCode)
	}
	for _, ch := range streams {
		got := false
		deadline := time.After(time.Second)
		for !got {
			select {
			case msg := <-ch:
				got = strings.Contains(msg, "approval.updated")
			case <-deadline:
				t.Fatal("other tab missed approval update")
			}
		}
	}
	if time.Since(start) > time.Second {
		t.Fatal("cross-tab update took over one second")
	}
	turns, err := q.ListTurns(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := turns[len(turns)-1]
	if last.Trigger != "approval" || last.Status != "pending" || !strings.Contains(last.InputMessage, "Proceed") {
		t.Fatalf("no queued resume: %+v", last)
	}
	searchResp, err := http.Get(ts.URL + "/api/v1/search?q=Atlas")
	if err != nil {
		t.Fatal(err)
	}
	defer searchResp.Body.Close()
	var results api.Search200JSONResponse
	if err = json.NewDecoder(searchResp.Body).Decode(&results); err != nil {
		t.Fatal(err)
	}
	found = false
	for _, hit := range results.Hits {
		if hit.Target.Kind == api.LinkTargetKindAgent {
			found = true
		}
	}
	if !found {
		t.Fatal("global search missed agent")
	}
	searchResp2, err := http.Get(ts.URL + "/api/v1/search?q=PAY-214&project=BIL")
	if err != nil {
		t.Fatal(err)
	}
	defer searchResp2.Body.Close()
	if err = json.NewDecoder(searchResp2.Body).Decode(&results); err != nil {
		t.Fatal(err)
	}
	for _, hit := range results.Hits {
		if hit.Target.Kind == api.LinkTargetKindTicket {
			t.Fatal("project scope leaked ticket")
		}
	}
}
