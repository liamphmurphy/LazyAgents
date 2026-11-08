package tracker

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// LockTicketProjects acquires all ticket projects in UUID order before ticket locks.
func LockTicketProjects(ctx context.Context, q *store.Queries, ws uuid.UUID, keys []string) error {
	ids := map[uuid.UUID]bool{}
	for _, key := range keys {
		t, err := Get(ctx, q, ws, strings.ToUpper(strings.TrimSpace(key)))
		if err != nil {
			return err
		}
		ids[t.ProjectID] = true
	}
	ordered := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].String() < ordered[j].String() })
	for _, id := range ordered {
		if _, err := q.LockProject(ctx, store.LockProjectParams{ID: id, WorkspaceID: ws}); err != nil {
			return notFound(err)
		}
	}
	return nil
}

func lockEpicProject(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (store.Epic, error) {
	e, err := q.GetEpic(ctx, store.GetEpicParams{ID: id, WorkspaceID: ws})
	if err != nil {
		return e, notFound(err)
	}
	if _, err = q.LockProject(ctx, store.LockProjectParams{ID: e.ProjectID, WorkspaceID: ws}); err != nil {
		return e, notFound(err)
	}
	e, err = q.GetEpic(ctx, store.GetEpicParams{ID: id, WorkspaceID: ws})
	return e, notFound(err)
}

// EpicBlockers describes the state preventing an epic from being closed.
type EpicBlockers struct {
	UncheckedPositions []int32
	OpenTicketKeys     []string
}

func epicBlockers(ctx context.Context, q *store.Queries, id uuid.UUID) (EpicBlockers, error) {
	b := EpicBlockers{UncheckedPositions: []int32{}}
	cs, err := q.ListEpicCriteria(ctx, id)
	if err != nil {
		return b, err
	}
	for _, c := range cs {
		if !c.Done {
			b.UncheckedPositions = append(b.UncheckedPositions, c.Position)
		}
	}
	b.OpenTicketKeys, err = q.EpicOpenTicketKeys(ctx, &id)
	return b, err
}
func (s *Service) EpicBlockers(ctx context.Context, ws, id uuid.UUID) (EpicBlockers, error) {
	var b EpicBlockers
	err := s.tx(ctx, func(q *store.Queries) error {
		if _, err := lockEpicProject(ctx, q, ws, id); err != nil {
			return err
		}
		var err error
		b, err = epicBlockers(ctx, q, id)
		return err
	})
	return b, err
}

// UpdateEpic atomically applies the status and mockup requirement.
func (s *Service) UpdateEpic(ctx context.Context, ws, id uuid.UUID, status *string, requires *bool) (store.Epic, error) {
	var out store.Epic
	if status != nil {
		if err := checkEnum("status", *status, []string{"open", "done"}); err != nil {
			return out, err
		}
	}
	err := s.tx(ctx, func(q *store.Queries) error {
		e, err := lockEpicProject(ctx, q, ws, id)
		if err != nil {
			return err
		}
		out = e
		if status != nil && *status != e.Status {
			if *status == "done" {
				b, err := epicBlockers(ctx, q, id)
				if err != nil {
					return err
				}
				if len(b.UncheckedPositions) > 0 || len(b.OpenTicketKeys) > 0 {
					return apperr.Conflict("epic has unchecked criteria %v and open tickets %v", b.UncheckedPositions, b.OpenTicketKeys)
				}
			}
			out, err = q.SetEpicStatus(ctx, store.SetEpicStatusParams{ID: id, WorkspaceID: ws, Status: *status})
			if err != nil {
				return err
			}
		}
		if requires != nil && *requires != out.RequiresMockup {
			out, err = q.SetEpicRequiresMockup(ctx, store.SetEpicRequiresMockupParams{ID: id, RequiresMockup: *requires})
			if err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}
func (s *Service) SetEpicStatus(ctx context.Context, ws, id uuid.UUID, status string) (store.Epic, error) {
	return s.UpdateEpic(ctx, ws, id, &status, nil)
}
func (s *Service) ListEpicCriteria(ctx context.Context, ws, id uuid.UUID) ([]store.EpicCriterion, error) {
	var out []store.EpicCriterion
	err := s.tx(ctx, func(q *store.Queries) error {
		if _, err := q.GetEpic(ctx, store.GetEpicParams{ID: id, WorkspaceID: ws}); err != nil {
			return notFound(err)
		}
		var err error
		out, err = q.ListEpicCriteria(ctx, id)
		return err
	})
	return out, err
}
func (s *Service) CreateEpicCriterion(ctx context.Context, ws, id uuid.UUID, text string) (store.EpicCriterion, error) {
	var out store.EpicCriterion
	text, err := validCriterion(text)
	if err != nil {
		return out, err
	}
	err = s.tx(ctx, func(q *store.Queries) error {
		e, err := lockEpicProject(ctx, q, ws, id)
		if err != nil {
			return err
		}
		if e.Status == "done" {
			return apperr.Conflict("epic is done")
		}
		out, err = q.CreateEpicCriterion(ctx, store.CreateEpicCriterionParams{EpicID: id, Text: text})
		return err
	})
	return out, err
}

// PatchEpicCriterion edits and optionally reorders a criterion (positions are one-based).
func (s *Service) PatchEpicCriterion(ctx context.Context, ws uuid.UUID, actor Actor, id uuid.UUID, text *string, done *bool, position *int) (store.EpicCriterion, error) {
	var out store.EpicCriterion
	err := s.tx(ctx, func(q *store.Queries) error {
		c, err := q.GetEpicCriterion(ctx, store.GetEpicCriterionParams{ID: id, WorkspaceID: ws})
		if err != nil {
			return notFound(err)
		}
		e, err := lockEpicProject(ctx, q, ws, c.EpicID)
		if err != nil {
			return err
		}
		if e.Status == "done" {
			return apperr.Conflict("epic is done")
		}
		c, err = q.GetEpicCriterion(ctx, store.GetEpicCriterionParams{ID: id, WorkspaceID: ws})
		if err != nil {
			return notFound(err)
		}
		if position != nil {
			cs, err := q.ListEpicCriteria(ctx, e.ID)
			if err != nil {
				return err
			}
			if *position < 1 || *position > len(cs) {
				return invalid("position", "must be between 1 and %d", len(cs))
			}
			if int(c.Position) != *position {
				if err = q.ReorderEpicCriterion(ctx, store.ReorderEpicCriterionParams{ID: id, EpicID: e.ID, OldPosition: c.Position, NewPosition: int32(*position)}); err != nil {
					return err
				}
			}
		}
		if text != nil {
			v, err := validCriterion(*text)
			if err != nil {
				return err
			}
			if v != c.Text {
				if _, err = q.UpdateEpicCriterionText(ctx, store.UpdateEpicCriterionTextParams{ID: id, Text: v}); err != nil {
					return err
				}
			}
		}
		if done != nil && *done != c.Done {
			if *done && (actor.ID == nil || (actor.Type != "user" && actor.Type != "agent")) {
				return invalid("actor", "checking needs a user or agent")
			}
			if _, err = q.SetEpicCriterionDone(ctx, store.SetEpicCriterionDoneParams{ID: id, Done: *done, ByType: &actor.Type, ByID: actor.ID}); err != nil {
				return err
			}
		}
		out, err = q.GetEpicCriterion(ctx, store.GetEpicCriterionParams{ID: id, WorkspaceID: ws})
		return err
	})
	return out, err
}
func (s *Service) DeleteEpicCriterion(ctx context.Context, ws, id uuid.UUID) error {
	return s.tx(ctx, func(q *store.Queries) error {
		c, err := q.GetEpicCriterion(ctx, store.GetEpicCriterionParams{ID: id, WorkspaceID: ws})
		if err != nil {
			return notFound(err)
		}
		e, err := lockEpicProject(ctx, q, ws, c.EpicID)
		if err != nil {
			return err
		}
		if e.Status == "done" {
			return apperr.Conflict("epic is done")
		}
		c, err = q.GetEpicCriterion(ctx, store.GetEpicCriterionParams{ID: id, WorkspaceID: ws})
		if err != nil {
			return notFound(err)
		}
		if _, err = q.DeleteEpicCriterion(ctx, id); err != nil {
			return err
		}
		return q.CompactEpicCriteria(ctx, store.CompactEpicCriteriaParams{EpicID: c.EpicID, Position: c.Position})
	})
}

// CreateEpic creates an epic with optional initial acceptance criteria, all or nothing.
func (s *Service) CreateEpic(ctx context.Context, p store.CreateEpicParams, criteria []string) (store.Epic, error) {
	texts := make([]string, 0, len(criteria))
	for _, c := range criteria {
		t, err := validCriterion(c)
		if err != nil {
			return store.Epic{}, invalid("criteria", "each criterion must be 1–1000 characters")
		}
		texts = append(texts, t)
	}
	var out store.Epic
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		if out, err = q.CreateEpic(ctx, p); err != nil {
			return err
		}
		for _, t := range texts {
			if _, err := q.CreateEpicCriterion(ctx, store.CreateEpicCriterionParams{EpicID: out.ID, Text: t}); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}
