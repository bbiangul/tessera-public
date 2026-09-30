package expr

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Regression tests for the adversarial review of 2026-09-23.

// naiveBusinessDays is the original day-by-day definition, kept as the
// oracle for the constant-time AddBusinessDays.
func naiveBusinessDays(t time.Time, n int) time.Time {
	step := 1
	if n < 0 {
		step, n = -1, -n
	}
	for n > 0 {
		t = t.AddDate(0, 0, step)
		if wd := t.Weekday(); wd != time.Saturday && wd != time.Sunday {
			n--
		}
	}
	return t
}

func TestAddBusinessDaysMatchesOracle(t *testing.T) {
	base := time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC)
	for s := 0; s < 14; s++ { // every weekday, twice
		start := base.AddDate(0, 0, s)
		for n := -40; n <= 40; n++ {
			if got, want := AddBusinessDays(start, n), naiveBusinessDays(start, n); !got.Equal(want) {
				t.Fatalf("start %s, n=%d: got %v want %v", start.Weekday(), n, got, want)
			}
		}
	}
}

func TestAddBusinessDaysKeepsWallClockAcrossDST(t *testing.T) {
	sp := mustLoc(t, "America/Sao_Paulo")
	// 2018-11-04 00:00 did not exist in São Paulo (DST started at midnight).
	fri := time.Date(2018, 11, 2, 0, 30, 0, 0, sp)
	got := AddBusinessDays(fri, 1)
	if got.Weekday() != time.Monday || got.Hour() != 0 || got.Minute() != 30 {
		t.Errorf("Friday 00:30 + 1 business day = %v, want Monday 00:30", got)
	}
}

func TestAddBusinessDaysIsBoundedAndFast(t *testing.T) {
	x, err := engine(t).Compile("addBusinessDays(event.time, 20000000, 'UTC')", Any)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = x.Eval(context.Background(), Activation{"event": map[string]any{"time": time.Now()}})
	if err == nil || !strings.Contains(err.Error(), "at most 3660") {
		t.Errorf("want the bound error, got %v", err)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("took %v; must fail fast", d)
	}
	if got := AddBusinessDays(time.Now(), MaxBusinessDays); got.IsZero() {
		t.Error("the maximum must work")
	}
}

func TestStartOfDayWhenMidnightDoesNotExist(t *testing.T) {
	for _, c := range []struct {
		zone string
		at   time.Time
	}{
		{"America/Sao_Paulo", time.Date(2018, 11, 4, 15, 0, 0, 0, time.UTC)},
		{"America/Santiago", time.Date(2026, 9, 6, 15, 0, 0, 0, time.UTC)},
		{"America/Havana", time.Date(2026, 3, 8, 15, 0, 0, 0, time.UTC)},
	} {
		loc := mustLoc(t, c.zone)
		local := c.at.In(loc)
		got := StartOfDay(local)
		if got.Day() != local.Day() || got.After(local) {
			t.Errorf("%s: start of %v = %v (must be the same date, not after)", c.zone, local, got)
		}
		if before := got.Add(-time.Second); before.Day() == local.Day() {
			t.Errorf("%s: %v is not the first instant of the day", c.zone, got)
		}
	}
}

func TestTemplateFixes(t *testing.T) {
	e := engine(t)
	a := Activation{"inputs": map[string]any{"p": "120,00"}}
	cases := map[string]string{
		"Preço: R${{ inputs.p }}":   "Preço: R$120,00", // pt-BR currency is text, not ${ }
		`{{ "}}" }} fim`:            "}} fim",
		`{{ {'a': {'b': 1}}.a.b }}`: "1",
		`{{ 'it\'s' }}`:             "it's",
		"Sem {{ '{{' }} problema":   "Sem {{ problema",
	}
	for src, want := range cases {
		tpl, err := e.CompileTemplate(src)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if got, err := tpl.Render(context.Background(), a, PlainText); err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", src, got, err, want)
		}
	}
	if _, err := e.CompileTemplate("Olá ${ inputs.p }"); err == nil {
		t.Error("${ } in a template must still be refused")
	}
}

func TestTemplateLimitsAndCancellation(t *testing.T) {
	e := engine(t)
	if _, err := e.CompileTemplate(strings.Repeat("{{ 1 }}", MaxInterpolations+1)); err == nil {
		t.Error("too many interpolations must be refused")
	}
	tpl, err := e.CompileTemplate(strings.Repeat("{{ 1 }}", MaxInterpolations))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tpl.Render(ctx, nil, PlainText); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled render must stop, got %v", err)
	}
	many := map[string]any{}
	for i := range MaxInterpolations + 1 {
		many[strings.Repeat("k", i+1)] = "${ 1 }"
	}
	if _, err := e.CompileValue(many); err == nil {
		t.Error("too many ${ } in one value must be refused")
	}
}

