package expr

import (
	"time"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
)

// timeFunctions adds calendar helpers. Every one takes an explicit IANA time
// zone: "D+3" or "business hours" mean nothing without one, and the legacy platform
// engine had UTC vs UTC-3 bugs from implicit zones. CEL's own accessors
// already take a zone too, e.g. event.time.getHours("America/Sao_Paulo").
func timeFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("addBusinessDays",
			cel.Overload("add_business_days_timestamp_int_string",
				[]*cel.Type{cel.TimestampType, cel.IntType, cel.StringType}, cel.TimestampType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					t, loc, err := timeArgs(args[0], args[2])
					if err != nil {
						return err
					}
					days, ok := args[1].(types.Int)
					if !ok {
						return types.NewErr("addBusinessDays: days must be an int")
					}
					if days > MaxBusinessDays || days < -MaxBusinessDays {
						return types.NewErr("addBusinessDays: at most %d days", MaxBusinessDays)
					}
					return types.Timestamp{Time: AddBusinessDays(t.In(loc), int(days))}
				}))),
		cel.Function("startOfDay",
			cel.Overload("start_of_day_timestamp_string",
				[]*cel.Type{cel.TimestampType, cel.StringType}, cel.TimestampType,
				cel.BinaryBinding(func(ts, zone ref.Val) ref.Val {
					t, loc, err := timeArgs(ts, zone)
					if err != nil {
						return err
					}
					return types.Timestamp{Time: StartOfDay(t.In(loc))}
				}))),
		cel.Function("date",
			cel.Overload("date_string_string",
				[]*cel.Type{cel.StringType, cel.StringType}, cel.TimestampType,
				cel.BinaryBinding(func(dayArg, zone ref.Val) ref.Val {
					d, ok := dayArg.(types.String)
					z, zok := zone.(types.String)
					if !ok || !zok {
						return types.NewErr("date: want date('YYYY-MM-DD', zone)")
					}
					loc, err := time.LoadLocation(string(z))
					if err != nil {
						return types.NewErr("unknown time zone")
					}
					day, err := time.Parse("2006-01-02", string(d))
					if err != nil {
						return types.NewErr("date: invalid YYYY-MM-DD")
					}
					// Go resolves a missing local midnight (a DST gap) to the
					// previous day; noon always exists, and StartOfDay finds the
					// day's first instant from there.
					noon := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc)
					return types.Timestamp{Time: StartOfDay(noon)}
				}))),
		cel.Function("formatDate",
			cel.Overload("format_date_timestamp_string",
				[]*cel.Type{cel.TimestampType, cel.StringType}, cel.StringType,
				cel.BinaryBinding(func(ts, zone ref.Val) ref.Val {
					t, loc, err := timeArgs(ts, zone)
					if err != nil {
						return err
					}
					return types.String(t.In(loc).Format("2006-01-02"))
				}))),
		// atTime(t, "HH:MM", zone): the same local day as t, at that
		// local time. With addBusinessDays it writes cadences: "the next
		// business day at 10h". A time skipped by a DST change moves forward.
		cel.Function("atTime",
			cel.Overload("at_time_timestamp_string_string",
				[]*cel.Type{cel.TimestampType, cel.StringType, cel.StringType}, cel.TimestampType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					t, loc, err := timeArgs(args[0], args[2])
					if err != nil {
						return err
					}
					hm, ok := args[1].(types.String)
					if !ok {
						return types.NewErr("atTime: want a time as HH:MM")
					}
					clock, perr := time.Parse("15:04", string(hm))
					if perr != nil {
						return types.NewErr("atTime: want a time as HH:MM")
					}
					l := t.In(loc)
					return types.Timestamp{Time: time.Date(l.Year(), l.Month(), l.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)}
				}))),
		cel.Function("isWeekend",
			cel.Overload("is_weekend_timestamp_string",
				[]*cel.Type{cel.TimestampType, cel.StringType}, cel.BoolType,
				cel.BinaryBinding(func(ts, zone ref.Val) ref.Val {
					t, loc, err := timeArgs(ts, zone)
					if err != nil {
						return err
					}
					wd := t.In(loc).Weekday()
					return types.Bool(wd == time.Saturday || wd == time.Sunday)
				}))),
	}
}

func timeArgs(ts, zone ref.Val) (time.Time, *time.Location, ref.Val) {
	t, ok := ts.(types.Timestamp)
	if !ok {
		return time.Time{}, nil, types.NewErr("expected a timestamp")
	}
	z, ok := zone.(types.String)
	if !ok {
		return time.Time{}, nil, types.NewErr("expected a time zone name")
	}
	// "" and "Local" would silently use the server's zone; refuse them.
	loc, err := time.LoadLocation(string(z))
	if err != nil || z == "" || z == "Local" {
		return time.Time{}, nil, types.NewErr("unknown time zone %q", string(z))
	}
	return t.Time, loc, nil
}

// MaxBusinessDays bounds addBusinessDays: CEL's cost limit does not see the
// work done inside a Go function, so the function bounds itself.
const MaxBusinessDays = 3660

// AddBusinessDays returns the n-th day after t (before t, for negative n)
// that is not a Saturday or Sunday, keeping t's wall-clock time in its
// location. Holidays are not known here; they belong to a calendar plugin.
//
// It does constant work: every block of 5 business days is exactly 7
// calendar days whatever the start, so all blocks but the last are added at
// once and at most 5 business days are walked. Day math happens on the
// calendar date and the time is rebuilt once, so a daylight-saving change on
// the way does not shift the time of day.
func AddBusinessDays(t time.Time, n int) time.Time {
	if n == 0 {
		return t
	}
	step := 1
	if n < 0 {
		step, n = -1, -n
	}
	y, m, d := t.Date()
	date := time.Date(y, m, d, 12, 0, 0, 0, time.UTC) // noon UTC: no DST
	weeks := (n - 1) / 5
	date = date.AddDate(0, 0, step*7*weeks)
	for rem := n - 5*weeks; rem > 0; {
		date = date.AddDate(0, 0, step)
		if wd := date.Weekday(); wd != time.Saturday && wd != time.Sunday {
			rem--
		}
	}
	return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

// StartOfDay returns the first instant of t's calendar day in t's location.
// Where daylight saving starts at midnight, 00:00 does not exist; time.Date
// would then fall back into the previous day, so the result is moved forward
// to the first instant that is on the right date.
func StartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	s := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	for i := 0; i < 4 && s.Day() != d; i++ {
		s = s.Add(time.Hour)
		s = time.Date(s.Year(), s.Month(), s.Day(), s.Hour(), 0, 0, 0, t.Location())
	}
	return s
}
