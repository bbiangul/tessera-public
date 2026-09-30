package cron

import (
	"testing"
	"time"
)

func TestParseAndMatch(t *testing.T) {
	sp, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	at := func(y int, m time.Month, d, hh, mm int) time.Time { return time.Date(y, m, d, hh, mm, 0, 0, sp) }
	for _, c := range []struct {
		expr string
		t    time.Time
		want bool
	}{
		{"0 9 * * 1-5", at(2026, 9, 25, 9, 0), true},  // Friday 9h
		{"0 9 * * 1-5", at(2026, 9, 26, 9, 0), false}, // Saturday
		{"0 9 * * 1-5", at(2026, 9, 25, 9, 1), false}, // 9:01
		{"*/15 8-18 * * *", at(2026, 9, 26, 18, 45), true},
		{"*/15 8-18 * * *", at(2026, 9, 26, 19, 0), false},
		{"0 0 1 * *", at(2026, 10, 1, 0, 0), true},
		{"0 12 * * 7", at(2026, 9, 27, 12, 0), true}, // 7 is Sunday
		{"0 12 1 * 1", at(2026, 9, 28, 12, 0), true}, // day 1 OR Monday
		{"5,35 * * * *", at(2026, 9, 26, 3, 35), true},
		{"5/20 * * * *", at(2026, 9, 26, 3, 45), true},
	} {
		s, err := Parse(c.expr)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if got := s.Matches(c.t, sp); got != c.want {
			t.Errorf("%s at %v: %v, want %v", c.expr, c.t, got, c.want)
		}
	}
	for _, bad := range []string{"", "* * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 8", "5-1 * * * *", "*/0 * * * *", "a * * * *"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

// Last finds the slot a scheduler should fire, in the zone's local time,
// and nothing older than the window.
func TestLast(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	s, _ := Parse("0 9 * * 1-5")
	now := time.Date(2026, 9, 28, 9, 0, 40, 0, sp) // Monday 9:00:40
	got, ok := s.Last(now, time.Hour, sp)
	if !ok || !got.Equal(time.Date(2026, 9, 28, 9, 0, 0, 0, sp)) {
		t.Errorf("got %v %v", got, ok)
	}
	if _, ok := s.Last(time.Date(2026, 9, 28, 11, 0, 0, 0, sp), time.Hour, sp); ok {
		t.Error("a slot older than the window must not fire")
	}
	if got, ok := s.Last(time.Date(2026, 9, 28, 8, 0, 0, 0, sp), 72*time.Hour, sp); !ok || got.Weekday() != time.Friday {
		t.Errorf("over a weekend the last slot is Friday: %v %v", got, ok)
	}
}
