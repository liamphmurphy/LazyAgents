package runner

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/harness"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

func completionTest(t *testing.T) (*Runner, runCtx, store.RunTurn) {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	r := &Runner{Pool: pool, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var ws, user, agent uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'U','u@example.com','U','x') RETURNING id`, ws).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO agents(workspace_id,name,handle) VALUES($1,'A','assistant') RETURNING id`, ws).Scan(&agent); err != nil {
		t.Fatal(err)
	}
	run, err := r.q().CreateRun(ctx, store.CreateRunParams{WorkspaceID: ws, AgentID: agent, Mode: "managed", State: "running", ResolvedSpec: []byte(`{}`), Harness: "claude-code", Model: "test", Purpose: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := r.q().CreateChatSession(ctx, store.CreateChatSessionParams{WorkspaceID: ws, CreatedBy: user, AgentID: agent, Title: "Test", Harness: "claude-code", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.q().SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, CurrentRunID: &run.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.q().StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "comment", InputMessage: "Answer"}); err != nil {
		t.Fatal(err)
	}
	token, expires := uuid.New(), time.Now().Add(executionTTL)
	turn, err := r.q().ClaimPendingTurn(ctx, store.ClaimPendingTurnParams{ExecutionToken: &token, ExecutionExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	return r, runCtx{run: run}, turn
}

func TestChatCompletionIsIdempotent(t *testing.T) {
	r, c, turn := completionTest(t)
	ctx := context.WithValue(context.Background(), executionKey{}, turn.ExecutionToken)
	res := harness.Result{Final: "Only once", Usage: harness.Usage{ToolCalls: 2, InputTokens: 10}}
	for range 2 {
		r.finishChatTurn(ctx, c, turn, res, 0, nil, nil, chatTurnCode{}, chatImages{})
	}
	chat, err := r.q().ChatSessionForRun(ctx, &c.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := r.q().ListChatMessages(ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 10})
	if err != nil || len(msgs) != 1 || msgs[0].BodyMd != res.Final {
		t.Fatalf("duplicate completion: %+v, %v", msgs, err)
	}
	run, err := r.q().GetRun(ctx, store.GetRunParams{ID: c.run.ID, WorkspaceID: c.run.WorkspaceID})
	if err != nil || run.ToolCalls != 2 || run.InputTokens != 10 {
		t.Fatalf("duplicate usage: %+v, %v", run, err)
	}
}

func TestLostExecutionCannotCompleteOrFailNewOwnersTurn(t *testing.T) {
	r, c, turn := completionTest(t)
	ctx := context.WithValue(context.Background(), executionKey{}, turn.ExecutionToken)
	newToken := uuid.New()
	if _, err := r.Pool.Exec(ctx, "UPDATE run_turns SET execution_token=$2 WHERE id=$1", turn.ID, newToken); err != nil {
		t.Fatal(err)
	}
	r.finishChatTurn(ctx, c, turn, harness.Result{Final: "Stale answer"}, 0, nil, nil, chatTurnCode{}, chatImages{})
	r.failChat(ctx, c, "Stale failure")
	chat, err := r.q().ChatSessionForRun(ctx, &c.run.ID)
	if err != nil || chat.State != "running" {
		t.Fatalf("stale owner changed chat: %+v, %v", chat, err)
	}
	msgs, err := r.q().ListChatMessages(ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 10})
	if err != nil || len(msgs) != 0 {
		t.Fatalf("stale owner stored messages: %+v, %v", msgs, err)
	}
	got, err := r.q().GetTurn(ctx, turn.ID)
	if err != nil || got.Status != "running" || got.ExecutionToken == nil || *got.ExecutionToken != newToken {
		t.Fatalf("stale owner finished new claim: %+v, %v", got, err)
	}
}

func TestLeaseLossStopsProcessBeforeReclaim(t *testing.T) {
	r, _, turn := completionTest(t)
	expires := time.Now().Add(15*time.Second + 100*time.Millisecond)
	if _, err := r.Pool.Exec(context.Background(), "UPDATE run_turns SET execution_expires_at=$2 WHERE id=$1", turn.ID, expires); err != nil {
		t.Fatal(err)
	}
	b := &Local{Dir: t.TempDir()}
	if err := b.EnsureWorkspace(context.Background(), turn.RunID.String()); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r.execute(context.Background(), turn.ID, true, turn.ExecutionToken, &expires, func(ctx context.Context) {
		p, err := b.Start(ctx, turn.RunID.String(), ExecSpec{Argv: []string{"sleep", "30"}})
		if err != nil {
			t.Fatal(err)
		}
		defer p.(io.Closer).Close()
		_, _ = p.Wait()
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatal("lease safety deadline did not cancel process")
		}
	})
	if time.Since(start) > 2*time.Second {
		t.Fatal("lost lease did not stop process promptly")
	}
	got, err := r.q().GetTurn(context.Background(), turn.ID)
	if err != nil || got.ExecutionExpiresAt == nil || got.ExecutionExpiresAt.After(time.Now()) {
		t.Fatalf("worker exit did not release lease: %+v, %v", got, err)
	}
	until := time.Now().Add(executionTTL)
	_, err = r.q().RenewTurnExecution(context.Background(), store.RenewTurnExecutionParams{ID: turn.ID, Token: turn.ExecutionToken, ExpiresAt: &until})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired lease renewed: %v", err)
	}
}

func TestExecutionLeaseRenewsWhileWorkerRuns(t *testing.T) {
	r, _, turn := completionTest(t)
	r.execute(context.Background(), turn.ID, true, turn.ExecutionToken, turn.ExecutionExpiresAt, func(ctx context.Context) {
		deadline := time.NewTimer(12 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-deadline.C:
				t.Fatal("worker did not renew its execution lease")
			case <-ctx.Done():
				t.Fatal("worker lost a renewable execution lease")
			case <-tick.C:
				got, err := r.q().GetTurn(ctx, turn.ID)
				if err != nil {
					t.Fatal(err)
				}
				if got.ExecutionExpiresAt != nil && got.ExecutionExpiresAt.After(*turn.ExecutionExpiresAt) {
					return
				}
			}
		}
	})
}
