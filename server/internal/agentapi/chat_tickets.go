package agentapi

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
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
	Ticket            string             `json:"ticket" jsonschema:"Ticket key, e.g. PAY-214"`
	Revision          int                `json:"revision" jsonschema:"The revision from your latest get_ticket. A stale one is refused: read the ticket again and redo your edit"`
	Title             *string            `json:"title,omitempty"`
	Description       *string            `json:"description,omitempty" jsonschema:"Markdown; replaces the whole description"`
	Status            *string            `json:"status,omitempty" jsonschema:"backlog, todo, in_progress, in_review, done or canceled. Canceling, or reopening a canceled ticket, always waits for the person. Refused while the ticket has a live run, which owns its status"`
	Priority          *string            `json:"priority,omitempty" jsonschema:"none, low, medium, high or urgent"`
	Points            *int               `json:"points,omitempty" jsonschema:"Story points: 1, 2, 3, 5, 8 or 13; 0 clears the estimate"`
	EstimateRationale string             `json:"estimate_rationale,omitempty" jsonschema:"Why that estimate, in a sentence"`
	EpicID            *uuid.UUID         `json:"epic_id,omitempty" jsonschema:"Move the ticket into this epic of its project"`
	RemoveFromEpic    bool               `json:"remove_from_epic,omitempty" jsonschema:"Take the ticket out of its epic"`
	Agent             *string            `json:"agent,omitempty" jsonschema:"Assign a main agent by handle, e.g. pike; empty string unassigns"`
	RequiresMockup    *bool              `json:"requires_mockup,omitempty"`
	Labels            *[]string          `json:"labels,omitempty" jsonschema:"Replaces the ticket's labels with these label names (existing labels only); an empty list removes them all"`
	DueDate           *string            `json:"due_date,omitempty" jsonschema:"YYYY-MM-DD; an empty string clears it"`
	PRURL             string             `json:"pr_url,omitempty" jsonschema:"GitHub pull request URL for work on this ticket done from this chat. Vesper then follows the PR and moves the ticket to Done when it merges"`
	PRDraft           bool               `json:"pr_draft,omitempty"`
	Criteria          *[]string          `json:"criteria,omitempty" jsonschema:"Replaces the acceptance criteria with this list, in order. A criterion whose text is unchanged keeps its tick; an empty list removes them all"`
	TickCriteria      []CriterionTickArg `json:"tick_criteria,omitempty" jsonschema:"Tick or untick acceptance criteria, numbered as in get_ticket. Not together with criteria"`
}

type CriterionTickArg struct {
	N    int  `json:"n" jsonschema:"The criterion's number in get_ticket"`
	Done bool `json:"done"`
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

// updateChatTicket edits an existing ticket from a chat, or proposes the
// edit when the person reviews this persona's ticket changes (see
// chat_ticket_changes.go). Tickets with a live run are the run's to move, so
// status and PR edits are refused then; work a persona does itself in the chat
// has no run, and this is how its ticket keeps up (LAZYAGENTS-100).
func updateChatTicket(ctx context.Context, d Deps, c agents.Caller, in UpdateTicketIn) (any, error) {
	s, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	ws := c.Agent.WorkspaceID
	u, err := planTicketUpdate(ctx, d, ws, in)
	if err != nil {
		return nil, err
	}
	if len(u.changes) == 0 {
		return ok(map[string]any{"ticket": u.ticket.Key, "status": u.ticket.Status, "revision": u.ticket.Revision, "unchanged": true}), nil
	}
	ask, err := ticketEditsAsk(ctx, d, ws, s)
	if err != nil {
		return nil, err
	}
	if ask || u.personDecides {
		return proposeTicketChange(ctx, d, c, s, u.ticket, toolUpdateTicket, in, u.changes)
	}
	t, err := u.apply(ctx, d, ws, chatActor(c, s))
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"ticket": t.Key, "status": t.Status, "revision": t.Revision}), nil
}

// ticketUpdate is an update_ticket call checked against the ticket as it is
// now: the patch to apply and the before/after the person sees.
type ticketUpdate struct {
	ticket  store.Ticket
	patch   tracker.Patch
	prURL   string
	prDraft bool
	changes []TicketChange
	// personDecides marks a cancel or a reopen of a canceled ticket, which
	// only a person decides: it always waits for them.
	personDecides bool
}

