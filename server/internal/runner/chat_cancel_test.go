package runner_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/liamphmurphy/lazyagents/server/internal/runner"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Pause immediately before the pending UPDATE reaches PostgreSQL. With the
// original ordering the active SELECT has already returned false here.
type cancelBoundary struct{ reached, release chan struct{} }

func (b *cancelBoundary) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "-- name: DropPendingTurns") {
		close(b.reached)
		select {
		case <-b.release:
		case <-ctx.Done():
		}
	}
	return ctx
}
func (*cancelBoundary) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Keep the actual fake process alive in the runner until the drain assertion.
type heldChatBackend struct {
	runner.Backend
	started, release chan struct{}
	once             sync.Once
}

func (b *heldChatBackend) Start(ctx context.Context, id string, spec runner.ExecSpec) (runner.Process, error) {
	p, err := b.Backend.Start(ctx, id, spec)
	if err == nil {
		b.once.Do(func() {
			close(b.started)
			select {
			case <-b.release:
			case <-ctx.Done():
			}
		})
	}
	return p, err
}

func TestChatCancelClaimBoundary(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	ctx, cancel := context.WithTimeout(e.ctx, 20*time.Second)
	defer cancel()
	chat := e.startChat("Warm up")
	e.until("warmup", func() bool { return e.chat(chat.ID).State == "idle" })
	if _, err := e.runs.PostChatMessage(ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "slow"}); err != nil {
		t.Fatal(err)
	}
	boundary := &cancelBoundary{make(chan struct{}), make(chan struct{})}
	cfg := e.pool.Config()
	cfg.ConnConfig.Tracer = boundary
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	svc := *e.runs
	svc.Pool = pool
	gate := &heldChatBackend{Backend: e.runner.Backend, started: make(chan struct{}), release: make(chan struct{})}
	e.runner.Backend = gate
	var releaseCancel, releaseProcess sync.Once
	defer releaseCancel.Do(func() { close(boundary.release) })
	defer releaseProcess.Do(func() { close(gate.release) })
	result := make(chan error, 1)
	go func() { _, err := svc.CancelChat(ctx, e.ws, e.me.ID, chat.ID); result <- err }()
	select {
	case <-boundary.reached:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// The cancellation owns session/run locks, but the claimant locks only
	// the turn. Start the real fake harness before allowing cancellation on.
	e.runner.Tick(ctx)
	// Issuing the turn token takes a foreign-key lock on the run. This
	// proves the worker loaded the pre-cancel running state before release.
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for {
		var blocked bool
		err := e.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
		 WHERE datname=current_database() AND query LIKE '%INSERT INTO agent_tokens%'
		 AND cardinality(pg_blocking_pids(pid)) > 0)`).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case <-poll.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	releaseCancel.Do(func() { close(boundary.release) })
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if e.chat(chat.ID).State != "running" || e.chatRun(chat.ID).State != "paused" {
		t.Fatal("cancel missed the winning claimant")
	}
	_, err = e.runs.PostChatMessage(ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "Still draining"})
	requireConflict(t, err)
	releaseProcess.Do(func() { close(gate.release) })
	e.runner.Wait()
	if e.chat(chat.ID).State != "idle" {
		t.Fatal("cancel did not drain")
	}
	if _, err = e.runs.PostChatMessage(ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "After boundary"}); err != nil {
		t.Fatal(err)
	}
	e.until("reply after boundary", func() bool { return e.chat(chat.ID).State == "idle" })
	msgs, err := e.q.ListChatMessages(ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 5 || msgs[4].Role != "assistant" || !strings.Contains(msgs[4].BodyMd, "After boundary") {
		t.Fatalf("history %+v", msgs)
	}
	turns, err := e.q.ListTurns(ctx, e.chatRun(chat.ID).ID)
	if err != nil || len(turns) != 3 {
		t.Fatalf("turns %+v: %v", turns, err)
	}
	for _, turn := range turns {
		if turn.Status != "done" {
			t.Fatalf("stranded turn %+v", turn)
		}
	}
}

// Exercise PostgreSQL's UPDATE recheck when a claimant is still uncommitted:
// DropPendingTurns must wait, skip the newly running row, and the next statement
// must see the claimant's commit under the service's default READ COMMITTED.
func TestChatCancelWaitsForClaimCommit(t *testing.T) {
	e := setup(t)
	ctx, cancel := context.WithTimeout(e.ctx, 15*time.Second)
	defer cancel()
	chat := e.startChat("Pending")
	run := e.chatRun(chat.ID)
	if _, err := e.pool.Exec(ctx, "UPDATE runs SET state='running' WHERE id=$1", run.ID); err != nil {
		t.Fatal(err)
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	turn, err := store.New(tx).ClaimPendingTurn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := e.runs.CancelChat(ctx, e.ws, e.me.ID, chat.ID); result <- err }()
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for {
		var blocked bool
		if err := e.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND query LIKE '%-- name: DropPendingTurns%' AND cardinality(pg_blocking_pids(pid)) > 0)`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case <-poll.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if e.chatRun(chat.ID).State != "paused" || e.chat(chat.ID).State != "running" {
		t.Fatal("uncommitted claimant missed")
	}
	got, err := e.q.GetTurn(ctx, turn.ID)
	if err != nil || got.Status != "running" {
		t.Fatalf("claimed turn %+v: %v", got, err)
	}
	_, err = e.runs.PostChatMessage(ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "Busy"})
	requireConflict(t, err)
}
