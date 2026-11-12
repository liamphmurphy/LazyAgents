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
