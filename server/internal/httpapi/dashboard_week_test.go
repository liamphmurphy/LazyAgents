package httpapi

import (
	"testing"
	"time"
)

func TestWeekStartIsMondayUTC(t *testing.T) {
	want := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC) // Monday
	for _, at := range []time.Time{
		want,
		time.Date(2026, 11, 4, 15, 30, 0, 0, time.UTC),                   // Wednesday
		time.Date(2026, 11, 8, 23, 59, 0, 0, time.UTC),                   // Sunday
		time.Date(2026, 11, 8, 18, 0, 0, 0, time.FixedZone("", -8*3600)), // Monday 02:00 UTC
	} {
		got := weekStart(at)
		if at.UTC().Weekday() == time.Monday && at.UTC().Day() == 9 {
			want = time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
		}
		if !got.Equal(want) {
			t.Errorf("weekStart(%s) = %s, want %s", at, got, want)
		}
	}
}
