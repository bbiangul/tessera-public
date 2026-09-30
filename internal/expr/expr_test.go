package expr

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func engine(t *testing.T, opts ...Option) *Engine {
	t.Helper()
	e, err := New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

var ctx = context.Background()

// act mirrors the RFC-0001 reschedule example.
func act() Activation {
	eventTime := time.Date(2026, 9, 24, 13, 0, 0, 0, time.UTC) // Thursday 10:00 in São Paulo
	return Activation{
		"inputs": map[string]any{
			"appointment": map[string]any{"id": "ap-1", "dentistId": "d-9"},
			"newSlot":     map[string]any{"id": "s-3", "label": "sexta 9h", "start": eventTime.Add(20 * time.Hour)},
			"days":        int64(7),
		},
		"sources": map[string]any{
			"upcoming": map[string]any{"output": []any{map[string]any{"id": "ap-1"}}},
			"slots": map[string]any{"output": []any{
				map[string]any{"label": "quinta 10h"}, map[string]any{"label": "sexta 9h"},
			}},
		},
		"steps": map[string]any{
			"reply":  map[string]any{"status": "succeeded", "output": map[string]any{"text": "sim"}},
			"intent": map[string]any{"status": "skipped"},
		},
		"event":  map[string]any{"time": eventTime},
		"caller": map[string]any{"role": "patient", "patientId": "p-1"},
	}
}

func TestRFCExampleExpressions(t *testing.T) {
	e := engine(t)
	cases := []struct {
		src  string
		want Result
		out  any
		refs []string
	}{
		{"caller.role == 'patient'", Bool, true, []string{"caller.role"}},
		{"size(sources.upcoming.output) == 1", Bool, true, []string{"sources.upcoming"}},
		{"sources.upcoming.output[0]", Any, map[string]any{"id": "ap-1"}, []string{"sources.upcoming"}},
		{"inputs.newSlot.start > event.time + duration('2h')", Bool, true, []string{"event.time", "inputs.newSlot"}},
		{"inputs.appointment.id + ':' + inputs.newSlot.id", String, "ap-1:s-3", []string{"inputs.appointment", "inputs.newSlot"}},
		{"steps.reply.status == 'succeeded'", Bool, true, []string{"steps.reply"}},
		{"has(steps.intent.output)", Bool, false, []string{"steps.intent"}},
		{"has(inputs.missing)", Bool, false, []string{"inputs.missing"}},
		{"inputs['days'] * 2", Any, int64(14), []string{"inputs.days"}},
		{"sources.slots.output.map(s, s.label).join(', ')", String, "quinta 10h, sexta 9h", []string{"sources.slots"}},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			x, err := e.Compile(c.src, c.want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := x.Eval(ctx, act())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.out) {
				t.Errorf("got %#v, want %#v", got, c.out)
			}
			if !reflect.DeepEqual(x.References(), c.refs) {
				t.Errorf("references %v, want %v", x.References(), c.refs)
			}
		})
	}
}

func TestCompileRejects(t *testing.T) {
	e := engine(t)
	cases := map[string]struct {
		src  string
		want Result
		msg  string
	}{
		"undeclared variable (RFC v0.1 bug)": {"size(slots) > 0", Bool, "undeclared reference"},
		"hyphenated call (RFC v0.1 bug)":     {"fn.find-slots(inputs.x)", Any, "undeclared reference"},
		"no now()":                           {"now() > event.time", Bool, "undeclared reference"},
		"wrong result type":                  {"caller.role + 'x'", Bool, "must produce bool"},
		"syntax":                             {"inputs.x ==", Any, "Syntax error"},
		"empty":                              {"  ", Any, "empty expression"},
		"too long":                           {strings.Repeat("1+", MaxSourceLength) + "1", Any, "longer than"},
		"too deep":                           {strings.Repeat("[", 80) + "1" + strings.Repeat("]", 80), Any, "nested deeper"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := e.Compile(c.src, c.want)
			if err == nil || !strings.Contains(err.Error(), c.msg) {
				t.Fatalf("want error containing %q, got %v", c.msg, err)
			}
			var xe *Error
			if !errors.As(err, &xe) {
				t.Errorf("want *Error, got %T", err)
			}
		})
	}
}

