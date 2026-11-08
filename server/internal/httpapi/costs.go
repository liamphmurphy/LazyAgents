package httpapi

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const (
	costDay        = "2006-01-02"
	costMaxDays    = 366
	costTopTickets = 10
)

var costPurposeLabels = map[string]string{"ticket": "Ticket work", "planning": "Planning", "chat": "Chat", "research": "Research", "workflow": "Workflows", "learning": "Learning"}

// costAcc sums one breakdown row. Runs are counted once per row, so a chat run
// split across models counts in each of its model rows.
type costAcc struct {
	key, label, detail string
	cost, tokens       int64
	runs, unpriced     map[uuid.UUID]struct{}
}

func newCostAcc(key, label, detail string) *costAcc {
	return &costAcc{key: key, label: label, detail: detail, runs: map[uuid.UUID]struct{}{}, unpriced: map[uuid.UUID]struct{}{}}
}

func (a *costAcc) add(run uuid.UUID, r store.CostUnitsRow) {
	a.cost += r.CostMicros
	a.tokens += r.Tokens
	a.runs[run] = struct{}{}
	if r.Unpriced {
		a.unpriced[run] = struct{}{}
	}
}

func (a *costAcc) slice() api.CostSlice {
	out := api.CostSlice{Key: a.key, Label: a.label, Runs: len(a.runs), Tokens: a.tokens, CostMicros: a.cost, UnpricedRuns: len(a.unpriced)}
	if a.detail != "" {
		out.Detail = &a.detail
	}
	return out
}

type costGroup map[string]*costAcc

func (g costGroup) add(key, label, detail string, run uuid.UUID, r store.CostUnitsRow) {
	a := g[key]
	if a == nil {
		a = newCostAcc(key, label, detail)
		g[key] = a
	}
	a.add(run, r)
}

// slices sorts the rows by spend, most expensive first.
func (g costGroup) slices() []api.CostSlice {
	out := make([]api.CostSlice, 0, len(g))
	for _, a := range g {
		out = append(out, a.slice())
	}
	slices.SortFunc(out, func(a, b api.CostSlice) int {
		return cmp.Or(cmp.Compare(b.CostMicros, a.CostMicros), cmp.Compare(a.Key, b.Key))
	})
	return out
}

func costParts(m map[string]int64) []api.CostPart {
	out := make([]api.CostPart, 0, len(m))
	for k, v := range m {
		out = append(out, api.CostPart{Key: k, CostMicros: v})
	}
	slices.SortFunc(out, func(a, b api.CostPart) int {
		return cmp.Or(cmp.Compare(b.CostMicros, a.CostMicros), cmp.Compare(a.Key, b.Key))
	})
	return out
}

type costDayAcc struct {
	cost           int64
	runs           map[uuid.UUID]struct{}
	agents, models map[string]int64
}

type costTicketAcc struct {
	id   uuid.UUID
	cost int64
	runs map[uuid.UUID]*api.CostRun
}

