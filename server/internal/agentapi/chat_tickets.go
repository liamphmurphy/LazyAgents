package agentapi

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

type UpdateTicketIn struct {
	Ticket            string     `json:"ticket" jsonschema:"Ticket key, e.g. PAY-214"`
	Revision          int        `json:"revision" jsonschema:"The revision from your latest get_ticket. A stale one is refused: read the ticket again and redo your edit"`
	Title             *string    `json:"title,omitempty"`
	Description       *string    `json:"description,omitempty" jsonschema:"Markdown; replaces the whole description"`
	Status            *string    `json:"status,omitempty" jsonschema:"backlog, todo, in_progress, in_review or done. Refused while the ticket has a live run, which owns its status"`
	Priority          *string    `json:"priority,omitempty" jsonschema:"none, low, medium, high or urgent"`
	Points            *int       `json:"points,omitempty" jsonschema:"Story points: 1, 2, 3, 5, 8 or 13"`
	EstimateRationale string     `json:"estimate_rationale,omitempty" jsonschema:"Why that estimate, in a sentence"`
	EpicID            *uuid.UUID `json:"epic_id,omitempty" jsonschema:"Move the ticket into this epic of its project"`
	Agent             *string    `json:"agent,omitempty" jsonschema:"Assign a main agent by handle, e.g. pike; empty string unassigns"`
	RequiresMockup    *bool      `json:"requires_mockup,omitempty"`
	PRURL             string     `json:"pr_url,omitempty" jsonschema:"GitHub pull request URL for work on this ticket done from this chat. Vesper then follows the PR and moves the ticket to Done when it merges"`
	PRDraft           bool       `json:"pr_draft,omitempty"`
	Criteria          *[]string  `json:"criteria,omitempty" jsonschema:"Replaces the acceptance criteria with this list, in order. A criterion whose text is unchanged keeps its tick; an empty list removes them all"`
}

type CommentTicketIn struct {
	Ticket string `json:"ticket" jsonschema:"Ticket key, e.g. PAY-214"`
	Body   string `json:"body" jsonschema:"Markdown"`
}

type TicketBlockersIn struct {
	Ticket string   `json:"ticket" jsonschema:"Ticket key, e.g. PAY-214"`
	Add    []string `json:"add,omitempty" jsonschema:"Keys of tickets that must be Done before this one starts"`
	Remove []string `json:"remove,omitempty" jsonschema:"Keys of blockers to drop"`
}

type GetEpicIn struct {
	EpicID uuid.UUID `json:"epic_id" jsonschema:"Epic id from list_epics"`
}

type UpdateEpicIn struct {
	EpicID         uuid.UUID          `json:"epic_id" jsonschema:"Epic id from list_epics"`
	RequiresMockup *bool              `json:"requires_mockup,omitempty" jsonschema:"Every ticket in the epic needs a person-approved UI mockup before it can be finished"`
	Status         *string            `json:"status,omitempty" jsonschema:"open or done. Done is refused while a criterion is unticked or a ticket is open"`
	AddCriteria    []string           `json:"add_criteria,omitempty" jsonschema:"Acceptance criteria to append, one checkable sentence each"`
	EditCriteria   []EpicCriterionArg `json:"edit_criteria,omitempty" jsonschema:"New text for criteria, numbered as in get_epic"`
	RemoveCriteria []int              `json:"remove_criteria,omitempty" jsonschema:"Criteria to remove, numbered as in get_epic"`
}

type EpicCriterionArg struct {
	N    int    `json:"n" jsonschema:"The criterion's number in get_epic"`
	Text string `json:"text"`
}

// maxNewCriteria bounds one tool call's criteria; people can add more by hand.
const maxNewCriteria = 50

// criteriaTexts checks acceptance criteria from a tool call: a person reads each
// one as a checkbox, so blank or essay-length entries are refused, not dropped.
func criteriaTexts(in []string) ([]string, error) {
	if len(in) > maxNewCriteria {
		return nil, apperr.Invalid("criteria", "pass at most %d criteria", maxNewCriteria)
	}
	out := make([]string, len(in))
	for i, c := range in {
		c = strings.TrimSpace(c)
		if c == "" {
			return nil, apperr.Invalid("criteria", "criterion %d is empty", i+1)
		}
		if utf8.RuneCountInString(c) > 1000 {
			return nil, apperr.Invalid("criteria", "criterion %d is over 1000 characters", i+1)
		}
		out[i] = c
	}
	return out, nil
}

