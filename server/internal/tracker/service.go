package tracker

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

var (
	Statuses   = []string{"backlog", "todo", "in_progress", "in_review", "done"}
	Priorities = []string{"none", "low", "medium", "high", "urgent"}
)

type Service struct {
	Pool *pgxpool.Pool
}

func (s *Service) tx(ctx context.Context, fn func(q *store.Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error { return fn(store.New(tx)) })
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Opt distinguishes "leave unchanged" (Set=false) from "clear" (Set, Value=nil).
type Opt[T any] struct {
	Set   bool
	Value *T
}

func SetTo[T any](v T) Opt[T] { return Opt[T]{Set: true, Value: &v} }
func Clear[T any]() Opt[T]    { return Opt[T]{Set: true} }

type NewTicket struct {
	ProjectKey      string
	Title           string
	DescriptionMd   string
	Status          string
	Priority        string
	EpicID          *uuid.UUID
	AssigneeAgentID *uuid.UUID
	ReviewerUserID  *uuid.UUID
	LabelIDs        []uuid.UUID
	Estimate        *string
	DueDate         *time.Time
	Criteria        []string
	// CreatedAt backdates the "created" event (seeding only).
	CreatedAt *time.Time
}

type Patch struct {
	Title           *string
	DescriptionMd   *string
	Status          *string
	Priority        *string
	EpicID          Opt[uuid.UUID]
	AssigneeAgentID Opt[uuid.UUID]
	ReviewerUserID  Opt[uuid.UUID]
	Estimate        Opt[string]
	DueDate         Opt[time.Time]
	LabelIDs        *[]uuid.UUID
}

func validTitle(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" {
		return "", invalid("title", "is required")
	}
	if len([]rune(t)) > 300 {
		return "", invalid("title", "must be at most 300 characters")
	}
	return t, nil
}

func checkEnum(field, v string, allowed []string) error {
	if !slices.Contains(allowed, v) {
		return invalid(field, "must be one of %s", strings.Join(allowed, ", "))
	}
	return nil
}

// refs validates that referenced records exist in the workspace and returns
// display names for activity payloads.
type refs struct {
	q  *store.Queries
	ws uuid.UUID
}

func (r refs) agent(ctx context.Context, id *uuid.UUID) (store.Agent, error) {
	a, err := r.q.GetAgent(ctx, store.GetAgentParams{ID: *id, WorkspaceID: r.ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return a, invalid("assignee_agent_id", "unknown agent")
	}
	return a, err
}

func (r refs) user(ctx context.Context, id *uuid.UUID) (store.User, error) {
	u, err := r.q.GetUser(ctx, store.GetUserParams{ID: *id, WorkspaceID: r.ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return u, invalid("reviewer_user_id", "unknown user")
	}
	return u, err
}

func (r refs) epic(ctx context.Context, id *uuid.UUID, projectID uuid.UUID) (store.Epic, error) {
	e, err := r.q.GetEpic(ctx, store.GetEpicParams{ID: *id, WorkspaceID: r.ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return e, invalid("epic_id", "unknown epic")
	}
	if err == nil && e.ProjectID != projectID {
		return e, invalid("epic_id", "epic belongs to another project")
	}
	return e, err
}

func (r refs) labels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]store.Label, error) {
	all, err := r.q.ListLabels(ctx, r.ws)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]store.Label, len(all))
	for _, l := range all {
		byID[l.ID] = l
	}
	for _, id := range ids {
		if _, ok := byID[id]; !ok {
			return nil, invalid("label_ids", "unknown label %s", id)
		}
	}
	return byID, nil
}

