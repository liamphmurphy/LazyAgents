package runs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const (
	scheduledTriggerSchedule = "schedule"
	scheduledTriggerManual   = "manual"
	scheduledTriggerCatchUp  = "catchup"
	// A task this late was missed while Vesper was off, not just picked up by the poll.
	scheduledLateAfter = 2 * time.Minute
	// A run with no chat this long means the process died while starting it.
	scheduledStartGrace = 5 * time.Minute

	scheduledPromptNote = "\n\n(Vesper: this is a scheduled task and nobody is watching it run. Don't ask questions: do the work and end with the result, which goes to the person's Inbox.)"
)

// ScheduledTaskInput creates or replaces a task.
type ScheduledTaskInput struct {
	Revision      int32
	Name          string
	Kind          string
	Instructions  string
	WorkflowID    *uuid.UUID
	PersonaID     *uuid.UUID
	Project       string
	ConnectorIDs  []uuid.UUID
	Schedule      Schedule
	Enabled       bool
	MaxMinutes    int32
	MaxCostMicros int64
}

func (s *Service) scheduledFields(ctx context.Context, q *store.Queries, ws, user uuid.UUID, in *ScheduledTaskInput) (*uuid.UUID, *time.Time, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 120 {
		return nil, nil, apperr.Invalid("name", "must be 1–120 characters")
	}
	if in.MaxMinutes == 0 {
		in.MaxMinutes = 15
	}
	if in.MaxCostMicros == 0 {
		in.MaxCostMicros = 1_000_000
	}
	if in.MaxMinutes < 1 || in.MaxMinutes > 240 {
		return nil, nil, apperr.Invalid("max_minutes", "must be 1–240")
	}
	if in.MaxCostMicros < 1 {
		return nil, nil, apperr.Invalid("max_cost_micros", "must be positive")
	}
	switch in.Kind {
	case "prompt":
		in.Instructions = strings.TrimSpace(in.Instructions)
		if in.Instructions == "" || utf8.RuneCountInString(in.Instructions) > 20000 {
			return nil, nil, apperr.Invalid("instructions", "provide 1–20000 characters")
		}
		in.WorkflowID = nil
	case "workflow":
		if in.WorkflowID == nil {
			return nil, nil, apperr.Invalid("workflow_id", "choose a workflow")
		}
		w, err := q.GetWorkflow(ctx, store.GetWorkflowParams{ID: *in.WorkflowID, WorkspaceID: ws, UserID: user})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, apperr.Invalid("workflow_id", "no such workflow")
		}
		if err != nil {
			return nil, nil, err
		}
		if w.Published == nil {
			return nil, nil, apperr.Invalid("workflow_id", "publish this workflow first")
		}
		in.ConnectorIDs = nil // Workflow steps don't use connectors yet.
	default:
		return nil, nil, apperr.Invalid("kind", "must be prompt or workflow")
	}
	if in.ConnectorIDs == nil {
		in.ConnectorIDs = []uuid.UUID{}
	}
	if in.PersonaID != nil {
		if _, err := q.GetPersona(ctx, store.GetPersonaParams{ID: *in.PersonaID, WorkspaceID: ws, UserID: user}); errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, apperr.Invalid("persona_id", "unknown persona")
		} else if err != nil {
			return nil, nil, err
		}
	}
	for _, id := range in.ConnectorIDs {
		if _, err := q.GetConnector(ctx, store.GetConnectorParams{ID: id, WorkspaceID: ws, UserID: user}); errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, apperr.Invalid("connector_ids", "unknown connector")
		} else if err != nil {
			return nil, nil, err
		}
	}
	var project *uuid.UUID
	if in.Project != "" {
		p, err := lockAttachableProject(ctx, q, ws, in.Project)
		if err != nil {
			return nil, nil, err
		}
		project = &p.ID
	}
	next, err := in.Schedule.Next(time.Now())
	if err != nil {
		return nil, nil, err
	}
	if !in.Enabled {
		return project, nil, nil
	}
	return project, &next, nil
}

func (s *Service) CreateScheduledTask(ctx context.Context, ws, user uuid.UUID, in ScheduledTaskInput) (store.ScheduledTask, error) {
	var out store.ScheduledTask
	err := s.tx(ctx, func(q *store.Queries) error {
		project, next, err := s.scheduledFields(ctx, q, ws, user, &in)
		if err != nil {
			return err
		}
		out, err = q.CreateScheduledTask(ctx, store.CreateScheduledTaskParams{WorkspaceID: ws, UserID: user, Name: in.Name, Kind: in.Kind, Instructions: in.Instructions, WorkflowID: in.WorkflowID, PersonaID: in.PersonaID, ProjectID: project, ConnectorIds: in.ConnectorIDs,
			Frequency: in.Schedule.Frequency, Weekdays: in.Schedule.Weekdays, MonthDay: in.Schedule.MonthDay, AtTime: in.Schedule.AtTime, Timezone: in.Schedule.Timezone, Cron: in.Schedule.Cron, Enabled: in.Enabled, MaxMinutes: in.MaxMinutes, MaxCostMicros: in.MaxCostMicros, NextRunAt: next})
		return err
	})
	return out, err
}