// GetCosts aggregates spend from the run cost counters (LAZYAGENTS-60). Every
// figure comes from one CostUnits snapshot, so the total, each breakdown, the
// days and the top tickets reconcile with each other and with the cost chips.
func (s *Server) GetCosts(ctx context.Context, req api.GetCostsRequestObject) (api.GetCostsResponseObject, error) {
	u, _, ws := me(ctx)
	p := req.Params
	arg := store.CostUnitsParams{WorkspaceID: ws}
	var days []time.Time
	if (p.From == nil) != (p.To == nil) {
		return nil, apperr.Invalid("from", "give both from and to, or neither")
	}
	if p.From != nil {
		from, to := p.From.Time, p.To.Time
		if to.Before(from) {
			return nil, apperr.Invalid("to", "must not be before from")
		}
		if to.Sub(from) >= costMaxDays*24*time.Hour {
			return nil, apperr.Invalid("from", "the range is at most 366 days")
		}
		arg.FromDay, arg.ToDay = &from, &to
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			days = append(days, d)
		}
	}
	out := api.GetCosts200JSONResponse{
		From: p.From, To: p.To,
		Total: api.CostSlice{Key: "total", Label: "Total"},
		Days:  make([]api.CostDay, 0, len(days)), Agents: []api.CostSlice{}, Projects: []api.CostSlice{},
		Models: []api.CostSlice{}, Purposes: []api.CostSlice{}, TopTickets: []api.CostTicket{},
	}
	if p.Project != nil && *p.Project != "" {
		pr, err := s.Q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: ws, Key: *p.Project})
		if errors.Is(err, pgx.ErrNoRows) {
			fillCostDays(&out, days, nil)
			return out, nil
		} else if err != nil {
			return nil, err
		}
		arg.ProjectID = &pr.ID
	}
	if p.Ticket != nil && *p.Ticket != "" {
		t, err := s.Q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: ws, Key: *p.Ticket})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrNotFound
		} else if err != nil {
			return nil, err
		}
		arg.TicketID = &t.ID
	}
	if p.Chat != nil {
		// Chats are private to their creator, like their runs (readableRun).
		c, err := s.Q.GetChatSession(ctx, store.GetChatSessionParams{ID: *p.Chat, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.CreatedBy != u.ID) {
			return nil, apperr.ErrNotFound
		} else if err != nil {
			return nil, err
		}
		arg.ChatID = &c.ID
	}

	rows, err := s.Q.CostUnits(ctx, arg)
	if err != nil {
		return nil, err
	}
	agentList, err := s.Q.ListAllAgents(ctx, ws)
	if err != nil {
		return nil, err
	}
	agents := make(map[uuid.UUID]store.Agent, len(agentList))
	for _, a := range agentList {
		agents[a.ID] = a
	}
	projectList, err := s.Q.ListProjects(ctx, ws)
	if err != nil {
		return nil, err
	}
	projects := make(map[string]store.Project, len(projectList))
	for _, pr := range projectList {
		projects[pr.ID.String()] = pr
	}

	total := newCostAcc("total", "Total", "")
	byAgent, byProject, byModel, byPurpose := costGroup{}, costGroup{}, costGroup{}, costGroup{}
	byDay := map[string]*costDayAcc{}
	tickets := map[uuid.UUID]*costTicketAcc{}
	for _, r := range rows {
		if r.RunID == nil {
			continue
		}
		run := *r.RunID
		total.add(run, r)
		a := agents[r.AgentID]
		byAgent.add(a.Handle, a.Name, "", run, r)
		if pr, ok := projects[r.ProjectID]; ok {
			byProject.add(pr.Key, pr.Name, pr.Key, run, r)
		} else {
			byProject.add("", "No project", "", run, r)
		}
		modelKey := r.Harness + "/" + r.Model
		byModel.add(modelKey, r.Model, r.Harness, run, r)
		byPurpose.add(r.Purpose, cmp.Or(costPurposeLabels[r.Purpose], r.Purpose), "", run, r)

		day := r.Day.Format(costDay)
		d := byDay[day]
		if d == nil {
			d = &costDayAcc{runs: map[uuid.UUID]struct{}{}, agents: map[string]int64{}, models: map[string]int64{}}
			byDay[day] = d
		}
		d.cost += r.CostMicros
		d.runs[run] = struct{}{}
		d.agents[a.Handle] += r.CostMicros
		d.models[modelKey] += r.CostMicros

		if r.TicketID != nil {
			t := tickets[*r.TicketID]
			if t == nil {
				t = &costTicketAcc{id: *r.TicketID, runs: map[uuid.UUID]*api.CostRun{}}
				tickets[*r.TicketID] = t
			}
			t.cost += r.CostMicros
			cr := t.runs[run]
			if cr == nil {
				cr = &api.CostRun{Id: run, ParentRunId: r.ParentRunID, Agent: a.Handle, Model: r.Model, State: r.State, StartedAt: r.StartedAt}
				t.runs[run] = cr
			}
			cr.CostMicros += r.CostMicros
		}
	}
	out.Total = total.slice()
	out.UnpricedRuns = len(total.unpriced)
	out.Agents, out.Projects, out.Models, out.Purposes = byAgent.slices(), byProject.slices(), byModel.slices(), byPurpose.slices()
	fillCostDays(&out, days, byDay)
	if err := s.fillCostTickets(ctx, ws, &out, tickets); err != nil {
		return nil, err
	}
	return out, nil
}

// fillCostDays lists every day of the range, so gaps in spend show as empty days.
func fillCostDays(out *api.GetCosts200JSONResponse, days []time.Time, byDay map[string]*costDayAcc) {
	for _, day := range days {
		item := api.CostDay{Date: openapi_types.Date{Time: day}, Agents: []api.CostPart{}, Models: []api.CostPart{}}
		if d := byDay[day.Format(costDay)]; d != nil {
			item.CostMicros, item.Runs = d.cost, len(d.runs)
			item.Agents, item.Models = costParts(d.agents), costParts(d.models)
		}
		out.Days = append(out.Days, item)
	}
}

// fillCostTickets adds the most expensive tickets and the spend and points of
// every estimated ticket in the range (cost per point).
func (s *Server) fillCostTickets(ctx context.Context, ws uuid.UUID, out *api.GetCosts200JSONResponse, tickets map[uuid.UUID]*costTicketAcc) error {
	if len(tickets) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(tickets))
	for id := range tickets {
		ids = append(ids, id)
	}
	info, err := s.Q.CostTickets(ctx, store.CostTicketsParams{WorkspaceID: ws, Ids: ids})
	if err != nil {
		return err
	}
	ranked := make([]api.CostTicket, 0, len(info))
	for _, t := range info {
		acc := tickets[t.ID]
		if t.EstimatePoints != nil {
			out.Estimated.Tickets++
			out.Estimated.Points += int(*t.EstimatePoints)
			out.Estimated.CostMicros += acc.cost
		}
		if acc.cost <= 0 {
			continue
		}
		item := api.CostTicket{Key: t.Key, Title: t.Title, ProjectKey: t.ProjectKey, CostMicros: acc.cost, Runs: make([]api.CostRun, 0, len(acc.runs))}
		if t.EstimatePoints != nil {
			pts := int(*t.EstimatePoints)
			item.Points = &pts
		}
		for _, r := range acc.runs {
			item.Runs = append(item.Runs, *r)
		}
		slices.SortFunc(item.Runs, func(a, b api.CostRun) int {
			return cmp.Or(b.StartedAt.Compare(a.StartedAt), cmp.Compare(a.Id.String(), b.Id.String()))
		})
		ranked = append(ranked, item)
	}
	slices.SortFunc(ranked, func(a, b api.CostTicket) int {
		return cmp.Or(cmp.Compare(b.CostMicros, a.CostMicros), cmp.Compare(a.Key, b.Key))
	})
	out.TopTickets = ranked[:min(len(ranked), costTopTickets)]
	return nil
}
