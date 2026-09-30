// Package ledger records what model calls cost and enforces budgets.
//
// The same ledger serves production (per Run) and tests (per scenario or
// suite): tests assert budgets on it, and live CI jobs use it as a kill
// switch so a run cannot spend more than it was given (RFC-0002).
//
// The kill switch works by reservation. Before a call, Reserve sets aside an
// estimate of its cost and refuses the call if spent + reserved + estimate
// would pass the budget; Settle replaces the estimate with the real cost.
// So concurrent callers cannot overshoot by more than the difference between
// their estimates and what they actually cost.
package ledger

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// Entry is one model call.
type Entry struct {
	Engine       string        `json:"engine"`  // "decide", "generate"
	Backend      string        `json:"backend"` // "typesafe", "llm-constrained", ...
	Model        string        `json:"model"`
	Provider     string        `json:"provider,omitempty"`
	InputTokens  int           `json:"inputTokens"`
	OutputTokens int           `json:"outputTokens"`
	CostUSD      float64       `json:"costUSD"`
	Latency      time.Duration `json:"latency"`
	Err          bool          `json:"err,omitempty"`
}

// Budget limits a ledger. Zero values mean "no limit".
type Budget struct {
	MaxCostUSD float64
	// MaxCalls limits calls per engine, e.g. {"decide": 15}.
	MaxCalls map[string]int
	// MaxP95Latency limits the 95th percentile latency over all calls.
	MaxP95Latency time.Duration
}

// Validate rejects budgets that would silently disable the kill switch.
func (b Budget) Validate() error {
	if math.IsNaN(b.MaxCostUSD) || math.IsInf(b.MaxCostUSD, 0) || b.MaxCostUSD < 0 {
		return fmt.Errorf("max cost %v is not a finite, non-negative number", b.MaxCostUSD)
	}
	for e, n := range b.MaxCalls {
		if n < 0 {
			return fmt.Errorf("max calls for %s is negative", e)
		}
	}
	if b.MaxP95Latency < 0 {
		return errors.New("max p95 latency is negative")
	}
	return nil
}

var (
	// ErrBudgetExceeded is returned once a budget would be passed.
	ErrBudgetExceeded = errors.New("budget exceeded")
	// ErrInvalidEntry is returned for costs or token counts a provider
	// should never report: negative, NaN or infinite.
	ErrInvalidEntry = errors.New("invalid ledger entry")
)

// Ledger is safe for concurrent use.
type Ledger struct {
	mu       sync.Mutex
	entries  []Entry
	budget   Budget
	reserved float64        // estimates of calls in flight
	inFlight map[string]int // calls in flight per engine
	invalid  int            // entries refused by Settle
}

// New returns a ledger. It panics on a budget that fails Validate, because a
// broken budget is a programming error that must not silently allow spending.
func New(b Budget) *Ledger {
	if err := b.Validate(); err != nil {
		panic("ledger: " + err.Error())
	}
	return &Ledger{budget: b, inFlight: map[string]int{}}
}

// Reservation is a pending call; settle it exactly once.
type Reservation struct {
	l        *Ledger
	engine   string
	estimate float64
	done     bool
}

// Reserve sets aside estimate (USD) for a call on engine. It fails when the
// money or call budget, counting calls already in flight, would be passed.
func (l *Ledger) Reserve(engine string, estimate float64) (*Reservation, error) {
	if math.IsNaN(estimate) || math.IsInf(estimate, 0) || estimate < 0 {
		return nil, fmt.Errorf("%w: estimate %v", ErrInvalidEntry, estimate)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if max := l.budget.MaxCostUSD; max > 0 {
		committed := l.costLocked() + l.reserved
		if committed+estimate > max || committed >= max {
			return nil, fmt.Errorf("%w: spent $%.6f and reserved $%.6f of $%.6f", ErrBudgetExceeded, l.costLocked(), l.reserved, max)
		}
	}
	if max, ok := l.budget.MaxCalls[engine]; ok && l.callsLocked(engine)+l.inFlight[engine] >= max {
		return nil, fmt.Errorf("%w: %d %s calls of %d", ErrBudgetExceeded, l.callsLocked(engine)+l.inFlight[engine], engine, max)
	}
	l.reserved += estimate
	l.inFlight[engine]++
	return &Reservation{l: l, engine: engine, estimate: estimate}, nil
}

// Settle records the call and releases its reservation. An entry with a
// negative, NaN or infinite cost or token count is refused; the reservation
// is then charged as spent, so a bad provider report never frees budget.
func (r *Reservation) Settle(e Entry) error {
	l := r.l
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.done {
		return errors.New("ledger: reservation settled twice")
	}
	r.done = true
	l.reserved -= r.estimate
	l.inFlight[r.engine]--
	e.Engine = r.engine
	if err := checkEntry(e); err != nil {
		l.invalid++
		l.entries = append(l.entries, Entry{Engine: r.engine, Backend: e.Backend, Model: e.Model, CostUSD: r.estimate, Err: true})
		return err
	}
	l.entries = append(l.entries, e)
	return nil
}

func checkEntry(e Entry) error {
	if math.IsNaN(e.CostUSD) || math.IsInf(e.CostUSD, 0) || e.CostUSD < 0 {
		return fmt.Errorf("%w: cost %v", ErrInvalidEntry, e.CostUSD)
	}
	if e.InputTokens < 0 || e.OutputTokens < 0 {
		return fmt.Errorf("%w: negative token count", ErrInvalidEntry)
	}
	return nil
}

// Add records a call made without a reservation (for tests and imports).
// Invalid entries are refused.
func (l *Ledger) Add(e Entry) error {
	if err := checkEntry(e); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
	return nil
}

// Entries returns a copy of all entries.
func (l *Ledger) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Entry(nil), l.entries...)
}

