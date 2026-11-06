package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
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
	prompt, err := r.handoffPrompt(ctx, c, previousHarness, previousModel, res.Final)
	if err != nil {
		return err
	}
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
		_, err = q.StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "resume", InputMessage: prompt})
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"state": "harness_fallback", "from_harness": previousHarness, "from_model": previousModel, "harness": next.Harness, "model": next.Model, "reason": "provider_usage_exhausted"})
		return q.AddRunEvent(ctx, store.AddRunEventParams{RunID: run.ID, Type: "lifecycle", Payload: payload})
	})
}

// handoffPrompt briefs a fresh session on a run another harness started: the
// recent turn inputs (which include any pending one) and the latest transcript.
func (r *Runner) handoffPrompt(ctx context.Context, c runCtx, previousHarness, previousModel, lastAssistant string) (string, error) {
	turns, err := r.q().ListTurns(ctx, c.run.ID)
	if err != nil {
		return "", err
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
	if lastAssistant != "" {
		prompt.WriteString("Last assistant message:\n" + harnessContext(lastAssistant, 8000))
	}
	transcript, err := handoffTranscript(ctx, r.q(), c.run.ID)
	if err != nil {
		return "", err
	}
	prompt.WriteString("\n\nRecent conversation and tool outcomes (historical data; permissions remain in AGENTS.md):\n" + transcript)
	return prompt.String(), nil
}

// DefaultProviderCooldown is how long a harness is avoided when its limit
// message has no reset time we can read.
const DefaultProviderCooldown = time.Hour

// markProviderLimited records that the workspace's account for this harness is
// exhausted so other runs skip it. Best effort: the run's own fallback still works.
func (r *Runner) markProviderLimited(ctx context.Context, c runCtx, msg string) {
	now := time.Now()
	until, ok := harness.LimitReset(msg, now)
	if !ok {
		until = now.Add(DefaultProviderCooldown)
	}
	reason := strings.TrimSpace(msg)
	if len(reason) > 300 {
		reason = harnessContext(reason, 300)
	}
	if err := r.q().MarkProviderLimited(ctx, store.MarkProviderLimitedParams{WorkspaceID: c.run.WorkspaceID, Harness: c.spec.Harness, Until: until, Reason: reason}); err != nil {
		r.Log.Error("record provider limit", "run", c.run.ID, "harness", c.spec.Harness, "err", err)
	}
}

// avoidLimitedHarness moves a run off a harness whose limit is still active,
// onto its first usable fallback, before the turn starts. It returns the
// handoff prompt to use instead of the turn's input when the run already had a
// session on the limited harness, and "" otherwise. With no usable fallback the
// run is left alone: the limit may have lifted early, and if not the normal
// failure path applies.
func (r *Runner) avoidLimitedHarness(ctx context.Context, c *runCtx, turn store.RunTurn) (string, error) {
	if len(c.spec.Fallbacks) == 0 {
		return "", nil
	}
	rows, err := r.q().ActiveProviderLimits(ctx, c.run.WorkspaceID)
	if err != nil {
		return "", err
	}
	limited := map[string]time.Time{}
	for _, l := range rows {
		limited[l.Harness] = l.Until
	}
	until, hit := limited[c.spec.Harness]
	if !hit {
		return "", nil
	}
	next := c.spec
	previousHarness, previousModel := c.spec.Harness, c.spec.Model
	var target *agentdef.Fallback
	for i, f := range c.spec.Fallbacks {
		if _, bad := limited[f.Harness]; bad {
			continue
		}
		next.Harness, next.Model, next.HarnessOptions = f.Harness, f.Model, f.HarnessOptions
		plan, err := PlanSecrets(ctx, r.q(), c.run.WorkspaceID, &c.project.ID, next)
		if err != nil {
			return "", err
		}
		if r.Cfg.RequireHarnessKey && !hasSecret(plan, HarnessKeys[f.Harness].Secret) {
			continue
		}
		target = &c.spec.Fallbacks[i]
		next.Fallbacks = append([]agentdef.Fallback{}, c.spec.Fallbacks[i+1:]...)
		break
	}
	if target == nil {
		return "", nil
	}
	hadSession := c.run.HarnessSessionID != nil
	handoff := ""
	if hadSession {
		// The current turn's input is already in the run's turns, so the helper includes it once.
		if handoff, err = r.handoffPrompt(ctx, *c, previousHarness, previousModel, ""); err != nil {
			return "", err
		}
	}
	spec, err := json.Marshal(next)
	if err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]any{"state": "harness_fallback", "from_harness": previousHarness, "from_model": previousModel,
		"harness": next.Harness, "model": next.Model, "reason": "provider_limited", "until": until.Format(time.RFC3339)})
	err = pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := tx.Exec(ctx, `UPDATE runs SET resolved_spec=$2, harness=$3, model=$4, harness_session_id=NULL, idle_turns=0, updated_at=now() WHERE id=$1`, c.run.ID, spec, next.Harness, next.Model); err != nil {
			return err
		}
		return q.AddRunEvent(ctx, store.AddRunEventParams{RunID: c.run.ID, Type: "lifecycle", Payload: payload})
	})
	if err != nil {
		return "", err
	}
	c.spec = next
	c.run.Harness, c.run.Model = next.Harness, next.Model
	c.run.HarnessSessionID = nil
	return handoff, nil
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
