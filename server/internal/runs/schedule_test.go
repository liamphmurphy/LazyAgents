package runs

import (
	"testing"
	"time"
)

func TestScheduleNext(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, la)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct {
		name string
		s    Schedule
		from string
		want string
	}{
		{"daily later today", Schedule{Frequency: "daily", AtTime: "17:00"}, "2026-11-08 09:00", "2026-11-08 17:00"},
		{"daily already passed", Schedule{Frequency: "daily", AtTime: "08:00"}, "2026-11-08 09:00", "2026-11-09 08:00"},
		{"exact time is not after", Schedule{Frequency: "daily", AtTime: "09:00"}, "2026-11-08 09:00", "2026-11-09 09:00"},
		{"weekly monday", Schedule{Frequency: "weekly", Weekdays: []int32{1}, AtTime: "08:00"}, "2026-11-08 09:00", "2026-11-09 08:00"},
		{"monthly clamps to last day", Schedule{Frequency: "monthly", MonthDay: 31, AtTime: "09:00"}, "2026-11-08 09:00", "2026-11-30 09:00"},
		{"keeps wall clock across dst", Schedule{Frequency: "daily", AtTime: "08:00"}, "2026-10-31 09:00", "2026-11-01 08:00"},
		{"cron weekdays", Schedule{Frequency: "cron", Cron: "30 18 * * 1-5"}, "2026-11-07 09:00", "2026-11-09 18:30"},
		{"cron step", Schedule{Frequency: "cron", Cron: "*/15 * * * *"}, "2026-11-08 09:01", "2026-11-08 09:15"},
		{"cron sunday as 7", Schedule{Frequency: "cron", Cron: "0 9 * * 7"}, "2026-11-09 09:00", "2026-11-15 09:00"},
	}
	for _, c := range cases {
		c.s.Timezone = "America/Los_Angeles"
		got, err := c.s.Next(at(c.from))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !got.Equal(at(c.want)) {
			t.Errorf("%s: got %s, want %s", c.name, got.In(la), c.want)
		}
	}
	for _, bad := range []Schedule{
		{Frequency: "cron", Cron: "* * *", Timezone: "UTC"},
		{Frequency: "cron", Cron: "61 * * * *", Timezone: "UTC"},
		{Frequency: "weekly", AtTime: "08:00", Timezone: "UTC"},
		{Frequency: "daily", AtTime: "25:00", Timezone: "UTC"},
		{Frequency: "daily", AtTime: "08:00", Timezone: "Mars/Base"},
		{Frequency: "cron", Cron: "0 0 31 2 *", Timezone: "UTC"},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v should be invalid", bad)
		}
	}
}
