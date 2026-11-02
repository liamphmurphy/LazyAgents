package tracker

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/links"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

func validCriterion(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", invalid("text", "is required")
	}
	if len([]rune(text)) > 1000 {
		return "", invalid("text", "must be at most 1000 characters")
	}
	return text, nil
}

func (s *Service) AddCriterion(ctx context.Context, ws uuid.UUID, actor Actor, key, text string) (store.Criterion, error) {
	text, err := validCriterion(text)
	if err != nil {
		return store.Criterion{}, err
	}
	var c store.Criterion
	err = s.tx(ctx, func(q *store.Queries) error {
		t, err := Get(ctx, q, ws, key)
		if err != nil {
			return err
		}
		if c, err = q.CreateCriterion(ctx, store.CreateCriterionParams{TicketID: t.ID, Text: text}); err != nil {
			return err
		}
		return record(ctx, q, ws, t.ID, t.EpicID, actor, event{verb: VerbCriterionAdded,
			payload: map[string]any{"criterion_id": c.ID, "text": c.Text}})
	})
	return c, err
}

// UpdateCriterion edits text and/or ticks the box. Ticking is logged; text edits aren't.
func (s *Service) UpdateCriterion(ctx context.Context, ws uuid.UUID, actor Actor, id uuid.UUID, text *string, done *bool) (store.Criterion, error) {
	var c store.Criterion
	err := s.tx(ctx, func(q *store.Queries) error {
		cur, err := q.GetCriterion(ctx, store.GetCriterionParams{ID: id, WorkspaceID: ws})
		if err != nil {
			return notFound(err)
		}
		next := store.UpdateCriterionParams{ID: id, Text: cur.Text, Done: cur.Done}
		if text != nil {
			if next.Text, err = validCriterion(*text); err != nil {
				return err
			}
		}
		if done != nil {
			next.Done = *done
		}
		if c, err = q.UpdateCriterion(ctx, next); err != nil {
			return err
		}
		if next.Done != cur.Done {
			verb := VerbCriterionUnchecked
			if next.Done {
				verb = VerbCriterionChecked
			}
			return record(ctx, q, ws, cur.TicketID, nil, actor, event{verb: verb,
				payload: map[string]any{"criterion_id": c.ID, "text": c.Text}})
		}
		return nil
	})
	return c, err
}

func (s *Service) DeleteCriterion(ctx context.Context, ws uuid.UUID, actor Actor, id uuid.UUID) error {
	return s.tx(ctx, func(q *store.Queries) error {
		cur, err := q.GetCriterion(ctx, store.GetCriterionParams{ID: id, WorkspaceID: ws})
		if err != nil {
			return notFound(err)
		}
		if err := q.DeleteCriterion(ctx, id); err != nil {
			return err
		}
		return record(ctx, q, ws, cur.TicketID, nil, actor, event{verb: VerbCriterionRemoved,
			payload: map[string]any{"text": cur.Text}})
	})
}

type NewComment struct {
	BodyMd          string
	ParentCommentID *uuid.UUID
	At              *time.Time // seeding only
}

// Comment adds a comment and its timeline event, returning the event.
func (s *Service) Comment(ctx context.Context, ws uuid.UUID, actor Actor, key string, in NewComment) (store.ActivityEvent, error) {
	body := strings.TrimSpace(in.BodyMd)
	if body == "" {
		return store.ActivityEvent{}, invalid("body_md", "is required")
	}
	if len(body) > 20000 {
		return store.ActivityEvent{}, invalid("body_md", "is too long")
	}
	if actor.ID == nil {
		return store.ActivityEvent{}, invalid("actor", "comments need a user or agent author")
	}
	var ev store.ActivityEvent
	err := s.tx(ctx, func(q *store.Queries) error {
		t, err := Get(ctx, q, ws, key)
		if err != nil {
			return err
		}
		if in.ParentCommentID != nil {
			parent, err := q.GetComment(ctx, store.GetCommentParams{ID: *in.ParentCommentID, TicketID: t.ID})
			if err != nil {
				return invalid("parent_comment_id", "unknown comment on this ticket")
			}
			// Threads are one level deep: replies to a reply attach to its root.
			if parent.ParentCommentID != nil {
				in.ParentCommentID = parent.ParentCommentID
			}
		}
		c, err := q.CreateComment(ctx, store.CreateCommentParams{
			WorkspaceID: ws, TicketID: t.ID, ParentCommentID: in.ParentCommentID,
			AuthorType: actor.Type, AuthorID: *actor.ID, BodyMd: body,
		})
		if err != nil {
			return err
		}
		if err := links.Index(ctx, q, ws, links.Source{Kind: links.SourceComment, ID: c.ID, TicketID: &t.ID}, c.BodyMd); err != nil {
			return err
		}
		payload := map[string]any{"comment_id": c.ID, "body_md": c.BodyMd}
		if c.ParentCommentID != nil {
			payload["parent_comment_id"] = *c.ParentCommentID
		}
		ev, err = insert(ctx, q, ws, t.ID, t.EpicID, actor, event{verb: VerbCommented, payload: payload, at: in.At})
		return err
	})
	return ev, err
}
