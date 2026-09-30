package decide_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/decide/decidetest"
	"github.com/bbiangul/tessera/internal/httpx"
	"github.com/bbiangul/tessera/internal/ledger"
)

var noulReq = decide.Request{
	State:     map[string]string{"user_reply": "pode ser"},
	Questions: map[string]decide.Question{"confirmed": {Kind: decide.KindNoul, Instructions: "Did the user agree?"}},
}

func TestScripted(t *testing.T) {
	s := &decidetest.Scripted{Answers: map[string]map[string]float64{"confirmed": {"true": .9, "false": .1}}}
	resp, err := s.Decide(context.Background(), noulReq)
	if err != nil || resp.Answers["confirmed"].Noul != .9 || len(s.Calls()) != 1 {
		t.Fatalf("resp=%+v err=%v calls=%d", resp, err, len(s.Calls()))
	}
	other := decide.Request{Questions: map[string]decide.Question{"x": {Kind: decide.KindNoul, Instructions: "?"}}}
	if _, err := s.Decide(context.Background(), other); err == nil || !strings.Contains(err.Error(), "no scripted answer") {
		t.Errorf("unscripted question must fail, got %v", err)
	}
	bad := &decidetest.Scripted{Answers: map[string]map[string]float64{"confirmed": {"maybe": 1}}}
	if _, err := bad.Decide(context.Background(), noulReq); err == nil {
		t.Error("unknown outcome must fail")
	}
}

// costly returns a fixed cost per call.
type costly struct{ decidetest.Scripted }

func (c *costly) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	r, err := c.Scripted.Decide(ctx, req)
	if r != nil {
		r.Usage.CostUSD = 0.004
	}
	return r, err
}

func TestMeteredNeverOverspends(t *testing.T) {
	inner := &costly{decidetest.Scripted{Answers: map[string]map[string]float64{"confirmed": {"true": 1}}}}
	l := ledger.New(ledger.Budget{MaxCostUSD: 0.01})
	m := &decide.Metered{Inner: inner, Ledger: l}
	calls := 0
	for range 5 {
		if _, err := m.Decide(context.Background(), noulReq); err != nil {
			if !errors.Is(err, ledger.ErrBudgetExceeded) {
				t.Fatalf("unexpected error: %v", err)
			}
			break
		}
		calls++
	}
	// Each call costs $0.004. After the first, the estimate is $0.004, so the
	// third call ($0.008 + $0.004 > $0.01) is refused before it runs.
	if calls != 2 || len(inner.Calls()) != 2 {
		t.Errorf("calls made = %d, inner saw %d; want 2", calls, len(inner.Calls()))
	}
	if tot := l.Totals(); tot.CostUSD > 0.01 {
		t.Errorf("spent $%v, over the $0.01 budget", tot.CostUSD)
	}
}

// failing reports a billed call that still failed, like a malformed answer.
type failing struct{ withUsage bool }

func (f failing) Name() string { return "failing" }
func (f failing) Decide(context.Context, decide.Request) (*decide.Response, error) {
	if f.withUsage {
		return &decide.Response{Usage: decide.Usage{InputTokens: 10, CostUSD: 0.02}}, errors.New("malformed answer")
	}
	return nil, errors.New("connection reset")
}

func TestMeteredChargesFailedCalls(t *testing.T) {
	l := ledger.New(ledger.Budget{MaxCostUSD: 1})
	m := &decide.Metered{Inner: failing{withUsage: true}, Ledger: l}
	if _, err := m.Decide(context.Background(), noulReq); err == nil {
		t.Fatal("want the call error")
	}
	if tot := l.Totals(); tot.CostUSD != 0.02 || tot.Errors != 1 {
		t.Errorf("a billed failure must be recorded at its real cost: %+v", tot)
	}

	l2 := ledger.New(ledger.Budget{MaxCostUSD: 1})
	m2 := &decide.Metered{Inner: failing{}, Ledger: l2, EstimateUSD: 0.05}
	_, _ = m2.Decide(context.Background(), noulReq)
	if tot := l2.Totals(); tot.CostUSD != 0.05 {
		t.Errorf("a failure with no usage must be charged its estimate: %+v", tot)
	}
}

func TestCassetteRecordThenReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	inner := &decidetest.Scripted{Answers: map[string]map[string]float64{"confirmed": {"true": .7, "false": .3}}}

	rec, err := decide.LoadCassette(path, "scripted:v1", inner, decide.CassetteRecord)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Decide(context.Background(), noulReq); err != nil {
		t.Fatal(err)
	}
	if err := rec.Save(path); err != nil {
		t.Fatal(err)
	}

	// Replay: no inner backend at all; the answer comes from the file.
	rep, err := decide.LoadCassette(path, "scripted:v1", nil, decide.CassetteReplay)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := rep.Decide(context.Background(), noulReq)
	if err != nil || resp.Answers["confirmed"].Noul != .7 {
		t.Fatalf("replay resp=%+v err=%v", resp, err)
	}

	// A changed prompt is a miss that shows the new request.
	changed := noulReq
	changed.Questions = map[string]decide.Question{"confirmed": {Kind: decide.KindNoul, Instructions: "Did the user clearly agree?"}}
	_, err = rep.Decide(context.Background(), changed)
	if !errors.Is(err, decide.ErrCassetteMiss) || !strings.Contains(err.Error(), "clearly agree") {
		t.Errorf("want miss with the new request in the message, got %v", err)
	}

	// Another target (model upgrade) is also a miss.
	other, _ := decide.LoadCassette(path, "scripted:v2", nil, decide.CassetteReplay)
	if _, err := other.Decide(context.Background(), noulReq); !errors.Is(err, decide.ErrCassetteMiss) {
		t.Errorf("different target must miss, got %v", err)
	}
}

func TestCassetteReplaysRepeatsInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	inner := &decidetest.Scripted{Answers: map[string]map[string]float64{"confirmed": {"true": .6, "false": .4}}}
	rec, _ := decide.LoadCassette(path, "t", inner, decide.CassetteRecord)
	_, _ = rec.Decide(context.Background(), noulReq)
	inner.Answers["confirmed"] = map[string]float64{"true": .2, "false": .8}
	_, _ = rec.Decide(context.Background(), noulReq)
	if err := rec.Save(path); err != nil {
		t.Fatal(err)
	}
	rep, _ := decide.LoadCassette(path, "t", nil, decide.CassetteReplay)
	var got []float64
	for range 2 {
		r, err := rep.Decide(context.Background(), noulReq)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, r.Answers["confirmed"].Noul)
	}
	if got[0] != .6 || got[1] != .2 {
		t.Errorf("replayed %v, want [.6 .2] in recording order", got)
	}
	// A third call was never recorded: a regression that asks the model
	// more often must miss, not repeat an old answer (review 2026-09-23).
	if _, err := rep.Decide(context.Background(), noulReq); !errors.Is(err, decide.ErrCassetteMiss) {
		t.Errorf("want a miss, got %v", err)
	}
}

type httpFailing struct{ stats httpx.Stats }

func (httpFailing) Name() string { return "http-failing" }
func (f httpFailing) Decide(context.Context, decide.Request) (*decide.Response, error) {
	return nil, &httpx.Error{Err: errors.New("x"), Stats: f.stats}
}

// #7: Decide charges by what may have been billed, like Generate.
func TestMeteredChargesByWhatMayHaveBeenBilled(t *testing.T) {
	for _, c := range []struct {
		stats httpx.Stats
		want  float64
	}{
		{httpx.Stats{Attempts: 3}, 0},
		{httpx.Stats{Attempts: 1, MaybeBilled: 1}, 0.05},
	} {
		l := ledger.New(ledger.Budget{MaxCostUSD: 1})
		m := &decide.Metered{Inner: httpFailing{c.stats}, Ledger: l, EstimateUSD: 0.05}
		_, _ = m.Decide(context.Background(), noulReq)
		if got := l.Totals().CostUSD; got != c.want {
			t.Errorf("%+v: charged %v, want %v", c.stats, got, c.want)
		}
	}
}
