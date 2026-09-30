package expr

import (
	"testing"
	"time"
)

func TestWorkHoursAdd(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	w, err := ParseWorkHours(nil, "09:00", "18:00", sp)
	if err != nil {
		t.Fatal(err)
	}
	at := func(m time.Month, d, h, min int) time.Time { return time.Date(2026, m, d, h, min, 0, 0, sp) }
	for _, c := range []struct {
		start time.Time
		d     time.Duration
		want  time.Time
	}{
		{at(9, 25, 17, 0), 4 * time.Hour, at(9, 28, 12, 0)},   // Friday 17h + 4h -> Monday 12h
		{at(9, 26, 10, 0), time.Hour, at(9, 28, 10, 0)},       // Saturday -> Monday 9h + 1h
		{at(9, 28, 7, 0), 30 * time.Minute, at(9, 28, 9, 30)}, // before opening
		{at(9, 28, 9, 0), 9 * time.Hour, at(9, 28, 18, 0)},    // a whole day, ends at closing
		{at(9, 28, 20, 0), 18 * time.Hour, at(9, 30, 18, 0)},  // after closing, two days
		{at(9, 28, 12, 0), 0, at(9, 28, 12, 0)},
	} {
		if got := w.Add(c.start, c.d); !got.Equal(c.want) {
			t.Errorf("%v + %v = %v, want %v", c.start, c.d, got, c.want)
		}
	}
	if _, err := ParseWorkHours([]int{7}, "09:00", "18:00", sp); err == nil {
		t.Error("day 7 must be refused")
	}
	if _, err := ParseWorkHours(nil, "18:00", "09:00", sp); err == nil {
		t.Error("from after to must be refused")
	}
}

// On a daylight-saving day the bounds stay wall-clock: New York moved its
// clocks on Sunday 2026-03-08.
func TestWorkHoursAcrossDaylightSaving(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	w, err := ParseWorkHours([]int{0, 1, 2, 3, 4, 5, 6}, "09:00", "18:00", ny)
	if err != nil {
		t.Fatal(err)
	}
	got := w.Add(time.Date(2026, 3, 7, 17, 0, 0, 0, ny), 2*time.Hour)
	if want := time.Date(2026, 3, 8, 10, 0, 0, 0, ny); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