// planTicketUpdate validates in and resolves every name in it, so a bad
// call fails before the person is asked, and an approved one is planned again
// against the ticket as it is then (its revision must still match).
func planTicketUpdate(ctx context.Context, d Deps, ws uuid.UUID, in UpdateTicketIn) (ticketUpdate, error) {
	var u ticketUpdate
	key := strings.ToUpper(strings.TrimSpace(in.Ticket))
	if key == "" {
		return u, apperr.Invalid("ticket", "is required")
	}
	if in.Revision <= 0 {
		return u, apperr.Invalid("revision", "is required: pass the revision from get_ticket")
	}
	t, err := tracker.Get(ctx, d.Q, ws, key)
	if err != nil {
		return u, apperr.Invalid("ticket", "no ticket %s", key)
	}
	if in.Revision != int(t.Revision) {
		return u, apperr.ConflictField("revision", "ticket revision is stale: read it again with get_ticket and redo your edit")
	}
	u.ticket = t
	rev := in.Revision
	u.patch = tracker.Patch{Revision: &rev}
	change := func(field, label string, before, after string) {
		u.changes = append(u.changes, TicketChange{Field: field, Label: label, Before: before, After: after})
	}

	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return u, apperr.Invalid("title", "can't be empty")
	}
	if in.Title != nil && strings.TrimSpace(*in.Title) != t.Title {
		u.patch.Title = in.Title
		change("title", "Title", t.Title, strings.TrimSpace(*in.Title))
	}
	if in.Description != nil && *in.Description != t.DescriptionMd {
		u.patch.DescriptionMd = in.Description
		change("description", "Description", t.DescriptionMd, *in.Description)
	}
	if in.Status != nil && *in.Status != t.Status {
		if !slices.Contains(tracker.Statuses, *in.Status) {
			return u, apperr.Invalid("status", "must be one of %s", strings.Join(tracker.Statuses, ", "))
		}
		u.patch.Status = in.Status
		u.personDecides = *in.Status == "canceled" || t.Status == "canceled"
		change("status", "Status", t.Status, *in.Status)
	}
	if in.Priority != nil && *in.Priority != t.Priority {
		if !slices.Contains(tracker.Priorities, *in.Priority) {
			return u, apperr.Invalid("priority", "must be one of %s", strings.Join(tracker.Priorities, ", "))
		}
		u.patch.Priority = in.Priority
		change("priority", "Priority", t.Priority, *in.Priority)
	}
	if in.Points != nil {
		if *in.Points == 0 {
			if t.EstimatePoints != nil {
				u.patch.EstimatePoints = tracker.Clear[int]()
				change("estimate", "Estimate", pointsLabel(t.EstimatePoints), "none")
			}
		} else {
			est, rationale, err := NewTicketIn{Points: in.Points, EstimateRationale: in.EstimateRationale}.estimate()
			if err != nil {
				return u, err
			}
			after := int32(*est)
			if !ptrEqual(t.EstimatePoints, &after) || rationale != nil {
				u.patch.EstimatePoints, u.patch.EstimateRationale = tracker.SetTo(*est), rationale
				change("estimate", "Estimate", pointsLabel(t.EstimatePoints), pointsLabel(&after))
			}
		}
	}
	if in.EpicID != nil && in.RemoveFromEpic {
		return u, apperr.Invalid("epic_id", "pass epic_id or remove_from_epic, not both")
	}
	if in.EpicID != nil || in.RemoveFromEpic {
		before, err := epicLabel(ctx, d, ws, t.EpicID)
		if err != nil {
			return u, err
		}
		if in.RemoveFromEpic && t.EpicID != nil {
			u.patch.EpicID = tracker.Clear[uuid.UUID]()
			change("epic", "Epic", before, "none")
		}
		if in.EpicID != nil && !ptrEqual(t.EpicID, in.EpicID) {
			e, err := d.Q.GetEpic(ctx, store.GetEpicParams{ID: *in.EpicID, WorkspaceID: ws})
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && e.ProjectID != t.ProjectID) {
				return u, apperr.Invalid("epic_id", "no epic %s in %s's project; list_epics shows them", *in.EpicID, t.Key)
			}
			if err != nil {
				return u, err
			}
			u.patch.EpicID = tracker.SetTo(e.ID)
			change("epic", "Epic", before, e.Name)
		}
	}
	if in.Agent != nil {
		id, err := mainAgent(ctx, d, ws, *in.Agent)
		if err != nil {
			return u, err
		}
		if !ptrEqual(t.AssigneeAgentID, id) {
			before, err := agentLabel(ctx, d, ws, t.AssigneeAgentID)
			if err != nil {
				return u, err
			}
			after, err := agentLabel(ctx, d, ws, id)
			if err != nil {
				return u, err
			}
			u.patch.AssigneeAgentID = tracker.Opt[uuid.UUID]{Set: true, Value: id}
			change("agent", "Agent", before, after)
		}
	}
	if in.RequiresMockup != nil && *in.RequiresMockup != t.RequiresMockup {
		u.patch.RequiresMockup = in.RequiresMockup
		change("requires_mockup", "Needs a mockup", yesNo(t.RequiresMockup), yesNo(*in.RequiresMockup))
	}
	if in.Labels != nil {
		if err := planLabels(ctx, d, ws, t, *in.Labels, &u); err != nil {
			return u, err
		}
	}
	if in.DueDate != nil {
		before := "none"
		if t.DueDate != nil {
			before = t.DueDate.Format(time.DateOnly)
		}
		after := strings.TrimSpace(*in.DueDate)
		if after == "" {
			if t.DueDate != nil {
				u.patch.DueDate = tracker.Clear[time.Time]()
				change("due_date", "Due", before, "none")
			}
		} else {
			due, err := time.Parse(time.DateOnly, after)
			if err != nil {
				return u, apperr.Invalid("due_date", "must be YYYY-MM-DD")
			}
			if before != after {
				u.patch.DueDate = tracker.SetTo(due)
				change("due_date", "Due", before, after)
			}
		}
	}
	if in.Criteria != nil && len(in.TickCriteria) > 0 {
		return u, apperr.Invalid("tick_criteria", "pass criteria or tick_criteria, not both")
	}
	if in.Criteria != nil || len(in.TickCriteria) > 0 {
		if err := planCriteria(ctx, d, ws, t, in, &u); err != nil {
			return u, err
		}
	}
	if u.prURL = strings.TrimSpace(in.PRURL); u.prURL != "" {
		if _, _, err := runs.ParsePRURL(u.prURL); err != nil {
			return u, apperr.Invalid("pr_url", "must be a GitHub pull request URL")
		}
		u.prDraft = in.PRDraft
		before := "none"
		if t.PrUrl != nil {
			before = *t.PrUrl
		}
		change("pr_url", "Pull request", before, u.prURL)
	}
	if u.patch.Status != nil || u.prURL != "" {
		if run, err := d.Q.ActiveRunForTicket(ctx, &t.ID); err == nil {
			return u, liveRunOwns(t.Key, run.ID)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return u, err
		}
	}
	return u, nil
}