// Create allocates the next number in the project and records "created".
func (s *Service) Create(ctx context.Context, ws uuid.UUID, actor Actor, in NewTicket) (store.Ticket, error) {
	title, err := validTitle(in.Title)
	if err != nil {
		return store.Ticket{}, err
	}
	if in.Status == "" {
		in.Status = "backlog"
	}
	if in.Priority == "" {
		in.Priority = "none"
	}
	if err := checkEnum("status", in.Status, Statuses); err != nil {
		return store.Ticket{}, err
	}
	if err := checkEnum("priority", in.Priority, Priorities); err != nil {
		return store.Ticket{}, err
	}

	var t store.Ticket
	err = s.tx(ctx, func(q *store.Queries) error {
		r := refs{q, ws}
		p, err := q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: ws, Key: in.ProjectKey})
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("project_key", "unknown project")
		} else if err != nil {
			return err
		}
		if in.EpicID != nil {
			if _, err := r.epic(ctx, in.EpicID, p.ID); err != nil {
				return err
			}
		}
		var agent *store.Agent
		if in.AssigneeAgentID != nil {
			a, err := r.agent(ctx, in.AssigneeAgentID)
			if err != nil {
				return err
			}
			agent = &a
		}
		if in.ReviewerUserID != nil {
			if _, err := r.user(ctx, in.ReviewerUserID); err != nil {
				return err
			}
		}
		if _, err := r.labels(ctx, in.LabelIDs); err != nil {
			return err
		}
		n, err := q.AllocateTicketNumber(ctx, p.ID)
		if err != nil {
			return err
		}
		t, err = q.CreateTicket(ctx, store.CreateTicketParams{
			WorkspaceID: ws, ProjectID: p.ID, Number: n, Key: fmt.Sprintf("%s-%d", p.Key, n),
			Title: title, DescriptionMd: in.DescriptionMd, Status: in.Status, Priority: in.Priority,
			EpicID: in.EpicID, AssigneeAgentID: in.AssigneeAgentID, ReviewerUserID: in.ReviewerUserID,
			CreatedByUserID: userID(actor), Estimate: emptyToNil(in.Estimate), DueDate: in.DueDate,
		})
		if err != nil {
			return err
		}
		if len(in.LabelIDs) > 0 {
			if err := q.AddTicketLabels(ctx, store.AddTicketLabelsParams{TicketID: t.ID, LabelIds: in.LabelIDs}); err != nil {
				return err
			}
		}
		for _, c := range in.Criteria {
			if c = strings.TrimSpace(c); c != "" {
				if _, err := q.CreateCriterion(ctx, store.CreateCriterionParams{TicketID: t.ID, Text: c}); err != nil {
					return err
				}
			}
		}
		at := in.CreatedAt
		if err := record(ctx, q, ws, t.ID, t.EpicID, actor, event{verb: VerbCreated, at: at}); err != nil {
			return err
		}
		if agent != nil {
			if err := record(ctx, q, ws, t.ID, t.EpicID, actor, event{verb: VerbAssigned, at: at,
				payload: map[string]any{"agent_id": agent.ID, "agent_name": agent.Name}}); err != nil {
				return err
			}
		}
		return nil
	})
	return t, err
}

func userID(a Actor) *uuid.UUID {
	if a.Type == "user" {
		return a.ID
	}
	return nil
}

func emptyToNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	v := strings.TrimSpace(*s)
	return &v
}

func Get(ctx context.Context, q *store.Queries, ws uuid.UUID, key string) (store.Ticket, error) {
	t, err := q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: ws, Key: key})
	return t, notFound(err)
}

// Update applies a patch to one ticket.
func (s *Service) Update(ctx context.Context, ws uuid.UUID, actor Actor, key string, p Patch) (store.Ticket, error) {
	var t store.Ticket
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		t, err = applyPatch(ctx, q, ws, actor, key, p)
		return err
	})
	return t, err
}

// Bulk applies the same patch to many tickets atomically, optionally
// attaching one artifact to each. Title and description are per-ticket and
// rejected here.
func (s *Service) Bulk(ctx context.Context, ws uuid.UUID, actor Actor, keys []string, p Patch, attach *uuid.UUID) (int, error) {
	if p.Title != nil || p.DescriptionMd != nil {
		return 0, invalid("patch", "title and description can't be bulk-edited")
	}
	if len(keys) == 0 || len(keys) > 200 {
		return 0, invalid("keys", "must list 1 to 200 tickets")
	}
	n := 0
	err := s.tx(ctx, func(q *store.Queries) error {
		for _, k := range keys {
			if _, err := applyPatch(ctx, q, ws, actor, k, p); err != nil {
				if errors.Is(err, ErrNotFound) {
					return invalid("keys", "unknown ticket %s", k)
				}
				return err
			}
			if attach != nil {
				if err := attachArtifact(ctx, q, ws, actor, k, *attach, "context"); err != nil {
					return err
				}
			}
			n++
		}
		return nil
	})
	return n, err
}

