package httpapi

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/broker"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

func (s *Server) toRun(ctx context.Context, r store.Run) (api.Run, error) {
	a, err := s.Q.GetAgent(ctx, store.GetAgentParams{ID: r.AgentID, WorkspaceID: r.WorkspaceID})
	if err != nil {
		return api.Run{}, err
	}
	t, err := s.Q.GetTicketByID(ctx, r.TicketID)
	if err != nil {
		return api.Run{}, err
	}
	out := api.Run{
		Id: r.ID, TicketKey: t.Key, Agent: *toAgentRef(a), Mode: api.RunMode(r.Mode), State: api.RunState(r.State),
		Harness: r.Harness, Model: r.Model, Outcome: r.Outcome, StartedAt: r.StartedAt, EndedAt: r.EndedAt,
		ToolCalls: int(r.ToolCalls), InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
		CostUsd: float32(r.CostUsdMicros) / 1e6, CostSource: r.CostSource,
		Plan: []api.PlanStep{}, Scopes: []string{}, ApprovalScopes: []string{},
	}
	if r.WaitReason != nil {
		w := api.RunWaitReason(*r.WaitReason)
		out.WaitReason = &w
	}
	if spec, err := runs.Spec(r); err == nil {
		for _, sc := range spec.Scopes {
			switch sc.Mode {
			case agentdef.Allowed:
				out.Scopes = append(out.Scopes, sc.Scope)
			case agentdef.RequiresApproval:
				out.ApprovalScopes = append(out.ApprovalScopes, sc.Scope)
			}
		}
	}
	plan, err := s.Q.ListPlan(ctx, r.ID)
	if err != nil {
		return out, err
	}
	for _, p := range plan {
		out.Plan = append(out.Plan, api.PlanStep{Position: int(p.Position), Text: p.Text, State: api.PlanStepState(p.State)})
	}
	return out, nil
}

func (s *Server) toApproval(ctx context.Context, ap store.ApprovalRequest, dir actorDirectory) (api.Approval, error) {
	t, err := s.Q.GetTicketByID(ctx, ap.TicketID)
	if err != nil {
		return api.Approval{}, err
	}
	detail := broker.DetailMap(ap.ActionDetail)
	out := api.Approval{
		Id: ap.ID, RunId: ap.RunID, TicketKey: t.Key, Scope: ap.Scope, ActionSummary: ap.ActionSummary,
		ActionDetail: detail, RiskSummary: ap.RiskSummary, State: api.ApprovalState(ap.State),
		DecidedAt: ap.DecidedAt, ExecutedAt: ap.ExecutedAt, CreatedAt: ap.CreatedAt,
	}
	if a, ok := dir.agents[ap.AgentID]; ok {
		out.Agent = *toAgentRef(a)
	}
	if action, ok := broker.For(ap.Scope); ok {
		out.Brokered = true
		preview := action.Describe(detail)
		out.ActionPreview = &preview
	}
	if ap.DecisionNote != "" {
		out.DecisionNote = &ap.DecisionNote
	}
	if ap.DecidedByUserID != nil {
		if u, ok := dir.users[*ap.DecidedByUserID]; ok {
			out.DecidedBy = toUserRef(u)
		}
	}
	if len(ap.Result) > 0 {
		m := map[string]any{}
		_ = json.Unmarshal(ap.Result, &m)
		out.Result = &m
	}
	return out, nil
}

// runParts fills a ticket's latest run and pending approvals.
func (s *Server) runParts(ctx context.Context, ws uuid.UUID, t store.Ticket, out *api.Ticket) error {
	out.Approvals = []api.Approval{}
	r, err := s.Q.LatestRunForTicket(ctx, t.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	run, err := s.toRun(ctx, r)
	if err != nil {
		return err
	}
	out.Run = &run
	pending, err := s.Q.PendingApprovalsForTicket(ctx, t.ID)
	if err != nil || len(pending) == 0 {
		return err
	}
	dir, err := s.directory(ctx, ws)
	if err != nil {
		return err
	}
	for _, ap := range pending {
		a, err := s.toApproval(ctx, ap, dir)
		if err != nil {
			return err
		}
		out.Approvals = append(out.Approvals, a)
	}
	return nil
}

func (s *Server) DecideApproval(ctx context.Context, req api.DecideApprovalRequestObject) (api.DecideApprovalResponseObject, error) {
	u, _, ws := me(ctx)
	note := ""
	if req.Body != nil && req.Body.Note != nil {
		note = *req.Body.Note
	}
	if req.Decision != "approve" && req.Decision != "deny" {
		return nil, apperr.Invalid("decision", "must be approve or deny")
	}
	ap, err := s.Runs.Decide(ctx, ws, u.ID, req.Id, req.Decision == "approve", note)
	if err != nil {
		return nil, err
	}
	dir, err := s.directory(ctx, ws)
	if err != nil {
		return nil, err
	}
	out, err := s.toApproval(ctx, ap, dir)
	return api.DecideApproval200JSONResponse(out), err
}

func (s *Server) ControlRun(ctx context.Context, req api.ControlRunRequestObject) (api.ControlRunResponseObject, error) {
	u, _, ws := me(ctx)
	r, err := s.Runs.SetRunControl(ctx, ws, u.ID, req.Id, string(req.Action))
	if err != nil {
		return nil, err
	}
	out, err := s.toRun(ctx, r)
	return api.ControlRun200JSONResponse(out), err
}

func (s *Server) GetRun(ctx context.Context, req api.GetRunRequestObject) (api.GetRunResponseObject, error) {
	_, _, ws := me(ctx)
	r, err := s.Q.GetRun(ctx, store.GetRunParams{ID: req.Id, WorkspaceID: ws})
	if err != nil {
		return nil, apperr.ErrNotFound
	}
	run, err := s.toRun(ctx, r)
	if err != nil {
		return nil, err
	}
	out := api.GetRun200JSONResponse{Run: run, Turns: []api.RunTurn{}, Approvals: []api.Approval{}, Events: []api.RunEvent{}}
	turns, err := s.Q.ListTurns(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	for _, t := range turns {
		rt := api.RunTurn{Seq: int(t.Seq), Trigger: api.RunTurnTrigger(t.Trigger), InputMessage: t.InputMessage, StartedAt: t.StartedAt, EndedAt: t.EndedAt}
		if t.ExitCode != nil {
			c := int(*t.ExitCode)
			rt.ExitCode = &c
		}
		out.Turns = append(out.Turns, rt)
	}
	dir, err := s.directory(ctx, ws)
	if err != nil {
		return nil, err
	}
	aps, err := s.Q.ListApprovalsForRun(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	for _, ap := range aps {
		a, err := s.toApproval(ctx, ap, dir)
		if err != nil {
			return nil, err
		}
		out.Approvals = append(out.Approvals, a)
	}
	after := int64(0)
	if req.Params.After != nil {
		after = *req.Params.After
	}
	evs, err := s.Q.ListRunEvents(ctx, store.ListRunEventsParams{RunID: r.ID, Seq: after})
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		m := map[string]any{}
		_ = json.Unmarshal(e.Payload, &m)
		out.Events = append(out.Events, api.RunEvent{Seq: e.Seq, Type: api.RunEventType(e.Type), Payload: m, At: e.At})
	}
	return out, nil
}