func liveRunOwns(key string, run uuid.UUID) error {
	return apperr.Conflict("%s has a live run (%s) that owns its status and PR; leave those to it, or ask the person to cancel the run", key, run)
}

// apply saves the planned update as actor. The live-run check repeats under
// the ticket's lock: a run may have started since the plan.
func (u ticketUpdate) apply(ctx context.Context, d Deps, ws uuid.UUID, actor tracker.Actor) (store.Ticket, error) {
	t, err := d.Tracker.UpdateThen(ctx, ws, actor, u.ticket.Key, u.patch, func(q *store.Queries, before, after store.Ticket) error {
		if before.Status != after.Status || u.prURL != "" {
			if run, err := q.ActiveRunForTicket(ctx, &before.ID); err == nil {
				return liveRunOwns(before.Key, run.ID)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if u.prURL != "" {
			return runs.LinkPR(ctx, q, after, actor, u.prURL, u.prDraft)
		}
		return nil
	})
	if errors.Is(err, tracker.ErrNotFound) {
		return t, apperr.Invalid("ticket", "no ticket %s", u.ticket.Key)
	}
	return t, err
}

func planLabels(ctx context.Context, d Deps, ws uuid.UUID, t store.Ticket, names []string, u *ticketUpdate) error {
	all, err := d.Q.ListLabels(ctx, ws)
	if err != nil {
		return err
	}
	byName := make(map[string]store.Label, len(all))
	known := make([]string, len(all))
	for i, l := range all {
		byName[strings.ToLower(l.Name)] = l
		known[i] = l.Name
	}
	ids := []uuid.UUID{}
	var after []string
	seen := map[uuid.UUID]bool{}
	for _, n := range names {
		l, found := byName[strings.ToLower(strings.TrimSpace(n))]
		if !found {
			if len(known) == 0 {
				return apperr.Invalid("labels", "no label %q: this workspace has no labels yet", n)
			}
			return apperr.Invalid("labels", "no label %q; labels are: %s", n, strings.Join(known, ", "))
		}
		if !seen[l.ID] {
			seen[l.ID] = true
			ids = append(ids, l.ID)
			after = append(after, l.Name)
		}
	}
	cur, err := d.Q.ListLabelsForTickets(ctx, []uuid.UUID{t.ID})
	if err != nil {
		return err
	}
	before := make([]string, len(cur))
	for i, l := range cur {
		before[i] = l.Name
	}
	slices.Sort(after)
	if slices.Equal(before, after) {
		return nil
	}
	u.patch.LabelIDs = &ids
	u.changes = append(u.changes, TicketChange{Field: "labels", Label: "Labels", Before: listLabel(before), After: listLabel(after)})
	return nil
}

// planCriteria replaces the criteria or ticks some of them. Either way the
// draft carries the snapshot it was planned against, so an edit made
// meanwhile is a conflict rather than lost.
func planCriteria(ctx context.Context, d Deps, ws uuid.UUID, t store.Ticket, in UpdateTicketIn, u *ticketUpdate) error {
	cur, err := d.Q.ListCriteria(ctx, t.ID)
	if err != nil {
		return err
	}
	var draft *tracker.CriteriaDraft
	if in.Criteria != nil {
		texts, err := criteriaTexts(*in.Criteria)
		if err != nil {
			return err
		}
		if draft, err = criteriaDraft(ctx, d, ws, t.Key, texts); err != nil {
			return err
		}
	} else {
		draft = &tracker.CriteriaDraft{}
		ticks := map[int]bool{}
		for _, tk := range in.TickCriteria {
			if tk.N < 1 || tk.N > len(cur) {
				return apperr.Invalid("tick_criteria", "%s has no criterion %d; read it with get_ticket", t.Key, tk.N)
			}
			if _, dup := ticks[tk.N]; dup {
				return apperr.Invalid("tick_criteria", "criterion %d appears twice", tk.N)
			}
			ticks[tk.N] = tk.Done
		}
		for i, c := range cur {
			id := c.ID
			done := c.Done
			if v, set := ticks[i+1]; set {
				done = v
			}
			draft.Expected = append(draft.Expected, tracker.ExpectedCriterion{ID: c.ID, Position: int(c.Position), Text: c.Text, Done: c.Done})
			draft.Items = append(draft.Items, tracker.CriterionDraft{ID: &id, Text: c.Text, Done: done})
		}
	}
	before := make([]tracker.CriterionDraft, len(cur))
	for i, c := range cur {
		before[i] = tracker.CriterionDraft{Text: c.Text, Done: c.Done}
	}
	b, a := checklist(before), checklist(draft.Items)
	if b == a {
		return nil
	}
	u.patch.Criteria = draft
	u.changes = append(u.changes, TicketChange{Field: "criteria", Label: "Acceptance criteria", Before: b, After: a})
	return nil
}

func commentChatTicket(ctx context.Context, d Deps, c agents.Caller, in CommentTicketIn) (any, error) {
	s, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	ws := c.Agent.WorkspaceID
	t, changes, err := planTicketComment(ctx, d, ws, in)
	if err != nil {
		return nil, err
	}
	ask, err := ticketEditsAsk(ctx, d, ws, s)
	if err != nil {
		return nil, err
	}
	if ask {
		return proposeTicketChange(ctx, d, c, s, t, toolCommentTicket, in, changes)
	}
	ev, err := applyTicketComment(ctx, d, ws, chatActor(c, s), t.Key, in)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"ticket": t.Key, "event_id": ev.ID}), nil
}

