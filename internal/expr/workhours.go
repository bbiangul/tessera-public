package expr

import (
	"errors"
	"time"
)

// WorkHours are working days and hours in a time zone.
type WorkHours struct {
	Days     map[time.Weekday]bool
	From, To time.Duration // since midnight
	Loc      *time.Location
}

// ParseWorkHours reads days (0 Sunday .. 6; empty: Monday to Friday) and
// HH:MM bounds.
func ParseWorkHours(days []int, from, to string, loc *time.Location) (WorkHours, error) {
	f, err1 := time.Parse("15:04", from)
	t, err2 := time.Parse("15:04", to)
	if err1 != nil || err2 != nil || !t.After(f) {
		return WorkHours{}, errors.New("business hours: from and to are HH:MM, from before to")
	}
	w := WorkHours{Days: map[time.Weekday]bool{}, From: time.Duration(f.Hour())*time.Hour + time.Duration(f.Minute())*time.Minute,
		To: time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, Loc: loc}
	if len(days) == 0 {
		days = []int{1, 2, 3, 4, 5}
	}
	for _, d := range days {
		if d < 0 || d > 6 {
			return WorkHours{}, errors.New("business hours: days are 0 (Sunday) to 6")
		}
		w.Days[time.Weekday(d)] = true
	}
	return w, nil
}

// Add is start plus d of working time: "4h from Friday 17h" with 9h-18h
// on weekdays is Monday 12h.
func (w WorkHours) Add(start time.Time, d time.Duration) time.Time {
	cur := start.In(w.Loc)
	for i := 0; i < 3660; i++ { // at most ten years of days
		day := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, w.Loc)
		// Wall-clock times: on a daylight-saving day midnight plus 9h is
		// not 9h.
		at := func(d time.Duration) time.Time {
			return time.Date(day.Year(), day.Month(), day.Day(), int(d/time.Hour), int(d%time.Hour/time.Minute), 0, 0, w.Loc)
		}
		open, close := at(w.From), at(w.To)
		if w.Days[day.Weekday()] && cur.Before(close) {
			if cur.Before(open) {
				cur = open
			}
			if left := close.Sub(cur); d <= left {
				return cur.Add(d)
			} else {
				d -= left
			}
		}
		next := day.AddDate(0, 0, 1)
		cur = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, w.Loc)
	}
	return cur
}
