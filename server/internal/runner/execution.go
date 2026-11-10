package runner

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const executionTTL = time.Minute

type executionKey struct{}

func executionToken(ctx context.Context) *uuid.UUID {
	token, _ := ctx.Value(executionKey{}).(*uuid.UUID)
	return token
}

func verifyExecution(ctx context.Context, q *store.Queries, runID uuid.UUID) error {
	if token := executionToken(ctx); token != nil {
		owned, err := q.OwnsRunExecution(ctx, store.OwnsRunExecutionParams{RunID: runID, Token: token})
		if err != nil {
			return err
		}
		if !owned {
			return errors.New("execution lease was lost")
		}
	}
	return nil
}

// execute keeps a claim alive only while its worker exists. Losing the lease
// cancels the process before another runner may reclaim it. Releasing an
// unfinished claim makes it eligible for recovery even if this runner is live.
func (r *Runner) execute(ctx context.Context, id uuid.UUID, turn bool, token *uuid.UUID, expires *time.Time, work func(context.Context)) {
	ctx, cancel := context.WithCancel(context.WithValue(ctx, executionKey{}, token))
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		defer cancel()
		guard := time.NewTimer(time.Until(expires.Add(-15 * time.Second)))
		defer guard.Stop()
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-guard.C:
				r.Log.Warn("execution lease could not be renewed; stopping worker", "id", id)
				return
			case <-tick.C:
				until := time.Now().Add(executionTTL)
				qctx, stop := context.WithTimeout(ctx, controlTimeout)
				var err error
				if turn {
					_, err = r.q().RenewTurnExecution(qctx, store.RenewTurnExecutionParams{ID: id, Token: token, ExpiresAt: &until})
				} else {
					_, err = r.q().RenewSetupExecution(qctx, store.RenewSetupExecutionParams{ID: id, Token: token, ExpiresAt: &until})
				}
				stop()
				if errors.Is(err, pgx.ErrNoRows) {
					r.Log.Warn("execution lease was replaced; stopping worker", "id", id)
					return
				}
				if err == nil {
					guard.Reset(time.Until(until.Add(-15 * time.Second)))
				}
			}
		}
	}()
	defer func() {
		close(done)
		cancel()
		<-stopped
		qctx, stop := context.WithTimeout(context.Background(), controlTimeout)
		defer stop()
		var err error
		if turn {
			err = r.q().ReleaseTurnExecution(qctx, store.ReleaseTurnExecutionParams{ID: id, ExecutionToken: token})
		} else {
			err = r.q().ReleaseSetupExecution(qctx, store.ReleaseSetupExecutionParams{ID: id, ExecutionToken: token})
		}
		if err != nil {
			r.Log.Warn("release execution lease", "id", id, "err", err)
		}
	}()
	work(ctx)
}

// reconcile is safe during normal polling: active execution leases are never
// reset. Unlike Recover, it does not clean up any backend processes.
func (r *Runner) reconcile(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, controlTimeout)
	defer cancel()
	q := r.q()
	if err := q.RequeueStarting(ctx, &r.Cfg.ID); err != nil {
		return err
	}
	if err := q.RequeueInterrupted(ctx, &r.Cfg.ID); err != nil {
		return err
	}
	if err := r.resumeStranded(ctx); err != nil {
		return err
	}
	ids, err := q.AbandonedChatCancellations(ctx, &r.Cfg.ID)
	if err != nil {
		return err
	}
	for _, row := range ids {
		err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
			q := store.New(tx)
			chat, err := lockRunChat(ctx, q, store.Run{ID: row.ID, WorkspaceID: row.WorkspaceID})
			if err != nil {
				return err
			}
			run, err := q.LockRun(ctx, row.ID)
			if err != nil || run.State != "paused" {
				return err
			}
			if err := q.FinishUnfinishedChatTurns(ctx, run.ID); err != nil {
				return err
			}
			if err := idleChat(ctx, q, chat, run, "idle"); err != nil {
				return err
			}
			return runs.ResumeInterruptedChat(ctx, q, chat.ID, run.ID, nil, 0)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// retryChatTransaction reuses the completed result, rather than invoking the
// harness again. Each attempt has a deadline; exhausted attempts leave a lease
// that can be recovered when storage is available again.
func (r *Runner) retryChatTransaction(ctx context.Context, fn func(context.Context, *store.Queries) error) error {
	var err error
	for attempt := range 3 {
		tctx, cancel := context.WithTimeout(ctx, controlTimeout)
		err = pgx.BeginFunc(tctx, r.Pool, func(tx pgx.Tx) error { return fn(tctx, store.New(tx)) })
		cancel()
		if err == nil || ctx.Err() != nil {
			return err
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
			}
			timer.Stop()
		}
	}
	return err
}
