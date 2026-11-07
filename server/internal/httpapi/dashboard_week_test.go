package httpapi

import (
	"testing"
	"time"
)

func TestWeekStartIsMondayUTC(t *testing.T) {
	monday := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	next := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		at, want time.Time
	}{
		{monday, monday},
		{time.Date(2026, 11, 4, 15, 30, 0, 0, time.UTC), monday},                 // Wednesday
		{time.Date(2026, 11, 8, 23, 59, 0, 0, time.UTC), monday},                 // Sunday
		{time.Date(2026, 11, 8, 18, 0, 0, 0, time.FixedZone("", -8*3600)), next}, // Monday 02:00 UTC
	} {
		if got := weekStart(c.at); !got.Equal(c.want) {
			t.Errorf("weekStart(%s) = %s, want %s", c.at, got, c.want)
		}
	}
}
