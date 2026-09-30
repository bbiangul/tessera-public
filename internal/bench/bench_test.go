package bench

import (
	"bytes"
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestPercentileInterpolates(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if got := Percentile(xs, 0.5); !near(got, 5.5) {
		t.Errorf("p50 = %v, want 5.5", got)
	}
	if got := Percentile(xs, 0.95); !near(got, 9.55) {
		t.Errorf("p95 = %v, want 9.55", got)
	}
	if got := Percentile(nil, 0.5); got != 0 {
		t.Errorf("empty = %v", got)
	}
}

func TestWilson(t *testing.T) {
	ci := Wilson(9, 10, 1.96)
	// Reference values for 9/10 at 95%: [0.5958, 0.9821].
	if !near(math.Round(ci[0]*1e4)/1e4, 0.5958) || !near(math.Round(ci[1]*1e4)/1e4, 0.9821) {
		t.Errorf("wilson = %v", ci)
	}
	if ci := Wilson(0, 0, 1.96); ci != [2]float64{} {
		t.Errorf("empty = %v", ci)
	}
}

func trial(caseID, gold, pred string, pGold, pTop float64) Trial {
	probs := map[string]float64{pred: pTop}
	if gold != pred {
		probs[gold] = pGold
	}
	return Trial{Target: "x", CaseID: caseID, Task: "t", Gold: gold, Pred: pred, Probs: probs, Source: decide.ProbLogprobs}
}

func TestSummarizeProperScores(t *testing.T) {
	trials := []Trial{
		trial("a", "yes", "yes", 0.9, 0.9), // right, confident
		trial("b", "yes", "no", 0.4, 0.6),  // wrong, unsure
	}
	s := Summarize("x", trials)
	// Brier: case a (0.9-1)^2 = .01 (the "no" key is absent); case b (.4-1)^2 + (.6-0)^2 = .36+.36 = .72.
	if !near(s.Brier, (0.01+0.72)/2) {
		t.Errorf("brier = %v", s.Brier)
	}
	if !near(s.LogLoss, -(math.Log(0.9)+math.Log(0.4))/2) {
		t.Errorf("logloss = %v", s.LogLoss)
	}
	// ECE: bin 9 has conf .9 acc 1 (gap .1); bin 6 has conf .6 acc 0 (gap .6). Weighted: .35.
	if !near(s.ECE, 0.35) {
		t.Errorf("ece = %v", s.ECE)
	}
	if s.TrialAccuracy != 0.5 || s.CaseAccuracy != 0.5 {
		t.Errorf("acc trial=%v case=%v", s.TrialAccuracy, s.CaseAccuracy)
	}
	if sel := s.Select[1]; sel.Threshold != 0.7 || sel.Coverage != 0.5 || sel.Accuracy != 1 {
		t.Errorf("selective@0.7 = %+v", sel)
	}
}

func TestSummarizeMajorityAndConsistency(t *testing.T) {
	trials := []Trial{
		trial("a", "y", "y", 1, 1), trial("a", "y", "y", 1, 1), trial("a", "y", "n", 0, 1),
		trial("b", "y", "y", 1, 1), trial("b", "y", "y", 1, 1), trial("b", "y", "y", 1, 1),
		{Target: "x", CaseID: "c", Task: "t", Gold: "y", Err: "boom"},
	}
	s := Summarize("x", trials)
	if s.Cases != 3 || s.Errors != 1 {
		t.Fatalf("cases=%d errors=%d", s.Cases, s.Errors)
	}
	// a: majority y (right, inconsistent); b: right, consistent; c: error counts as wrong, consistent.
	if !near(s.CaseAccuracy, 2.0/3) || !near(s.Consistency, 2.0/3) {
		t.Errorf("caseAcc=%v consistency=%v", s.CaseAccuracy, s.Consistency)
	}
}

func TestReadDatasetRejectsBadCases(t *testing.T) {
	good := `{"id":"1","task":"t","state":{},"question":{"kind":"noul","instructions":"x?"},"gold":"true"}`
	cases := map[string]string{
		"unknown field": `{"id":"1","task":"t","state":{},"question":{"kind":"noul","instructions":"x?"},"gold":"true","extra":1}`,
		"bad gold":      `{"id":"1","task":"t","state":{},"question":{"kind":"noul","instructions":"x?"},"gold":"maybe"}`,
		"invalid q":     `{"id":"1","task":"t","state":{},"question":{"kind":"choice","instructions":"x?"},"gold":"a"}`,
		"duplicate id":  good + "\n" + good,
		"empty":         "",
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadDataset(strings.NewReader(data)); err == nil {
				t.Fatal("want error")
			}
		})
	}
	if cs, err := ReadDataset(strings.NewReader(good)); err != nil || len(cs) != 1 {
		t.Fatalf("good dataset: %v %v", cs, err)
	}
}

func TestShippedDatasetIsValid(t *testing.T) {
	cs, err := LoadDataset("../../bench/datasets/travel-decisions-v1.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 12 {
		t.Errorf("cases = %d, want 12", len(cs))
	}
}

// fixedDecider answers every noul with a fixed probability.
type fixedDecider struct{ p float64 }

func (f fixedDecider) Name() string { return "fixed" }
func (f fixedDecider) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	out := &decide.Response{Answers: map[string]decide.Answer{}, Backend: "fixed", Model: "m", Latency: time.Millisecond}
	for id := range req.Questions {
		out.Answers[id] = decide.Answer{Kind: decide.KindNoul, Noul: f.p,
			Probabilities: map[string]float64{"true": f.p, "false": 1 - f.p}, Source: decide.ProbNative}
	}
	return out, nil
}

func TestRunAndReport(t *testing.T) {
	cases := []Case{
		{ID: "1", Task: "t", Question: decide.Question{Kind: decide.KindNoul, Instructions: "x?"}, Gold: "true"},
		{ID: "2", Task: "t", Question: decide.Question{Kind: decide.KindNoul, Instructions: "y?"}, Gold: "false"},
	}
	trials := Run(context.Background(), cases, []Target{{Label: "fixed", Decider: fixedDecider{0.8}}}, Options{Repeats: 2})
	if len(trials) != 4 {
		t.Fatalf("trials = %d", len(trials))
	}
	s := Summarize("fixed", trials)
	if s.TrialAccuracy != 0.5 || s.Consistency != 1 {
		t.Errorf("summary = %+v", s)
	}
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, "Test", "d.jsonl", 2, 2, []Summary{s}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "| fixed | 50.0%") {
		t.Errorf("report:\n%s", buf.String())
	}
}
