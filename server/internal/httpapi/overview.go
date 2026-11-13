package httpapi

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/metrics"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func workTicket(id uuid.UUID, key, title, status, priority string) api.WorkTicket {
	return api.WorkTicket{Id: id, Key: key, Title: title, Status: api.TicketStatus(status), Priority: api.Priority(priority)}
}
func agentStatus(work []api.AgentWork) api.AgentStatus {
	status := "idle"
	rank := map[string]int{"idle": 0, "waiting": 1, "blocked": 2, "running": 3}
	for _, w := range work {
		st := "running"
		switch w.State {
		case "queued":
			continue
		case "waiting", "waiting_for_quota", "paused":
			st = "waiting"
		case "blocked":
			st = "blocked"
		}
		if rank[st] > rank[status] {
			status = st
		}
	}
	return api.AgentStatus(status)
}
func (s *Server) agentOverviews(ctx context.Context, ws uuid.UUID) (api.AgentOverviewList, error) {
	out := api.AgentOverviewList{Agents: []api.AgentOverview{}}
	snap, err := metrics.Get(ctx, s.Pool, ws, "7d")
	if err != nil {
		return out, err
	}
	out.StatsAsOf = snap.GeneratedAt
	liveStatuses, err := s.liveAgentStatuses(ctx, ws)
	if err != nil {
		return out, err
	}
	roster, err := s.Q.ListAgents(ctx, ws)
	if err != nil {
		return out, err
	}
	planning, err := s.Q.ActivePlanningWork(ctx, ws)
	if err != nil {
		return out, err
	}
	work, err := s.Q.OverviewRuns(ctx, ws)
	if err != nil {
		return out, err
	}
	queue, err := s.Q.OverviewAgentTickets(ctx, ws)
	if err != nil {
		return out, err
	}
	for _, a := range roster {
		spec, _, err := agents.CurrentSpec(ctx, s.Q, a)
		if err != nil {
			return out, err
		}
		stats, err := recode[api.PerformanceStats](snap.Agents[a.ID])
		if err != nil {
			return out, err
		}
		o := api.AgentOverview{Agent: toAgent(a), MaxConcurrency: spec.MaxConcurrency, Harness: spec.Harness, PlanningRuns: []api.PlanningWork{}, Assignments: []api.AgentWork{}, Queue: []api.WorkTicket{}, Scopes: []api.AgentScopeSummary{}, Stats: stats}
		for _, p := range planning {
			if p.AgentID == a.ID {
				o.PlanningRuns = append(o.PlanningRuns, api.PlanningWork{Id: p.ID, RunId: p.RunID, Goal: p.Goal, State: p.State, Harness: p.Harness, Model: p.Model})
			}
		}
		for _, w := range work {
			if w.AgentID == a.ID {
				o.Assignments = append(o.Assignments, api.AgentWork{RunId: w.ID, Ticket: workTicket(*w.TicketID, w.Key, w.Title, w.Status, w.Priority), State: w.State, Harness: w.Harness, Model: w.Model, StartedAt: w.StartedAt, Step: w.Step, StepPosition: int(w.StepPosition), StepsDone: int(w.StepsDone), StepsTotal: int(w.StepsTotal)})
			}
		}
		o.Agent.Status = api.AgentStatusIdle
		if st, ok := liveStatuses[a.ID]; ok {
			o.Agent.Status = st
		}
		for _, t := range queue {
			if t.AssigneeAgentID != nil && *t.AssigneeAgentID == a.ID {
				o.Queue = append(o.Queue, workTicket(t.ID, t.Key, t.Title, t.Status, t.Priority))
				o.QueueCount = int(t.QueueCount)
			}
		}
		names := make([]string, 0, len(spec.Scopes))
		for name := range spec.Scopes {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			mode := spec.Scopes[name]
			level, how := agentdef.EnforcementFor(name, mode, spec.Harness)
			o.Scopes = append(o.Scopes, api.AgentScopeSummary{Scope: name, Mode: api.ScopeMode(mode), Enforcement: api.AgentScopeSummaryEnforcement(level), How: how})
		}
		out.Agents = append(out.Agents, o)
	}
	return out, nil
}
func (s *Server) ListAgentOverviews(ctx context.Context, _ api.ListAgentOverviewsRequestObject) (api.ListAgentOverviewsResponseObject, error) {
	_, _, ws := me(ctx)
	out, err := s.agentOverviews(ctx, ws)
	return api.ListAgentOverviews200JSONResponse(out), err
}
func increment(c *api.WorkflowCounts, status string, n int) {
	c.Total += n
	switch status {
	case "done":
		c.Done += n
	case "in_review":
		c.InReview += n
	case "in_progress":
		c.InProgress += n
	case "todo":
		c.Todo += n
	case "backlog":
		c.Backlog += n
	}
}
func overviewArtifact(a store.OverviewArtifactsRow) api.OverviewArtifact {
	href := "/artifacts/" + a.ID.String()
	if a.Type == "note" && a.Path != nil {
		href = links.NoteHref(*a.Path)
	}
	return api.OverviewArtifact{Id: a.ID, Title: a.Title, Type: api.ArtifactType(a.Type), Href: href, UpdatedAt: a.UpdatedAt}
}

