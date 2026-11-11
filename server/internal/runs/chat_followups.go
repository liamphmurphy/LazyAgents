package runs

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A follow-up is a message the person queued while a reply ran, instead of
// interrupting it. When a turn ends with a reply, the runner sends the oldest
// one as an ordinary message (SendNextFollowup), so each gets a turn of its
// own, in order. A turn the person stopped, or one that failed, leaves the
// queue held: the person sends what's left, or it goes after their next reply.

// MaxChatFollowups bounds a chat's queue.
const MaxChatFollowups = 20

// QueueChatFollowup adds a follow-up to the chat's queue. If nothing is
// running and the queue was empty, it is sent at once: the reply it was
// waiting for ended between the person typing it and it arriving here.
func (s *Service) QueueChatFollowup(ctx context.Context, ws, user, id uuid.UUID, body string) (store.ChatFollowup, error) {
	var out store.ChatFollowup
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 100000 {
		return out, apperr.Invalid("body", "provide 1–100000 characters")
	}
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		if chat.Archived {
			return apperr.Conflict("the chat is archived")
		}
		if chat.ParentChatID != nil {
			return apperr.Conflict("an aside answers one question; ask another aside instead")
		}
		queued, err := q.CountChatFollowups(ctx, chat.ID)
		if err != nil {
			return err
		}
		if queued >= MaxChatFollowups {
			return apperr.Conflict("you can queue at most %d follow-ups", MaxChatFollowups)
		}
		if out, err = q.AddChatFollowup(ctx, store.AddChatFollowupParams{SessionID: chat.ID, Body: body}); err != nil {
			return err
		}
		if _, err = q.BumpChatRevision(ctx, chat.ID); err != nil {
			return err
		}
		if queued == 0 && chat.State == "idle" {
			return sendNextFollowup(ctx, q, chat, s.HandoffChars)
		}
		return nil
	})
	return out, err
}

// ListChatFollowups is the chat's queue, oldest (next to send) first.
func (s *Service) ListChatFollowups(ctx context.Context, ws, user, id uuid.UUID) ([]store.ChatFollowup, error) {
	var out []store.ChatFollowup
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		out, err = q.ListChatFollowups(ctx, chat.ID)
		return err
	})
	return out, err
}

// DeleteChatFollowup removes a queued follow-up before it is sent.
func (s *Service) DeleteChatFollowup(ctx context.Context, ws, user, id, followupID uuid.UUID) error {
	return s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		n, err := q.DeleteChatFollowup(ctx, store.DeleteChatFollowupParams{ID: followupID, SessionID: chat.ID})
		if err != nil {
			return err
		}
		if n == 0 {
			// Already sent, or removed in another tab.
			return apperr.ErrNotFound
		}
		_, err = q.BumpChatRevision(ctx, chat.ID)
		return err
	})
}

// SendNextFollowup runs in the runner's transaction once a turn has ended
// with a reply and any hand-on or interruption it led to has been started.
// A chat that is busy again keeps its queue for the turn that is running.
func SendNextFollowup(ctx context.Context, q *store.Queries, chatID uuid.UUID, handoffChars int) error {
	chat, err := q.LockChatSessionByID(ctx, chatID)
	if err != nil {
		return err
	}
	return sendNextFollowup(ctx, q, chat, handoffChars)
}

func sendNextFollowup(ctx context.Context, q *store.Queries, chat store.ChatSession, handoffChars int) error {
	if chat.Archived || chat.State != "idle" || chat.InterruptMessageID != nil {
		return nil
	}
	p, err := chatProject(ctx, q, chat)
	if err != nil || p.Archived {
		return err
	}
	next, err := q.TakeNextChatFollowup(ctx, chat.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Sent as if typed now: it keeps the chat's model and effort, so the
	// catalog is never consulted.
	_, _, err = postChatMessage(ctx, q, chatmodels.Catalog{}, chat, ChatMessageInput{Body: next.Body}, handoffChars)
	return err
}