func ptrEq[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func dateEq(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Format(time.DateOnly) == b.Format(time.DateOnly)
}

func applyPatch(ctx context.Context, q *store.Queries, ws uuid.UUID, actor Actor, key string, p Patch) (store.Ticket, error) {
	cur, err := q.LockTicketByKey(ctx, store.LockTicketByKeyParams{WorkspaceID: ws, Key: key})
	if err != nil {
		return cur, notFound(err)
	}
	r := refs{q, ws}
	next := store.UpdateTicketParams{
		ID: cur.ID, Title: cur.Title, DescriptionMd: cur.DescriptionMd, Status: cur.Status, Priority: cur.Priority,
		EpicID: cur.EpicID, AssigneeAgentID: cur.AssigneeAgentID, ReviewerUserID: cur.ReviewerUserID,
		Estimate: cur.Estimate, DueDate: cur.DueDate,
	}
	var events []event
	add := func(verb string, payload map[string]any) {
		events = append(events, event{verb: verb, payload: payload})
	}

	if p.Title != nil {
		title, err := validTitle(*p.Title)
		if err != nil {
			return cur, err
		}
		if title != cur.Title {
			next.Title = title
			add(VerbTitleChanged, map[string]any{"from": cur.Title, "to": title})
		}
	}
	if p.DescriptionMd != nil && *p.DescriptionMd != cur.DescriptionMd {
		next.DescriptionMd = *p.DescriptionMd
		add(VerbDescriptionEdited, nil)
	}
	if p.Status != nil && *p.Status != cur.Status {
		if err := checkEnum("status", *p.Status, Statuses); err != nil {
			return cur, err
		}
		next.Status = *p.Status
		add(VerbStatusChanged, map[string]any{"from": cur.Status, "to": *p.Status})
	}
	if p.Priority != nil && *p.Priority != cur.Priority {
		if err := checkEnum("priority", *p.Priority, Priorities); err != nil {
			return cur, err
		}
		next.Priority = *p.Priority
		add(VerbPriorityChanged, map[string]any{"from": cur.Priority, "to": *p.Priority})
	}
	if p.AssigneeAgentID.Set && !ptrEq(p.AssigneeAgentID.Value, cur.AssigneeAgentID) {
		if cur.AssigneeAgentID != nil {
			old, err := r.agent(ctx, cur.AssigneeAgentID)
			if err != nil {
				return cur, err
			}
			add(VerbUnassigned, map[string]any{"agent_id": old.ID, "agent_name": old.Name})
		}
		if p.AssigneeAgentID.Value != nil {
			a, err := r.agent(ctx, p.AssigneeAgentID.Value)
			if err != nil {
				return cur, err
			}
			add(VerbAssigned, map[string]any{"agent_id": a.ID, "agent_name": a.Name})
		}
		next.AssigneeAgentID = p.AssigneeAgentID.Value
	}
	if p.ReviewerUserID.Set && !ptrEq(p.ReviewerUserID.Value, cur.ReviewerUserID) {
		if p.ReviewerUserID.Value != nil {
			u, err := r.user(ctx, p.ReviewerUserID.Value)
			if err != nil {
				return cur, err
			}
			add(VerbReviewerSet, map[string]any{"user_id": u.ID, "user_name": u.Name})
		} else {
			add(VerbReviewerCleared, nil)
		}
		next.ReviewerUserID = p.ReviewerUserID.Value
	}
	if p.EpicID.Set && !ptrEq(p.EpicID.Value, cur.EpicID) {
		payload := map[string]any{}
		if p.EpicID.Value != nil {
			e, err := r.epic(ctx, p.EpicID.Value, cur.ProjectID)
			if err != nil {
				return cur, err
			}
			payload["epic_id"], payload["epic_name"] = e.ID, e.Name
		}
		next.EpicID = p.EpicID.Value
		add(VerbEpicChanged, payload)
	}
	if p.Estimate.Set {
		v := emptyToNil(p.Estimate.Value)
		if !ptrEq(v, cur.Estimate) {
			next.Estimate = v
			add(VerbEstimateChanged, map[string]any{"to": v})
		}
	}
	if p.DueDate.Set && !dateEq(p.DueDate.Value, cur.DueDate) {
		next.DueDate = p.DueDate.Value
		var to any
		if p.DueDate.Value != nil {
			to = p.DueDate.Value.Format(time.DateOnly)
		}
		add(VerbDueChanged, map[string]any{"to": to})
	}

	t := cur
	if len(events) > 0 {
		if t, err = q.UpdateTicket(ctx, next); err != nil {
			return cur, err
		}
	}

	if p.LabelIDs != nil {
		ev, err := setLabels(ctx, q, r, cur.ID, *p.LabelIDs)
		if err != nil {
			return cur, err
		}
		if ev != nil {
			events = append(events, *ev)
		}
	}
	for _, e := range events {
		if err := record(ctx, q, ws, t.ID, t.EpicID, actor, e); err != nil {
			return cur, err
		}
	}
	return t, nil
}

func setLabels(ctx context.Context, q *store.Queries, r refs, ticketID uuid.UUID, ids []uuid.UUID) (*event, error) {
	byID, err := r.labels(ctx, ids)
	if err != nil {
		return nil, err
	}
	existing, err := q.ListLabelsForTickets(ctx, []uuid.UUID{ticketID})
	if err != nil {
		return nil, err
	}
	had := map[uuid.UUID]bool{}
	for _, l := range existing {
		had[l.ID] = true
	}
	want := map[uuid.UUID]bool{}
	for _, id := range ids {
		want[id] = true
	}
	added, removed := []string{}, []string{}
	for id := range want {
		if !had[id] {
			added = append(added, byID[id].Name)
		}
	}
	for _, l := range existing {
		if !want[l.ID] {
			removed = append(removed, l.Name)
		}
	}
	if len(added) == 0 && len(removed) == 0 {
		return nil, nil
	}
	slices.Sort(added)
	slices.Sort(removed)
	if err := q.ClearTicketLabels(ctx, ticketID); err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		if err := q.AddTicketLabels(ctx, store.AddTicketLabelsParams{TicketID: ticketID, LabelIds: ids}); err != nil {
			return nil, err
		}
	}
	return &event{verb: VerbLabelsChanged, payload: map[string]any{"added": added, "removed": removed}}, nil
}

