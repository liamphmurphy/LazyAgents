package runner_test

import (
	"testing"

	"github.com/google/uuid"
)

// A restarted runner gets a new host-pid id: Recover must still requeue turns
// left running by a runner that stopped heartbeating, but not a live one's.
func TestRecoverReclaimsDeadRunnersTurns(t *testing.T) {
	e := setup(t)
	if _, err := e.pool.Exec(e.ctx, `INSERT INTO runners (id, backend, max_concurrent, running, version) VALUES ('other-live', 'local', 1, 1, 'test')`); err != nil {
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
