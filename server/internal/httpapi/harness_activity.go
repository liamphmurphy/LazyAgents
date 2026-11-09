package httpapi

import (
	"context"
	"math"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/usage"
)

// orbLabels are short: the orb panel has little room.
var orbLabels = map[string]string{agentdef.HarnessClaudeCode: "Claude", agentdef.HarnessCodex: "Codex"}

// buildHarnessActivity is the activity orb's view of each harness: what it's
// running, whether it's out of allowance, and the windows last read.
func buildHarnessActivity(running []store.ActiveHarnessRunsRow, limits []store.ActiveProviderLimitsRow, readings map[string]usage.Snapshot) []api.HarnessActivity {
	out := make([]api.HarnessActivity, 0, len(usage.Harnesses))
	for _, h := range usage.Harnesses {
		a := api.HarnessActivity{Harness: h, Label: orbLabels[h], Windows: []api.UsageWindow{}}
		for _, r := range running {
			if r.Harness == h {
				a.Running = int(r.Runs)
			}
		}
		for _, l := range limits {
			if l.Harness == h {
				until := l.Until
				a.LimitedUntil = &until
			}
		}
		if snap, ok := readings[h]; ok {
			checked := snap.ObservedAt
			a.UsageCheckedAt = &checked
			if snap.Plan != "" {
				plan := snap.Plan
				a.Plan = &plan
			}
			a.UsageAvailable = snap.Available && len(snap.Windows) > 0
			for _, w := range snap.Windows {
				a.Windows = append(a.Windows, api.UsageWindow{Id: w.ID, Kind: api.UsageWindowKind(w.Kind), Label: w.Label,
					UsedPercent: float32(math.Round(w.UsedPercent*10) / 10), ResetsAt: w.ResetsAt})
			}
		}
		out = append(out, a)
	}
	return out
}

// harnessActivity reads the workspace's harness activity and starts any
// usage checks that are due; their results show on a later poll.
func (s *Server) harnessActivity(ctx context.Context, ws uuid.UUID) ([]api.HarnessActivity, error) {
	running, err := s.Q.ActiveHarnessRuns(ctx, ws)
	if err != nil {
		return nil, err
	}
	limits, err := s.Q.ActiveProviderLimits(ctx, ws)
	if err != nil {
		return nil, err
	}
	readings := map[string]usage.Snapshot{}
	if s.Usage != nil {
		if readings, err = s.Usage.Readings(ctx, ws); err != nil {
			return nil, err
		}
		s.Usage.Refresh(ctx, ws, readings)
	}
	return buildHarnessActivity(running, limits, readings), nil
}
