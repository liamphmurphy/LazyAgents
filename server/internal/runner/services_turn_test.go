package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/runner"
)

// svcBackend is the local backend plus fake services, recording what the
// turn was given.
type svcBackend struct {
	*runner.Local
	fail     error
	mu       sync.Mutex
	started  []string
	stopped  int
	turnEnv  map[string]string
	turnNets []string
}

func (b *svcBackend) StartServices(_ context.Context, runID, key string, services []agentdef.Service) (string, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fail != nil {
		return "", nil, b.fail
	}
	b.started = append(b.started, key)
	return "net-" + key, func() { b.mu.Lock(); b.stopped++; b.mu.Unlock() }, nil
}

func (b *svcBackend) Start(ctx context.Context, runID string, spec runner.ExecSpec) (runner.Process, error) {
	if strings.HasPrefix(spec.Name, "lazyagents-turn-") {
		b.mu.Lock()
		b.turnEnv, b.turnNets = spec.Env, spec.Networks
		b.mu.Unlock()
	}
	return b.Local.Start(ctx, runID, spec)
}

func withServices(e env, key string) {
	e.t.Helper()
	svc, _ := json.Marshal([]agentdef.Service{{Name: "postgres", Image: "postgres:17-alpine",
		Expose: map[string]string{"TEST_DATABASE_URL": "postgres://postgres@postgres:5432/postgres"}}})
	if _, err := e.pool.Exec(e.ctx, `UPDATE runs SET resolved_spec = jsonb_set(resolved_spec, '{services}', $2::jsonb) WHERE id = $1`, e.run(key).ID, svc); err != nil {
		e.t.Fatal(err)
	}
}

// A turn runs with its services up, sees what they expose, joins their
// network, and they are stopped when it ends.
func TestTurnRunsWithItsServices(t *testing.T) {
	e := setup(t)
	b := &svcBackend{Local: e.runner.Backend.(*runner.Local)}
	e.runner.Backend = b
	e.dispatch("PAY-214")
	withServices(e, "PAY-214")
	e.until("waiting for approval", e.state("PAY-214=waiting"))
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.started) != 1 || b.stopped != 1 {
		t.Fatalf("services started %v, stopped %d times", b.started, b.stopped)
	}
	if b.turnEnv["TEST_DATABASE_URL"] != "postgres://postgres@postgres:5432/postgres" || len(b.turnNets) != 1 || b.turnNets[0] != "net-"+b.started[0] {
		t.Fatalf("turn env %q networks %v", b.turnEnv["TEST_DATABASE_URL"], b.turnNets)
	}
	if evs := e.eventTypes(e.run("PAY-214").ID); !strings.Contains(evs, "lifecycle:services_ready") {
		t.Fatalf("no services_ready event: %s", evs)
	}
}

// A service that can't start fails the run, saying which and why.
func TestTurnFailsWhenItsServicesDont(t *testing.T) {
	e := setup(t)
	b := &svcBackend{Local: e.runner.Backend.(*runner.Local), fail: errors.New("service postgres: exited with code 1")}
	e.runner.Backend = b
	e.dispatch("PAY-214")
	withServices(e, "PAY-214")
	e.until("failed", e.state("PAY-214=failed"))
	if r := e.run("PAY-214"); r.Outcome == nil || !strings.Contains(*r.Outcome, "Couldn't start the run's services: service postgres") {
		t.Fatalf("outcome %v", r.Outcome)
	}
}

// The local runner can't start services: the turn runs anyway and says so.
func TestLocalRunnerSkipsServices(t *testing.T) {
	e := setup(t)
	e.dispatch("PAY-214")
	withServices(e, "PAY-214")
	e.until("waiting for approval", e.state("PAY-214=waiting"))
	if evs := e.eventTypes(e.run("PAY-214").ID); !strings.Contains(evs, "lifecycle:services_skipped") {
		t.Fatalf("no services_skipped event: %s", evs)
	}
}