// AttachArtifact links an artifact to a ticket. Attaching twice is a no-op.
func (s *Service) AttachArtifact(ctx context.Context, ws uuid.UUID, actor Actor, key string, artifactID uuid.UUID, relation string) error {
	return s.tx(ctx, func(q *store.Queries) error { return attachArtifact(ctx, q, ws, actor, key, artifactID, relation) })
}

func attachArtifact(ctx context.Context, q *store.Queries, ws uuid.UUID, actor Actor, key string, artifactID uuid.UUID, relation string) error {
	if relation == "" {
		relation = "context"
	}
	if relation != "context" && relation != "generated" {
		return invalid("relation", "must be context or generated")
	}
	t, err := Get(ctx, q, ws, key)
	if err != nil {
		return err
	}
	a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: artifactID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid("artifact_id", "unknown artifact")
	} else if err != nil {
		return err
	}
	n, err := q.AttachArtifact(ctx, store.AttachArtifactParams{TicketID: t.ID, ArtifactID: a.ID, Relation: relation})
	if err != nil || n == 0 {
		return err
	}
	return record(ctx, q, ws, t.ID, t.EpicID, actor, event{verb: VerbArtifactAttached,
		payload: map[string]any{"artifact_id": a.ID, "title": a.Title, "type": a.Type, "relation": relation}})
}

// DetachArtifact unlinks an artifact from a ticket.
func (s *Service) DetachArtifact(ctx context.Context, ws uuid.UUID, actor Actor, key string, artifactID uuid.UUID) error {
	return s.tx(ctx, func(q *store.Queries) error {
		t, err := Get(ctx, q, ws, key)
		if err != nil {
			return err
		}
		a, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: artifactID, WorkspaceID: ws})
		if err != nil {
			return notFound(err)
		}
		n, err := q.DetachArtifact(ctx, store.DetachArtifactParams{TicketID: t.ID, ArtifactID: a.ID})
		if err != nil || n == 0 {
			return err
		}
		return record(ctx, q, ws, t.ID, t.EpicID, actor, event{verb: VerbArtifactDetached,
			payload: map[string]any{"artifact_id": a.ID, "title": a.Title, "type": a.Type}})
	})
}
