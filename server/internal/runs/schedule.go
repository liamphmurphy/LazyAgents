package runs

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// Schedule says when a task runs, in its own time zone, so 8:00 AM stays
// 8:00 AM across daylight saving changes.
type Schedule struct {
	Frequency string
	Weekdays  []int32 // 0 is Sunday; weekly
	MonthDay  int32   // monthly; shorter months use their last day
	AtTime    string  // HH:MM
	Timezone  string
	Cron      string
}

func (s Schedule) location() (*time.Location, error) {
	if s.Timezone == "" {
		return nil, apperr.Invalid("timezone", "choose a time zone")
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return nil, apperr.Invalid("timezone", "unknown time zone")
	}
	return loc, nil
}

func clockOf(at string) (int, int, error) {
	t, err := time.Parse("15:04", at)
	if err != nil {
		return 0, 0, apperr.Invalid("at_time", "use a 24-hour time like 08:30")
	}
	return t.Hour(), t.Minute(), nil
}

// Validate reports a schedule that could never run.
func (s Schedule) Validate() error {
	_, err := s.Next(time.Now())
	return err
}

// Next is the first run strictly after after.
func (s Schedule) Next(after time.Time) (time.Time, error) {
	loc, err := s.location()
	if err != nil {
		return time.Time{}, err
	}
	local := after.In(loc)
	if s.Frequency == "cron" {
		return nextCron(s.Cron, local)
	}
	hour, minute, err := clockOf(s.AtTime)
	if err != nil {
		return time.Time{}, err
	}
	if s.Frequency == "weekly" && len(s.Weekdays) == 0 {
		return time.Time{}, apperr.Invalid("weekdays", "pick at least one day")
	}
	if s.Frequency == "monthly" && (s.MonthDay < 1 || s.MonthDay > 31) {
		return time.Time{}, apperr.Invalid("month_day", "must be 1–31")
	}
	for i := range 400 {
		day := time.Date(local.Year(), local.Month(), local.Day()+i, hour, minute, 0, 0, loc)
		if !day.After(after) {
			continue
		}
		switch s.Frequency {
		case "daily":
			return day, nil
		case "weekly":
			if slices.Contains(s.Weekdays, int32(day.Weekday())) {
				return day, nil
			}
		case "monthly":
			last := time.Date(day.Year(), day.Month()+1, 0, 0, 0, 0, 0, loc).Day()
			if day.Day() == min(int(s.MonthDay), last) {
				return day, nil
			}
		default:
			return time.Time{}, apperr.Invalid("frequency", "must be daily, weekly, monthly or cron")
		}
	}
	return time.Time{}, apperr.Invalid("frequency", "this schedule never runs")
}

// cronField parses one field of a five-field expression into its allowed values.
func cronField(field string, low, high int) (map[int]bool, error) {
	out := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		rng, stepText, stepped := strings.Cut(part, "/")
		step := 1
		if stepped {
			n, err := strconv.Atoi(stepText)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("bad step in %q", part)
			}
			step = n
		}
		from, to := low, high
		if rng != "*" {
			a, b, isRange := strings.Cut(rng, "-")
			var err error
			if from, err = strconv.Atoi(a); err != nil {
				return nil, fmt.Errorf("bad value %q", part)
			}
			to = from
			if isRange {
				if to, err = strconv.Atoi(b); err != nil {
					return nil, fmt.Errorf("bad value %q", part)
				}
			} else if stepped {
				to = high
			}
		}
		if from < low || to > high || from > to {
			return nil, fmt.Errorf("%q is out of range", part)
		}
		for v := from; v <= to; v += step {
			out[v] = true
		}
	}
	return out, nil
}

func nextCron(expr string, after time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, apperr.Invalid("cron", "use five fields: minute hour day month weekday")
	}
	ranges := [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}
	var sets [5]map[int]bool
	for i, f := range fields {
		set, err := cronField(f, ranges[i][0], ranges[i][1])
		if err != nil {
			return time.Time{}, apperr.Invalid("cron", "%s", err.Error())
		}
		sets[i] = set
	}
	if sets[4][7] {
		sets[4][0] = true
	}
	// Classic cron: when both day fields are restricted, either may match.
	domAny, dowAny := fields[2] == "*", fields[4] == "*"
	// Walking days first keeps a rare expression cheap.
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.AddDate(2, 0, 1)
	for t.Before(limit) {
		dom, dow := sets[2][t.Day()], sets[4][int(t.Weekday())]
		match := (dom && dow) || (!domAny && !dowAny && (dom || dow))
		if !sets[3][int(t.Month())] || !match {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if sets[1][t.Hour()] && sets[0][t.Minute()] {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, apperr.Invalid("cron", "this schedule never runs")
}
