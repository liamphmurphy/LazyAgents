package runner_test

import (
	"context"
	"maps"
	"strings"
	"sync"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type commitBackend struct {
	runner.Backend
	mu    sync.Mutex
	turns []map[string]string
}

func (b *commitBackend) Start(ctx context.Context, id string, spec runner.ExecSpec) (runner.Process, error) {
	if strings.HasPrefix(spec.Name, "lazyagents-turn-") {
		b.mu.Lock()
		b.turns = append(b.turns, maps.Clone(spec.Env))
		b.mu.Unlock()
	}
	return b.Backend.Start(ctx, id, spec)
}

func TestTicketTurnsUseCurrentDispatcherCommitIdentity(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "dispatcher", true: "legacy local user"}[legacy], func(t *testing.T) {
			e := setup(t)
			b := &commitBackend{Backend: e.runner.Backend}
			e.runner.Backend = b
			t.Setenv("FAKE_HARNESS_MODE", "idle")
			t.Setenv("FAKE_HARNESS_RESUME", "idle")
			e.dispatch("PAY-224")
			run := e.run("PAY-224")
			owner := e.me
			if legacy {
				if err := e.q.SetRunDispatcher(e.ctx, store.SetRunDispatcherParams{ID: run.ID}); err != nil {
					t.Fatal(err)
				}
			} else {
				// The dispatcher can be someone other than the local user.
				users, err := e.q.ListUsers(e.ctx, e.ws)
				if err != nil {
					t.Fatal(err)
				}
				for _, user := range users {
					if user.ID != e.me.ID {
						owner = user
						break
					}
				}
				if owner.ID == e.me.ID {
					t.Fatal("fixture needs a second user")
				}
				if err := e.q.SetRunDispatcher(e.ctx, store.SetRunDispatcherParams{ID: run.ID, DispatchedByUserID: &owner.ID}); err != nil {
					t.Fatal(err)
				}
			}
			e.until("waiting for reply", e.state("PAY-224=waiting"))
			check := func(want string, turns []map[string]string) {
				t.Helper()
				if len(turns) == 0 {
					t.Fatal("no harness turns")
				}
				for _, env := range turns {
					for key, value := range map[string]string{"GIT_AUTHOR_NAME": owner.Name, "GIT_AUTHOR_EMAIL": want, "GIT_COMMITTER_NAME": owner.Name, "GIT_COMMITTER_EMAIL": want} {
						if env[key] != value {
							t.Fatalf("%s = %q, want %q", key, env[key], value)
						}
					}
				}
			}
			check(owner.Email, b.turns)
			before := len(b.turns)
			if _, err := e.pool.Exec(e.ctx, "UPDATE users SET email='updated@example.com' WHERE id=$1", owner.ID); err != nil {
				t.Fatal(err)
			}
			t.Setenv("FAKE_HARNESS_RESUME", "")
			if err := e.runs.OnHumanComment(e.ctx, e.ws, "PAY-224", "@atlas please wrap up"); err != nil {
				t.Fatal(err)
			}
			e.until("completed", e.state("PAY-224=completed"))
			check("updated@example.com", b.turns[before:])
		})
	}
}
