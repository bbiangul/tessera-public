package compute

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const scoring = `
WEIGHTS = {"price": 0.6, "duration": 0.4}

def _norm(x, lo, hi):
    if hi == lo:
        return 1.0
    return 1.0 - (x - lo) / (hi - lo)

def score(flights, max_results = 3):
    prices = [f["price"] for f in flights]
    mins = [f["minutes"] for f in flights]
    out = []
    for f in flights:
        s = WEIGHTS["price"] * _norm(f["price"], min(prices), max(prices)) + \
            WEIGHTS["duration"] * _norm(f["minutes"], min(mins), max(mins))
        out.append({"id": f["id"], "score": s})
    return sorted(out, key = lambda r: -r["score"])[:max_results]

def arrive(departs, minutes):
    return departs + minutes * time.minute

def loop_forever():
    n = 0
    while True:
        n += 1

def fails(secret):
    return {"a": 1}[secret]
`

func compile(t *testing.T) *Script {
	t.Helper()
	s, err := Compile("scoring.star", []byte(scoring), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCallConvertsBothWays(t *testing.T) {
	s := compile(t)
	out, err := s.Call(context.Background(), "score", map[string]any{
		"flights": []any{
			map[string]any{"id": "a", "price": 1000.0, "minutes": int64(300)},
			map[string]any{"id": "b", "price": 800.0, "minutes": int64(420)},
			map[string]any{"id": "c", "price": 1200.0, "minutes": int64(240)},
		},
		"max_results": int64(2),
	}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	got := out.([]any)
	if len(got) != 2 || got[0].(map[string]any)["id"] != "b" {
		t.Errorf("score = %v", got)
	}

	dep := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	out, err = s.Call(context.Background(), "arrive", map[string]any{"departs": dep, "minutes": int64(90)}, Limits{})
	if err != nil || !out.(time.Time).Equal(dep.Add(90*time.Minute)) {
		t.Errorf("arrive = %v, %v", out, err)
	}
}

func TestIntrospection(t *testing.T) {
	s := compile(t)
	if got := s.Functions(); !reflect.DeepEqual(got, []string{"arrive", "fails", "loop_forever", "score"}) {
		t.Errorf("functions = %v (private _norm must be hidden)", got)
	}
	if sig, ok := s.Signature("score"); !ok || !reflect.DeepEqual(sig, Signature{Params: []string{"flights", "max_results"}, Required: []string{"flights"}}) {
		t.Errorf("signature = %+v", sig)
	}
	kw, _ := Compile("kw.star", []byte("def f(a, *rest, b = 1, **more):\n    pass\n"), Limits{})
	if sig, ok := kw.Signature("f"); !ok || !reflect.DeepEqual(sig, Signature{Params: []string{"a", "b"}, Required: []string{"a"}, Kwargs: true}) {
		t.Errorf("signature with *args and **kwargs = %+v", sig)
	}
	if _, ok := s.Signature("_norm"); ok {
		t.Error("private functions have no public signature")
	}
	if _, err := s.Call(context.Background(), "_norm", nil, Limits{}); !errors.Is(err, ErrCompute) {
		t.Errorf("private functions cannot be called: %v", err)
	}
}

func TestLimitsAndCancellation(t *testing.T) {
	s := compile(t)
	start := time.Now()
	_, err := s.Call(context.Background(), "loop_forever", nil, Limits{Steps: 100_000})
	if !errors.Is(err, ErrCompute) || !strings.Contains(err.Error(), "step limit") {
		t.Errorf("want the step limit, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Error("the step limit must stop the loop quickly")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := s.Call(ctx, "loop_forever", nil, Limits{Steps: 1 << 62}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want the deadline, got %v", err)
	}
	big, err := Compile("big.star", []byte("def f():\n    return list(range(1000))\n"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := big.Call(context.Background(), "f", nil, Limits{Values: 100}); err == nil {
		t.Error("the result size limit must hold")
	}
}

func TestDeterminismAndSandbox(t *testing.T) {
	for name, src := range map[string]string{
		"no clock": "x = time.now()",
		"no load":  "load('other.star', 'f')",
	} {
		if _, err := Compile(name, []byte(src), Limits{}); err == nil {
			t.Errorf("%s: must fail", name)
		}
	}
	// Globals are frozen after loading: a call cannot leave state behind.
	s, err := Compile("state.star", []byte("L = []\ndef f():\n    L.append(1)\n    return len(L)\n"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Call(context.Background(), "f", nil, Limits{}); err == nil {
		t.Error("mutating a global must fail")
	}
}

func TestErrorsCarryNoData(t *testing.T) {
	s := compile(t)
	_, err := s.Call(context.Background(), "fails", map[string]any{"secret": "4111111111111111"}, Limits{})
	if err == nil || strings.Contains(err.Error(), "4111") {
		t.Fatalf("error leaks the value: %v", err)
	}
	if !strings.Contains(err.Error(), "scoring.star:") {
		t.Errorf("error must keep the position: %v", err)
	}
	if _, err := Compile("bad.star", []byte("def f(:\n"), Limits{}); err == nil || !strings.Contains(err.Error(), "bad.star:1") {
		t.Errorf("syntax errors keep the position: %v", err)
	}
}

func TestConversionRefusesWhatCannotRoundTrip(t *testing.T) {
	if _, err := ToStarlark(struct{}{}); err == nil {
		t.Error("unknown Go types must be refused")
	}
	s, _ := Compile("r.star", []byte("def nan():\n    return float('nan')\ndef keys():\n    return {1: 2}\ndef fn():\n    return nan\n"), Limits{})
	for _, fn := range []string{"nan", "keys", "fn"} {
		if _, err := s.Call(context.Background(), fn, nil, Limits{}); !errors.Is(err, ErrCompute) {
			t.Errorf("%s: want ErrCompute, got %v", fn, err)
		}
	}
}

// Review 2026-09-23: dict order came from Go's random map order.
func TestDictOrderIsDeterministic(t *testing.T) {
	s, err := Compile("order.star", []byte("def first(d):\n    for k in d:\n        return k\n"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"d": map[string]any{"e": 1, "a": 1, "c": 1, "b": 1, "d": 1}}
	for range 50 {
		if got, _ := s.Call(context.Background(), "first", args, Limits{}); got != "a" {
			t.Fatalf("first key %v, want a (sorted)", got)
		}
	}
}

// Review 2026-09-23: fail() carried personal data into run errors.
func TestFailKeepsCodesOnly(t *testing.T) {
	s, _ := Compile("f.star", []byte("def code():\n    fail('NO_PLACE')\ndef data(x):\n    fail(x)\n"), Limits{})
	_, err := s.Call(context.Background(), "code", nil, Limits{})
	if err == nil || !strings.Contains(err.Error(), "fail: NO_PLACE") {
		t.Errorf("a code must survive: %v", err)
	}
	_, err = s.Call(context.Background(), "data", map[string]any{"x": "maria.silva@example.com"}, Limits{})
	if err == nil || strings.Contains(err.Error(), "maria") || !strings.Contains(err.Error(), "withheld") {
		t.Errorf("free text must be withheld: %v", err)
	}
}

func TestMemoryWatchdogStopsARunawayScript(t *testing.T) {
	s, _ := Compile("m.star", []byte("def grow():\n    x = []\n    for i in range(100000000):\n        x.append('ab' * 1000)\n    return len(x)\n"), Limits{})
	start := time.Now()
	_, err := s.Call(context.Background(), "grow", nil, Limits{Steps: 1 << 40, Memory: 64 << 20})
	if err == nil || !strings.Contains(err.Error(), "memory limit") {
		t.Fatalf("want the memory limit, got %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("the watchdog must act quickly")
	}
}