func planTicketComment(ctx context.Context, d Deps, ws uuid.UUID, in CommentTicketIn) (store.Ticket, []TicketChange, error) {
	key := strings.ToUpper(strings.TrimSpace(in.Ticket))
	t, err := tracker.Get(ctx, d.Q, ws, key)
	if err != nil {
		return t, nil, apperr.Invalid("ticket", "no ticket %s", key)
	}
	if strings.TrimSpace(in.Body) == "" {
		return t, nil, apperr.Invalid("body", "is required")
	}
	return t, []TicketChange{{Field: "comment", Label: "Comment", After: in.Body}}, nil
}

func applyTicketComment(ctx context.Context, d Deps, ws uuid.UUID, actor tracker.Actor, key string, in CommentTicketIn) (store.ActivityEvent, error) {
	ev, err := d.Tracker.Comment(ctx, ws, actor, key, tracker.NewComment{BodyMd: in.Body})
	if errors.Is(err, tracker.ErrNotFound) {
		return ev, apperr.Invalid("ticket", "no ticket %s", key)
	}
	return ev, err
}

// updateTicketBlockers adds and removes the tickets one waits on. A started
// run of a blocked ticket queues until every blocker is Done.
func updateTicketBlockers(ctx context.Context, d Deps, c agents.Caller, in TicketBlockersIn) (any, error) {
	s, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	ws := c.Agent.WorkspaceID
	t, changes, err := planTicketBlockers(ctx, d, ws, in)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return ok(map[string]any{"ticket": t.Key, "unchanged": true}), nil
	}
	ask, err := ticketEditsAsk(ctx, d, ws, s)
	if err != nil {
		return nil, err
	}
	if ask {
		return proposeTicketChange(ctx, d, c, s, t, toolUpdateBlockers, in, changes)
	}
	after, err := applyTicketBlockers(ctx, d, ws, chatActor(c, s), t.Key, in)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"ticket": t.Key, "blocked_by": after}), nil
}

