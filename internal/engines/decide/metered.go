package decide

import (
	"context"
	"github.com/bbiangul/tessera/internal/httpx"
	"sync"

	"github.com/bbiangul/tessera/internal/ledger"
)

// DefaultEstimateUSD is what Metered reserves per call before it has seen a
// real cost. It is deliberately pessimistic for the Decide engine (a Jev call
// costs about $0.00002).
const DefaultEstimateUSD = 0.001

// Metered charges every call to a ledger and refuses calls the budget cannot
// cover. Before a call it reserves an estimate (the larger of EstimateUSD and
// the most expensive call seen so far); afterwards it settles the real cost.
// A call that fails without reporting usage is charged its estimate, because
// the provider may have billed it.
type Metered struct {
	Inner  Decider
	Ledger *ledger.Ledger
	// EstimateUSD overrides DefaultEstimateUSD.
	EstimateUSD float64

	mu      sync.Mutex
	maxSeen float64
}

func (m *Metered) Name() string { return m.Inner.Name() }

func (m *Metered) estimate() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	est := m.EstimateUSD
	if est <= 0 {
		est = DefaultEstimateUSD
	}
	return max(est, m.maxSeen)
}

func (m *Metered) Decide(ctx context.Context, req Request) (*Response, error) {
	est := m.estimate()
	res, err := m.Ledger.Reserve("decide", est)
	if err != nil {
		return nil, err
	}
	resp, callErr := m.Inner.Decide(ctx, req)
	e := ledger.Entry{Backend: m.Inner.Name(), Err: callErr != nil}
	if resp != nil {
		e.Model, e.Provider, e.Latency = resp.Model, resp.Provider, resp.Latency
		e.InputTokens, e.OutputTokens, e.CostUSD = resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CostUSD
	}
	// Reported cost, plus the estimate for each attempt that may have been
	// billed without a report; a failure httpx cannot describe costs at
	// least the estimate (#7).
	if resp != nil {
		e.CostUSD += est * float64(resp.Usage.UnreportedCalls)
	}
	if callErr != nil {
		if n, known := httpx.MaybeBilled(callErr); known {
			e.CostUSD += est * float64(n)
		} else {
			e.CostUSD = max(e.CostUSD, est)
		}
	}
	settleErr := res.Settle(e)
	if settleErr == nil {
		m.mu.Lock()
		m.maxSeen = max(m.maxSeen, e.CostUSD)
		m.mu.Unlock()
	}
	if callErr != nil {
		return resp, callErr
	}
	if settleErr != nil {
		return nil, settleErr // the provider reported an impossible cost
	}
	return resp, nil
}