// criteriaDraft replaces a ticket's criteria with texts. The snapshot it reads
// is checked again under the ticket's lock, so an edit made meanwhile is a
// conflict rather than lost. Unchanged texts keep their id and tick.
func criteriaDraft(ctx context.Context, d Deps, ws uuid.UUID, key string, texts []string) (*tracker.CriteriaDraft, error) {
	t, err := tracker.Get(ctx, d.Q, ws, key)
	if err != nil {
		return nil, apperr.Invalid("ticket", "no ticket %s", key)
	}
	cur, err := d.Q.ListCriteria(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	draft := &tracker.CriteriaDraft{}
	unused := make([]bool, len(cur))
	for i, c := range cur {
		draft.Expected = append(draft.Expected, tracker.ExpectedCriterion{ID: c.ID, Position: int(c.Position), Text: c.Text, Done: c.Done})
		unused[i] = true
	}
	for _, text := range texts {
		item := tracker.CriterionDraft{Text: text}
		for i, c := range cur {
			if unused[i] && c.Text == text {
				id := c.ID
				item.ID, item.Done, unused[i] = &id, c.Done, false
				break
			}
		}
		draft.Items = append(draft.Items, item)
	}
	return draft, nil
}

// chatActor is the assistant acting from this chat: each timeline event it
// writes names the chat and persona, so a person can see where an edit came
// from and open the conversation behind it.
func chatActor(c agents.Caller, s *chatScope) tracker.Actor {
	a := tracker.AgentActor(c.Agent.ID)
	a.Via = map[string]any{"chat_id": s.ChatID}
	if s.PersonaID != nil {
		a.Via["persona_id"] = *s.PersonaID
	}
	return a
}

// updateChatTicket edits an existing ticket from a chat. Tickets with a live
// run are the run's to move, so status and PR edits are refused then; work a
// persona does itself in the chat has no run, and this is how its ticket
// keeps up (LAZYAGENTS-100).
func updateChatTicket(ctx context.Context, d Deps, c agents.Caller, in UpdateTicketIn) (any, error) {
	s, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	key := strings.ToUpper(strings.TrimSpace(in.Ticket))
	if key == "" {
		return nil, apperr.Invalid("ticket", "is required")
	}
	if in.Revision <= 0 {
		return nil, apperr.Invalid("revision", "is required: pass the revision from get_ticket")
	}
	rev := in.Revision
	p := tracker.Patch{Revision: &rev, Title: in.Title, DescriptionMd: in.Description, Status: in.Status, Priority: in.Priority, RequiresMockup: in.RequiresMockup}
	if in.Status != nil && *in.Status == "canceled" {
		return nil, apperr.Conflict("only a person may cancel a ticket; ask them to")
	}
	if in.Points != nil {
		est, rationale, err := NewTicketIn{Points: in.Points, EstimateRationale: in.EstimateRationale}.estimate()
		if err != nil {
			return nil, err
		}
		p.EstimatePoints, p.EstimateRationale = tracker.SetTo(*est), rationale
	}
	if in.EpicID != nil {
		p.EpicID = tracker.SetTo(*in.EpicID)
	}
	if in.Agent != nil {
		id, err := mainAgent(ctx, d, c.Agent.WorkspaceID, *in.Agent)
		if err != nil {
			return nil, err
		}
		p.AssigneeAgentID = tracker.Opt[uuid.UUID]{Set: true, Value: id}
	}
	if in.Criteria != nil {
		texts, err := criteriaTexts(*in.Criteria)
		if err != nil {
			return nil, err
		}
		if p.Criteria, err = criteriaDraft(ctx, d, c.Agent.WorkspaceID, key, texts); err != nil {
			return nil, err
		}
	}
	prURL := strings.TrimSpace(in.PRURL)
	t, err := d.Tracker.UpdateThen(ctx, c.Agent.WorkspaceID, chatActor(c, s), key, p, func(q *store.Queries, before, after store.Ticket) error {
		if before.Status != after.Status || prURL != "" {
			if run, err := q.ActiveRunForTicket(ctx, &before.ID); err == nil {
				return apperr.Conflict("%s has a live run (%s) that owns its status and PR; leave those to it, or ask the person to cancel the run", before.Key, run.ID)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if prURL != "" {
			return runs.LinkPR(ctx, q, after, chatActor(c, s), prURL, in.PRDraft)
		}
		return nil
	})
	if errors.Is(err, tracker.ErrNotFound) {
		return nil, apperr.Invalid("ticket", "no ticket %s", key)
	}
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"ticket": t.Key, "status": t.Status, "revision": t.Revision}), nil
}

func commentChatTicket(ctx context.Context, d Deps, c agents.Caller, in CommentTicketIn) (any, error) {
	s, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	key := strings.ToUpper(strings.TrimSpace(in.Ticket))
	ev, err := d.Tracker.Comment(ctx, c.Agent.WorkspaceID, chatActor(c, s), key, tracker.NewComment{BodyMd: in.Body})
	if errors.Is(err, tracker.ErrNotFound) {
		return nil, apperr.Invalid("ticket", "no ticket %s", key)
	}
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"ticket": key, "event_id": ev.ID}), nil
}

// updateTicketBlockers adds and removes the tickets one waits on. A started
// run of a blocked ticket queues until every blocker is Done.
func updateTicketBlockers(ctx context.Context, d Deps, c agents.Caller, in TicketBlockersIn) (any, error) {
	s, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	key := strings.ToUpper(strings.TrimSpace(in.Ticket))
	after, err := d.Tracker.EditDependencies(ctx, c.Agent.WorkspaceID, chatActor(c, s), key, in.Add, in.Remove)
	if errors.Is(err, tracker.ErrNotFound) {
		return nil, apperr.Invalid("ticket", "no ticket %s", key)
	}
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"ticket": key, "blocked_by": after}), nil
}

