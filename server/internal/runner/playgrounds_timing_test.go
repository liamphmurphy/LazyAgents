package runner

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// Each step is logged with how long it took once the next one begins, and
// the start as a whole once it ends, naming the step a failure happened in.
func TestStepTimerLogsEachStepAndTheWholeStart(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want []string
	}{
		{"started", nil, []string{
			`msg="playground step" step="Starting postgres" took=2s`,
			`msg="playground step" step="Running setup" took=5s`,
			`msg="playground started" took=7s`,
		}},
		{"failed", errors.New("setup exited with code 1"), []string{
			`msg="playground step" step="Starting postgres" took=2s`,
			`msg="playground step" step="Running setup" took=5s`,
			`msg="playground start failed" at="Running setup" took=7s`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			log := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey || a.Key == slog.LevelKey {
					return slog.Attr{}
				}
				return a
			}}))
			clock := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
			timer := newStepTimer(log)
			timer.now = func() time.Time { return clock }
			timer.began = clock

			timer.step("Starting postgres")
			clock = clock.Add(2 * time.Second)
			timer.step("Running setup")
			clock = clock.Add(5 * time.Second)
			timer.done(tc.err)

			got := strings.Split(strings.TrimSpace(out.String()), "\n")
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("logged\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
}
