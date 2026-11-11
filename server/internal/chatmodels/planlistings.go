package chatmodels

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// errStillChecking is shown while no runner has answered yet.
var errStillChecking = errors.New("Vesper is still checking which models your plan offers. Try again in a moment, and check that the runner is running.")

// ErrStillChecking is errStillChecking for a Discover that reports a pending listing.
var ErrStillChecking = errStillChecking

// errListingUnavailable stands in for a database failure, which the person
// can't act on.
var errListingUnavailable = errors.New("Vesper couldn't check which models your plan offers.")

// QueuedListings lists a plan login's models through a runner: it queues a
// request in harness_model_listings, which a Docker runner answers by running
// the harness CLI in a throwaway container, and can wait briefly for the answer.
// Results are cached per login, so only a new login or an old listing asks again.
//
// It reads and writes through its own pool, never a caller's transaction: a
// request made inside an uncommitted transaction would be invisible to the
// runner, and waiting would hold the transaction's locks.
type QueuedListings struct {
	Pool *pgxpool.Pool
	// Wait is how long a waiting call waits for the runner before falling
	// back to older models or an error. TTL is how long a listing stays
	// fresh, Poll how often the wait checks, FailBackoff how long a failure
	// is reported without asking again.
	Wait, TTL, Poll, FailBackoff time.Duration
	now                          func() time.Time
}

func (l *QueuedListings) timing() (wait, ttl, poll, backoff time.Duration, now func() time.Time) {
	wait, ttl, poll, backoff, now = l.Wait, l.TTL, l.Poll, l.FailBackoff, l.now
	if wait <= 0 {
		wait = 20 * time.Second
	}
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	if backoff <= 0 {
		backoff = time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return
}

type currentLoginKey struct{}

// CurrentLoginOnly makes plan listings answer only with models listed for the
// login stored now, never an earlier login's while it refreshes: Verify must
// not pass a new login on the strength of the old one.
func CurrentLoginOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, currentLoginKey{}, true)
}

func currentLoginOnly(ctx context.Context) bool { return ctx.Value(currentLoginKey{}) != nil }

// Models matches Catalog.PlanModels. Its errors are public-safe. Without wait
// it only queues the request and answers from what is cached.
func (l *QueuedListings) Models(ctx context.Context, ws uuid.UUID, harness string, wait bool) ([]Model, error) {
	waitFor, ttl, poll, backoff, now := l.timing()
	strict := currentLoginOnly(ctx)
	q := store.New(l.Pool)
	sv, err := q.SecretVersion(ctx, store.SecretVersionParams{WorkspaceID: ws, Name: KeySecret[harness]})
	if err != nil {
		slog.Warn("plan models: read login version", "workspace", ws, "harness", harness, "err", err)
		return nil, errListingUnavailable
	}
	get := store.GetModelListingParams{WorkspaceID: ws, Harness: harness}
	row, err := q.GetModelListing(ctx, get)
	has := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("plan models: read listing", "workspace", ws, "harness", harness, "err", err)
		return nil, errListingUnavailable
	}
	if has {
		if models := listedModels(row); len(models) > 0 && row.ModelsVersion != nil && *row.ModelsVersion == sv &&
			row.ListedAt != nil && now().Sub(*row.ListedAt) < ttl {
			return models, nil
		}
		if row.Status == "failed" && row.LoginVersion == sv && row.FinishedAt != nil && now().Sub(*row.FinishedAt) < backoff {
			if strict {
				return nil, failure(row.Error)
			}
			return listingFailure(row)
		}
	}
	inFlight := has && row.LoginVersion == sv && (row.Status == "pending" || row.Status == "running")
	if !inFlight {
		if err := q.RequestModelListing(ctx, store.RequestModelListingParams{WorkspaceID: ws, Harness: harness, LoginVersion: sv}); err != nil {
			slog.Warn("plan models: request listing", "workspace", ws, "harness", harness, "err", err)
			return nil, errListingUnavailable
		}
	}
	// Expired or older-login models are served at once while the runner
	// refreshes them: only a login with nothing listed yet waits.
	if models := listedModels(row); has && len(models) > 0 && (!strict || current(row, sv)) {
		return models, nil
	}
	// A request nobody has answered for longer than a wait means the runner
	// isn't there; waiting again would only repeat the delay.
	if !wait || (inFlight && now().Sub(row.RequestedAt) > waitFor) {
		return nil, errStillChecking
	}
	timeout := time.NewTimer(waitFor)
	defer timeout.Stop()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		expired := false
		select {
		case <-ctx.Done():
			if strict && !(has && current(row, sv)) {
				return nil, errStillChecking
			}
			return fallback(row, has)
		case <-timeout.C:
			expired = true // one last look: the answer may have landed between polls
		case <-tick.C:
		}
		if r, err := q.GetModelListing(ctx, get); err == nil {
			row, has = r, true
			switch {
			case r.Status == "done" && r.ModelsVersion != nil && *r.ModelsVersion == sv && len(listedModels(r)) > 0:
				return listedModels(r), nil
			case r.Status == "failed" && r.LoginVersion == sv:
				if strict {
					return nil, failure(r.Error)
				}
				return listingFailure(r)
			}
		}
		if expired {
			if strict && !(has && current(row, sv)) {
				return nil, errStillChecking
			}
			return fallback(row, has)
		}
	}
}

// A refresh failure must not hide the last successful listing. Keep the
// failure/backoff on the row so serving cached models doesn't retry in a loop.
func listingFailure(row store.HarnessModelListing) ([]Model, error) {
	if models := listedModels(row); len(models) > 0 {
		return models, nil
	}
	return nil, failure(row.Error)
}

func failure(msg string) error {
	if msg == "" {
		return errListingUnavailable
	}
	return errors.New(msg)
}

// fallback is what a wait that ran out returns: older models beat none.
func fallback(row store.HarnessModelListing, has bool) ([]Model, error) {
	if has {
		if models := listedModels(row); len(models) > 0 {
			return models, nil
		}
	}
	return nil, errStillChecking
}

func listedModels(row store.HarnessModelListing) []Model {
	var models []Model
	if json.Unmarshal(row.Models, &models) != nil {
		return nil
	}
	return models
}

// current reports whether row's models were listed for login version sv.
func current(row store.HarnessModelListing, sv string) bool {
	return row.ModelsVersion != nil && *row.ModelsVersion == sv
}
