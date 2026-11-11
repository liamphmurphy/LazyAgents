package runner_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A restarted runner gets a new host-pid id: Recover must still requeue turns
// left running by a runner that stopped heartbeating, but not a live one's.
func TestRecoverReclaimsDeadRunnersTurns(t *testing.T) {
	e := setup(t)
	if _, err := e.pool.Exec(e.ctx, `INSERT INTO runners (id, backend, max_concurrent, running, version) VALUES ('other-live', 'fake', 1, 1, 'test')`); err != nil {
		t.Fatal(err)
	}
	n := 0
	turn := func(runnerID string) uuid.UUID {
		// One active run per ticket: give each run its own idle ticket.
		var ticket, run, id uuid.UUID
		if err := e.pool.QueryRow(e.ctx, `SELECT id FROM tickets t WHERE workspace_id=$1
			AND NOT EXISTS (SELECT 1 FROM runs r WHERE r.ticket_id=t.id) ORDER BY number OFFSET $2 LIMIT 1`, e.ws, n).Scan(&ticket); err != nil {
			t.Fatal(err)
		}
		n++
		if err := e.pool.QueryRow(e.ctx, `INSERT INTO runs (workspace_id, ticket_id, agent_id, mode, state, resolved_spec, harness, model, purpose, runner_id)
			VALUES ($1,$2,$3,'managed','running','{}','codex','test','ticket',$4) RETURNING id`, e.ws, ticket, e.atlas.ID, runnerID).Scan(&run); err != nil {
			t.Fatal(err)
		}
		if err := e.pool.QueryRow(e.ctx, `INSERT INTO run_turns (run_id, seq, trigger, status, process_started_at) VALUES ($1, 1, 'start', 'running', now()) RETURNING id`, run).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	dead, live := turn("gone-host-1234"), turn("other-live")
	if err := e.runner.Recover(e.ctx); err != nil {
		t.Fatal(err)
	}
	status := func(id uuid.UUID) (s string) {
		if err := e.pool.QueryRow(e.ctx, `SELECT status FROM run_turns WHERE id=$1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if s := status(dead); s != "pending" {
		t.Fatalf("dead runner's turn: %s, want pending", s)
	}
	if s := status(live); s != "running" {
		t.Fatalf("live runner's turn: %s, want running", s)
	}
}

// Runs an older runner stranded (running, their turn finished, nothing queued)
// get a resume turn; a live runner's run between turns is left alone.
func TestRecoverResumesStrandedRuns(t *testing.T) {
	e := setup(t)
	if _, err := e.pool.Exec(e.ctx, `INSERT INTO runners (id, backend, max_concurrent, running, version) VALUES ('other-live', 'fake', 1, 1, 'test')`); err != nil {
		t.Fatal(err)
	}
	n := 0
	stranded := func(runnerID string) uuid.UUID {
		var ticket, run uuid.UUID
		if err := e.pool.QueryRow(e.ctx, `SELECT id FROM tickets t WHERE workspace_id=$1
			AND NOT EXISTS (SELECT 1 FROM runs r WHERE r.ticket_id=t.id) ORDER BY number OFFSET $2 LIMIT 1`, e.ws, n).Scan(&ticket); err != nil {
			t.Fatal(err)
		}
		n++
		if err := e.pool.QueryRow(e.ctx, `INSERT INTO runs (workspace_id, ticket_id, agent_id, mode, state, resolved_spec, harness, model, purpose, runner_id)
			VALUES ($1,$2,$3,'managed','running','{}','codex','test','ticket',$4) RETURNING id`, e.ws, ticket, e.atlas.ID, runnerID).Scan(&run); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(e.ctx, `INSERT INTO run_turns (run_id, seq, trigger, status, process_started_at, ended_at, exit_code) VALUES ($1, 1, 'start', 'done', now(), now(), 137)`, run); err != nil {
			t.Fatal(err)
		}
		return run
	}
	dead, live := stranded("gone-host-1234"), stranded("other-live")
	if err := e.runner.Recover(e.ctx); err != nil {
		t.Fatal(err)
	}
	turns := func(id uuid.UUID) []store.RunTurn {
		ts, err := e.q.ListTurns(e.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return ts
	}
	if ts := turns(dead); len(ts) != 2 || ts[1].Status != "pending" || ts[1].InputMessage != runner.StrandedResumeMessage {
		t.Fatalf("dead runner's stranded run: %+v", ts)
	}
	if ts := turns(live); len(ts) != 1 {
		t.Fatalf("live runner's run got %d turns, want 1", len(ts))
	}
	// Recover is idempotent: the resume turn is pending, so no second one.
	if err := e.runner.Recover(e.ctx); err != nil {
		t.Fatal(err)
	}
	if ts := turns(dead); len(ts) != 2 {
		t.Fatalf("second Recover queued another turn: %d", len(ts))
	}
}
