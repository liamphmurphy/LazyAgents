package tracker

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
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

// lockCriterionTicket takes the same locks as Update (project, then ticket row)
// for the ticket owning criterion id, and returns the criterion as seen under them.
func lockCriterionTicket(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (store.Criterion, error) {
	c, err := q.GetCriterion(ctx, store.GetCriterionParams{ID: id, WorkspaceID: ws})
	if err != nil {
		return c, notFound(err)
	}
	pid, err := q.TicketProjectID(ctx, store.TicketProjectIDParams{WorkspaceID: ws, ID: c.TicketID})
	if err != nil {
		return c, notFound(err)
	}
	if _, err := q.LockProject(ctx, store.LockProjectParams{ID: pid, WorkspaceID: ws}); err != nil {
		return c, notFound(err)
	}
	if _, err := q.LockTicketByID(ctx, store.LockTicketByIDParams{WorkspaceID: ws, ID: c.TicketID}); err != nil {
		return c, notFound(err)
	}
	c, err = q.GetCriterion(ctx, store.GetCriterionParams{ID: id, WorkspaceID: ws})
	return c, notFound(err)
}

// ExpectedCriterion is the client's snapshot of one criterion.
type ExpectedCriterion struct {
	ID       uuid.UUID
	Position int
	Text     string
	Done     bool
}

// CriterionDraft is one desired criterion; a nil ID creates it.
type CriterionDraft struct {
	ID   *uuid.UUID
	Text string
	Done bool
}

// CriteriaDraft replaces a ticket's criteria atomically with its field patch.
type CriteriaDraft struct {
	Expected []ExpectedCriterion
	Items    []CriterionDraft
}

const maxCriteria = 200

// validateDraft checks shape only; ownership and snapshot are checked under the row lock.
func validateDraft(d *CriteriaDraft) ([]string, error) {
	if len(d.Expected) > maxCriteria || len(d.Items) > maxCriteria {
		return nil, invalid("criteria", "at most %d criteria", maxCriteria)
	}
	texts := make([]string, len(d.Items))
	seen := map[uuid.UUID]bool{}
	for i, it := range d.Items {
		text, err := validCriterion(it.Text)
		if err != nil {
			return nil, invalid("criteria", "criterion %d: %s", i+1, err.(*ValidationError).Msg)
		}
		texts[i] = text
		if it.ID != nil {
			if seen[*it.ID] {
				return nil, invalid("criteria", "duplicate criterion id %s", *it.ID)
			}
			seen[*it.ID] = true
		}
	}
	return texts, nil
}

// applyCriteriaDraft runs under the project and ticket row locks, right after the
// revision check. It verifies ids and the expected snapshot before its first write;
// any later failure in the caller's transaction rolls the criteria back too.
func applyCriteriaDraft(ctx context.Context, q *store.Queries, ws uuid.UUID, actor Actor, ticketID uuid.UUID, epicID *uuid.UUID, d *CriteriaDraft, texts []string) error {
	cur, err := q.ListCriteria(ctx, ticketID)
	if err != nil {
		return err
	}
	byID := make(map[uuid.UUID]store.Criterion, len(cur))
	for _, c := range cur {
		byID[c.ID] = c
	}
	stale := apperr.ConflictField("criteria", "acceptance criteria changed; reload and retry")
	exp := map[uuid.UUID]bool{}
	for _, e := range d.Expected {
		c, ok := byID[e.ID]
		if !ok || exp[e.ID] || int(c.Position) != e.Position || c.Text != e.Text || c.Done != e.Done {
			return stale
		}
		exp[e.ID] = true
	}
	if len(exp) != len(cur) {
		return stale
	}
	// Only once the snapshot matches is an unknown id the client's mistake (422);
	// before that it may be a criterion deleted concurrently (409 above).
	for _, it := range d.Items {
		if it.ID != nil {
			if _, ok := byID[*it.ID]; !ok {
				return invalid("criteria", "unknown criterion %s", *it.ID)
			}
		}
	}

	kept := map[uuid.UUID]bool{}
	for i, it := range d.Items {
		pos := int32(i + 1)
		if it.ID == nil {
			c, err := q.CreateCriterionAt(ctx, store.CreateCriterionAtParams{TicketID: ticketID, Position: pos, Text: texts[i], Done: it.Done})
			if err != nil {
				return err
			}
			payload := map[string]any{"criterion_id": c.ID, "text": c.Text}
			if err := record(ctx, q, ws, ticketID, epicID, actor, event{verb: VerbCriterionAdded, payload: payload}); err != nil {
				return err
			}
			if it.Done {
				if err := record(ctx, q, ws, ticketID, epicID, actor, event{verb: VerbCriterionChecked, payload: payload}); err != nil {
					return err
				}
			}
			continue
		}
		kept[*it.ID] = true
		old := byID[*it.ID]
		if old.Position == pos && old.Text == texts[i] && old.Done == it.Done {
			continue
		}
		c, err := q.SaveCriterionDraft(ctx, store.SaveCriterionDraftParams{ID: old.ID, Position: pos, Text: texts[i], Done: it.Done})
		if err != nil {
			return err
		}
		if it.Done != old.Done {
			verb := VerbCriterionUnchecked
			if it.Done {
				verb = VerbCriterionChecked
			}
			if err := record(ctx, q, ws, ticketID, epicID, actor, event{verb: verb,
				payload: map[string]any{"criterion_id": c.ID, "text": c.Text}}); err != nil {
				return err
			}
		}
	}
	for _, c := range cur {
		if kept[c.ID] {
			continue
		}
		if err := q.DeleteCriterion(ctx, c.ID); err != nil {
			return err
		}
		if err := record(ctx, q, ws, ticketID, epicID, actor, event{verb: VerbCriterionRemoved,
			payload: map[string]any{"text": c.Text}}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) AddCriterion(ctx context.Context, ws uuid.UUID, actor Actor, key, text string) (store.Criterion, error) {
	text, err := validCriterion(text)
	if err != nil {
		return store.Criterion{}, err
	}
	var c store.Criterion
	err = s.tx(ctx, func(q *store.Queries) error {
		if err := LockTicketProjects(ctx, q, ws, []string{key}); err != nil {
			return err
		}
		t, err := q.LockTicketByKey(ctx, store.LockTicketByKeyParams{WorkspaceID: ws, Key: key})
		if err != nil {
			return notFound(err)
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
		cur, err := lockCriterionTicket(ctx, q, ws, id)
		if err != nil {
			return err
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
		cur, err := lockCriterionTicket(ctx, q, ws, id)
		if err != nil {
			return err
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
