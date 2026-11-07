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
	var t store.Ticket
	if r.TicketID != nil {
		t, err = s.Q.GetTicketByID(ctx, *r.TicketID)
		if err != nil {
			return api.Run{}, err
		}
	}
	out := api.Run{
		Id: r.ID, TicketKey: t.Key, Purpose: api.RunPurpose(r.Purpose), Agent: *toAgentRef(a), Mode: api.RunMode(r.Mode), State: api.RunState(r.State),
		Harness: r.Harness, Model: r.Model, Outcome: r.Outcome, StartedAt: r.StartedAt, EndedAt: r.EndedAt,
		ToolCalls: int(r.ToolCalls), InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
		CostUsd: float32(r.CostUsdMicros) / 1e6, CostSource: r.CostSource,
		SetupMs: r.SetupMs, HarnessMs: r.HarnessMs, ChecksMs: r.ChecksMs,
		Plan: []api.PlanStep{}, Scopes: []string{}, ApprovalScopes: []string{},
	}
	cacheRows, err := s.Q.CacheUsageForRuns(ctx, []uuid.UUID{r.ID})
	if err != nil {
		return out, err
	}
	var cached, reported int64
	if len(cacheRows) > 0 {
		cached, reported = cacheRows[0].CachedInputTokens, cacheRows[0].ReportedInputTokens
	}
	cached, uncached, unknown := cacheBreakdown(r.InputTokens, cached, reported)
	out.CachedInputTokens, out.UncachedInputTokens, out.UnreportedInputTokens = &cached, &uncached, &unknown
	var projectID uuid.UUID
	switch {
	case r.TicketID != nil:
		projectID = t.ProjectID
		out.WorkTitle = &t.Title
		out.Branch, out.PrUrl, out.PrState = t.Branch, t.PrUrl, t.PrState
		if t.PrNumber != nil {
			n := int(*t.PrNumber)
			out.PrNumber = &n
		}
	case r.Purpose == "planning":
		p, err := s.Q.PlanningSessionForRun(ctx, r.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		if err == nil {
			out.PlanningId, out.WorkTitle, projectID = &p.ID, &p.Goal, p.ProjectID
		}
	case r.Purpose == "chat":
		// A chat run without its session (deleted chat) still renders.
		if c, err := s.Q.ChatSessionOfRun(ctx, &r.ID); err == nil {
			out.ChatId, out.WorkTitle, projectID = &c.ID, &c.Title, c.ProjectID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
	}
	if projectID != uuid.Nil {
		if p, err := s.Q.GetProject(ctx, store.GetProjectParams{ID: projectID, WorkspaceID: r.WorkspaceID}); err == nil {
			out.ProjectKey = &p.Key
		}
	}
	if r.ParentRunID == nil {
		n, err := s.Q.CountChildRuns(ctx, &r.ID)
		if err != nil {
			return out, err
		}
		c := int(n)
		out.ChildCount = &c
	}
	if r.WaitReason != nil {
		w := api.RunWaitReason(*r.WaitReason)
		out.WaitReason = &w
	}
	out.ParentRunId = r.ParentRunID
	out.SessionId = r.HarnessSessionID
	if r.DispatchedByUserID != nil {
		if u, err := s.Q.GetUser(ctx, store.GetUserParams{ID: *r.DispatchedByUserID, WorkspaceID: r.WorkspaceID}); err == nil {
			out.DispatchedBy = toUserRef(u)
		}
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
	} else if _, ok := detail["tool_name"]; ok {
		preview, _ := json.MarshalIndent(detail, "", "  ")
		text := string(preview)
		out.ActionPreview = &text
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
	r, err := s.Q.LatestRunForTicket(ctx, &t.ID)
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
	out := api.GetRun200JSONResponse{Run: run, Turns: []api.RunTurn{}, Approvals: []api.Approval{}, Events: []api.RunEvent{}, ChildRuns: []api.Run{}}
	children, err := s.Q.ChildRuns(ctx, &r.ID)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if child.WorkspaceID != ws {
			continue
		}
		cr, err := s.toRun(ctx, child)
		if err != nil {
			return nil, err
		}
		out.ChildRuns = append(out.ChildRuns, cr)
	}
	turns, err := s.Q.ListTurns(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	for _, t := range turns {
		rt := api.RunTurn{Seq: int(t.Seq), Trigger: api.RunTurnTrigger(t.Trigger), InputMessage: t.InputMessage, StartedAt: t.StartedAt, EndedAt: t.EndedAt,
			Status: api.RunTurnStatus(t.Status), ProcessStartedAt: t.ProcessStartedAt, HasLog: t.LogKey != nil}
		if len(t.Usage) > 2 {
			u := map[string]any{}
			if json.Unmarshal(t.Usage, &u) == nil {
				rt.Usage = &u
			}
		}
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

// liveRunStates are the states the Runs index shows by default.
var liveRunStates = []string{"queued", "starting", "running", "waiting", "paused", "blocked"}

func (s *Server) ListRuns(ctx context.Context, req api.ListRunsRequestObject) (api.ListRunsResponseObject, error) {
	_, _, ws := me(ctx)
	out := api.ListRuns200JSONResponse{Runs: []api.Run{}}
	pr := req.Params
	offset := 0
	if pr.Offset != nil {
		offset = *pr.Offset
	}
	if offset < 0 || offset > 100000 {
		return nil, apperr.Invalid("offset", "must be between 0 and 100000")
	}
	arg := store.ListRunsParams{WorkspaceID: ws, States: liveRunStates, Skip: int32(offset)}
	if pr.State != nil {
		switch st := *pr.State; st {
		case api.ListRunsParamsStateLive:
		case api.ListRunsParamsStateAll:
			arg.States = append(append([]string{}, liveRunStates...), "completed", "failed", "cancelled")
		default:
			if !st.Valid() {
				return nil, apperr.Invalid("state", "unknown run state")
			}
			arg.States = []string{string(st)}
		}
	}
	if pr.Purpose != nil {
		if !pr.Purpose.Valid() {
			return nil, apperr.Invalid("purpose", "must be ticket, planning or chat")
		}
		p := string(*pr.Purpose)
		arg.Purpose = &p
	}
	// An unknown agent or project matches nothing rather than failing, so a
	// stale filter in the URL shows an empty table.
	if pr.Agent != nil && *pr.Agent != "" {
		a, err := s.Q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: *pr.Agent})
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		} else if err != nil {
			return nil, err
		}
		arg.AgentID = &a.ID
	}
	if pr.Project != nil && *pr.Project != "" {
		p, err := s.Q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: ws, Key: *pr.Project})
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		} else if err != nil {
			return nil, err
		}
		arg.ProjectID = &p.ID
	}
	rows, err := s.Q.ListRuns(ctx, arg)
	if err != nil {
		return nil, err
	}
	if len(rows) > 50 {
		out.HasMore, rows = true, rows[:50]
	}
	for _, r := range rows {
		run, err := s.toRun(ctx, r)
		if err != nil {
			return nil, err
		}
		out.Runs = append(out.Runs, run)
	}
	return out, nil
}
