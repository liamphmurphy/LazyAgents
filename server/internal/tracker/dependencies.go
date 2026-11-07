package tracker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// DispatchBlockers is shared by unmanaged claims and the runner. Dependencies
// are satisfied only by Done, including after reopening.
func DispatchBlockers(ctx context.Context, q *store.Queries, t store.Ticket) ([]string, error) {
	out, err := HardBlockers(ctx, q, t)
	if err != nil {
		return nil, err
	}
	deps, err := QueuedBehind(ctx, q, t)
	return append(out, deps...), err
}

// HardBlockers are reasons a ticket can't be dispatched at all. Unfinished
// dependencies aren't among them: a managed run queues until they're Done.
func HardBlockers(ctx context.Context, q *store.Queries, t store.Ticket) ([]string, error) {
	out := []string{}
	p, err := q.GetProject(ctx, store.GetProjectParams{ID: t.ProjectID, WorkspaceID: t.WorkspaceID})
	if err != nil {
		return nil, err
	}
	if p.Archived {
		out = append(out, "Project "+p.Key+" is archived; restore it before starting work")
	}
	if t.EpicReview && t.EpicID != nil {
		open, err := q.EpicUnfinishedKeys(ctx, t.EpicID)
		if err != nil {
			return nil, err
		}
		if len(open) > 0 {
			var parts []string
			for i, o := range open {
				if i == 10 {
					parts = append(parts, fmt.Sprintf("and %d more", len(open)-10))
					break
				}
				parts = append(parts, o.Key+" ("+o.Status+")")
			}
			out = append(out, "Final review waits for the epic's other tickets to finish: "+strings.Join(parts, ", "))
		}
	}
	return out, nil
}

// QueuedBehind describes the unfinished dependencies a dispatched run waits on.
func QueuedBehind(ctx context.Context, q *store.Queries, t store.Ticket) ([]string, error) {
	deps, err := q.UnmetDependencies(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, d := range deps {
		out = append(out, "Blocked by "+d.Key+" — "+d.Title+" ("+d.Status+"); it must be Done")
	}
	return out, nil
}

func (s *Service) SetDependencies(ctx context.Context, ws uuid.UUID, actor Actor, key string, revision int32, keys []string) error {
	return s.tx(ctx, func(q *store.Queries) error {
		// Graph edits serialize per workspace so concurrent inverse edges cannot
		// both pass cycle detection. Each edit also locks its dependent ticket.
		if err := q.LockDependencyGraph(ctx, ws); err != nil {
			return err
		}
		t, err := q.LockTicketByKey(ctx, store.LockTicketByKeyParams{WorkspaceID: ws, Key: strings.ToUpper(strings.TrimSpace(key))})
		if err != nil {
			return notFound(err)
		}
		if t.DependencyRevision != revision {
			return apperr.Conflict("dependencies changed; reload before saving")
		}
		return SetDependenciesInTx(ctx, q, ws, actor, t, keys)
	})
}

// SetDependenciesInTx is also used by atomic planning acceptance. The caller
// holds the workspace graph lock and the ticket row (new inserts own theirs).
func SetDependenciesInTx(ctx context.Context, q *store.Queries, ws uuid.UUID, actor Actor, t store.Ticket, keys []string) error {
	if len(keys) > 50 {
		return apperr.Invalid("blocked_by", "use at most 50 blockers")
	}
	if _, err := q.ActiveRunForTicket(ctx, &t.ID); err == nil {
		return apperr.Conflict("finish or cancel this ticket's active run before changing dependencies")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	seen := map[uuid.UUID]bool{}
	ids := []uuid.UUID{}
	clean := []string{}
	for _, key := range keys {
		b, err := Get(ctx, q, ws, strings.ToUpper(strings.TrimSpace(key)))
		if err != nil {
			return apperr.Invalid("blocked_by", "unknown ticket %s", key)
		}
		if b.ID == t.ID {
			return apperr.Invalid("blocked_by", "a ticket cannot block itself")
		}
		if seen[b.ID] {
			continue
		}
		seen[b.ID] = true
		cycle, err := q.DependencyWouldCycle(ctx, store.DependencyWouldCycleParams{Blocker: b.ID, Ticket: t.ID})
		if err != nil {
			return err
		}
		if cycle {
			return apperr.Invalid("blocked_by", "%s would create a dependency cycle", b.Key)
		}
		ids = append(ids, b.ID)
		clean = append(clean, b.Key)
	}
	if err := q.ClearDependencies(ctx, t.ID); err != nil {
		return err
	}
	for _, id := range ids {
		if err := q.AddDependency(ctx, store.AddDependencyParams{TicketID: t.ID, BlockerID: id}); err != nil {
			return err
		}
	}
	if err := q.BumpDependencyRevision(ctx, t.ID); err != nil {
		return err
	}
	return Record(ctx, q, ws, t.ID, t.EpicID, actor, "dependencies_changed", map[string]any{"blocked_by": clean}, nil)
}
