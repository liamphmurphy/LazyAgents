package httpapi

import (
	"context"
	"sort"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// ListDelegationEfficiency reports, per sub agent, what delegating to it costs
// (child plus the delegating parent runs) and how often its first attempt passes.
func (s *Server) ListDelegationEfficiency(ctx context.Context, _ api.ListDelegationEfficiencyRequestObject) (api.ListDelegationEfficiencyResponseObject, error) {
	_, _, ws := me(ctx)
	q := store.New(s.Pool)
	rows, err := q.DelegationEfficiency(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := api.DelegationEfficiencyList{Agents: []api.DelegationEfficiency{}}
	for _, r := range rows {
		a, err := q.GetAgent(ctx, store.GetAgentParams{ID: r.AgentID, WorkspaceID: ws})
		if err != nil {
			return nil, err
		}
		e := api.DelegationEfficiency{
			AgentId: r.AgentID, Handle: a.Handle, Name: a.Name,
			Delegations: int(r.Delegations), Checked: int(r.Checked), FirstPass: int(r.FirstPass), RepairLoops: int(r.RepairLoops),
			ChildCostUsdMicros: r.ChildCostUsdMicros, ParentCostUsdMicros: r.ParentCostUsdMicros,
			TotalCostUsdMicros: r.ChildCostUsdMicros + r.ParentCostUsdMicros,
			AvgSetupSeconds:    r.AvgSetupSeconds, AvgHarnessSeconds: r.AvgHarnessSeconds, AvgChecksSeconds: r.AvgChecksSeconds,
		}
		if r.Checked > 0 {
			e.FirstPassRate = float64(r.FirstPass) / float64(r.Checked)
		}
		if r.Delegations > 0 {
			e.AvgSeconds = r.ChildSeconds / float64(r.Delegations)
		}
		out.Agents = append(out.Agents, e)
	}
	sort.Slice(out.Agents, func(i, j int) bool { return out.Agents[i].Handle < out.Agents[j].Handle })
	return api.ListDelegationEfficiency200JSONResponse(out), nil
}
