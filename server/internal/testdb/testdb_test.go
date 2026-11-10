package testdb

import (
	"fmt"
	"runtime"
	"testing"
)

// recorder stands in for a test so New's skip or failure can be observed.
type recorder struct {
	testing.TB
	skipped, failed string
}

func (r *recorder) Helper() {}

func (r *recorder) Skip(args ...any) {
	r.skipped = fmt.Sprint(args...)
	runtime.Goexit()
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.failed = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func callNew(r *recorder) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		New(r)
	}()
	<-done
}

func TestNewWithoutDatabase(t *testing.T) {
	for _, tc := range []struct {
		name, runID, ci string
		wantFail        bool
	}{
		{name: "locally it skips"},
		{name: "in a run it fails", runID: "01a1c6", wantFail: true},
		{name: "on CI it fails", ci: "true", wantFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_DATABASE_URL", "")
			t.Setenv("LAZYAGENTS_RUN_ID", tc.runID)
			t.Setenv("CI", tc.ci)
			r := &recorder{TB: t}
			callNew(r)
			if tc.wantFail && (r.failed == "" || r.skipped != "") {
				t.Fatalf("want a failure, got skipped=%q failed=%q", r.skipped, r.failed)
			}
			if !tc.wantFail && (r.skipped == "" || r.failed != "") {
				t.Fatalf("want a skip, got skipped=%q failed=%q", r.skipped, r.failed)
			}
		})
	}
}