func (s *Service) UpdateScheduledTask(ctx context.Context, ws, user, id uuid.UUID, in ScheduledTaskInput) (store.ScheduledTask, error) {
	var out store.ScheduledTask
	err := s.tx(ctx, func(q *store.Queries) error {
		if _, err := q.GetScheduledTask(ctx, store.GetScheduledTaskParams{ID: id, WorkspaceID: ws, UserID: user}); errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		project, next, err := s.scheduledFields(ctx, q, ws, user, &in)
		if err != nil {
			return err
		}
		out, err = q.UpdateScheduledTask(ctx, store.UpdateScheduledTaskParams{ID: id, WorkspaceID: ws, Revision: in.Revision, Name: in.Name, Kind: in.Kind, Instructions: in.Instructions, WorkflowID: in.WorkflowID, PersonaID: in.PersonaID, ProjectID: project, ConnectorIds: in.ConnectorIDs,
			Frequency: in.Schedule.Frequency, Weekdays: in.Schedule.Weekdays, MonthDay: in.Schedule.MonthDay, AtTime: in.Schedule.AtTime, Timezone: in.Schedule.Timezone, Cron: in.Schedule.Cron, Enabled: in.Enabled, MaxMinutes: in.MaxMinutes, MaxCostMicros: in.MaxCostMicros, NextRunAt: next})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("the task changed; reload and try again")
		}
		return err
	})
	return out, err
}

func (s *Service) projectKey(ctx context.Context, q *store.Queries, ws uuid.UUID, id *uuid.UUID) (string, error) {
	p, err := OptionalProject(ctx, q, ws, id)
	return p.Key, err
}

// launchScheduled opens the hidden chat a run lives in and starts its work.
func (s *Service) launchScheduled(ctx context.Context, q *store.Queries, t store.ScheduledTask, run store.ScheduledTaskRun) error {
	key, err := s.projectKey(ctx, q, t.WorkspaceID, t.ProjectID)
	if err != nil {
		return err
	}
	in := ChatInput{Project: key, Title: t.Name, PersonaID: t.PersonaID, Hidden: true}
	if t.Kind == "prompt" {
		in.ConnectorIDs = t.ConnectorIds
		in.Message = &ChatMessageInput{Body: t.Instructions + scheduledPromptNote}
	}
	chat, err := s.StartChat(ctx, t.WorkspaceID, t.UserID, in)
	if err != nil {
		return err
	}
	var workflowRun *uuid.UUID
	if t.Kind == "workflow" {
		wf, err := s.StartWorkflow(ctx, t.WorkspaceID, t.UserID, chat.ID, WorkflowStart{WorkflowID: *t.WorkflowID, Input: t.Instructions})
		if err != nil {
			return err
		}
		workflowRun = &wf.ID
	}
	return q.AttachScheduledRun(ctx, store.AttachScheduledRunParams{ID: run.ID, ChatID: &chat.ID, WorkflowRunID: workflowRun})
}

func (s *Service) startScheduled(ctx context.Context, t store.ScheduledTask, run store.ScheduledTaskRun) {
	q := store.New(s.Pool)
	if err := s.launchScheduled(ctx, q, t, run); err != nil {
		slog.Warn("scheduled task didn't start", "task", t.ID, "err", err)
		_ = q.FinishScheduledRun(context.WithoutCancel(ctx), store.FinishScheduledRunParams{ID: run.ID, State: "failed", Error: "It couldn't start: " + err.Error()})
	}
}

// RunScheduledTaskNow starts a task outside its schedule, leaving the schedule alone.
func (s *Service) RunScheduledTaskNow(ctx context.Context, ws, user, id uuid.UUID) error {
	var t store.ScheduledTask
	var run store.ScheduledTaskRun
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		if t, err = q.GetScheduledTask(ctx, store.GetScheduledTaskParams{ID: id, WorkspaceID: ws, UserID: user}); errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		busy, err := q.HasActiveScheduledRun(ctx, id)
		if err != nil {
			return err
		}
		if busy {
			return apperr.Conflict("this task is already running")
		}
		run, err = q.CreateScheduledRun(ctx, store.CreateScheduledRunParams{TaskID: id, Trigger: scheduledTriggerManual})
		return err
	})
	if err != nil {
		return err
	}
	s.startScheduled(ctx, t, run)
	return nil
}