// getChatEpic is one epic as its page shows it: numbered criteria and tickets.
func getChatEpic(ctx context.Context, d Deps, c agents.Caller, in GetEpicIn) (any, error) {
	if _, err := requireChat(ctx, d, c); err != nil {
		return nil, err
	}
	return chatEpicView(ctx, d, c.Agent.WorkspaceID, in.EpicID)
}

func chatEpicView(ctx context.Context, d Deps, ws, id uuid.UUID) (any, error) {
	e, err := d.Q.GetEpic(ctx, store.GetEpicParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.Invalid("epic_id", "no epic %s", id)
	}
	if err != nil {
		return nil, err
	}
	p, err := d.Q.GetProject(ctx, store.GetProjectParams{ID: e.ProjectID, WorkspaceID: ws})
	if err != nil {
		return nil, err
	}
	crit, err := d.Q.ListEpicCriteria(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	criteria := make([]map[string]any, len(crit))
	for i, cr := range crit {
		criteria[i] = map[string]any{"n": i + 1, "text": cr.Text, "done": cr.Done}
	}
	rows, err := d.Q.PlanningEpicTickets(ctx, &e.ID)
	if err != nil {
		return nil, err
	}
	tickets := make([]map[string]any, len(rows))
	for i, t := range rows {
		tickets[i] = map[string]any{"key": t.Key, "title": t.Title, "status": t.Status}
	}
	return map[string]any{"id": e.ID, "name": e.Name, "objective": e.Objective, "project": p.Key, "status": e.Status,
		"requires_mockup": e.RequiresMockup, "criteria": criteria, "tickets": tickets}, nil
}

// updateChatEpic changes an epic's settings and criteria. Criteria are
// addressed by their number in get_epic, resolved to ids once up front, so an
// edit lands on the criterion the persona read even if others move meanwhile.
// Changes apply in order (settings, edits, removals, additions); on an error
// the earlier ones stay, and the reply says what to re-read.
func updateChatEpic(ctx context.Context, d Deps, c agents.Caller, in UpdateEpicIn) (any, error) {
	s, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	ws := c.Agent.WorkspaceID
	if _, err := d.Q.GetEpic(ctx, store.GetEpicParams{ID: in.EpicID, WorkspaceID: ws}); errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.Invalid("epic_id", "no epic %s", in.EpicID)
	} else if err != nil {
		return nil, err
	}
	adds, err := criteriaTexts(in.AddCriteria)
	if err != nil {
		return nil, err
	}
	crit, err := d.Tracker.ListEpicCriteria(ctx, ws, in.EpicID)
	if err != nil {
		return nil, err
	}
	numbered := func(n int) (uuid.UUID, error) {
		if n < 1 || n > len(crit) {
			return uuid.Nil, apperr.Invalid("criteria", "the epic has no criterion %d; read it with get_epic", n)
		}
		return crit[n-1].ID, nil
	}
	touched := map[int]bool{}
	edits := make([]uuid.UUID, len(in.EditCriteria))
	for i, e := range in.EditCriteria {
		if edits[i], err = numbered(e.N); err != nil {
			return nil, err
		}
		if touched[e.N] {
			return nil, apperr.Invalid("edit_criteria", "criterion %d appears twice", e.N)
		}
		touched[e.N] = true
	}
	removes := make([]uuid.UUID, len(in.RemoveCriteria))
	for i, n := range in.RemoveCriteria {
		if removes[i], err = numbered(n); err != nil {
			return nil, err
		}
		if touched[n] {
			return nil, apperr.Invalid("remove_criteria", "criterion %d is already edited or removed in this call", n)
		}
		touched[n] = true
	}

	if in.Status != nil || in.RequiresMockup != nil {
		if _, err := d.Tracker.UpdateEpic(ctx, ws, in.EpicID, in.Status, in.RequiresMockup); err != nil {
			return nil, err
		}
	}
	partial := func(err error) error {
		var invalid *apperr.ValidationError
		var conflict *apperr.ConflictError
		if errors.As(err, &invalid) || errors.As(err, &conflict) {
			return apperr.Conflict("%s; changes before it were saved, so read the epic again with get_epic", err)
		}
		return err
	}
	for i, e := range in.EditCriteria {
		text := e.Text
		if _, err := d.Tracker.PatchEpicCriterion(ctx, ws, chatActor(c, s), edits[i], &text, nil, nil); err != nil {
			return nil, partial(err)
		}
	}
	for _, id := range removes {
		if err := d.Tracker.DeleteEpicCriterion(ctx, ws, id); err != nil {
			return nil, partial(err)
		}
	}
	for _, text := range adds {
		if _, err := d.Tracker.CreateEpicCriterion(ctx, ws, in.EpicID, text); err != nil {
			return nil, partial(err)
		}
	}
	return chatEpicView(ctx, d, ws, in.EpicID)
}
