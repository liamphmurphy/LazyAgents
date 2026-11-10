package agentapi

import (
	"context"
	"errors"
	"strings"

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
}

type CommentTicketIn struct {
	Ticket string `json:"ticket" jsonschema:"Ticket key, e.g. PAY-214"`
	Body   string `json:"body" jsonschema:"Markdown"`
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