// RunDueScheduled starts every task that has come due. Each is claimed once
// and moved past now, so downtime costs one catch-up run however many times
// were missed. A task still running skips this turn instead of stacking.
func (s *Service) RunDueScheduled(ctx context.Context) error {
	for ctx.Err() == nil {
		var t store.ScheduledTask
		var run *store.ScheduledTaskRun
		err := s.tx(ctx, func(q *store.Queries) error {
			var err error
			if t, err = q.NextDueScheduledTask(ctx); err != nil {
				return err
			}
			now := time.Now()
			late := t.NextRunAt != nil && now.Sub(*t.NextRunAt) > scheduledLateAfter
			sc := Schedule{Frequency: t.Frequency, Weekdays: t.Weekdays, MonthDay: t.MonthDay, AtTime: t.AtTime, Timezone: t.Timezone, Cron: t.Cron}
			next, err := sc.Next(now)
			if err != nil {
				// Can't be scheduled any more (say, a removed time zone): stop trying.
				return q.AdvanceScheduledTask(ctx, store.AdvanceScheduledTaskParams{ID: t.ID})
			}
			if err := q.AdvanceScheduledTask(ctx, store.AdvanceScheduledTaskParams{ID: t.ID, NextRunAt: &next}); err != nil {
				return err
			}
			busy, err := q.HasActiveScheduledRun(ctx, t.ID)
			if err != nil || busy {
				return err
			}
			trigger := scheduledTriggerSchedule
			if late {
				trigger = scheduledTriggerCatchUp
			}
			r, err := q.CreateScheduledRun(ctx, store.CreateScheduledRunParams{TaskID: t.ID, Trigger: trigger})
			run = &r
			return err
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if run != nil {
			s.startScheduled(ctx, t, *run)
		}
	}
	return ctx.Err()
}

// SettleScheduled records runs that ended and stops any over its limits. Time
// spent waiting on the person's approval doesn't count against a run's minutes.
func (s *Service) SettleScheduled(ctx context.Context) error {
	q := store.New(s.Pool)
	active, err := q.ActiveScheduledRuns(ctx)
	if err != nil {
		return err
	}
	for _, r := range active {
		if err := s.settleScheduled(ctx, q, r); err != nil && ctx.Err() == nil {
			slog.Warn("settle scheduled run", "run", r.ID, "err", err)
		}
	}
	return nil
}

func (s *Service) settleScheduled(ctx context.Context, q *store.Queries, r store.ActiveScheduledRunsRow) error {
	finish := func(state, summary, msg string, cost int64) error {
		return q.FinishScheduledRun(ctx, store.FinishScheduledRunParams{ID: r.ID, State: state, Summary: summary, Error: msg, CostMicros: cost})
	}
	if r.ChatID == nil {
		if time.Since(r.StartedAt) > scheduledStartGrace {
			return finish("failed", "", "It never started.", 0)
		}
		return nil
	}
	if r.WorkflowRunID != nil {
		wf, err := q.GetWorkflowRunByID(ctx, *r.WorkflowRunID)
		if err != nil {
			return err
		}
		switch wf.State {
		case "done":
			return finish("done", "The workflow finished.", "", 0)
		case "failed", "stopped":
			return finish("failed", "", firstNonEmpty(wf.Error, "The workflow stopped."), 0)
		case "waiting":
			return q.SetScheduledRunState(ctx, store.SetScheduledRunStateParams{ID: r.ID, State: "waiting"})
		}
		return q.SetScheduledRunState(ctx, store.SetScheduledRunStateParams{ID: r.ID, State: "running"})
	}
	chat, err := q.GetChatSession(ctx, store.GetChatSessionParams{ID: *r.ChatID, WorkspaceID: r.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return finish("failed", "", "Its conversation was deleted.", 0)
	}
	if err != nil {
		return err
	}
	var cost int64
	if chat.CurrentRunID != nil {
		run, err := q.GetRun(ctx, store.GetRunParams{ID: *chat.CurrentRunID, WorkspaceID: r.WorkspaceID})
		if err != nil {
			return err
		}
		cost = run.CostUsdMicros
	}
	switch chat.State {
	case "failed":
		return finish("failed", "", "The reply failed. Open the conversation for details.", cost)
	case "idle":
		body, err := q.LastAssistantBody(ctx, chat.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return finish("failed", "", "It ended without a result.", cost)
		}
		if err != nil {
			return err
		}
		return finish("done", body, "", cost)
	}
	waiting, err := q.HasPendingConnectorApproval(ctx, chat.ID)
	if err != nil {
		return err
	}
	if !waiting {
		if waiting, err = q.HasPendingUserSecretApproval(ctx, chat.ID); err != nil {
			return err
		}
	}
	if waiting {
		return q.SetScheduledRunState(ctx, store.SetScheduledRunStateParams{ID: r.ID, State: "waiting"})
	}
	if err := q.SetScheduledRunState(ctx, store.SetScheduledRunStateParams{ID: r.ID, State: "running"}); err != nil {
		return err
	}
	limit := ""
	if time.Since(r.StartedAt) > time.Duration(r.MaxMinutes)*time.Minute {
		limit = "It reached its time limit and was stopped."
	} else if cost > r.MaxCostMicros {
		limit = "It reached its cost limit and was stopped."
	}
	if limit == "" {
		return nil
	}
	if _, err := s.CancelChat(ctx, r.WorkspaceID, r.UserID, chat.ID); err != nil {
		return err
	}
	return finish("failed", "", limit, cost)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
