package runs

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// dispatchFenceRe finds a ```dispatch card in a persona's reply.
var dispatchFenceRe = regexp.MustCompile("(?m)^ {0,3}(`{3,}|~{3,})[ \t]*dispatch\\b")

// ChatDispatch asks to start a ticket's run from a chat. MessageID is the
// persona's reply that offered the card, when it came from one.
type ChatDispatch struct {
	Ticket    string
	MessageID *uuid.UUID
}

// StartChatTicketRun starts a ticket's managed run from a chat, as the person,
// and records that this chat started it so it can follow the run. The persona
// only offers the card: like every dispatch, nothing runs until a person says
// so (§6.6), and the run names them as its dispatcher.
func (s *Service) StartChatTicketRun(ctx context.Context, ws, user, chatID uuid.UUID, in ChatDispatch) (store.Run, error) {
	var run store.Run
	key := strings.ToUpper(strings.TrimSpace(in.Ticket))
	if key == "" {
		return run, apperr.Invalid("ticket", "is required")
	}
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, chatID)
		if err != nil {
			return err
		}
		if chat.Archived {
			return apperr.Conflict("the chat is archived")
		}
		if in.MessageID != nil {
			msg, err := q.GetChatMessage(ctx, *in.MessageID)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && msg.SessionID != chat.ID) {
				return apperr.Invalid("message_id", "that message isn't in this chat")
			} else if err != nil {
				return err
			}
			if msg.Role != "assistant" || !dispatchFenceRe.MatchString(msg.BodyMd) || !strings.Contains(strings.ToUpper(msg.BodyMd), key) {
				return apperr.Invalid("message_id", "that reply doesn't offer to start %s", key)
			}
			if _, err := q.ChatTicketRunForMessage(ctx, in.MessageID); err == nil {
				return apperr.Conflict("this card already started a run")
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if err := tracker.LockTicketProjects(ctx, q, ws, []string{key}); err != nil {
			if errors.Is(err, tracker.ErrNotFound) {
				return apperr.Invalid("ticket", "no ticket %s", key)
			}
			return err
		}
		if run, err = dispatchTicket(ctx, q, ws, user, key, "ticket"); err != nil {
			return err
		}
		return q.AddChatTicketRun(ctx, store.AddChatTicketRunParams{ChatID: chat.ID, RunID: run.ID, MessageID: in.MessageID})
	})
	return run, err
}
