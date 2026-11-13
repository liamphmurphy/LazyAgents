package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// A persona's ticket edits wait for the person unless they let that persona
// edit tickets without asking (its ticket_edits setting, or "Always allow" on
// a card). Waiting doesn't block the tool call: the edit is recorded as a
// ticket change request with a before/after preview, the chat shows it as a
// card, and applying it runs the same plan again as the persona, so a ticket
// that changed since is refused rather than overwritten.

const (
	toolUpdateTicket   = "update_ticket"
	toolCommentTicket  = "comment_ticket"
	toolUpdateBlockers = "update_ticket_blockers"

	// TicketEditsAsk and TicketEditsAllow are a persona's ticket_edits.
	TicketEditsAsk   = "ask"
	TicketEditsAllow = "allow"

	// maxPendingTicketChanges bounds the cards waiting in one chat.
	maxPendingTicketChanges = 20
)

// TicketChange is one line of a proposal's preview. Before is empty for a
// comment, which only adds.
type TicketChange struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Before string `json:"before,omitempty"`
	After  string `json:"after"`
}

// ticketEditsAsk reports whether this chat's persona must propose its ticket
// edits. A chat with no persona to hold the setting asks.
func ticketEditsAsk(ctx context.Context, d Deps, ws uuid.UUID, s *chatScope) (bool, error) {
	if s.PersonaID == nil {
		return true, nil
	}
	p, err := d.Q.GetPersona(ctx, store.GetPersonaParams{ID: *s.PersonaID, WorkspaceID: ws, UserID: s.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return p.TicketEdits != TicketEditsAllow, nil
}

func proposeTicketChange(ctx context.Context, d Deps, c agents.Caller, s *chatScope, t store.Ticket, tool string, in any, changes []TicketChange) (any, error) {
	n, err := d.Q.CountPendingTicketChangeRequests(ctx, s.ChatID)
	if err != nil {
		return nil, err
	}
	if n >= maxPendingTicketChanges {
		return nil, apperr.Conflict("%d ticket changes already wait for the person in this chat; ask them to decide those first", n)
	}
	args, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	preview, err := json.Marshal(changes)
	if err != nil {
		return nil, err
	}
	r, err := d.Q.CreateTicketChangeRequest(ctx, store.CreateTicketChangeRequestParams{
		WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID, ChatID: s.ChatID, PersonaID: s.PersonaID, AgentID: c.Agent.ID,
		TicketID: t.ID, Tool: tool, Arguments: args, Changes: preview,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ticket": t.Key, "state": "proposed", "request_id": r.ID, "changes": changes,
		"tell_the_person": "Proposed, not applied: the person applies or declines it on the card in this chat. Don't say it's done; get_ticket shows the ticket once they decide.",
	}, nil
}

// ApplyTicketChange applies a request the person approved, as the persona
// that proposed it with the person's approval recorded on each event. It
// plans the edit again first, so a ticket that changed since the proposal is
// refused (a stale revision, a run that has since started) rather than
// overwritten.
func ApplyTicketChange(ctx context.Context, d Deps, r store.TicketChangeRequest) error {
	actor := tracker.AgentActor(r.AgentID)
	actor.ApprovedBy = &r.UserID
	actor.Via = map[string]any{"chat_id": r.ChatID, "approved_by": r.UserID}
	if r.PersonaID != nil {
		actor.Via["persona_id"] = *r.PersonaID
	}
	ws := r.WorkspaceID
	switch r.Tool {
	case toolUpdateTicket:
		var in UpdateTicketIn
		if err := json.Unmarshal(r.Arguments, &in); err != nil {
			return err
		}
		u, err := planTicketUpdate(ctx, d, ws, in)
		if err != nil {
			return err
		}
		if len(u.changes) == 0 {
			return nil
		}
		_, err = u.apply(ctx, d, ws, actor)
		return err
	case toolCommentTicket:
		var in CommentTicketIn
		if err := json.Unmarshal(r.Arguments, &in); err != nil {
			return err
		}
		t, _, err := planTicketComment(ctx, d, ws, in)
		if err != nil {
			return err
		}
		_, err = applyTicketComment(ctx, d, ws, actor, t.Key, in)
		return err
	case toolUpdateBlockers:
		var in TicketBlockersIn
		if err := json.Unmarshal(r.Arguments, &in); err != nil {
			return err
		}
		t, changes, err := planTicketBlockers(ctx, d, ws, in)
		if err != nil || len(changes) == 0 {
			return err
		}
		_, err = applyTicketBlockers(ctx, d, ws, actor, t.Key, in)
		return err
	}
	return fmt.Errorf("agentapi: unknown ticket change tool %q", r.Tool)
}

func ptrEqual[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func pointsLabel(p *int32) string {
	if p == nil {
		return "none"
	}
	if *p == 1 {
		return "1 point"
	}
	return fmt.Sprintf("%d points", *p)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func listLabel(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

// checklist renders criteria as Markdown task items, one per line.
func checklist(items []tracker.CriterionDraft) string {
	if len(items) == 0 {
		return "none"
	}
	var b strings.Builder
	for i, c := range items {
		if i > 0 {
			b.WriteByte('\n')
		}
		if c.Done {
			b.WriteString("- [x] ")
		} else {
			b.WriteString("- [ ] ")
		}
		b.WriteString(strings.TrimSpace(c.Text))
	}
	return b.String()
}

func epicLabel(ctx context.Context, d Deps, ws uuid.UUID, id *uuid.UUID) (string, error) {
	if id == nil {
		return "none", nil
	}
	e, err := d.Q.GetEpic(ctx, store.GetEpicParams{ID: *id, WorkspaceID: ws})
	if err != nil {
		return "", err
	}
	return e.Name, nil
}

func agentLabel(ctx context.Context, d Deps, ws uuid.UUID, id *uuid.UUID) (string, error) {
	if id == nil {
		return "unassigned", nil
	}
	a, err := d.Q.GetAgent(ctx, store.GetAgentParams{ID: *id, WorkspaceID: ws})
	if err != nil {
		return "", err
	}
	return "@" + a.Handle, nil
}
