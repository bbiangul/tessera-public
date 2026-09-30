// Package cron reads five-field cron expressions (minute hour day-of-month
// month day-of-week) and finds the times they name in a time zone. It has
// what Triggers need and nothing else: lists, ranges and steps; no
// seconds, no names, no "L" or "W".
package cron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed expression.
type Schedule struct {
	minute, hour, dom, month, dow uint64 // bit i set: value i allowed
	domAny, dowAny                bool   // "*": the other day field decides
	src                           string
}

var fields = []struct {
	name     string
	min, max int
}{{"minute", 0, 59}, {"hour", 0, 23}, {"day of month", 1, 31}, {"month", 1, 12}, {"day of week", 0, 7}}

// Parse reads an expression such as "0 9 * * 1-5" (9h on weekdays).
func Parse(src string) (*Schedule, error) {
	parts := strings.Fields(src)
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron %q: want 5 fields (minute hour day month weekday), got %d", src, len(parts))
	}
	var bits [5]uint64
	for i, p := range parts {
		b, err := field(p, fields[i].min, fields[i].max)
		if err != nil {
			return nil, fmt.Errorf("cron %q: %s: %w", src, fields[i].name, err)
		}
		bits[i] = b
	}
	if bits[4]&(1<<7) != 0 { // 7 is Sunday too
		bits[4] |= 1
	}
	return &Schedule{minute: bits[0], hour: bits[1], dom: bits[2], month: bits[3], dow: bits[4],
		domAny: parts[2] == "*", dowAny: parts[4] == "*", src: src}, nil
}

func field(p string, lo, hi int) (uint64, error) {
	var out uint64
	for _, item := range strings.Split(p, ",") {
		step := 1
		if base, s, ok := strings.Cut(item, "/"); ok {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				return 0, fmt.Errorf("bad step %q", s)
			}
			item, step = base, n
		}
		from, to := lo, hi
		switch {
		case item == "*":
		case strings.Contains(item, "-"):
			a, b, _ := strings.Cut(item, "-")
			x, err1 := strconv.Atoi(a)
			y, err2 := strconv.Atoi(b)
			if err1 != nil || err2 != nil || x > y {
				return 0, fmt.Errorf("bad range %q", item)
			}
			from, to = x, y
		default:
			x, err := strconv.Atoi(item)
			if err != nil {
				return 0, fmt.Errorf("bad value %q", item)
			}
			from, to = x, x
			if step > 1 { // "5/15": from 5 to the end
				to = hi
			}
		}
		if from < lo || to > hi {
			return 0, fmt.Errorf("%d-%d is outside %d-%d", from, to, lo, hi)
		}
		for v := from; v <= to; v += step {
			out |= 1 << uint(v)
		}
	}
	if out == 0 {
		return 0, errors.New("empty")
	}
	return out, nil
}

// String returns the expression as written.
func (s *Schedule) String() string { return s.src }

// Matches reports whether t, read in loc, is a time of the schedule (to the
// minute).
func (s *Schedule) Matches(t time.Time, loc *time.Location) bool {
	l := t.In(loc)
	if s.minute&(1<<uint(l.Minute())) == 0 || s.hour&(1<<uint(l.Hour())) == 0 || s.month&(1<<uint(l.Month())) == 0 {
		return false
	}
	dom := s.dom&(1<<uint(l.Day())) != 0
	dow := s.dow&(1<<uint(l.Weekday())) != 0
	switch {
	case s.domAny && s.dowAny:
		return true
	case s.domAny:
		return dow
	case s.dowAny:
		return dom
	}
	return dom || dow // both restricted: either matches, as in Vixie cron
}

// Last returns the latest time of the schedule at or before now and not
// before now-within, if there is one. Minutes are walked backwards, so
// within should be hours or days, not months.
func (s *Schedule) Last(now time.Time, within time.Duration, loc *time.Location) (time.Time, bool) {
	t := now.Truncate(time.Minute)
	for earliest := now.Add(-within); !t.Before(earliest); t = t.Add(-time.Minute) {
		if s.Matches(t, loc) {
			return t, true
		}
	}
	return time.Time{}, false
}
