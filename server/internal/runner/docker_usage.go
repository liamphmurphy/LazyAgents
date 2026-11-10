package runner

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/usage"
)

// UsageProber reads a plan login's allowance. The Docker API image has no
// harness CLI, so the runner does it for the API (tickUsageChecks).
type UsageProber interface {
	// claim names the check's container; login is the saved harness secret;
	// codexScope is where a refreshed Codex login is stored (CodexAuthScope).
	ProbeUsage(ctx context.Context, claim, harness, login, codexScope string) (usage.Snapshot, error)
}

const (
	// probeUsageTimeout covers the container's start as well as the CLI's
	// own usage.ProbeTimeout.
	probeUsageTimeout = 60 * time.Second
	// maxConcurrentUsageChecks caps the throwaway containers checking at once.
	maxConcurrentUsageChecks = 2
)

// ProbeUsage asks the harness CLI for its allowance in a throwaway container
// with the sign-in sandbox and the saved login, as ListModels does. Like the
// local check it sends no prompt and spends no tokens.
func (d *Docker) ProbeUsage(ctx context.Context, claim, harness, login, codexScope string) (usage.Snapshot, error) {
	argv, err := usage.Argv(harness)
	if err != nil {
		return usage.Snapshot{}, err
	}
	timeout := d.UsageTimeout
	if timeout <= 0 {
		timeout = probeUsageTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var snap usage.Snapshot
	var answer error
	err = d.converseCLI(ctx, "usage", claim, harness, login, codexScope, argv, func(in io.Writer, out io.Reader) error {
		snap, answer = usage.Converse(harness, in, out)
		return nil
	})
	if err != nil {
		return usage.Snapshot{}, err
	}
	// The CLI's own answer, such as ErrSignedOut, comes back as it is so
	// usage.Service.Settle can tell it from a container that failed.
	return snap, answer
}

// tickUsageChecks answers the usage checks the API queued while someone
// watches the activity orb. Like sign-ins they take no run slot.
func (r *Runner) tickUsageChecks(ctx context.Context) {
	up, ok := r.Backend.(UsageProber)
	if !ok || r.Secrets == nil {
		return
	}
	q := r.q()
	for r.usageChecks.Add(1) <= maxConcurrentUsageChecks {
		c, err := q.ClaimUsageCheck(ctx, &r.Cfg.ID)
		if err != nil {
			r.usageChecks.Add(-1)
			if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
				r.Log.Error("claim usage check", "err", err)
			}
			return
		}
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			defer r.usageChecks.Add(-1)
			r.runUsageCheck(ctx, up, c)
		}()
	}
	r.usageChecks.Add(-1) // the attempt that found the cap reached
}

func (r *Runner) runUsageCheck(parent context.Context, up UsageProber, c store.HarnessUsageCheck) {
	login, err := r.Secrets.Reveal(parent, c.WorkspaceID, nil, chatmodels.KeySecret[c.Harness])
	if parent.Err() != nil {
		return // shutting down: the claim times out and another runner retries
	}
	var snap usage.Snapshot
	switch {
	case errors.Is(err, apperr.ErrNotFound):
		err = usage.ErrSignedOut
	case err != nil:
		// Not the person's to fix: back off as for any failed check.
	case !planLogin(c.Harness, login):
		// An API key has no plan windows; it never reaches the CLI.
		err = usage.ErrSignedOut
	default:
		snap, err = up.ProbeUsage(parent, c.ClaimID.String(), c.Harness, login, CodexAuthScope(c.WorkspaceID, nil))
		if parent.Err() != nil {
			return
		}
	}
	if err != nil && !errors.Is(err, usage.ErrSignedOut) && !errors.Is(err, usage.ErrOutdated) {
		// The error can carry CLI output; it goes to the log only.
		r.Log.Warn("check harness usage", "workspace", c.WorkspaceID, "harness", c.Harness, "err", err)
	}
	// The caller's context may be what ended; the outcome is still recorded.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer cancel()
	svc := usage.Service{Q: r.q(), Log: r.Log}
	wait := svc.Settle(ctx, c.WorkspaceID, c.Harness, snap, err)
	if _, err := r.q().FinishUsageCheck(ctx, store.FinishUsageCheckParams{ClaimID: c.ClaimID, RetryAfter: time.Now().Add(wait)}); err != nil {
		r.Log.Warn("finish usage check", "workspace", c.WorkspaceID, "harness", c.Harness, "err", err)
	}
}

// planLogin reports whether a saved harness secret is a Claude or ChatGPT
// plan login rather than an API key.
func planLogin(harness, login string) bool {
	if harness == agentdef.HarnessCodex {
		_, plan := chatmodels.ParseCodexAuth(login)
		return plan
	}
	return chatmodels.ClaudeOAuthToken(login)
}
