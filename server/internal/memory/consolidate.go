package memory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Consolidation thresholds. Stricter than Save's near-duplicate check: nobody
// confirms what the pass merges, so it only retires entries that are clearly
// the same thing.
const (
	consolidateTitle   = 0.75
	consolidateSummary = 0.85
	consolidateBatch   = 500
)

// Merge is one entry the consolidation pass retired in favour of another.
type Merge struct {
	Owner     Owner
	RetiredID uuid.UUID
	KeptID    uuid.UUID
	Retired   string // titles, for the log
	Kept      string
	Score     float64
}

// Consolidate merges each person's duplicate memories: of two live entries of
// the same type with a near-identical title or summary, the older one is
// retired in favour of the more recently changed one. Retiring is reversible
// (restore clears superseded_by) and every merge is recorded on the retired
// entry (retired_reason, retired_at) and returned, so the person can see
// what was merged. Fewer live entries also keeps the always-loaded index
// small. Entries the person restored after a merge are left alone. It runs
// batches until no duplicates are left; a retire that fails is skipped and
// reported in the returned error.
func (s *Service) Consolidate(ctx context.Context) ([]Merge, error) {
	var out []Merge
	var errs []error
	for {
		pairs, err := s.Q.DuplicateMemoryPairs(ctx, store.DuplicateMemoryPairsParams{
			TitleThreshold: consolidateTitle, SummaryThreshold: consolidateSummary, Lim: consolidateBatch})
		if err != nil {
			return out, errors.Join(append(errs, err)...)
		}
		n := 0
		retired := map[uuid.UUID]bool{}
		for _, p := range pairs {
			// Pairs come newest keep first, so an entry retired here was
			// already merged into a newer one; skip pairs that touch it.
			if retired[p.KeepID] || retired[p.RetireID] {
				continue
			}
			reason := truncate(fmt.Sprintf("Merged into \u201c%s\u201d by memory consolidation (duplicate)", p.KeepTitle), maxReason)
			_, err := s.Q.RetireDuplicateMemory(ctx, store.RetireDuplicateMemoryParams{
				ID: p.RetireID, KeepID: &p.KeepID, WorkspaceID: p.WorkspaceID, UserID: p.UserID,
				UpdatedAt: p.RetireUpdatedAt, RetiredReason: reason})
			if errors.Is(err, pgx.ErrNoRows) {
				continue // changed, retired or deleted since the pairs were read
			}
			if err != nil {
				// One bad row mustn't stop everyone else's consolidation.
				errs = append(errs, fmt.Errorf("retire %s into %s: %w", p.RetireID, p.KeepID, err))
				continue
			}
			retired[p.RetireID] = true
			n++
			out = append(out, Merge{Owner: Owner{WorkspaceID: p.WorkspaceID, UserID: p.UserID},
				RetiredID: p.RetireID, KeptID: p.KeepID, Retired: p.RetireTitle, Kept: p.KeepTitle, Score: p.Score})
		}
		// A short batch was everything; a batch that merged nothing would
		// return the same pairs again.
		if len(pairs) < consolidateBatch || n == 0 {
			return out, errors.Join(errs...)
		}
	}
}

// RunConsolidation consolidates every interval until ctx is cancelled,
// logging each merge.
func RunConsolidation(ctx context.Context, q *store.Queries, log *slog.Logger, interval time.Duration) {
	s := New(q)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		merges, err := s.Consolidate(ctx)
		for _, m := range merges {
			log.Info("memory consolidation: merged duplicate", "user", m.Owner.UserID, "retired", m.RetiredID, "kept", m.KeptID, "score", m.Score)
		}
		if err != nil && ctx.Err() == nil {
			log.Warn("memory consolidation", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
