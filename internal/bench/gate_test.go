package bench

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMcNemarWorse(t *testing.T) {
	// 5 worse, 0 better: P(X >= 5 | n=5) = 1/32.
	if p := McNemarWorse(5, 0); !near(p, 1.0/32) {
		t.Errorf("p = %v, want 1/32", p)
	}
	// 1 worse, 1 better: P(X >= 1 | n=2) = 3/4.
	if p := McNemarWorse(1, 1); !near(p, 0.75) {
		t.Errorf("p = %v, want .75", p)
	}
	if p := McNemarWorse(0, 0); p != 1 {
		t.Errorf("no flips: p = %v, want 1", p)
	}
}

// run builds trials for n cases where the first `wrong` cases are wrong.
func run(n, wrong int, pRight, pWrong float64, lat time.Duration, cost float64) []Trial {
	var ts []Trial
	for i := range n {
		id := fmt.Sprintf("c%02d", i)
		if i < wrong {
			ts = append(ts, Trial{Target: "x", CaseID: id, Task: "t", Gold: "y", Pred: "n",
				Probs: map[string]float64{"n": pWrong, "y": 1 - pWrong}, Latency: lat, CostUSD: cost})
		} else {
			ts = append(ts, Trial{Target: "x", CaseID: id, Task: "t", Gold: "y", Pred: "y",
				Probs: map[string]float64{"y": pRight, "n": 1 - pRight}, Latency: lat, CostUSD: cost})
		}
	}
	return ts
}

func TestGatePassesIdenticalRuns(t *testing.T) {
	base := run(40, 2, .95, .6, 300*time.Millisecond, 1e-5)
	g := Gate(base, base, GateConfig{})
	if !g.Pass() {
		t.Fatalf("identical runs must pass:\n%s", g.Markdown("t"))
	}
}

func TestGateCatchesRegressions(t *testing.T) {
	base := run(40, 1, .95, .6, 300*time.Millisecond, 1e-5)
	cases := map[string]struct {
		cand []Trial
		gate string
	}{
		"accuracy":   {run(40, 8, .95, .6, 300*time.Millisecond, 1e-5), "McNemar"},
		"overconfid": {run(40, 1, .95, .99, 300*time.Millisecond, 1e-5), "selective accuracy"},
		"latency":    {run(40, 1, .95, .6, time.Second, 1e-5), "p95 latency"},
		"cost":       {run(40, 1, .95, .6, 300*time.Millisecond, 2e-5), "cost per question"},
		"errors":     {append(run(39, 1, .95, .6, 300*time.Millisecond, 1e-5), Trial{Target: "x", CaseID: "c39", Gold: "y", Err: "boom"}), "errors"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			g := Gate(base, c.cand, GateConfig{})
			if g.Pass() {
				t.Fatalf("want failure on %s:\n%s", c.gate, g.Markdown("t"))
			}
			for _, ch := range g.Checks {
				if strings.Contains(ch.Name, c.gate) && ch.Pass {
					t.Errorf("gate %q should fail:\n%s", c.gate, g.Markdown("t"))
				}
			}
		})
	}
}

func TestGateToleratesNoise(t *testing.T) {
	// One case flips each way: not a significant regression.
	base := run(40, 1, .95, .6, 300*time.Millisecond, 1e-5)
	cand := run(40, 1, .95, .6, 300*time.Millisecond, 1e-5)
	cand[0].Pred, cand[0].Probs = "y", map[string]float64{"y": .9, "n": .1}
	cand[5].Pred, cand[5].Probs = "n", map[string]float64{"n": .6, "y": .4}
	g := Gate(base, cand, GateConfig{})
	if !g.Checks[0].Pass || len(g.Worse) != 1 || len(g.Better) != 1 {
		t.Errorf("one flip each way must pass McNemar:\n%s", g.Markdown("t"))
	}
}