type overview struct {
	projects  []api.ProjectOverview
	epics     []api.EpicOverview
	artifacts []api.OverviewArtifact
	counts    api.WorkflowCounts
}

func (s *Server) overview(ctx context.Context, ws uuid.UUID) (overview, error) {
	out := overview{projects: []api.ProjectOverview{}, epics: []api.EpicOverview{}, artifacts: []api.OverviewArtifact{}}
	projects, err := s.Q.ListProjects(ctx, ws)
	if err != nil {
		return out, err
	}
	activeProjects := map[uuid.UUID]bool{}
	filtered := projects[:0]
	for _, p := range projects {
		if !p.Archived {
			filtered = append(filtered, p)
			activeProjects[p.ID] = true
		}
	}
	projects = filtered
	epics, err := s.Q.ListEpics(ctx, ws)
	if err != nil {
		return out, err
	}
	rollups, err := s.epicRollups(ctx, epics)
	if err != nil {
		return out, err
	}
	counts, err := s.Q.OverviewTicketCounts(ctx, ws)
	if err != nil {
		return out, err
	}
	tickets, err := s.Q.OverviewEpicTickets(ctx, ws)
	if err != nil {
		return out, err
	}
	docs, err := s.Q.OverviewArtifacts(ctx, store.OverviewArtifactsParams{WorkspaceID: ws, Viewer: pageview.From(ctx).UserID, IncludeAgent: pageview.From(ctx).IncludeAgent})
	if err != nil {
		return out, err
	}
	hours, err := s.Q.EpicAgentTime(ctx, ws)
	if err != nil {
		return out, err
	}
	dir, err := s.directory(ctx, ws)
	if err != nil {
		return out, err
	}
	for _, d := range docs {
		if len(out.artifacts) < 6 {
			out.artifacts = append(out.artifacts, overviewArtifact(d))
		}
	}
	for _, p := range projects {
		o := api.ProjectOverview{Project: toProject(p), Agents: []api.AgentRef{}, UpdatedAt: p.UpdatedAt}
		seen := map[uuid.UUID]bool{}
		for _, c := range counts {
			if c.ProjectID == p.ID {
				increment(&o.Counts, c.Status, int(c.Count))
				increment(&out.counts, c.Status, int(c.Count))
			}
		}
		for _, t := range tickets {
			if t.ProjectID != p.ID {
				continue
			}
			if t.UpdatedAt.After(o.UpdatedAt) {
				o.UpdatedAt = t.UpdatedAt
			}
			if t.Status != "done" && t.AssigneeAgentID != nil && !seen[*t.AssigneeAgentID] {
				if a, ok := dir.agents[*t.AssigneeAgentID]; ok {
					o.Agents = append(o.Agents, *toAgentRef(a))
					seen[a.ID] = true
				}
			}
		}
		out.projects = append(out.projects, o)
	}
	for _, e := range epics {
		if !activeProjects[e.ProjectID] {
			continue
		}
		o := api.EpicOverview{Epic: toEpic(e, rollups[e.ID]), Objective: e.Objective, StartDate: toDate(e.StartDate), TargetDate: toDate(e.TargetDate), Agents: []api.AgentRef{}, Tickets: []api.WorkTicket{}}
		for _, p := range projects {
			if p.ID == e.ProjectID {
				o.Project = toProject(p)
				break
			}
		}
		if e.OwnerUserID != nil {
			if u, ok := dir.users[*e.OwnerUserID]; ok {
				o.Owner = toUserRef(u)
			}
		}
		for _, d := range docs {
			if d.Type == "prd" && d.EpicID != nil && *d.EpicID == e.ID {
				source := overviewArtifact(d)
				o.Source = &source
				break
			}
		}
		for _, h := range hours {
			if h.EpicID != nil && *h.EpicID == e.ID {
				o.AgentSeconds = h.Seconds
			}
		}
		for _, c := range counts {
			if c.EpicID != nil && *c.EpicID == e.ID {
				increment(&o.Counts, c.Status, int(c.Count))
			}
		}
		seen := map[uuid.UUID]bool{}
		for _, t := range tickets {
			if t.EpicID == nil || *t.EpicID != e.ID {
				continue
			}
			if len(o.Tickets) < 8 {
				o.Tickets = append(o.Tickets, workTicket(t.ID, t.Key, t.Title, t.Status, t.Priority))
			}
			if t.AssigneeAgentID != nil && !seen[*t.AssigneeAgentID] {
				if a, ok := dir.agents[*t.AssigneeAgentID]; ok {
					o.Agents = append(o.Agents, *toAgentRef(a))
					seen[a.ID] = true
				}
			}
		}
		out.epics = append(out.epics, o)
	}
	return out, nil
}
func (s *Server) ListEpicOverviews(ctx context.Context, _ api.ListEpicOverviewsRequestObject) (api.ListEpicOverviewsResponseObject, error) {
	_, _, ws := me(ctx)
	data, err := s.overview(ctx, ws)
	return api.ListEpicOverviews200JSONResponse{Epics: data.epics}, err
}
func (s *Server) GetDashboard(ctx context.Context, req api.GetDashboardRequestObject) (api.GetDashboardResponseObject, error) {
	_, _, ws := me(ctx)
	window := "7d"
	if req.Params.Range != nil {
		window = string(*req.Params.Range)
	}
	if _, ok := metrics.Windows[window]; !ok {
		return nil, &tracker.ValidationError{Field: "range", Msg: "choose 24h, 7d or 30d"}
	}
	// Calculated fresh, not read from the rollup cache: the dashboard refetches
	// on live events and its throughput must reflect the change that caused it.
	now := time.Now().UTC()
	snap, err := metrics.Calculate(ctx, s.Pool, ws, window, now)
	if err != nil {
		return nil, err
	}
	stats, err := recode[api.RangeStats](snap)
	if err != nil {
		return nil, err
	}
	overview, err := s.overview(ctx, ws)
	if err != nil {
		return nil, err
	}
	agents, err := s.agentOverviews(ctx, ws)
	if err != nil {
		return nil, err
	}
	inbox, err := s.ListInbox(ctx, api.ListInboxRequestObject{})
	if err != nil {
		return nil, err
	}
	attention := []api.InboxItem{}
	for _, item := range inbox.(api.ListInbox200JSONResponse).Items {
		if !item.Resolved && !item.Read {
			attention = append(attention, item)
			if len(attention) == 8 {
				break
			}
		}
	}
	runner, err := s.runnerStatus(ctx)
	if err != nil {
		return nil, err
	}
	old, err := s.Q.ReviewOverDay(ctx, ws)
	if err != nil {
		return nil, err
	}
	since := weekStart(now)
	spend, err := s.Q.WeeklySpend(ctx, store.WeeklySpendParams{WorkspaceID: ws, Since: since})
	if err != nil {
		return nil, err
	}
	weekly := api.WeeklySpend{Since: since, CostUsd: float64(spend.CostUsdMicros) / 1e6, Runs: int(spend.Runs), UnpricedRuns: int(spend.UnpricedRuns)}
	return api.GetDashboard200JSONResponse{Stats: stats, Counts: overview.counts, Projects: overview.projects, Agents: agents.Agents, Attention: attention, RecentArtifacts: overview.artifacts, Runner: runner, ReviewOverDay: int(old), WeeklySpend: &weekly}, nil
}

