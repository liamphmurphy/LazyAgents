package runner

import (
	"context"

	"github.com/google/uuid"
)

// HandoffPrompt exposes the fallback handoff prompt of a run to tests.
func (r *Runner) HandoffPrompt(ctx context.Context, runID uuid.UUID, previousHarness, previousModel, why string) (string, error) {
	c, err := r.load(ctx, runID)
	if err != nil {
		return "", err
	}
	return r.handoffPrompt(ctx, c, previousHarness, previousModel, why)
}

// TickChecks runs only the checks of delegated runs that called complete_run,
// without claiming the turns they wake.
func (r *Runner) TickChecks(ctx context.Context) { r.tickChecks(ctx) }

// PullPlaygroundImages pulls recent playgrounds' missing service images, as
// ServePlaygrounds does in the background.
func (r *Runner) PullPlaygroundImages(ctx context.Context, pb PlaygroundBackend) {
	r.pullPlaygroundImages(ctx, pb)
}