// planTicketBlockers previews the blocker list; cycles and a live run are
// refused when it applies, under the dependency graph lock.
func planTicketBlockers(ctx context.Context, d Deps, ws uuid.UUID, in TicketBlockersIn) (store.Ticket, []TicketChange, error) {
	key := strings.ToUpper(strings.TrimSpace(in.Ticket))
	t, err := tracker.Get(ctx, d.Q, ws, key)
	if err != nil {
		return t, nil, apperr.Invalid("ticket", "no ticket %s", key)
	}
	if len(in.Add) == 0 && len(in.Remove) == 0 {
		return t, nil, apperr.Invalid("add", "pass blockers to add or remove")
	}
	cur, err := d.Q.TicketDependencies(ctx, t.ID)
	if err != nil {
		return t, nil, err
	}
	drop := map[string]bool{}
	for _, k := range in.Remove {
		drop[strings.ToUpper(strings.TrimSpace(k))] = true
	}
	var before, after []string
	for _, dep := range cur {
		before = append(before, dep.Key)
		if !drop[dep.Key] {
			after = append(after, dep.Key)
		}
	}
	for _, k := range in.Add {
		k = strings.ToUpper(strings.TrimSpace(k))
		if k == t.Key {
			return t, nil, apperr.Invalid("add", "%s can't wait on itself", k)
		}
		if _, err := tracker.Get(ctx, d.Q, ws, k); err != nil {
			return t, nil, apperr.Invalid("add", "no ticket %s", k)
		}
		if !slices.Contains(after, k) {
			after = append(after, k)
		}
	}
	slices.Sort(before)
	slices.Sort(after)
	if slices.Equal(before, after) {
		return t, nil, nil
	}
	return t, []TicketChange{{Field: "blocked_by", Label: "Blocked by", Before: listLabel(before), After: listLabel(after)}}, nil
}

func applyTicketBlockers(ctx context.Context, d Deps, ws uuid.UUID, actor tracker.Actor, key string, in TicketBlockersIn) ([]string, error) {
	after, err := d.Tracker.EditDependencies(ctx, ws, actor, key, in.Add, in.Remove)
	if errors.Is(err, tracker.ErrNotFound) {
		return nil, apperr.Invalid("ticket", "no ticket %s", key)
	}
	return after, err
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
