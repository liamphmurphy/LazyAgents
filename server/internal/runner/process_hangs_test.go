package runner

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/harness"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

type interceptTransport func(*http.Request) (*http.Response, error)

func (f interceptTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDockerAttachHonorsCancellation(t *testing.T) {
	_, d := startFakeEngine(t)
	base := d.http().Transport
	reached, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	d.client.Transport = interceptTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/attach") {
			close(reached)
			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-release:
			}
		}
		return base.RoundTrip(r)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := d.Start(ctx, "test", ExecSpec{Name: "test", Argv: []string{"true"}})
		done <- err
	}()
	<-reached
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Start returned %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled Start remained blocked attaching to Docker")
	}
}

func TestDockerFiniteRequestsHaveDeadlines(t *testing.T) {
	_, d := startFakeEngine(t)
	d.RequestTimeout = 50 * time.Millisecond
	d.client = &http.Client{Transport: interceptTransport(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	for _, path := range []string{"/containers/c1/kill", "/containers/c1", "/containers/c1/json"} {
		start := time.Now()
		_, err := d.do(context.Background(), "DELETE", path, nil, "", nil)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
			t.Fatalf("unbounded Docker request %s: %v", path, err)
		}
	}
}

func TestDockerKillUnblocksLostWait(t *testing.T) {
	_, d := startFakeEngine(t)
	base := d.http().Transport
	reached := make(chan struct{})
	d.client.Transport = interceptTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/wait") {
			close(reached)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return base.RoundTrip(r)
	})
	p := &dockerProc{d: d, id: "c1"}
	defer p.Close()
	done := make(chan error, 1)
	go func() { _, err := p.Wait(); done <- err }()
	<-reached
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Kill did not release the lost wait connection")
	}
}

func TestOversizedHarnessLineFailsWithoutBlockingWriter(t *testing.T) {
	for _, tc := range []struct{ stream, command string }{
		{"stderr", "head -c 6291456 /dev/zero >&2"},
		{"stdout", "head -c 34603008 /dev/zero"},
	} {
		t.Run(tc.stream, func(t *testing.T) {
			b := &Local{Dir: t.TempDir()}
			id := uuid.NewString()
			if err := b.EnsureWorkspace(context.Background(), id); err != nil {
				t.Fatal(err)
			}
			p, err := b.Start(context.Background(), id, ExecSpec{Argv: []string{"sh", "-c", tc.command}})
			if err != nil {
				t.Fatal(err)
			}
			defer p.Kill()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := &Runner{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			done := make(chan error, 1)
			go func() {
				_, _, _, err := r.watchSession(ctx, runCtx{run: store.Run{Purpose: "chat"}}, p, harness.ClaudeCode{}.NewParser(""), nil, 0, func(string) {})
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), tc.stream) {
					t.Fatalf("oversized %s returned %v, want a stream error", tc.stream, err)
				}
			case <-time.After(2 * time.Second):
				cancel()
				<-done
				t.Fatal("oversized line stopped draining without stopping the process")
			}
		})
	}
}

type heldProcess struct {
	killed chan struct{}
	once   sync.Once
}

func (*heldProcess) Stdout() io.Reader    { return strings.NewReader("") }
func (*heldProcess) Stderr() io.Reader    { return strings.NewReader("") }
func (*heldProcess) Interrupt() error     { return nil }
func (p *heldProcess) Kill() error        { p.once.Do(func() { close(p.killed) }); return nil }
func (p *heldProcess) Wait() (int, error) { <-p.killed; return 137, nil }

func TestWatchdogStopsProcessWhileDatabaseIsBlocked(t *testing.T) {
	for _, runtime := range []bool{false, true} {
		t.Run(map[bool]string{false: "shutdown", true: "runtime"}[runtime], func(t *testing.T) {
			pool := testdb.New(t)
			tx, err := pool.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err := tx.Exec(context.Background(), "LOCK TABLE runs IN ACCESS EXCLUSIVE MODE"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := &heldProcess{killed: make(chan struct{})}
			defer p.Kill()
			r := &Runner{Pool: pool, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			done := make(chan struct{})
			remaining := time.Duration(0)
			if runtime {
				remaining = 1500 * time.Millisecond
			}
			go func() {
				r.watchSession(ctx, runCtx{run: store.Run{ID: uuid.New(), Purpose: "chat"}}, p, harness.ClaudeCode{}.NewParser(""), nil, remaining, func(string) {})
				close(done)
			}()
			deadline := time.Now().Add(4 * time.Second)
			for {
				var blocked bool
				err := pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
				 WHERE datname=current_database() AND query LIKE '%-- name: GetRun :one%'
				 AND cardinality(pg_blocking_pids(pid))>0)`).Scan(&blocked)
				if err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("watchdog did not query the database")
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !runtime {
				cancel()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				_ = tx.Rollback(context.Background())
				cancel()
				<-done
				t.Fatal("database wait blocked process cancellation")
			}
		})
	}
}
