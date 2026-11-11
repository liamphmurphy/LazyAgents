package usage_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/usage"
)

type queuedEnv struct {
	ctx context.Context
	q   *store.Queries
	svc *usage.Service
	ws  uuid.UUID
	set func(name, value string)
}

func newQueuedEnv(t *testing.T) queuedEnv {
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
	sec := &secrets.Service{Q: q, Box: box}
	return queuedEnv{ctx: ctx, q: q, svc: &usage.Service{Q: q, Queued: true}, ws: me.WorkspaceID, set: func(name, value string) {
		if _, err := sec.Set(ctx, me.WorkspaceID, me.ID, secrets.SetInput{Name: name, Value: value}); err != nil {
			t.Fatal(err)
		}
	}}
}

func (e queuedEnv) refresh(t *testing.T) {
	t.Helper()
	readings, err := e.svc.Readings(e.ctx, e.ws)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Refresh(e.ctx, e.ws, readings)
}

func (e queuedEnv) checking(t *testing.T) map[string]bool {
	t.Helper()
	c, err := e.svc.Checking(e.ctx, e.ws)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// answer plays a runner: claims the queued check and settles it.
func (e queuedEnv) answer(t *testing.T, snap usage.Snapshot, err error) store.HarnessUsageCheck {
	t.Helper()
	runner := "r1"
	c, cerr := e.q.ClaimUsageCheck(e.ctx, &runner)
	if cerr != nil {
		t.Fatalf("claim: %v", cerr)
	}
	wait := e.svc.Settle(e.ctx, e.ws, c.Harness, snap, err)
	if n, ferr := e.q.FinishUsageCheck(e.ctx, store.FinishUsageCheckParams{ClaimID: c.ClaimID, RetryAfter: time.Now().Add(wait)}); ferr != nil || n != 1 {
		t.Fatalf("finish: %d %v", n, ferr)
	}
	return c
}

func TestQueuedRefreshAsksARunnerForStaleReadings(t *testing.T) {
	e := newQueuedEnv(t)
	e.set("harness:anthropic", "sk-ant-oat01-abcdefghijklmnopqrstuvwxyz")
	e.refresh(t)

	// Codex has no login: no limits to read, said at once rather than queued.
	readings, err := e.svc.Readings(e.ctx, e.ws)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := readings[agentdef.HarnessCodex]; !ok || r.Available {
		t.Fatalf("codex reading %+v (present %v)", r, ok)
	}
	if c := e.checking(t); !c[agentdef.HarnessClaudeCode] || c[agentdef.HarnessCodex] {
		t.Fatalf("checking %v", c)
	}

	// Polls while the check is in flight don't queue it again.
	runner := "r1"
	c, err := e.q.ClaimUsageCheck(e.ctx, &runner)
	if err != nil {
		t.Fatal(err)
	}
	e.refresh(t)
	if _, err := e.q.ClaimUsageCheck(e.ctx, &runner); err == nil {
		t.Fatal("a poll re-queued the check in flight")
	}
	snap := usage.Snapshot{Harness: agentdef.HarnessClaudeCode, Plan: "max", Available: true, ObservedAt: time.Now(),
		Windows: []usage.Window{{ID: "five_hour", Kind: usage.Session, Label: "Session", UsedPercent: 42}}}
	wait := e.svc.Settle(e.ctx, e.ws, c.Harness, snap, nil)
	if n, err := e.q.FinishUsageCheck(e.ctx, store.FinishUsageCheckParams{ClaimID: c.ClaimID, RetryAfter: time.Now().Add(wait)}); err != nil || n != 1 {
		t.Fatalf("finish: %d %v", n, err)
	}
	if c := e.checking(t); c[agentdef.HarnessClaudeCode] {
		t.Fatalf("still checking after the answer: %v", c)
	}
	readings, _ = e.svc.Readings(e.ctx, e.ws)
	if r := readings[agentdef.HarnessClaudeCode]; !r.Available || r.Plan != "max" || len(r.Windows) != 1 || r.Windows[0].UsedPercent != 42 {
		t.Fatalf("claude reading %+v", r)
	}

	// A fresh reading isn't checked again.
	e.refresh(t)
	if c := e.checking(t); c[agentdef.HarnessClaudeCode] {
		t.Fatalf("checked a fresh reading: %v", c)
	}
}

func TestQueuedRefreshBacksOffUntilTheLoginChanges(t *testing.T) {
	e := newQueuedEnv(t)
	e.set("harness:anthropic", "sk-ant-oat01-abcdefghijklmnopqrstuvwxyz")
	e.refresh(t)
	e.answer(t, usage.Snapshot{}, usage.ErrSignedOut)

	// The signed-out reading goes stale, but the check is backing off.
	e.svc.Now = func() time.Time { return time.Now().Add(usage.QueuedFresh + time.Minute) }
	e.refresh(t)
	if c := e.checking(t); c[agentdef.HarnessClaudeCode] {
		t.Fatalf("queued during back-off: %v", c)
	}

	// Signing in again is checked at once.
	e.set("harness:anthropic", "sk-ant-oat01-zyxwvutsrqponmlkjihgfedcba")
	e.refresh(t)
	if c := e.checking(t); !c[agentdef.HarnessClaudeCode] {
		t.Fatalf("new login not checked: %v", c)
	}
}

func TestFinishUsageCheckNeedsTheCurrentClaim(t *testing.T) {
	e := newQueuedEnv(t)
	e.set("harness:anthropic", "sk-ant-oat01-abcdefghijklmnopqrstuvwxyz")
	e.refresh(t)
	runner := "r1"
	stale, err := e.q.ClaimUsageCheck(e.ctx, &runner)
	if err != nil {
		t.Fatal(err)
	}
	// A new login re-queues the check under the stale claim's feet.
	e.set("harness:anthropic", "sk-ant-oat01-zyxwvutsrqponmlkjihgfedcba")
	e.refresh(t)
	if n, err := e.q.FinishUsageCheck(e.ctx, store.FinishUsageCheckParams{ClaimID: stale.ClaimID, RetryAfter: time.Now()}); err != nil || n != 0 {
		t.Fatalf("stale claim finished %d rows, err %v", n, err)
	}
	if c := e.checking(t); !c[agentdef.HarnessClaudeCode] {
		t.Fatalf("re-queued check lost: %v", c)
	}
}

func TestLocalRefreshIgnoresTheQueue(t *testing.T) {
	e := newQueuedEnv(t)
	e.set("harness:anthropic", "sk-ant-oat01-abcdefghijklmnopqrstuvwxyz")
	probed := make(chan string, 2)
	e.svc.Queued = false
	e.svc.Probe = func(_ context.Context, h string) (usage.Snapshot, error) {
		probed <- h
		return usage.Snapshot{}, usage.ErrOutdated
	}
	e.refresh(t)
	for range usage.Harnesses {
		<-probed
	}
	runner := "r1"
	if _, err := e.q.ClaimUsageCheck(e.ctx, &runner); err == nil {
		t.Fatal("the fake backend queued a check")
	}
}