func TestRuntimeErrorsCarryNoData(t *testing.T) {
	e := engine(t)
	secret := "4111111111111111"
	for _, src := range []string{"inputs.cards[inputs.pan]", "timestamp(inputs.pan)", "inputs.pan.matches('(')"} {
		x, err := e.Compile(src, Any)
		if err != nil {
			t.Fatal(err)
		}
		_, err = x.Eval(context.Background(), Activation{"inputs": map[string]any{"pan": secret, "cards": map[string]any{}}})
		if err == nil {
			t.Fatalf("%s: want an error", src)
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("%s: error leaks the value: %v", src, err)
		}
		if !errors.Is(err, ErrEval) {
			t.Errorf("%s: want ErrEval, got %v", src, err)
		}
	}
	// A missing key named "cost limit" is not a cost-limit error.
	x, _ := e.Compile("inputs['cost limit']", Any)
	if _, err := x.Eval(context.Background(), Activation{}); errors.Is(err, ErrCostLimit) {
		t.Error("key names must not be mistaken for the cost limit")
	}
}

func TestLoopVariablesCannotShadowScopes(t *testing.T) {
	e := engine(t)
	for _, src := range []string{"[1].exists(steps, steps > 0)", "[{'y': 1}].all(inputs, inputs.y == 1)", "[1].map(caller, caller + 1)"} {
		if _, err := e.Compile(src, Any); err == nil || !strings.Contains(err.Error(), "name of a scope") {
			t.Errorf("%s: want a shadowing error, got %v", src, err)
		}
	}
	if _, err := e.Compile("[1].exists(x, x > 0)", Bool); err != nil {
		t.Errorf("ordinary loop variables must work: %v", err)
	}
}

func TestEmptyListsStayLists(t *testing.T) {
	v, err := engine(t).CompileValue(map[string]any{"ids": "${ [1, 2].filter(x, x > 5) }"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"ids": []any{}}) {
		t.Errorf("got %#v, want an empty list (never null)", got)
	}
}

func TestBytesAreNotPrinted(t *testing.T) {
	tpl, _ := engine(t).CompileTemplate("{{ b'hi' }}")
	if _, err := tpl.Render(context.Background(), nil, PlainText); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Errorf("want a bytes error, got %v", err)
	}
}

func TestEscapersRemoveInvisibleAndReorderingCharacters(t *testing.T) {
	for _, r := range []string{"\u202e", "\u2066", "\u200b", "\u200d", "\ufeff", "\u2028", "\u2029", "\U000e0041", "\x7f", "\ufdd0"} {
		for name, esc := range map[string]Escaper{"PlainText": PlainText, "HTML": HTML} {
			if got := esc("a" + r + "b"); got != "ab" {
				t.Errorf("%s(%q) = %q", name, r, got)
			}
		}
	}
	// WhatsApp adds its own zero-width spaces, but never keeps the value's.
	if got := WhatsApp("a\u202eb"); got != "ab" {
		t.Errorf("WhatsApp kept a bidi override: %q", got)
	}
}

func TestWhatsAppNeutralizesLineFormatting(t *testing.T) {
	got := WhatsApp("Ana\n> citação\n- item\n* item\n1. número\nnormal")
	for _, line := range strings.Split(got, "\n")[1:5] {
		if !strings.HasPrefix(line, zwsp) {
			t.Errorf("line %q must start with a zero-width space", line)
		}
	}
	if strings.HasSuffix(got, zwsp+"normal") {
		t.Error("ordinary lines must not change")
	}
}

func TestDateAndFormatDate(t *testing.T) {
	e := engine(t)
	act := Activation{"event": map[string]any{"time": time.Date(2026, 9, 23, 2, 30, 0, 0, time.UTC)}}
	for src, want := range map[string]any{
		// 02:30 UTC is still the 22nd in São Paulo.
		"formatDate(event.time, 'America/Sao_Paulo')":                                       "2026-09-22",
		"formatDate(date('2026-10-12', 'America/Sao_Paulo'), 'America/Sao_Paulo')":          "2026-10-12",
		"date('2026-10-12', 'America/Sao_Paulo') > event.time":                              true,
		"date('2026-10-12', 'America/Sao_Paulo') - date('2026-10-05', 'America/Sao_Paulo')": 7 * 24 * time.Hour,
		"date('2018-11-04', 'America/Sao_Paulo').getHours('America/Sao_Paulo')":             int64(1), // midnight did not exist
	} {
		x, err := e.Compile(src, Any)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		got, err := x.Eval(context.Background(), act)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, %v; want %#v", src, got, err, want)
		}
	}
	for _, bad := range []string{"date('12/10/2026', 'UTC')", "date('2026-02-30', 'UTC')", "date('2026-10-12', 'Mars/Base')"} {
		x, _ := e.Compile(bad, Any)
		if _, err := x.Eval(context.Background(), act); !errors.Is(err, ErrEval) {
			t.Errorf("%s: want ErrEval, got %v", bad, err)
		}
	}
}
