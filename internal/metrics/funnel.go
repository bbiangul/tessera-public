// Package metrics computes business numbers from milestones (RFC-0001,
// "Métricas de negócio e experimentos"): funnels by cohort, with the time
// each stage takes and the subjects stuck in one.
package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// ParseDuration reads Go durations and whole days: "36h", "14d".
func ParseDuration(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		d, err := strconv.Atoi(n)
		if err != nil || d <= 0 {
			return 0, fmt.Errorf("%q: want a positive number of days", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%q: want a positive duration such as 36h or 14d", s)
	}
	return d, nil
}

// Stage is one stage of a cohort.
type Stage struct {
	Name string `json:"name"`
	// Reached counts subjects that got here from the previous stage in time.
	Reached int `json:"reached"`
	// Pending are still inside the window from the previous stage.
	Pending int `json:"pending"`
	// Lost ran out of the window without getting here.
	Lost int `json:"lost"`
	// Conversion is Reached / (Reached + Lost): pending subjects are not
	// counted as failures. NaN-free: 0 when nobody is decided yet.
	Conversion float64 `json:"conversion"`
	// Durations from the previous stage, over the subjects that reached it.
	Median time.Duration `json:"median"`
	P90    time.Duration `json:"p90"`
	Mean   time.Duration `json:"mean"`
	// Stuck are waiting for this stage longer than 1.5 times Expected.
	Stuck []Stuck `json:"stuck,omitempty"`
}

// Stuck is a subject waiting for a stage since it reached the previous one.
type Stuck struct {
	ID    string    `json:"id"`
	Since time.Time `json:"since"`
}

// Cohort is the subjects that entered in one period.
type Cohort struct {
	Start   time.Time `json:"start"`
	Group   string    `json:"group,omitempty"` // the dimension value, when cut
	Entered int       `json:"entered"`
	Stages  []Stage   `json:"stages"` // from the second stage on
	// Total is the time from entry to the last stage.
	Total Stage `json:"total"`
}

// Report is a funnel's numbers.
type Report struct {
	Funnel  string    `json:"funnel"`
	At      time.Time `json:"at"`
	Cohorts []Cohort  `json:"cohorts"`
}

type stage struct {
	name, milestone  string
	within, expected time.Duration
}

// Compute builds a funnel's report from the milestones of its subject
// kind, at now, in loc (cohort boundaries). by names a dimension to cut by,
// or is empty.
func Compute(f *agents.Funnel, ms []journal.Milestone, now time.Time, loc *time.Location, by string) (Report, error) {
	var stages []stage
	for _, s := range f.Spec.Stages {
		st := stage{name: s.Name, milestone: s.Milestone}
		var err error
		if s.Within != "" {
			if st.within, err = ParseDuration(s.Within); err != nil {
				return Report{}, fmt.Errorf("stage %s: within %w", s.Name, err)
			}
		}
		if s.Expected != "" {
			if st.expected, err = ParseDuration(s.Expected); err != nil {
				return Report{}, fmt.Errorf("stage %s: expected %w", s.Name, err)
			}
		}
		stages = append(stages, st)
	}
	// Each subject's milestones, in time order (the store returns them so).
	bySubject := map[string][]journal.Milestone{}
	var ids []string
	for _, m := range ms {
		if m.At.After(now) { // a report at a time sees only what had happened
			continue
		}
		if _, ok := bySubject[m.ID]; !ok {
			ids = append(ids, m.ID)
		}
		bySubject[m.ID] = append(bySubject[m.ID], m)
	}
	type key struct {
		start time.Time
		group string
	}
	type acc struct {
		entered int
		reached []int
		pending []int
		lost    []int
		durs    [][]time.Duration
		totals  []time.Duration
		stuck   [][]Stuck
	}
	accs := map[key]*acc{}
	for _, id := range ids {
		history := bySubject[id]
		entry, ok := first(history, stages[0].milestone, time.Time{})
		if !ok {
			continue
		}
		k := key{start: cohortStart(entry.At, f.Spec.Cohort, loc), group: attribute(history, by)}
		a := accs[k]
		if a == nil {
			n := len(stages) - 1
			a = &acc{reached: make([]int, n), pending: make([]int, n), lost: make([]int, n), durs: make([][]time.Duration, n), stuck: make([][]Stuck, n)}
			accs[k] = a
		}
		a.entered++
		prev := entry.At
		for i, st := range stages[1:] {
			next, ok := first(history, st.milestone, prev)
			if ok && (st.within == 0 || next.At.Sub(prev) <= st.within) {
				a.reached[i]++
				a.durs[i] = append(a.durs[i], next.At.Sub(prev))
				prev = next.At
				if i == len(stages)-2 {
					a.totals = append(a.totals, prev.Sub(entry.At))
				}
				continue
			}
			if st.within > 0 && now.Sub(prev) > st.within {
				a.lost[i]++
			} else {
				a.pending[i]++
				if st.expected > 0 && now.Sub(prev) > st.expected*3/2 {
					a.stuck[i] = append(a.stuck[i], Stuck{ID: id, Since: prev})
				}
			}
			break // the later stages are not reached either
		}
	}
	rep := Report{Funnel: f.Metadata.Name, At: now}
	for k, a := range accs {
		c := Cohort{Start: k.start, Group: k.group, Entered: a.entered}
		for i, st := range stages[1:] {
			s := Stage{Name: st.name, Reached: a.reached[i], Pending: a.pending[i], Lost: a.lost[i], Stuck: a.stuck[i]}
			if d := s.Reached + s.Lost; d > 0 {
				s.Conversion = float64(s.Reached) / float64(d)
			}
			s.Median, s.P90, s.Mean = summarize(a.durs[i])
			c.Stages = append(c.Stages, s)
		}
		c.Total = Stage{Name: "total", Reached: len(a.totals)}
		c.Total.Median, c.Total.P90, c.Total.Mean = summarize(a.totals)
		rep.Cohorts = append(rep.Cohorts, c)
	}
	sort.Slice(rep.Cohorts, func(i, j int) bool {
		if !rep.Cohorts[i].Start.Equal(rep.Cohorts[j].Start) {
			return rep.Cohorts[i].Start.Before(rep.Cohorts[j].Start)
		}
		return rep.Cohorts[i].Group < rep.Cohorts[j].Group
	})
	return rep, nil
}

// first is the subject's first milestone of a name at or after t.
func first(history []journal.Milestone, name string, after time.Time) (journal.Milestone, bool) {
	for _, m := range history {
		if m.Name == name && !m.At.Before(after) {
			return m, true
		}
	}
	return journal.Milestone{}, false
}

// attribute is the subject's latest value of a dimension, or "".
func attribute(history []journal.Milestone, name string) string {
	if name == "" {
		return ""
	}
	v := ""
	for _, m := range history {
		if x, ok := m.Attributes[name]; ok && x != "" {
			v = x
		}
	}
	if v == "" {
		return "(sem valor)"
	}
	return v
}

func cohortStart(t time.Time, cohort string, loc *time.Location) time.Time {
	l := t.In(loc)
	day := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
	switch cohort {
	case "day":
		return day
	case "month":
		return time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, loc)
	}
	back := (int(day.Weekday()) + 6) % 7 // weeks start on Monday
	return day.AddDate(0, 0, -back)
}

// summarize returns the median, the 90th percentile (nearest rank) and the
// mean.
func summarize(ds []time.Duration) (median, p90, mean time.Duration) {
	if len(ds) == 0 {
		return 0, 0, 0
	}
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	var sum time.Duration
	for _, d := range s {
		sum += d
	}
	rank := func(p float64) time.Duration {
		i := int(p*float64(len(s))+0.999999) - 1
		if i < 0 {
			i = 0
		}
		return s[i]
	}
	return rank(0.5), rank(0.9), sum / time.Duration(len(s))
}
