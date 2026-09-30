package bench

import (
	"fmt"
	"math"
	"testing"
)

func TestAUROC(t *testing.T) {
	// Right answers at .9 and .8, wrong ones at .6 and .8.
	// Pairs: (.9,.6) win, (.9,.8) win, (.8,.6) win, (.8,.8) tie -> 3.5/4.
	ts := []Trial{
		trial("a", "y", "y", .9, .9), trial("b", "y", "y", .8, .8),
		trial("c", "y", "n", .4, .6), trial("d", "y", "n", .2, .8),
	}
	if got := AUROC(ts); !near(got, 3.5/4) {
		t.Errorf("auroc = %v", got)
	}
	if got := AUROC(ts[:2]); got != -1 {
		t.Errorf("no wrong answers: auroc = %v, want -1", got)
	}
}

func TestScaleKeepsArgmaxAndSoftens(t *testing.T) {
	p := scale(map[string]float64{"a": .9, "b": .1}, 2)
	if p["a"] <= p["b"] || p["a"] >= .9 || !near(p["a"]+p["b"], 1) {
		t.Errorf("scaled = %v", p)
	}
	if q := scale(map[string]float64{"a": .9, "b": .1}, 1); !near(q["a"], .9) {
		t.Errorf("T=1 must be identity, got %v", q)
	}
}

// overconfident builds trials that claim p=0.99 but are right 80% of the time.
func overconfident(n int) []Trial {
	var ts []Trial
	for i := range n {
		pred := "y"
		if i%5 == 0 {
			pred = "n"
		}
		ts = append(ts, Trial{Target: "x", CaseID: fmt.Sprint(i), Task: "t", Gold: "y", Pred: pred,
			Probs: map[string]float64{pred: .99, other(pred): .01}})
	}
	return ts
}

func other(k string) string {
	if k == "y" {
		return "n"
	}
	return "y"
}

func TestFitTemperatureFixesOverconfidence(t *testing.T) {
	ts := overconfident(100)
	temp := FitTemperature(ts)
	// The ideal scaled top probability is 0.8: 0.99^(1/T) / (0.99^(1/T)+0.01^(1/T)) = 0.8
	// gives T = ln(99)/ln(4) ≈ 3.31.
	if math.Abs(temp-math.Log(99)/math.Log(4)) > 0.2 {
		t.Errorf("temperature = %v, want ≈3.31", temp)
	}
	res := CrossValidateTemperature("x", ts, 5)
	if res.CV.LogLoss >= res.Raw.LogLoss || res.CV.ECE >= res.Raw.ECE {
		t.Errorf("CV scaling should improve: raw ll=%v ece=%v, cv ll=%v ece=%v",
			res.Raw.LogLoss, res.Raw.ECE, res.CV.LogLoss, res.CV.ECE)
	}
	if res.CV.TrialAccuracy != res.Raw.TrialAccuracy {
		t.Error("temperature scaling must not change predictions")
	}
	if len(res.FoldTemps) != 5 {
		t.Errorf("folds = %d", len(res.FoldTemps))
	}
}
