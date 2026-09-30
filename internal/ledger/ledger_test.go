package ledger

import (
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func mustReserve(t *testing.T, l *Ledger, engine string, est float64) *Reservation {
	t.Helper()
	r, err := l.Reserve(engine, est)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	return r
}

func TestReserveIsAKillSwitch(t *testing.T) {
	l := New(Budget{MaxCostUSD: 0.01})
	r := mustReserve(t, l, "decide", 0.004)
	if err := r.Settle(Entry{CostUSD: 0.006}); err != nil {
		t.Fatal(err)
	}
	r = mustReserve(t, l, "decide", 0.004) // 0.006 + 0.004 = 0.010: exactly the budget is allowed
	_ = r.Settle(Entry{CostUSD: 0.004})
	if _, err := l.Reserve("decide", 0.000001); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("budget spent: want ErrBudgetExceeded, got %v", err)
	}
	if !l.Exceeded() {
		t.Error("Exceeded must report a spent budget")
	}
}

func TestReserveCountsCallsInFlight(t *testing.T) {
	// The v0.1 ledger only counted finished calls: 200 concurrent callers
	// spent $8.40 against a $0.05 budget. Reservations close that gap.
	l := New(Budget{MaxCostUSD: 0.05})
	var admitted atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := l.Reserve("decide", 0.01)
			if err != nil {
				return
			}
			admitted.Add(1)
			time.Sleep(time.Millisecond)
			_ = r.Settle(Entry{CostUSD: 0.01})
		}()
	}
	close(start)
	wg.Wait()
	if tot := l.Totals(); tot.CostUSD > 0.05+1e-12 {
		t.Errorf("spent $%.4f with %d admitted calls; budget $0.05", tot.CostUSD, admitted.Load())
	}
}

func TestCallBudgetCountsCallsInFlight(t *testing.T) {
	l := New(Budget{MaxCalls: map[string]int{"decide": 2}})
	r1 := mustReserve(t, l, "decide", 0)
	_ = mustReserve(t, l, "decide", 0) // in flight, not settled
	if _, err := l.Reserve("decide", 0); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("third concurrent call must be refused, got %v", err)
	}
	if _, err := l.Reserve("generate", 0); err != nil {
		t.Fatalf("other engines are not limited: %v", err)
	}
	_ = r1.Settle(Entry{})
	if _, err := l.Reserve("decide", 0); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("one settled + one in flight = 2: still refused, got %v", err)
	}
}

func TestImpossibleReportsNeverFreeBudget(t *testing.T) {
	cases := map[string]Entry{
		"negative cost":   {CostUSD: -4.2},
		"NaN cost":        {CostUSD: math.NaN()},
		"infinite cost":   {CostUSD: math.Inf(1)},
		"negative tokens": {InputTokens: -100000000},
	}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			l := New(Budget{MaxCostUSD: 0.01})
			r := mustReserve(t, l, "decide", 0.003)
			if err := r.Settle(e); !errors.Is(err, ErrInvalidEntry) {
				t.Fatalf("want ErrInvalidEntry, got %v", err)
			}
			tot := l.Totals()
			if tot.CostUSD != 0.003 || tot.Invalid != 1 {
				t.Errorf("the reservation must be charged: %+v", tot)
			}
			if err := l.Check(); !errors.Is(err, ErrBudgetExceeded) || !strings.Contains(err.Error(), "impossible") {
				t.Errorf("Check must flag impossible reports, got %v", err)
			}
		})
	}
}

func TestBudgetValidation(t *testing.T) {
	for _, b := range []Budget{{MaxCostUSD: math.NaN()}, {MaxCostUSD: -1}, {MaxCostUSD: math.Inf(1)}, {MaxCalls: map[string]int{"x": -1}}, {MaxP95Latency: -1}} {
		if err := b.Validate(); err == nil {
			t.Errorf("%+v must be invalid", b)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("New must panic on an invalid budget")
		}
	}()
	New(Budget{MaxCostUSD: math.NaN()})
}

func TestReserveRejectsBadEstimate(t *testing.T) {
	l := New(Budget{MaxCostUSD: 1})
	for _, est := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := l.Reserve("decide", est); !errors.Is(err, ErrInvalidEntry) {
			t.Errorf("estimate %v: want ErrInvalidEntry, got %v", est, err)
		}
	}
}

func TestSettleTwice(t *testing.T) {
	l := New(Budget{})
	r := mustReserve(t, l, "decide", 0)
	_ = r.Settle(Entry{})
	if err := r.Settle(Entry{}); err == nil {
		t.Error("second settle must fail")
	}
	if l.Totals().Calls["decide"] != 1 {
		t.Error("a double settle must not record twice")
	}
}

func TestTotalsAndCheck(t *testing.T) {
	l := New(Budget{MaxCostUSD: 0.001, MaxCalls: map[string]int{"decide": 1}, MaxP95Latency: time.Second})
	_ = l.Add(Entry{Engine: "decide", InputTokens: 100, CostUSD: 0.001, Latency: 100 * time.Millisecond})
	_ = l.Add(Entry{Engine: "decide", InputTokens: 50, OutputTokens: 5, CostUSD: 0.0005, Latency: 3 * time.Second, Err: true})
	_ = l.Add(Entry{Engine: "generate", OutputTokens: 200, Latency: 200 * time.Millisecond})

	tot := l.Totals()
	if tot.Calls["decide"] != 2 || tot.Calls["generate"] != 1 || tot.InputTokens != 150 ||
		tot.OutputTokens != 205 || tot.Errors != 1 || math.Abs(tot.CostUSD-0.0015) > 1e-12 {
		t.Errorf("totals = %+v", tot)
	}
	// p95 over [100ms, 200ms, 3s] with interpolation: 200ms + .9*(2.8s) = 2.72s.
	if tot.P95Latency != 2720*time.Millisecond {
		t.Errorf("p95 = %v", tot.P95Latency)
	}
	err := l.Check()
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want budget error, got %v", err)
	}
	for _, want := range []string{"cost", "2 decide calls", "p95 latency"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
	if err := New(Budget{}).Check(); err != nil {
		t.Errorf("empty budget must pass: %v", err)
	}
	if err := l.Add(Entry{CostUSD: -1}); !errors.Is(err, ErrInvalidEntry) {
		t.Errorf("Add must refuse impossible entries, got %v", err)
	}
}

func TestCheckBoundaries(t *testing.T) {
	exact := New(Budget{MaxCostUSD: 0.002, MaxCalls: map[string]int{"decide": 2}})
	_ = exact.Add(Entry{Engine: "decide", CostUSD: 0.001})
	_ = exact.Add(Entry{Engine: "decide", CostUSD: 0.001})
	if err := exact.Check(); err != nil {
		t.Errorf("spending exactly the budget must pass Check: %v", err)
	}
	_ = exact.Add(Entry{Engine: "decide", CostUSD: 0.0000001})
	if err := exact.Check(); err == nil {
		t.Error("passing the budget must fail Check")
	}
}
