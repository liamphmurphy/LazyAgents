package httpapi

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (s *Server) toPlanning(ctx context.Context, p store.PlanningSession) (api.PlanningSession, error) {
	project, err := s.Q.GetProject(ctx, store.GetProjectParams{ID: p.ProjectID, WorkspaceID: p.WorkspaceID})
	if err != nil {
		return api.PlanningSession{}, err
	}
	run, err := s.Q.GetRun(ctx, store.GetRunParams{ID: p.RunID, WorkspaceID: p.WorkspaceID})
	if err != nil {
		return api.PlanningSession{}, err
	}
	out := api.PlanningSession{Id: p.ID, Project: project.Key, EpicId: p.EpicID, ArtifactId: p.ArtifactID, SourceVersionId: p.SourceVersionID, Goal: p.Goal, RunId: p.RunID, RunState: run.State, Outcome: run.Outcome, Harness: run.Harness, Model: run.Model, State: api.PlanningSessionState(p.State), Revision: int(p.Revision), Proposals: []api.TicketProposal{}, TicketKeys: p.TicketKeys, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
	out.ReadyKeys = []string{}
	if len(p.TicketKeys) > 0 {
		out.ReadyKeys, err = s.Q.ReadyPlanningTickets(ctx, store.ReadyPlanningTicketsParams{WorkspaceID: p.WorkspaceID, Keys: p.TicketKeys})
		if err != nil {
			return out, err
		}
	}
	out.ProposalCount = int(p.ProposalCount)
	if out.ContextRefs, err = recode[[]api.ChatContextRef](jsonOrEmpty(p.ContextRefs)); err != nil {
		return out, err
	}
	err = json.Unmarshal(p.Proposals, &out.Proposals)
	return out, err
}
func (s *Server) ListPlanningSessions(ctx context.Context, _ api.ListPlanningSessionsRequestObject) (api.ListPlanningSessionsResponseObject, error) {
	_, _, ws := me(ctx)
	plans, err := s.Q.ListPlanningSessions(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := api.ListPlanningSessions200JSONResponse{}
	for _, p := range plans {
		item, err := s.toPlanning(ctx, p)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}
func (s *Server) GetPlanningSession(ctx context.Context, req api.GetPlanningSessionRequestObject) (api.GetPlanningSessionResponseObject, error) {
	_, _, ws := me(ctx)
	p, err := s.Q.GetPlanningSession(ctx, store.GetPlanningSessionParams{ID: req.Id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	out, err := s.toPlanning(ctx, p)
	return api.GetPlanningSession200JSONResponse(out), err
}
func (s *Server) StartPlanning(ctx context.Context, req api.StartPlanningRequestObject) (api.StartPlanningResponseObject, error) {
	u, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	in, err := recode[runs.PlanningInput](req.Body)
	if err != nil {
		return nil, err
	}
	p, err := s.Runs.StartPlanning(ctx, ws, u.ID, in)
	if err != nil {
		return nil, err
	}
	out, err := s.toPlanning(ctx, p)
	return api.StartPlanning201JSONResponse(out), err
}
func (s *Server) ReviewPlanning(ctx context.Context, req api.ReviewPlanningRequestObject) (api.ReviewPlanningResponseObject, error) {
	u, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	items, err := recode[[]runs.Proposal](req.Body.Proposals)
	if err != nil {
		return nil, err
	}
	if req.Body.Revision < 1 || req.Body.Revision > 2147483647 {
		return nil, apperr.Invalid("revision", "invalid revision")
	}
	p, err := s.Runs.ReviewPlanning(ctx, ws, u.ID, req.Id, int32(req.Body.Revision), string(req.Body.Action), items)
	if err != nil {
		return nil, err
	}
	out, err := s.toPlanning(ctx, p)
	return api.ReviewPlanning200JSONResponse(out), err
}
