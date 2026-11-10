package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

func TestIdleRunnerPollingDoesNotNotify(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	var runID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO runs(workspace_id,ticket_id,agent_id,mode,state,resolved_spec,harness,model,purpose)
		SELECT t.workspace_id,t.id,a.id,'managed','completed','{}','codex','test','ticket'
		FROM tickets t JOIN agents a ON a.workspace_id=t.workspace_id
		WHERE t.key='PAY-214' AND a.handle='atlas' RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	// A quiet queue, including the recovery queries used at runner startup.
	if _, err := pool.Exec(ctx, "UPDATE runs SET state='completed'"); err != nil {
		t.Fatal(err)
	}
	listener, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Release()
	if _, err = listener.Exec(ctx, "LISTEN lazyagents_changes"); err != nil {
		t.Fatal(err)
	}
	runner := "idle-test"
	for i := 0; i < 3; i++ {
		if _, err = q.ClaimQueuedRun(ctx, store.ClaimQueuedRunParams{RunnerID: &runner}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("claim queued: %v", err)
		}
		if _, err = q.ClaimPendingTurn(ctx, store.ClaimPendingTurnParams{}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("claim turn: %v", err)
		}
	}
	for _, sql := range []string{
		"UPDATE runs SET state=state WHERE false", "DELETE FROM runs WHERE false",
		"UPDATE run_turns SET status=status WHERE false", "DELETE FROM run_turns WHERE false",
		"UPDATE plan_steps SET state=state WHERE false", "DELETE FROM plan_steps WHERE false",
	} {
		if _, err = pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	quiet, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	notification, err := listener.Conn().WaitForNotification(quiet)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("idle polling notified: %+v %v", notification, err)
	}
	// Real changes must still reach subscribers, carrying the affected run id.
	if _, err = pool.Exec(ctx, "UPDATE runs SET outcome='updated' WHERE id=$1", runID); err != nil {
		t.Fatal(err)
	}
	wait, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	notification, err = listener.Conn().WaitForNotification(wait)
	if err != nil {
		t.Fatal(err)
	}
	var message struct {
		Type string   `json:"type"`
		IDs  []string `json:"ids"`
	}
	if err = json.Unmarshal([]byte(notification.Payload), &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "run.updated" || len(message.IDs) != 1 || message.IDs[0] != runID.String() {
		t.Fatalf("%+v", message)
	}
}