func TestDynResultCheckedAtRuntime(t *testing.T) {
	x, err := engine(t).Compile("inputs.appointment.id", Bool) // dyn: allowed at compile time
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.Eval(ctx, act()); err == nil || !strings.Contains(err.Error(), "must produce bool") {
		t.Fatalf("want runtime type error, got %v", err)
	}
}

func TestMissingScopesAreEmpty(t *testing.T) {
	x, err := engine(t).Compile("size(conversation) == 0 && !has(env.region)", Bool)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := x.EvalBool(ctx, Activation{}); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestCostLimit(t *testing.T) {
	xs := make([]any, 400)
	for i := range xs {
		xs[i] = int64(i)
	}
	x, err := engine(t, WithCostLimit(10_000)).Compile("inputs.xs.all(a, inputs.xs.all(b, a + b >= 0))", Bool)
	if err != nil {
		t.Fatal(err)
	}
	_, err = x.Eval(ctx, Activation{"inputs": map[string]any{"xs": xs}})
	if !errors.Is(err, ErrCostLimit) {
		t.Fatalf("want ErrCostLimit, got %v", err)
	}
}

func TestTimeFunctions(t *testing.T) {
	e := engine(t)
	sp := "'America/Sao_Paulo'"
	// 2026-09-27 02:00 UTC is Saturday 23:00 in São Paulo, Sunday in UTC.
	sat := map[string]any{"time": time.Date(2026, 9, 27, 2, 0, 0, 0, time.UTC)}
	cases := []struct {
		src string
		ev  map[string]any
		out any
	}{
		{"isWeekend(event.time, " + sp + ")", sat, true},
		{"event.time.getDayOfWeek(" + sp + ") == 6", sat, true}, // CEL: 0 = Sunday
		{"event.time.getDayOfWeek('UTC') == 0", sat, true},
		{"addBusinessDays(event.time, 1, " + sp + ")", map[string]any{"time": time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)},
			time.Date(2026, 9, 28, 12, 0, 0, 0, mustLoc(t, "America/Sao_Paulo"))},
		{"startOfDay(event.time, " + sp + ")", sat, time.Date(2026, 9, 26, 0, 0, 0, 0, mustLoc(t, "America/Sao_Paulo"))},
		// Friday 15h UTC (12h in São Paulo): the next business day at 10h
		// is Monday 10h local.
		{"atTime(addBusinessDays(event.time, 1, " + sp + "), '10:00', " + sp + ")", map[string]any{"time": time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)},
			time.Date(2026, 9, 28, 10, 0, 0, 0, mustLoc(t, "America/Sao_Paulo"))},
		// 02:00 UTC Saturday is still Friday 23h in São Paulo.
		{"atTime(event.time, '09:30', " + sp + ")", sat, time.Date(2026, 9, 26, 9, 30, 0, 0, mustLoc(t, "America/Sao_Paulo"))},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			x, err := e.Compile(c.src, Any)
			if err != nil {
				t.Fatal(err)
			}
			got, err := x.Eval(ctx, Activation{"event": c.ev})
			if err != nil {
				t.Fatal(err)
			}
			if gt, ok := got.(time.Time); ok {
				if !gt.Equal(c.out.(time.Time)) {
					t.Errorf("got %v, want %v", gt, c.out)
				}
				return
			}
			if got != c.out {
				t.Errorf("got %v, want %v", got, c.out)
			}
		})
	}
	for _, zone := range []string{"'Mars/Olympus'", "''", "'Local'"} {
		x, err := e.Compile("isWeekend(event.time, "+zone+")", Bool)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := x.Eval(ctx, Activation{"event": sat}); err == nil {
			t.Errorf("zone %s must be refused", zone)
		}
	}
}

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestAddBusinessDaysBackwards(t *testing.T) {
	mon := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	if got := AddBusinessDays(mon, -1); got.Weekday() != time.Friday || got.Day() != 25 {
		t.Errorf("got %v", got)
	}
	if got := AddBusinessDays(mon, 0); !got.Equal(mon) {
		t.Errorf("zero days changed the time: %v", got)
	}
}
