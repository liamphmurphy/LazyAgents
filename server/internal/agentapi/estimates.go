package agentapi

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// Agent estimates and epic criteria (Architecture/ADR - Kanban board,
// estimates and epic criteria, Decision 6).

type EstimateIn struct {
	Ticket    string `json:"ticket" jsonschema:"Ticket key to estimate, e.g. PAY-214: your own ticket or another in its project"`
	Points    int    `json:"points" jsonschema:"Story points: 1, 2, 3, 5, 8 or 13"`
	Rationale string `json:"rationale" jsonschema:"One or two sentences on why (required, at most 2000 characters)"`
}

type EpicCriterionIn struct {
	TicketArg
	Epic string `json:"epic" jsonschema:"Epic id or exact name, from get_ticket's epic"`
	N    int    `json:"n" jsonschema:"Epic criterion number from get_ticket, from 1"`
	Done *bool  `json:"done,omitempty" jsonschema:"Default true; false unticks"`
}

// leadRun is the caller's own live, non-delegated ticket run. Chat, planning
// and delegated run tokens are already refused by define(); this also covers
// unmanaged sessions, which have no run token.
func leadRun(ctx context.Context, d Deps, c agents.Caller, ticket, tool string) (store.Run, store.Ticket, error) {
	run, t, err := d.Runs.Active(ctx, d.Q, c, ticket)
	if err != nil && c.RunID == nil && ticket != "" {
		run, t, err = d.Runs.Active(ctx, d.Q, c, "")
	}
	if err != nil {
		return run, t, err
	}
	if run.Purpose != "ticket" || run.TicketID == nil || run.ParentRunID != nil {
		return run, t, apperr.Conflict("%s is only available to the main agent of a ticket run", tool)
	}
	if slices.Contains([]string{"completed", "failed", "cancelled"}, run.State) {
		return run, t, apperr.Conflict("the run is %s", run.State)
	}
	return run, t, nil
}

func estimateTicket(ctx context.Context, d Deps, c agents.Caller, in EstimateIn) (any, error) {
	key := strings.ToUpper(strings.TrimSpace(in.Ticket))
	if key == "" {
		return nil, apperr.Invalid("ticket", "is required")
	}
	if !slices.Contains(tracker.EstimateScale, in.Points) {
		return nil, apperr.Invalid("points", "%d is off the scale: use one of 1, 2, 3, 5, 8, 13", in.Points)
	}
	rationale := strings.TrimSpace(in.Rationale)
	if rationale == "" {
		return nil, apperr.Invalid("rationale", "is required: say briefly why")
	}
	if len([]rune(rationale)) > 2000 {
		return nil, apperr.Invalid("rationale", "must be at most 2000 characters")
	}
	_, own, err := leadRun(ctx, d, c, key, "estimate_ticket")
	if err != nil {
		return nil, err
	}
	target, err := tracker.Get(ctx, d.Q, c.Agent.WorkspaceID, key)
	if err != nil || target.ProjectID != own.ProjectID {
		return nil, apperr.Invalid("ticket", "no ticket %s in this project", in.Ticket)
	}
	t, err := d.Tracker.Update(ctx, c.Agent.WorkspaceID, tracker.AgentActor(c.Agent.ID), target.Key,
		tracker.Patch{EstimatePoints: tracker.SetTo(in.Points), EstimateRationale: &rationale})
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"ticket": t.Key, "estimate_points": t.EstimatePoints, "rationale": t.EstimateRationale}), nil
}

func checkEpicCriterion(ctx context.Context, d Deps, c agents.Caller, in EpicCriterionIn) (any, error) {
	_, own, err := leadRun(ctx, d, c, in.Ticket, "check_epic_criterion")
	if err != nil {
		return nil, err
	}
	scopes, _, err := scopesFor(ctx, d, c, own.Key)
	if err != nil {
		return nil, err
	}
	if mode, _ := agentdef.ModeFor(scopes, "tracker:epics"); mode != agentdef.Allowed {
		return nil, apperr.Invalid("scope", "only agents whose tracker:epics scope is allowed can check epic criteria; comment on the ticket instead")
	}
	ws := c.Agent.WorkspaceID
	ref := strings.TrimSpace(in.Epic)
	var epic *store.Epic
	if id, err := uuid.Parse(ref); err == nil {
		if e, err := d.Q.GetEpic(ctx, store.GetEpicParams{ID: id, WorkspaceID: ws}); err == nil {
			epic = &e
		}
	} else if ref != "" {
		all, err := d.Q.ListEpics(ctx, ws)
		if err != nil {
			return nil, err
		}
		for i := range all {
			if all[i].ProjectID == own.ProjectID && all[i].Name == ref {
				epic = &all[i]
				break
			}
		}
	}
	if epic == nil || epic.ProjectID != own.ProjectID {
		return nil, apperr.Invalid("epic", "no epic %q in this project", in.Epic)
	}
	if epic.Status == "done" {
		return nil, apperr.Conflict("epic is done")
	}
	cs, err := d.Q.ListEpicCriteria(ctx, epic.ID)
	if err != nil {
		return nil, err
	}
	if in.N < 1 || in.N > len(cs) {
		return nil, apperr.Invalid("n", "must be between 1 and %d", len(cs))
	}
	done := in.Done == nil || *in.Done
	cr, err := d.Tracker.PatchEpicCriterion(ctx, ws, tracker.AgentActor(c.Agent.ID), cs[in.N-1].ID, nil, &done, nil)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"epic": epic.Name, "n": cr.Position, "done": cr.Done}), nil
}

// ticketEstimate is get_ticket's estimate, or nil when there is none.
func ticketEstimate(t store.Ticket) map[string]any {
	if t.EstimatePoints == nil {
		return nil
	}
	out := map[string]any{"points": *t.EstimatePoints, "rationale": t.EstimateRationale, "at": t.EstimatedAt}
	if t.EstimatedByType != nil {
		out["by"] = map[string]any{"type": *t.EstimatedByType, "id": t.EstimatedByID}
	}
	return out
}

// ticketEpic is get_ticket's parent epic with its numbered criteria. Epics
// have no key or title of their own: key is the id and title the name.
func ticketEpic(ctx context.Context, d Deps, t store.Ticket) (map[string]any, error) {
	if t.EpicID == nil {
		return nil, nil
	}
	e, err := d.Q.GetEpic(ctx, store.GetEpicParams{ID: *t.EpicID, WorkspaceID: t.WorkspaceID})
	if err != nil {
		return nil, err
	}
	cs, err := d.Q.ListEpicCriteria(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	criteria := make([]map[string]any, len(cs))
	for i, cr := range cs {
		criteria[i] = map[string]any{"n": i + 1, "text": cr.Text, "done": cr.Done}
	}
	out := map[string]any{"id": e.ID, "key": e.ID.String(), "name": e.Name, "title": e.Name, "status": e.Status, "criteria": criteria}
	roll, err := d.Q.EpicRollups(ctx, []uuid.UUID{e.ID})
	if err != nil {
		return nil, err
	}
	if len(roll) == 1 {
		out["points_total"], out["points_done"], out["tickets_open"] = roll[0].PointsTotal, roll[0].PointsDone, roll[0].TicketsOpen
	}
	return out, nil
}
