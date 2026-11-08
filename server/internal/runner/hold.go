package runner

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/harness"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const (
	// DefaultMaxHold is how long a run may wait for provider usage to reset.
	DefaultMaxHold = 24 * time.Hour
	// DefaultMaxHoldAttempts bounds consecutive limited turns of a held run.
	DefaultMaxHoldAttempts = 12
	// DefaultHoldBackoff is the first wait when the limit message has no reset
	// time. It doubles on each attempt, up to maxHoldBackoff.
	DefaultHoldBackoff = 5 * time.Minute
	maxHoldBackoff     = time.Hour
	// holdResetGrace is added to a parsed reset. The CLIs report it to the
	// minute, and resuming early would read the same time as tomorrow's reset.
	holdResetGrace = time.Minute
)

// QuotaResumeMessage continues a held run's session once its limit has reset.
const QuotaResumeMessage = "Your provider usage limit has reset. Continue where you left off, in the same checkout and following the same plan."

func (r *Runner) maxHold() time.Duration {
	return cmp.Or(r.Cfg.MaxHold, DefaultMaxHold)
}

// holdWait says when a held run should next try, given its message and how
// many times it has already been held.
func (r *Runner) holdWait(msg string, attempts int32, now time.Time) (until time.Time, resetKnown bool) {
	if reset, ok := harness.LimitReset(msg, now); ok {
		return reset.Add(holdResetGrace), true
	}
	wait := cmp.Or(r.Cfg.HoldBackoff, DefaultHoldBackoff)
	for i := int32(0); i < attempts && wait < maxHoldBackoff; i++ {
		wait *= 2
	}
	return now.Add(min(wait, maxHoldBackoff)), false
}

// hold parks a run whose turn hit a usage limit with no fallback left. It
// holds no runner slot, and only harness time counts against max_runtime, so
// the wait costs nothing. Past the configured bounds the run fails instead.
func (r *Runner) hold(ctx context.Context, c runCtx, msg string) {
	maxAttempts := cmp.Or(r.Cfg.MaxHoldAttempts, DefaultMaxHoldAttempts)
	if int(c.run.HoldAttempts) >= maxAttempts {
		r.end(ctx, c, "failed", fmt.Sprintf("Provider usage exhausted: still limited after %d resume attempts.", maxAttempts), runs.VerbRunFailed)
		return
	}
	now := time.Now()
	until, known := r.holdWait(msg, c.run.HoldAttempts, now)
	err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		run, err := q.LockRun(ctx, c.run.ID)
		if err != nil {
			return err
		}
		if run.State != "running" {
			return nil
		}
		pending, err := q.HasPendingTurn(ctx, run.ID)
		if err != nil || pending {
			return err
		}
		held, err := q.HoldRun(ctx, store.HoldRunParams{ID: run.ID, HoldUntil: &until, HoldResetKnown: known})
		if err != nil {
			return err
		}
		b, _ := json.Marshal(map[string]any{"state": "waiting_for_quota", "harness": run.Harness, "until": until.Format(time.RFC3339), "reset_known": known, "attempt": held.HoldAttempts})
		if err := q.AddRunEvent(ctx, store.AddRunEventParams{RunID: run.ID, Type: "lifecycle", Payload: b}); err != nil {
			return err
		}
		return runs.RecomputeAgentStatus(ctx, q, run.AgentID)
	})
	if err != nil {
		r.end(ctx, c, "failed", "Provider usage exhausted, and the run could not be held: "+err.Error(), runs.VerbRunFailed)
	}
}

// resumeHeld wakes held runs whose wait is over and fails those held too long.
// It runs on every Tick and takes no slot: a resumed run's turn queues like any other.
func (r *Runner) resumeHeld(ctx context.Context) {
	limit := r.maxHold()
	ids, err := r.q().DueHeldRuns(ctx, time.Now().Add(-limit))
	if err != nil {
		if ctx.Err() == nil {
			r.Log.Error("list held runs", "err", err)
		}
		return
	}
	for _, id := range ids {
		if err := r.resumeRun(ctx, id, limit); err != nil && ctx.Err() == nil && !errors.Is(err, pgx.ErrNoRows) {
			r.Log.Error("resume held run", "run", id, "err", err)
		}
	}
}

func (r *Runner) resumeRun(ctx context.Context, id uuid.UUID, limit time.Duration) error {
	c, err := r.load(ctx, id)
	if err != nil {
		return err
	}
	if c.run.State != "waiting_for_quota" {
		return nil
	}
	if c.run.HoldStartedAt != nil && time.Since(*c.run.HoldStartedAt) >= limit {
		r.end(ctx, c, "failed", fmt.Sprintf("Provider usage exhausted: still limited after being held for %s.", limit), runs.VerbRunFailed)
		return nil
	}
	return pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		run, err := q.LockRun(ctx, id)
		if err != nil || run.State != "waiting_for_quota" {
			return err
		}
		msg := QuotaResumeMessage
		if run.HarnessSessionID == nil {
			// The limit struck before a session existed, so nothing remembers
			// the turn's input: send it again.
			turns, err := q.ListTurns(ctx, id)
			if err != nil {
				return err
			}
			if len(turns) > 0 {
				msg = turns[len(turns)-1].InputMessage
			}
		}
		if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: id, State: "running"}); err != nil {
			return err
		}
		if _, err := q.StartTurn(ctx, store.StartTurnParams{RunID: id, Trigger: "resume", InputMessage: msg}); err != nil {
			return err
		}
		b, _ := json.Marshal(map[string]any{"state": "quota_resumed", "harness": run.Harness, "attempt": run.HoldAttempts})
		if err := q.AddRunEvent(ctx, store.AddRunEventParams{RunID: id, Type: "lifecycle", Payload: b}); err != nil {
			return err
		}
		return runs.RecomputeAgentStatus(ctx, q, run.AgentID)
	})
}
