package httpapi

import (
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/usage"
)

func TestBuildHarnessActivity(t *testing.T) {
	at := time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC)
	resets := at.Add(3 * time.Hour)
	running := []store.ActiveHarnessRunsRow{{Harness: "claude-code", Runs: 2}}
	limits := []store.ActiveProviderLimitsRow{{Harness: "codex", Until: resets}}
	readings := map[string]usage.Snapshot{
		"claude-code": {Harness: "claude-code", Plan: "max", Available: true, ObservedAt: at,
			Windows: []usage.Window{{ID: "five_hour", Kind: usage.Session, Label: "Session", UsedPercent: 38.44, ResetsAt: &resets}}},
		"codex": {Harness: "codex", Available: false, ObservedAt: at, Windows: []usage.Window{}},
	}
	got := buildHarnessActivity(running, limits, readings)
	if len(got) != 2 || got[0].Harness != "claude-code" || got[1].Harness != "codex" {
		t.Fatalf("order = %+v", got)
	}
	c := got[0]
	if c.Label != "Claude" || c.Running != 2 || c.LimitedUntil != nil || !c.UsageAvailable || c.Plan == nil || *c.Plan != "max" {
		t.Fatalf("claude = %+v", c)
	}
	if len(c.Windows) != 1 || c.Windows[0].UsedPercent != 38.4 || string(c.Windows[0].Kind) != "session" || !c.Windows[0].ResetsAt.Equal(resets) {
		t.Fatalf("claude windows = %+v", c.Windows)
	}
	x := got[1]
	if x.Running != 0 || x.LimitedUntil == nil || !x.LimitedUntil.Equal(resets) || x.UsageAvailable || x.UsageCheckedAt == nil || len(x.Windows) != 0 {
		t.Fatalf("codex = %+v", x)
	}
}

func TestBuildHarnessActivityNoReadings(t *testing.T) {
	got := buildHarnessActivity(nil, nil, map[string]usage.Snapshot{})
	for _, a := range got {
		if a.UsageAvailable || a.UsageCheckedAt != nil || a.Windows == nil {
			t.Fatalf("%s = %+v", a.Harness, a)
		}
	}
}
