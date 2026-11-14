package httpapi

import (
	"context"
	"math"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/usage"
)

// orbLabels are short: the orb panel has little room.
var orbLabels = map[string]string{agentdef.HarnessClaudeCode: "Claude", agentdef.HarnessCodex: "Codex", agentdef.HarnessAntigravity: "Antigravity"}

// buildHarnessActivity is the activity orb's view of each harness: what it's
// running, whether it's out of allowance, the windows last read, and whether
// a newer reading is on its way. Antigravity is listed with the others but has
// no allowance to read (agy exposes no headless usage endpoint), so its usage
// stays unavailable. Every harness is listed; connected says which ones can run,
// so the sidebar can leave out the rest.
func buildHarnessActivity(connected map[string]bool, running []store.ActiveHarnessRunsRow, limits []store.ActiveProviderLimitsRow, readings map[string]usage.Snapshot, checking map[string]bool) []api.HarnessActivity {
	out := make([]api.HarnessActivity, 0, len(agentdef.Harnesses))
	for _, h := range agentdef.Harnesses {
		a := api.HarnessActivity{Harness: h, Label: orbLabels[h], Connected: connected[h], Windows: []api.UsageWindow{}, UsageChecking: checking[h]}
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
	connected, err := s.connectedHarnesses(ctx, ws)
	if err != nil {
		return nil, err
	}
	running, err := s.Q.ActiveHarnessRuns(ctx, ws)
	if err != nil {
		return nil, err
	}
	limits, err := s.Q.ActiveProviderLimits(ctx, ws)
	if err != nil {
		return nil, err
	}
	readings := map[string]usage.Snapshot{}
	checking := map[string]bool{}
	if s.Usage != nil {
		if readings, err = s.Usage.Readings(ctx, ws); err != nil {
			return nil, err
		}
		s.Usage.Refresh(ctx, ws, readings)
		if checking, err = s.Usage.Checking(ctx, ws); err != nil {
			return nil, err
		}
	}
	return buildHarnessActivity(connected, running, limits, readings, checking), nil
}

// connectedHarnesses is harnessConnected per harness: each harness needs its
// own key secret, unless the runner runs without them.
func (s *Server) connectedHarnesses(ctx context.Context, ws uuid.UUID) (map[string]bool, error) {
	out := make(map[string]bool, len(agentdef.Harnesses))
	for _, h := range agentdef.Harnesses {
		if s.Runs == nil || !s.Runs.RequireHarnessKey {
			out[h] = true
			continue
		}
		ok, err := s.Q.SecretExists(ctx, store.SecretExistsParams{WorkspaceID: ws, Name: chatmodels.KeySecret[h]})
		if err != nil {
			return nil, err
		}
		out[h] = ok
	}
	return out, nil
}
