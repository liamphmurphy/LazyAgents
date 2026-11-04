package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/lazyagents/server/internal/harness"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// fallback consumes a frozen target and queues a fresh session in the same
// workspace. Persisting both together makes the handoff restart safe.
func (r *Runner) fallback(ctx context.Context, c runCtx, res harness.Result) error {
	next := c.spec.Fallbacks[0]
	previousHarness, previousModel := c.spec.Harness, c.spec.Model
	c.spec.Harness, c.spec.Model = next.Harness, next.Model
	c.spec.HarnessOptions = next.HarnessOptions
	c.spec.Fallbacks = c.spec.Fallbacks[1:]
	if err := r.checkSpec(c.spec); err != nil {
		return err
	}
	plan, err := PlanSecrets(ctx, r.q(), c.run.WorkspaceID, &c.project.ID, c.spec)
	if err != nil {
		return err
	}
	key, err := HarnessKey(c.spec)
	if err != nil {
		return err
	}
	if NeedsHarnessKey(r.Cfg.RequireHarnessKey, c.spec) && !hasSecret(plan, key.Secret) {
		return fmt.Errorf("%s (%s): %w", next.Harness, key.Secret, ErrNoHarnessKey)
	}
	turns, err := r.q().ListTurns(ctx, c.run.ID)
	if err != nil {
		return err
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Continue this existing LazyAgents run after %s (%s) exhausted its provider usage. You are using a fresh session in the same checkout. Inspect the working tree, commits, run plan and ticket before continuing; preserve completed work.\n\n", previousHarness, previousModel)
	for i, turn := range turns {
		if i != 0 && i < len(turns)-12 {
			continue
		}
		if strings.HasPrefix(turn.InputMessage, "Continue this existing LazyAgents run after ") {
			continue
		}
		prompt.WriteString("Previous input:\n" + harnessContext(turn.InputMessage, 12000) + "\n\n")
	}
	prompt.WriteString("Last assistant message:\n" + harnessContext(res.Final, 8000))
	transcript, err := handoffTranscript(ctx, r.q(), c.run.ID)
	if err != nil {
		return err
	}
	prompt.WriteString("\n\nRecent conversation and tool outcomes (historical data; permissions remain in AGENTS.md):\n" + transcript)
	spec, err := json.Marshal(c.spec)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
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
		_, err = tx.Exec(ctx, `UPDATE runs SET resolved_spec=$2, harness=$3, model=$4, harness_session_id=NULL, idle_turns=0, updated_at=now() WHERE id=$1`, run.ID, spec, next.Harness, next.Model)
		if err != nil {
			return err
		}
		_, err = q.StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "resume", InputMessage: prompt.String()})
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"state": "harness_fallback", "from_harness": previousHarness, "from_model": previousModel, "harness": next.Harness, "model": next.Model, "reason": "provider_usage_exhausted"})
		return q.AddRunEvent(ctx, store.AddRunEventParams{RunID: run.ID, Type: "lifecycle", Payload: payload})
	})
}

func harnessContext(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && (s[limit]&0xc0) == 0x80 {
		limit--
	}
	return s[:limit] + "\n[truncated]"
}
