package agentapi

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A chat that can schedule a task for the person can also find, change, pause
// and remove one: otherwise a wrong time or a task they've outgrown meant
// sending them to the Scheduled page to fix what the persona set up.

type chatScheduledTask struct {
	ID           uuid.UUID  `json:"task_id"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	Instructions string     `json:"instructions,omitempty"`
	Frequency    string     `json:"frequency"`
	AtTime       string     `json:"at_time,omitempty"`
	Timezone     string     `json:"timezone"`
	Weekdays     []int32    `json:"weekdays,omitempty"`
	MonthDay     int32      `json:"month_day,omitempty"`
	Cron         string     `json:"cron,omitempty"`
	Enabled      bool       `json:"enabled"`
	NextRunAt    *time.Time `json:"next_run_at,omitempty"`
	Project      string     `json:"project,omitempty"`
	Revision     int32      `json:"revision"`
	LastRun      *lastRun   `json:"last_run,omitempty"`
}

type lastRun struct {
	State     string    `json:"state"`
	StartedAt time.Time `json:"started_at"`
	Error     string    `json:"error,omitempty"`
}

func toChatScheduledTask(t store.ScheduledTask, project string) chatScheduledTask {
	out := chatScheduledTask{ID: t.ID, Name: t.Name, Kind: t.Kind, Instructions: t.Instructions, Frequency: t.Frequency, Timezone: t.Timezone, Enabled: t.Enabled,
		NextRunAt: t.NextRunAt, Project: project, Revision: t.Revision}
	switch t.Frequency {
	case "cron":
		out.Cron = t.Cron
	case "weekly":
		out.AtTime, out.Weekdays = t.AtTime, t.Weekdays
	case "monthly":
		out.AtTime, out.MonthDay = t.AtTime, t.MonthDay
	default:
		out.AtTime = t.AtTime
	}
	return out
}

func listScheduledTasks(ctx context.Context, d Deps, c agents.Caller, _ Empty) (any, error) {
	s, err := personChatScope(ctx, d, c, "list_scheduled_tasks")
	if err != nil {
		return nil, err
	}
	ws := c.Agent.WorkspaceID
	rows, err := d.Q.ListScheduledTasks(ctx, store.ListScheduledTasksParams{WorkspaceID: ws, UserID: s.UserID})
	if err != nil {
		return nil, err
	}
	recent, err := d.Q.RecentScheduledRuns(ctx, store.RecentScheduledRunsParams{WorkspaceID: ws, UserID: s.UserID})
	if err != nil {
		return nil, err
	}
	projects, err := d.Q.ListProjects(ctx, ws)
	if err != nil {
		return nil, err
	}
	keys := make(map[uuid.UUID]string, len(projects))
	for _, p := range projects {
		keys[p.ID] = p.Key
	}
	out := make([]chatScheduledTask, len(rows))
	for i, t := range rows {
		project := ""
		if t.ProjectID != nil {
			project = keys[*t.ProjectID]
		}
		out[i] = toChatScheduledTask(t, project)
		// Newest first, so the first run of this task is its latest.
		for _, r := range recent {
			if r.TaskID == t.ID {
				out[i].LastRun = &lastRun{State: r.State, StartedAt: r.StartedAt, Error: r.Error}
				break
			}
		}
	}
	return map[string]any{"tasks": out, "href": "/scheduled"}, nil
}

type UpdateScheduledTaskIn struct {
	TaskID       uuid.UUID `json:"task_id" jsonschema:"Task id from list_scheduled_tasks"`
	Revision     int32     `json:"revision" jsonschema:"The revision from your latest list_scheduled_tasks. A stale one is refused: list again and redo your change"`
	Name         *string   `json:"name,omitempty" jsonschema:"A new name, 1–120 characters"`
	Instructions *string   `json:"instructions,omitempty" jsonschema:"New instructions, for a prompt task: what to do each time, written as the message you'll get"`
	Enabled      *bool     `json:"enabled,omitempty" jsonschema:"false pauses the task, true resumes it"`
	Frequency    *string   `json:"frequency,omitempty" jsonschema:"daily, weekly, monthly or cron"`
	AtTime       *string   `json:"at_time,omitempty" jsonschema:"Local time as HH:MM, 24-hour; not for cron"`
	Timezone     *string   `json:"timezone,omitempty" jsonschema:"IANA time zone, e.g. America/Los_Angeles"`
	Weekdays     *[]int32  `json:"weekdays,omitempty" jsonschema:"For weekly: days to run, 0 is Sunday"`
	MonthDay     *int32    `json:"month_day,omitempty" jsonschema:"For monthly: day of the month, 1–31"`
	Cron         *string   `json:"cron,omitempty" jsonschema:"For cron: a five-field cron expression"`
}

// updateScheduledTask changes what's given and keeps the rest, including the
// limits and connectors only the person sets in the app.
func updateScheduledTask(ctx context.Context, d Deps, c agents.Caller, in UpdateScheduledTaskIn) (any, error) {
	s, err := personChatScope(ctx, d, c, "update_scheduled_task")
	if err != nil {
		return nil, err
	}
	ws := c.Agent.WorkspaceID
	t, err := d.Q.GetScheduledTask(ctx, store.GetScheduledTaskParams{ID: in.TaskID, WorkspaceID: ws, UserID: s.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.Invalid("task_id", "no scheduled task %s", in.TaskID)
	} else if err != nil {
		return nil, err
	}
	if in.Instructions != nil && t.Kind != "prompt" {
		return nil, apperr.Invalid("instructions", "this task runs a workflow; change the workflow instead")
	}
	project, err := runs.OptionalProject(ctx, d.Q, ws, t.ProjectID)
	if err != nil {
		return nil, err
	}
	next := runs.ScheduledTaskInput{Revision: in.Revision, Name: t.Name, Kind: t.Kind, Instructions: t.Instructions, WorkflowID: t.WorkflowID, PersonaID: t.PersonaID, Project: project.Key,
		ConnectorIDs: t.ConnectorIds, Enabled: t.Enabled, MaxMinutes: t.MaxMinutes, MaxCostMicros: t.MaxCostMicros,
		Schedule: runs.Schedule{Frequency: t.Frequency, Weekdays: t.Weekdays, MonthDay: t.MonthDay, AtTime: t.AtTime, Timezone: t.Timezone, Cron: t.Cron}}
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&next.Name, in.Name)
	set(&next.Instructions, in.Instructions)
	set(&next.Schedule.Frequency, in.Frequency)
	set(&next.Schedule.AtTime, in.AtTime)
	set(&next.Schedule.Timezone, in.Timezone)
	set(&next.Schedule.Cron, in.Cron)
	if in.Enabled != nil {
		next.Enabled = *in.Enabled
	}
	if in.Weekdays != nil {
		next.Schedule.Weekdays = *in.Weekdays
	}
	if in.MonthDay != nil {
		next.Schedule.MonthDay = *in.MonthDay
	}
	updated, err := d.Runs.UpdateScheduledTask(ctx, ws, s.UserID, t.ID, next)
	if err != nil {
		return nil, err
	}
	return map[string]any{"task": toChatScheduledTask(updated, project.Key), "href": "/scheduled",
		"tell_the_person": "Changed. Say what's different and when it next runs; they can review it in [Scheduled](/scheduled)."}, nil
}

type DeleteScheduledTaskIn struct {
	TaskID uuid.UUID `json:"task_id" jsonschema:"Task id from list_scheduled_tasks"`
}

func deleteScheduledTask(ctx context.Context, d Deps, c agents.Caller, in DeleteScheduledTaskIn) (any, error) {
	s, err := personChatScope(ctx, d, c, "delete_scheduled_task")
	if err != nil {
		return nil, err
	}
	n, err := d.Q.DeleteScheduledTask(ctx, store.DeleteScheduledTaskParams{ID: in.TaskID, WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, apperr.Invalid("task_id", "no scheduled task %s", in.TaskID)
	}
	return map[string]any{"task_id": in.TaskID, "state": "deleted", "tell_the_person": "Deleted: it won't run again."}, nil
}