// weekStart is Monday 00:00 UTC of the week containing t.
func weekStart(t time.Time) time.Time {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

// Used by bootstrap so seeded status labels cannot masquerade as live work.
func (s *Server) liveAgentStatuses(ctx context.Context, ws uuid.UUID) (map[uuid.UUID]api.AgentStatus, error) {
	rows, err := s.Q.LiveAgentRunStates(ctx, ws)
	if err != nil {
		return nil, err
	}
	groups := map[uuid.UUID][]api.AgentWork{}
	for _, r := range rows {
		groups[r.AgentID] = append(groups[r.AgentID], api.AgentWork{State: r.State})
	}
	out := map[uuid.UUID]api.AgentStatus{}
	for id, group := range groups {
		out[id] = agentStatus(group)
	}
	return out, nil
}

func (s *Server) ListAgentRuns(ctx context.Context, req api.ListAgentRunsRequestObject) (api.ListAgentRunsResponseObject, error) {
	_, _, ws := me(ctx)
	a, err := s.agentByHandle(ctx, ws, req.Handle)
	if err != nil {
		return nil, err
	}
	offset := 0
	if req.Params.Offset != nil {
		offset = *req.Params.Offset
	}
	if offset < 0 || offset > 100000 {
		return nil, &tracker.ValidationError{Field: "offset", Msg: "must be between 0 and 100000"}
	}
	rows, err := s.Q.AgentRunHistory(ctx, store.AgentRunHistoryParams{WorkspaceID: ws, AgentID: a.ID, Skip: int32(offset)})
	if err != nil {
		return nil, err
	}
	out := api.ListAgentRuns200JSONResponse{Runs: []api.AgentRunSummary{}, HasMore: len(rows) > 50}
	if len(rows) > 50 {
		rows = rows[:50]
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	cacheRows, err := s.Q.CacheUsageForRuns(ctx, ids)
	if err != nil {
		return nil, err
	}
	cache := map[uuid.UUID]store.CacheUsageForRunsRow{}
	for _, row := range cacheRows {
		cache[row.RunID] = row
	}
	for _, r := range rows {
		c := cache[r.ID]
		cached, uncached, unknown := cacheBreakdown(r.InputTokens, c.CachedInputTokens, c.ReportedInputTokens)
		out.Runs = append(out.Runs, api.AgentRunSummary{CachedInputTokens: &cached, UncachedInputTokens: &uncached, UnreportedInputTokens: &unknown, Id: r.ID, Ticket: workTicket(r.TicketID, r.Key, r.Title, r.Status, r.Priority), State: r.State, Harness: r.Harness, Model: r.Model, StartedAt: r.StartedAt, EndedAt: r.EndedAt, CostUsd: float64(r.CostUsdMicros) / 1e6, CostSource: r.CostSource, ToolCalls: int(r.ToolCalls), Tokens: r.Tokens})
	}
	return out, nil
}
