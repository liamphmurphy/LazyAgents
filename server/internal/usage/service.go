package usage

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Harnesses are the ones with plan allowances, in display order.
var Harnesses = []string{agentdef.HarnessClaudeCode, agentdef.HarnessCodex}

const (
	// Fresh is how long a reading stands before a check asks again.
	Fresh = 2 * time.Minute
	// failedRetry waits out a check that didn't answer.
	failedRetry = 10 * time.Minute
	// settledRetry waits out what only the person can fix: a CLI that's
	// missing, signed out or too old.
	settledRetry = 30 * time.Minute
)

// Service saves readings and keeps them fresh.
type Service struct {
	Q *store.Queries
	// Probe checks a harness through its CLI. Nil turns active checks off,
	// as on the Docker backend, where the CLIs and their logins live in run
	// containers; readings then come only from runs (Record).
	Probe func(ctx context.Context, harness string) (Snapshot, error)
	Log   *slog.Logger
	// Now is for tests.
	Now func() time.Time

	mu   sync.Mutex
	next map[string]time.Time // workspace/harness → earliest next check
	busy map[string]bool
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// Save replaces a harness's reading.
func (s *Service) Save(ctx context.Context, ws uuid.UUID, snap Snapshot) error {
	windows, err := json.Marshal(Current(snap.Windows, s.now()))
	if err != nil {
		return err
	}
	return s.Q.SaveProviderUsage(ctx, store.SaveProviderUsageParams{WorkspaceID: ws, Harness: snap.Harness, Plan: snap.Plan,
		Available: snap.Available, Windows: windows, ObservedAt: snap.ObservedAt})
}

// Record lays windows a run reported over the saved reading, keeping the
// plan and the windows the run didn't mention.
func (s *Service) Record(ctx context.Context, ws uuid.UUID, harness string, windows []Window) error {
	if len(windows) == 0 {
		return nil
	}
	snap := Snapshot{Harness: harness, Available: true, ObservedAt: s.now()}
	row, err := s.Q.ProviderUsageFor(ctx, store.ProviderUsageForParams{WorkspaceID: ws, Harness: harness})
	switch {
	case err == nil:
		snap.Plan = row.Plan
		var old []Window
		_ = json.Unmarshal(row.Windows, &old)
		snap.Windows = Merge(old, windows, snap.ObservedAt)
	case errors.Is(err, pgx.ErrNoRows):
		snap.Windows = Merge(nil, windows, snap.ObservedAt)
	default:
		return err
	}
	return s.Save(ctx, ws, snap)
}

// Readings returns the saved readings by harness, with renewed windows dropped.
func (s *Service) Readings(ctx context.Context, ws uuid.UUID) (map[string]Snapshot, error) {
	rows, err := s.Q.ProviderUsage(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := map[string]Snapshot{}
	now := s.now()
	for _, r := range rows {
		var ws []Window
		_ = json.Unmarshal(r.Windows, &ws)
		out[r.Harness] = Snapshot{Harness: r.Harness, Plan: r.Plan, Available: r.Available, Windows: Current(ws, now), ObservedAt: r.ObservedAt}
	}
	return out, nil
}

// Refresh starts a check of each harness whose reading is due, in the
// background, and returns at once. Checks never overlap for a harness, and
// one that failed waits before trying again.
func (s *Service) Refresh(ws uuid.UUID, readings map[string]Snapshot) {
	if s.Probe == nil {
		return
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next == nil {
		s.next, s.busy = map[string]time.Time{}, map[string]bool{}
	}
	for _, h := range Harnesses {
		key := ws.String() + "/" + h
		if s.busy[key] || now.Before(s.next[key]) {
			continue
		}
		if r, ok := readings[h]; ok && now.Sub(r.ObservedAt) < Fresh {
			continue
		}
		s.busy[key] = true
		go s.check(ws, h, key)
	}
}

func (s *Service) check(ws uuid.UUID, harness, key string) {
	ctx, cancel := context.WithTimeout(context.Background(), ProbeTimeout+5*time.Second)
	defer cancel()
	snap, err := s.Probe(ctx, harness)
	wait := Fresh
	switch {
	case errors.Is(err, ErrSignedOut):
		// Saved so the orb can say limits don't apply, rather than nothing.
		wait = settledRetry
		snap.Harness, snap.Available, snap.ObservedAt = harness, false, s.now()
		if snap.Windows == nil {
			snap.Windows = []Window{}
		}
		if serr := s.Save(ctx, ws, snap); serr != nil {
			s.log().Warn("save harness usage", "harness", harness, "err", serr)
		}
	case errors.Is(err, ErrOutdated):
		wait = settledRetry
	case err != nil:
		wait = failedRetry
		s.log().Debug("harness usage check", "harness", harness, "err", err)
	default:
		if serr := s.Save(ctx, ws, snap); serr != nil {
			s.log().Warn("save harness usage", "harness", harness, "err", serr)
		}
	}
	s.mu.Lock()
	s.busy[key] = false
	s.next[key] = s.now().Add(wait)
	s.mu.Unlock()
}
