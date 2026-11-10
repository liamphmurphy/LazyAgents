package chatmodels

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

const testOAuth = "sk-ant-oat01-abcdefghijklmnopqrstuvwxyz"

type listingEnv struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *store.Queries
	secs   *secrets.Service
	ws, me uuid.UUID
}

func newListingEnv(t *testing.T) listingEnv {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, err := secrets.NewBox(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return listingEnv{ctx: ctx, pool: pool, q: q, secs: &secrets.Service{Q: q, Box: box}, ws: me.WorkspaceID, me: me.ID}
}

func (e listingEnv) setSecret(t *testing.T, name, value string) {
	t.Helper()
	if _, err := e.secs.Set(e.ctx, e.ws, e.me, secrets.SetInput{Name: name, Value: value}); err != nil {
		t.Fatal(err)
	}
}

func (e listingEnv) version(t *testing.T) string {
	t.Helper()
	v, err := e.q.SecretVersion(e.ctx, store.SecretVersionParams{WorkspaceID: e.ws, Name: KeySecret[agentdef.HarnessClaudeCode]})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (e listingEnv) row(t *testing.T) store.HarnessModelListing {
	t.Helper()
	r, err := e.q.GetModelListing(e.ctx, store.GetModelListingParams{WorkspaceID: e.ws, Harness: agentdef.HarnessClaudeCode})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// runner plays the runner: it claims the queued request and finishes it.
func (e listingEnv) runner(t *testing.T, models []Model, failure string) store.HarnessModelListing {
	t.Helper()
	l, err := e.q.ClaimModelListing(e.ctx, &testRunner)
	if err != nil {
		t.Fatal(err)
	}
	if failure != "" {
		if _, err := e.q.FailModelListing(e.ctx, store.FailModelListingParams{ClaimID: l.ClaimID, Error: failure}); err != nil {
			t.Fatal(err)
		}
		return l
	}
	b, _ := json.Marshal(models)
	if n, err := e.q.FinishModelListing(e.ctx, store.FinishModelListingParams{ClaimID: l.ClaimID, Models: b}); err != nil || n != 1 {
		t.Fatalf("finish: %d %v", n, err)
	}
	return l
}

var testRunner = "test-runner"

func (e listingEnv) quick() *QueuedListings {
	return &QueuedListings{Pool: e.pool, Wait: 300 * time.Millisecond, Poll: 10 * time.Millisecond}
}

func TestQueuedListingsFreshHitDoesNotRequest(t *testing.T) {
	e := newListingEnv(t)
	e.setSecret(t, KeySecret[agentdef.HarnessClaudeCode], testOAuth)
	cc := agentdef.HarnessClaudeCode
	if err := e.q.RequestModelListing(e.ctx, store.RequestModelListingParams{WorkspaceID: e.ws, Harness: cc, LoginVersion: e.version(t)}); err != nil {
		t.Fatal(err)
	}
	e.runner(t, []Model{{ID: "opus", Default: true}}, "")
	before := e.row(t)
	got, err := e.quick().Models(e.ctx, e.ws, cc, true)
	if err != nil || len(got) != 1 || got[0].ID != "opus" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if after := e.row(t); after.Status != "done" || !after.RequestedAt.Equal(before.RequestedAt) {
		t.Fatalf("a fresh listing was requested again: %+v", after)
	}
}

func TestQueuedListingsRequestsAndWaitsForRunner(t *testing.T) {
	e := newListingEnv(t)
	e.setSecret(t, KeySecret[agentdef.HarnessClaudeCode], testOAuth)
	go func() {
		for range 200 {
			time.Sleep(10 * time.Millisecond)
			if l, err := e.q.ClaimModelListing(e.ctx, &testRunner); err == nil {
				b, _ := json.Marshal([]Model{{ID: "sonnet", Default: true}})
				_, _ = e.q.FinishModelListing(e.ctx, store.FinishModelListingParams{ClaimID: l.ClaimID, Models: b})
				return
			}
		}
	}()
	l := &QueuedListings{Pool: e.pool, Wait: 5 * time.Second, Poll: 10 * time.Millisecond}
	got, err := l.Models(e.ctx, e.ws, agentdef.HarnessClaudeCode, true)
	if err != nil || len(got) != 1 || got[0].ID != "sonnet" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if r := e.row(t); r.ModelsVersion == nil || *r.ModelsVersion != e.version(t) {
		t.Fatalf("models not tied to the login version: %+v", r)
	}
}

func TestQueuedListingsFailureBacksOff(t *testing.T) {
	e := newListingEnv(t)
	e.setSecret(t, KeySecret[agentdef.HarnessClaudeCode], testOAuth)
	cc := agentdef.HarnessClaudeCode
	if err := e.q.RequestModelListing(e.ctx, store.RequestModelListingParams{WorkspaceID: e.ws, Harness: cc, LoginVersion: e.version(t)}); err != nil {
		t.Fatal(err)
	}
	e.runner(t, nil, "Couldn't list models.")
	before := e.row(t)
	_, err := e.quick().Models(e.ctx, e.ws, cc, true)
	if err == nil || err.Error() != "Couldn't list models." {
		t.Fatalf("err = %v", err)
	}
	if after := e.row(t); after.Status != "failed" || !after.RequestedAt.Equal(before.RequestedAt) {
		t.Fatalf("a failure inside the backoff was requested again: %+v", after)
	}
	// Past the backoff it asks again; with nobody answering, that times out.
	l := e.quick()
	l.FailBackoff = time.Nanosecond
	time.Sleep(time.Millisecond)
	if _, err := l.Models(e.ctx, e.ws, cc, true); !errors.Is(err, errStillChecking) {
		t.Fatalf("err = %v", err)
	}
	if r := e.row(t); r.Status != "pending" {
		t.Fatalf("status %s", r.Status)
	}
}

func TestQueuedListingsNewLoginInvalidatesCache(t *testing.T) {
	e := newListingEnv(t)
	cc := agentdef.HarnessClaudeCode
	e.setSecret(t, KeySecret[cc], testOAuth)
	old := e.version(t)
	if err := e.q.RequestModelListing(e.ctx, store.RequestModelListingParams{WorkspaceID: e.ws, Harness: cc, LoginVersion: old}); err != nil {
		t.Fatal(err)
	}
	e.runner(t, []Model{{ID: "opus", Default: true}}, "")
	e.setSecret(t, KeySecret[cc], testOAuth+"2")
	if e.version(t) == old {
		t.Fatal("secret version did not move")
	}
	// Nobody answers: the old models are better than none, but a new request is queued.
	got, err := e.quick().Models(e.ctx, e.ws, cc, true)
	if err != nil || len(got) != 1 || got[0].ID != "opus" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if r := e.row(t); r.Status != "pending" || r.LoginVersion != e.version(t) {
		t.Fatalf("new login not requested: %+v", r)
	}
	// When the runner answers, the new models replace the old.
	e.runner(t, []Model{{ID: "opus-2", Default: true}}, "")
	got, err = e.quick().Models(e.ctx, e.ws, cc, true)
	if err != nil || len(got) != 1 || got[0].ID != "opus-2" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestQueuedListingsRefreshFailureKeepsModelsDuringBackoff(t *testing.T) {
	for _, newLogin := range []bool{false, true} {
		name := "expired listing"
		if newLogin {
			name = "new login"
		}
		t.Run(name, func(t *testing.T) {
			e := newListingEnv(t)
			cc := agentdef.HarnessClaudeCode
			e.setSecret(t, KeySecret[cc], testOAuth)
			if err := e.q.RequestModelListing(e.ctx, store.RequestModelListingParams{WorkspaceID: e.ws, Harness: cc, LoginVersion: e.version(t)}); err != nil {
				t.Fatal(err)
			}
			e.runner(t, []Model{{ID: "opus", Default: true}}, "")
			l := e.quick()
			if newLogin {
				e.setSecret(t, KeySecret[cc], testOAuth+"2")
			} else {
				l.TTL = time.Nanosecond
			}
			if _, err := l.Models(e.ctx, e.ws, cc, false); err != nil {
				t.Fatal(err)
			}
			e.runner(t, nil, "Temporary discovery failure.")
			before := e.row(t)
			// Repeated reads must keep models selectable without queuing another
			// request during the failure backoff.
			cat := Catalog{RequireKey: true, Secrets: e.secs, PlanModels: l.Models}
			for range 2 {
				list, err := cat.List(e.ctx, e.q, e.ws)
				if err != nil || !list[0].Available || list[0].Checking || len(list[0].Models) != 1 || list[0].Models[0].ID != "opus" {
					t.Fatalf("catalog %+v err %v", list, err)
				}
			}
			if after := e.row(t); after.Status != "failed" || !after.RequestedAt.Equal(before.RequestedAt) {
				t.Fatalf("refresh failure retried during backoff: %+v", after)
			}
			// Once backoff expires, retain the old models until the retry lands.
			l.FailBackoff = time.Nanosecond
			if _, err := l.Models(e.ctx, e.ws, cc, false); err != nil {
				t.Fatal(err)
			}
			e.runner(t, []Model{{ID: "opus-new", Default: true}}, "")
			got, err := l.Models(e.ctx, e.ws, cc, false)
			if err != nil || len(got) != 1 || got[0].ID != "opus-new" {
				t.Fatalf("recovered models %+v err %v", got, err)
			}
		})
	}
}

func TestCatalogPendingDiscoveryIsCheckingUntilRunnerFinishes(t *testing.T) {
	e := newListingEnv(t)
	cc := agentdef.HarnessClaudeCode
	e.setSecret(t, KeySecret[cc], testOAuth)
	cat := Catalog{RequireKey: true, Secrets: e.secs, PlanModels: e.quick().Models}
	list, err := cat.List(e.ctx, e.q, e.ws)
	if err != nil || list[0].Available || !list[0].Checking || len(list[0].Models) != 0 || list[1].Checking {
		t.Fatalf("pending catalog %+v err %v", list, err)
	}
	e.runner(t, []Model{{ID: "opus", Default: true}}, "")
	list, err = cat.List(e.ctx, e.q, e.ws)
	if err != nil || !list[0].Available || list[0].Checking || len(list[0].Models) != 1 {
		t.Fatalf("finished catalog %+v err %v", list, err)
	}
}

func TestQueuedListingsTimeoutWithoutModels(t *testing.T) {
	e := newListingEnv(t)
	e.setSecret(t, KeySecret[agentdef.HarnessClaudeCode], testOAuth)
	_, err := e.quick().Models(e.ctx, e.ws, agentdef.HarnessClaudeCode, true)
	if !errors.Is(err, errStillChecking) {
		t.Fatalf("err = %v", err)
	}
}

func TestQueuedListingsStaleTTLReturnsModelsOnTimeout(t *testing.T) {
	e := newListingEnv(t)
	cc := agentdef.HarnessClaudeCode
	e.setSecret(t, KeySecret[cc], testOAuth)
	if err := e.q.RequestModelListing(e.ctx, store.RequestModelListingParams{WorkspaceID: e.ws, Harness: cc, LoginVersion: e.version(t)}); err != nil {
		t.Fatal(err)
	}
	e.runner(t, []Model{{ID: "opus", Default: true}}, "")
	l := e.quick()
	l.TTL = time.Nanosecond
	time.Sleep(time.Millisecond)
	got, err := l.Models(e.ctx, e.ws, cc, true)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if r := e.row(t); r.Status != "pending" {
		t.Fatalf("an expired listing was not requested again: %s", r.Status)
	}
}

func TestCatalogPlanLoginUsesPlanModels(t *testing.T) {
	e := newListingEnv(t)
	cc := agentdef.HarnessClaudeCode
	e.setSecret(t, KeySecret[cc], testOAuth)
	var asked []string
	cat := Catalog{RequireKey: true, Secrets: e.secs,
		PlanModels: func(_ context.Context, ws uuid.UUID, h string, _ bool) ([]Model, error) {
			asked = append(asked, h)
			return nil, errors.New("Still checking.")
		}}
	// With Discover set it wins (tests rely on that override).
	cat.Discover = func(_ context.Context, h, _ string) ([]Model, error) { return []Model{{ID: h, Default: true}}, nil }
	if list, err := cat.List(e.ctx, e.q, e.ws); err != nil || len(asked) != 0 || !list[0].Available {
		t.Fatalf("PlanModels used despite Discover: %v %v", asked, err)
	}
	cat.Discover = nil
	list, err := cat.List(e.ctx, e.q, e.ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != cc {
		t.Fatalf("asked %v", asked)
	}
	if h := list[0]; h.Available || h.UnavailableReason != "Still checking." {
		t.Fatalf("claude = %+v", h)
	}
	// No Codex secret: unavailable without asking a runner.
	if h := list[1]; h.Available || h.UnavailableReason != "no harness:openai secret" {
		t.Fatalf("codex = %+v", h)
	}
}

func TestCatalogPlanModelsListsModels(t *testing.T) {
	e := newListingEnv(t)
	e.setSecret(t, KeySecret[agentdef.HarnessClaudeCode], testOAuth)
	cat := Catalog{RequireKey: true, Secrets: e.secs,
		PlanModels: func(context.Context, uuid.UUID, string, bool) ([]Model, error) {
			return []Model{{ID: "opus", Default: true}}, nil
		}}
	list, err := cat.List(e.ctx, e.q, e.ws)
	if err != nil {
		t.Fatal(err)
	}
	if h := list[0]; !h.Available || len(h.Models) != 1 || h.Models[0].ID != "opus" {
		t.Fatalf("claude = %+v", h)
	}
}

func TestCatalogLocalIgnoresPlanModels(t *testing.T) {
	// No Discover: the local backend lists through the CLI binaries, which
	// don't exist here, and must never ask a runner.
	t.Setenv("RUNNER_CLAUDE_BIN", "/nonexistent/claude")
	t.Setenv("RUNNER_CODEX_BIN", "/nonexistent/codex")
	cat := Catalog{
		RequireKey: false,
		PlanModels: func(context.Context, uuid.UUID, string, bool) ([]Model, error) {
			t.Error("PlanModels used by the local backend")
			return nil, nil
		}}
	list, err := cat.List(context.Background(), nil, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range list {
		if h.Available {
			t.Fatalf("%s unexpectedly available: %+v", h.Harness, h)
		}
	}
}

func TestQueuedListingsFreshHitSurvivesRevealAndSetInvalidates(t *testing.T) {
	e := newListingEnv(t)
	cc := agentdef.HarnessClaudeCode
	e.setSecret(t, KeySecret[cc], testOAuth)
	if err := e.q.RequestModelListing(e.ctx, store.RequestModelListingParams{WorkspaceID: e.ws, Harness: cc, LoginVersion: e.version(t)}); err != nil {
		t.Fatal(err)
	}
	e.runner(t, []Model{{ID: "opus", Default: true}}, "")
	before := e.row(t)
	// Reveal bumps last_used_at (and updated_at) but must not invalidate.
	if _, err := e.secs.Reveal(e.ctx, e.ws, nil, KeySecret[cc]); err != nil {
		t.Fatal(err)
	}
	if got, err := e.quick().Models(e.ctx, e.ws, cc, true); err != nil || len(got) != 1 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if after := e.row(t); after.Status != "done" || !after.RequestedAt.Equal(before.RequestedAt) {
		t.Fatalf("a reveal invalidated the listing: %+v", after)
	}
	// Re-saving, even the same value, is a new version.
	e.setSecret(t, KeySecret[cc], testOAuth)
	if _, err := e.quick().Models(e.ctx, e.ws, cc, false); err != nil {
		t.Fatal(err)
	}
	if after := e.row(t); after.Status != "pending" || after.LoginVersion != e.version(t) {
		t.Fatalf("a new value was not requested: %+v", after)
	}
}

func TestRequestModelListingKeepsInFlightRow(t *testing.T) {
	e := newListingEnv(t)
	cc := agentdef.HarnessClaudeCode
	e.setSecret(t, KeySecret[cc], testOAuth)
	req := store.RequestModelListingParams{WorkspaceID: e.ws, Harness: cc, LoginVersion: e.version(t)}
	if err := e.q.RequestModelListing(e.ctx, req); err != nil {
		t.Fatal(err)
	}
	first, err := e.q.ClaimModelListing(e.ctx, &testRunner)
	if err != nil || first.ClaimID == nil {
		t.Fatalf("claim %+v %v", first, err)
	}
	if err := e.q.RequestModelListing(e.ctx, req); err != nil {
		t.Fatal(err)
	}
	if r := e.row(t); r.Status != "running" || r.ClaimID == nil || *r.ClaimID != *first.ClaimID {
		t.Fatalf("duplicate request disturbed the claim: %+v", r)
	}
	// The claim goes quiet and is reclaimed: the old claim can no longer settle it.
	if _, err := e.pool.Exec(e.ctx, "UPDATE harness_model_listings SET claimed_at = now() - interval '3 minutes'"); err != nil {
		t.Fatal(err)
	}
	second, err := e.q.ClaimModelListing(e.ctx, &testRunner)
	if err != nil || *second.ClaimID == *first.ClaimID {
		t.Fatalf("reclaim %+v %v", second, err)
	}
	if n, err := e.q.FinishModelListing(e.ctx, store.FinishModelListingParams{ClaimID: first.ClaimID, Models: []byte(`[]`)}); err != nil || n != 0 {
		t.Fatalf("old claim finished: %d %v", n, err)
	}
	if n, err := e.q.FailModelListing(e.ctx, store.FailModelListingParams{ClaimID: first.ClaimID, Error: "x"}); err != nil || n != 0 {
		t.Fatalf("old claim failed: %d %v", n, err)
	}
	if n, err := e.q.FinishModelListing(e.ctx, store.FinishModelListingParams{ClaimID: second.ClaimID, Models: []byte(`[{"id":"opus"}]`)}); err != nil || n != 1 {
		t.Fatalf("new claim finish: %d %v", n, err)
	}
}

func TestQueuedListingsRequestsOutsideCallersTransaction(t *testing.T) {
	e := newListingEnv(t)
	cc := agentdef.HarnessClaudeCode
	e.setSecret(t, KeySecret[cc], testOAuth)
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(e.ctx)
	cat := Catalog{RequireKey: true, Secrets: e.secs, PlanModels: e.quick().Models}
	start := time.Now()
	list, err := cat.List(e.ctx, store.New(tx), e.ws)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 200*time.Millisecond || list[0].Available {
		t.Fatalf("List waited or found models: %v %+v", time.Since(start), list[0])
	}
	// Visible to another connection while the transaction is still open.
	if r := e.row(t); r.Status != "pending" {
		t.Fatalf("row %+v", r)
	}
	// Resolve goes through the same path.
	if _, err := cat.Resolve(e.ctx, store.New(tx), e.ws, Selection{}, Selection{Harness: cc}); err == nil {
		t.Fatal("resolve should not find a model yet")
	}
}

func TestQueuedListingsStalePendingDoesNotWait(t *testing.T) {
	e := newListingEnv(t)
	cc := agentdef.HarnessClaudeCode
	e.setSecret(t, KeySecret[cc], testOAuth)
	l := &QueuedListings{Pool: e.pool, Wait: 2 * time.Second, Poll: 10 * time.Millisecond}
	if err := e.q.RequestModelListing(e.ctx, store.RequestModelListingParams{WorkspaceID: e.ws, Harness: cc, LoginVersion: e.version(t)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, "UPDATE harness_model_listings SET requested_at = now() - interval '1 minute'"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := l.Models(e.ctx, e.ws, cc, true); !errors.Is(err, errStillChecking) {
		t.Fatalf("err = %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("waited %v on a request nobody answers", d)
	}
}
