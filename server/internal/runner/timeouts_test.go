package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func (f *fakeEngine) count(call string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == call {
			n++
		}
	}
	return n
}

// A dropped /wait connection must not remove a container that is still
// running: Wait re-attaches and reports the real exit.
func TestDockerWaitRetriesADroppedConnection(t *testing.T) {
	old := waitBackoff
	waitBackoff = time.Millisecond
	t.Cleanup(func() { waitBackoff = old })
	f, d := startFakeEngine(t)
	f.dropWaits, f.waitCode = 2, 3
	close(f.exited)
	code, err := (&dockerProc{d: d, id: "c1"}).Wait()
	if code != 3 || err != nil {
		t.Fatalf("Wait = %d, %v; want 3, nil", code, err)
	}
	if n := f.count("POST /containers/c1/wait"); n != 3 {
		t.Fatalf("wait called %d times, want 3", n)
	}
	if n := f.count("DELETE /containers/c1"); n != 1 {
		t.Fatalf("container removed %d times, want once, after it exited", n)
	}
}

// An OOM kill names the memory limit instead of a bare exit 137.
func TestDockerWaitReportsOOMKill(t *testing.T) {
	f, d := startFakeEngine(t)
	f.waitCode, f.oom = 137, true
	close(f.exited)
	code, err := (&dockerProc{d: d, id: "c1"}).Wait()
	if code != 137 || err == nil || !strings.Contains(err.Error(), "out of memory") {
		t.Fatalf("Wait = %d, %v; want 137 and an out-of-memory error", code, err)
	}
}

// A housekeeping script that hangs (a checkpoint push on a dead network) is
// killed at ScriptTimeout instead of holding the sweep or a run's start.
func TestRunScriptTimesOut(t *testing.T) {
	old := ScriptTimeout
	ScriptTimeout = 200 * time.Millisecond
	t.Cleanup(func() { ScriptTimeout = old })
	r := &Runner{Backend: &Local{Dir: t.TempDir()}}
	start := time.Now()
	_, err := r.runScript(context.Background(), uuid.New(), "hang", "sleep 30", nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("runScript err = %v, want a timeout", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("runScript took %s", d)
	}
}
