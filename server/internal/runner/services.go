package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// ServiceBackend starts the services a run's spec declares (agentdef.Service)
// next to one turn. The local backend has no isolation to offer and doesn't
// implement it.
type ServiceBackend interface {
	// StartServices starts services on a new network named for key and waits
	// until each is ready. The turn joins network (ExecSpec.Networks); stop
	// removes the services and the network, and is safe to call once.
	StartServices(ctx context.Context, runID, key string, services []agentdef.Service) (network string, stop func(), err error)
}

// startServices starts the services the run's spec declares for this turn and
// puts what they expose into env. It returns the networks the turn joins and
// a stop to call when the turn ends, or a failure for the run's outcome.
func (r *Runner) startServices(ctx context.Context, c runCtx, turn store.RunTurn, env map[string]string) ([]string, func(), string) {
	if len(c.spec.Services) == 0 {
		return nil, func() {}, ""
	}
	names := make([]string, len(c.spec.Services))
	for i, s := range c.spec.Services {
		names[i] = s.Name
	}
	sb, ok := r.Backend.(ServiceBackend)
	if !ok {
		// The local runner shares the host; its runs use the host's services
		// (TEST_DATABASE_URL from `make local-runner`) instead.
		r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "services_skipped", "services": names,
			"reason": "the " + r.Backend.Name() + " runner can't start services"})
		return nil, func() {}, ""
	}
	started := time.Now()
	network, stop, err := sb.StartServices(ctx, c.run.ID.String(), fmt.Sprintf("%s-%d", c.run.ID, turn.Seq), c.spec.Services)
	if err != nil {
		return nil, nil, "Couldn't start the run's services: " + err.Error()
	}
	for _, s := range c.spec.Services {
		for k, v := range s.Expose {
			env[k] = v
		}
	}
	r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "services_ready", "services": names, "ms": time.Since(started).Milliseconds()})
	return []string{network}, stop, ""
}