func (l *Ledger) costLocked() float64 {
	var c float64
	for _, e := range l.entries {
		c += e.CostUSD
	}
	return c
}

func (l *Ledger) callsLocked(engine string) int {
	var n int
	for _, e := range l.entries {
		if e.Engine == engine {
			n++
		}
	}
	return n
}

// Totals summarizes a ledger.
type Totals struct {
	Calls        map[string]int `json:"calls"` // per engine
	InputTokens  int            `json:"inputTokens"`
	OutputTokens int            `json:"outputTokens"`
	CostUSD      float64        `json:"costUSD"`
	Errors       int            `json:"errors"`
	Invalid      int            `json:"invalid"` // entries refused as impossible
	P50Latency   time.Duration  `json:"p50Latency"`
	P95Latency   time.Duration  `json:"p95Latency"`
}

// Totals computes the summary.
func (l *Ledger) Totals() Totals {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := Totals{Calls: map[string]int{}, Invalid: l.invalid}
	var lat []time.Duration
	for _, e := range l.entries {
		t.Calls[e.Engine]++
		t.InputTokens += e.InputTokens
		t.OutputTokens += e.OutputTokens
		t.CostUSD += e.CostUSD
		if e.Err {
			t.Errors++
		}
		lat = append(lat, e.Latency)
	}
	t.P50Latency, t.P95Latency = percentile(lat, .5), percentile(lat, .95)
	return t
}

// Exceeded reports whether the money budget has been used up.
func (l *Ledger) Exceeded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.budget.MaxCostUSD > 0 && l.costLocked() >= l.budget.MaxCostUSD
}

// Check returns an error listing every budget the ledger exceeds. Spending
// exactly the budget is allowed; passing it is not.
func (l *Ledger) Check() error {
	t := l.Totals()
	var errs []error
	if b := l.budget.MaxCostUSD; b > 0 && t.CostUSD > b {
		errs = append(errs, fmt.Errorf("cost $%.6f > budget $%.6f", t.CostUSD, b))
	}
	engines := make([]string, 0, len(l.budget.MaxCalls))
	for e := range l.budget.MaxCalls {
		engines = append(engines, e)
	}
	sort.Strings(engines)
	for _, e := range engines {
		if max := l.budget.MaxCalls[e]; t.Calls[e] > max {
			errs = append(errs, fmt.Errorf("%d %s calls > budget %d", t.Calls[e], e, max))
		}
	}
	if b := l.budget.MaxP95Latency; b > 0 && t.P95Latency > b {
		errs = append(errs, fmt.Errorf("p95 latency %v > budget %v", t.P95Latency, b))
	}
	if t.Invalid > 0 {
		errs = append(errs, fmt.Errorf("%d impossible cost reports from providers", t.Invalid))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrBudgetExceeded, errors.Join(errs...))
	}
	return nil
}

// percentile uses linear interpolation between closest ranks.
func percentile(xs []time.Duration, q float64) time.Duration {
	if len(xs) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	pos := q * float64(len(s)-1)
	lo := int(pos)
	hi := min(lo+1, len(s)-1)
	frac := pos - float64(lo)
	return s[lo] + time.Duration(math.Round(frac*float64(s[hi]-s[lo])))
}
