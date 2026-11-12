package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// A ticket run that follows a failed one learns how it ended, what it planned
// and where its work was saved, and sees nothing from other tickets.
func TestTicketRunReadsPriorAttempts(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	tr := &tracker.Service{Pool: pool}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}, Artifacts: arts}).Handler()

	mkTicket := func(title string) store.Ticket {
		t.Helper()
		tk, err := tr.Create(ctx, ws, tracker.UserActor(me.ID), tracker.NewTicket{ProjectKey: "PAY", Title: title, AssigneeAgentID: &atlas.ID})
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}
	mkRun := func(ticket store.Ticket, parent *uuid.UUID, startedAgo time.Duration) store.Run {
		t.Helper()
		r, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: ws, TicketID: &ticket.ID, AgentID: atlas.ID, Mode: "managed", State: "running", ResolvedSpec: json.RawMessage(`{}`), Harness: "claude", Model: "test", ParentRunID: parent, Purpose: "ticket"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE runs SET started_at = now() - $2::interval WHERE id = $1`, r.ID, startedAgo.String()); err != nil {
			t.Fatal(err)
		}
		return r
	}
	finish := func(r store.Run, state, outcome string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE runs SET state = $2, outcome = $3, ended_at = now() WHERE id = $1`, r.ID, state, outcome); err != nil {
			t.Fatal(err)
		}
	}

	ticket := mkTicket("Partition the ledger")
	first := mkRun(ticket, nil, 3*time.Hour)
	finish(first, "cancelled", "Stopped by the person")
	second := mkRun(ticket, nil, 2*time.Hour)
	for i, step := range []string{"Write the migration", "Backfill in batches"} {
		state := "done"
		if i == 1 {
			state = "failed"
		}
		if err := q.InsertPlanStep(ctx, store.InsertPlanStepParams{RunID: second.ID, Position: int32(i + 1), Text: step, State: state}); err != nil {
			t.Fatal(err)
		}
	}
	child := mkRun(ticket, &second.ID, 90*time.Minute)
	finish(child, "failed", "Sub agent gave up")
	finish(second, "failed", "Backfill timed out on the 2019 partition")
	if err := q.SetRunCheckpoint(ctx, store.SetRunCheckpointParams{ID: second.ID, Branch: ptr("vesper/checkpoint/pay-ledger")}); err != nil {
		t.Fatal(err)
	}
	mkRun(mkTicket("Someone else's ticket"), nil, 4*time.Hour)
	current := mkRun(ticket, nil, time.Minute)
	token, _, err := agents.IssueToken(ctx, q, atlas.ID, "run", "lead", &current.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	call := func(name string, args map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(args)
		req := httptest.NewRequest("POST", "/api/v1/agent/tools/"+name, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	// Claiming says there's history to read before planning.
	w := call("claim_ticket", map[string]any{"ticket": ticket.Key})
	var claimed map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &claimed)
	if w.Code != 200 || claimed["prior_attempts"] != float64(2) || !strings.Contains(fmt.Sprint(claimed["next"]), "get_prior_attempts") {
		t.Fatalf("claim_ticket = %d %v", w.Code, claimed)
	}

	w = call("get_prior_attempts", map[string]any{})
	var out struct {
		Ticket   string             `json:"ticket"`
		Attempts []priorAttemptJSON `json:"attempts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 {
		t.Fatalf("get_prior_attempts = %d %s", w.Code, w.Body.String())
	}
	if out.Ticket != ticket.Key || len(out.Attempts) != 2 {
		t.Fatalf("attempts = %+v", out)
	}
	latest := out.Attempts[0]
	if latest.RunID != second.ID || latest.State != "failed" || latest.Outcome != "Backfill timed out on the 2019 partition" || latest.SavedWorkBranch != "vesper/checkpoint/pay-ledger" || latest.Agent != "atlas" ||
		len(latest.Plan) != 2 || latest.Plan[1]["state"] != "failed" {
		t.Fatalf("latest attempt = %+v", latest)
	}
	if out.Attempts[1].RunID != first.ID || out.Attempts[1].State != "cancelled" {
		t.Fatalf("first attempt = %+v", out.Attempts[1])
	}
}

// priorAttemptJSON mirrors the tool's JSON for the assertions above.
type priorAttemptJSON struct {
	RunID           uuid.UUID        `json:"run_id"`
	Agent           string           `json:"agent"`
	State           string           `json:"state"`
	Outcome         string           `json:"outcome"`
	SavedWorkBranch string           `json:"saved_work_branch"`
	Plan            []map[string]any `json:"plan"`
}
