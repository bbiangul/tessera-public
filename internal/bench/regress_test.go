package bench

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
)

// Regression tests for the adversarial review of 2026-09-23.

func failed(g GateResult, name string) bool {
	for _, c := range g.Checks {
		if strings.Contains(c.Name, name) && !c.Pass {
			return true
		}
	}
	return false
}

func withModel(ts []Trial, model string) []Trial {
	out := make([]Trial, len(ts))
	for i, t := range ts {
		t.Model = model
		out[i] = t
	}
	return out
}

func TestGateRejectsMissingCases(t *testing.T) {
	base := withModel(run(69, 2, .95, .6, 300*time.Millisecond, 1e-5), "m")
	cand := base[:10] // only 10 of 69 cases
	g := Gate(base, cand, GateConfig{})
	if g.Pass() || !failed(g, "same cases") {
		t.Errorf("a truncated run must fail:\n%s", g.Markdown("t"))
	}
	extra := append(append([]Trial{}, base...), Trial{CaseID: "zz", Gold: "y", Pred: "y", Probs: map[string]float64{"y": 1}, Model: "m"})
	if g := Gate(base, extra, GateConfig{}); !failed(g, "same cases") {
		t.Error("a case outside the baseline must fail")
	}
}

func TestGateRejectsCollapsedConfidence(t *testing.T) {
	base := withModel(run(40, 2, .95, .6, 300*time.Millisecond, 1e-5), "m")
	cand := withModel(run(40, 0, .5, .5, 300*time.Millisecond, 1e-5), "m") // all right, all p=0.5
	g := Gate(base, cand, GateConfig{})
	if g.Pass() || !failed(g, "selective accuracy") || !failed(g, "coverage") || !failed(g, "AUROC") {
		t.Errorf("zero coverage and undefined AUROC must fail when the baseline had them:\n%s", g.Markdown("t"))
	}
}

func TestGateCatchesFourLostCases(t *testing.T) {
	base := withModel(run(69, 2, .95, .6, 300*time.Millisecond, 1e-5), "m")
	cand := withModel(run(69, 6, .95, .55, 300*time.Millisecond, 1e-5), "m")
	g := Gate(base, cand, GateConfig{})
	if !failed(g, "at most a few cases lost") {
		t.Errorf("4 lost cases (McNemar p=0.0625) must still fail:\n%s", g.Markdown("t"))
	}
}

func TestGateRejectsUnknownModelVersion(t *testing.T) {
	base := withModel(run(20, 1, .95, .6, 300*time.Millisecond, 1e-5), "jev-1.13.0")
	cand := withModel(base, "jev-1.14.0")
	if g := Gate(base, cand, GateConfig{}); !failed(g, "same model versions") {
		t.Errorf("an alias moving to a new version must fail:\n%s", g.Markdown("t"))
	}
}

func TestGateBoundaries(t *testing.T) {
	base := withModel(run(40, 1, .95, .6, 1000*time.Millisecond, 1e-5), "m")
	exact := withModel(run(40, 1, .95, .6, 1500*time.Millisecond, 1.2e-5), "m") // ratios exactly 1.5 and 1.2
	g := Gate(base, exact, GateConfig{})
	if failed(g, "p95 latency") || failed(g, "cost per question") {
		t.Errorf("ratios exactly at the limit pass:\n%s", g.Markdown("t"))
	}
	over := withModel(run(40, 1, .95, .6, 1501*time.Millisecond, 1.21e-5), "m")
	g = Gate(base, over, GateConfig{})
	if !failed(g, "p95 latency") || !failed(g, "cost per question") {
		t.Errorf("ratios over the limit fail:\n%s", g.Markdown("t"))
	}
	if g := Gate(base, exact, GateConfig{SkipLatency: true}); strings.Contains(g.Markdown("t"), "p95 latency") {
		t.Error("SkipLatency must drop the latency gate")
	}
}

func TestMajorityTieIsNotRight(t *testing.T) {
	ts := []Trial{trial("a", "y", "y", 1, 1), trial("a", "y", "n", 0, 1)}
	pred, agree := majority(ts)
	if pred != "" || agree {
		t.Errorf("a 1-1 tie has no majority: %q %v", pred, agree)
	}
	if s := Summarize("x", ts); s.CaseAccuracy != 0 {
		t.Errorf("a tie must count as wrong: %v", s.CaseAccuracy)
	}
}

func TestSelectiveCountsErrorsInCoverage(t *testing.T) {
	ts := []Trial{trial("a", "y", "y", .9, .9), {CaseID: "b", Gold: "y", Err: "boom"}}
	sel := selective(ts, 0.7)
	if sel.Coverage != 0.5 || sel.Accuracy != 1 {
		t.Errorf("errors are not acted on but still count: %+v", sel)
	}
}

func TestConfidentlyWrongAndClipped(t *testing.T) {
	ts := []Trial{
		trial("a", "y", "n", 0, 1),     // wrong at p=1: confidently wrong and clipped
		trial("b", "y", "n", .3, .7),   // wrong but unsure
		trial("c", "y", "y", .95, .95), // right
	}
	s := Summarize("x", ts)
	if s.Clipped != 1 || fmt.Sprintf("%.3f", s.ConfidentlyWrong) != "0.333" {
		t.Errorf("clipped=%d confidentlyWrong=%v", s.Clipped, s.ConfidentlyWrong)
	}
}

func TestPinnedModel(t *testing.T) {
	for label, want := range map[string]string{
		"typesafe:jev-1.13.0":                       "jev-1.13.0",
		"typesafe:jev-latest":                       "",
		"llm:deepseek/deepseek-v4.1-flash@DeepSeek": "deepseek/deepseek-v4.1-flash",
		"llm:~deepseek/deepseek-v4-flash-latest":    "",
		"nocolon":                                   "",
	} {
		if got := PinnedModel(label); got != want {
			t.Errorf("PinnedModel(%q) = %q, want %q", label, got, want)
		}
	}
}

type servedModel struct{ model string }

func (s servedModel) Name() string { return "x" }
func (s servedModel) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	out := &decide.Response{Answers: map[string]decide.Answer{}, Model: s.model}
	for id, q := range req.Questions {
		a, _ := decide.AnswerFor(q, map[string]float64{"true": .9, "false": .1}, decide.ProbNative)
		out.Answers[id] = a
	}
	return out, nil
}

func TestRunnerRejectsAnUnpinnedModel(t *testing.T) {
	cases := []Case{{ID: "1", Task: "t", Question: decide.Question{Kind: decide.KindNoul, Instructions: "x?"}, Gold: "true"}}
	trials := Run(context.Background(), cases, []Target{{Label: "typesafe:jev-1.13.0", Decider: servedModel{"jev-1.14.0"}}}, Options{Repeats: 1})
	if !strings.Contains(trials[0].Err, "pinned") {
		t.Errorf("serving a different version than pinned must be an error: %+v", trials[0])
	}
}

func TestScoreQuestionsUseArgmax(t *testing.T) {
	q := decide.Question{Kind: decide.KindScore, Instructions: "x", Levels: []string{"low", "mid", "high"}}
	if got := argmax(map[string]float64{"0": .2, "1": .5, "2": .3}, q.Outcomes()); got != "1" {
		t.Errorf("argmax = %q", got)
	}
}

func TestSaturatedProbabilitiesAreNotScaled(t *testing.T) {
	sat := []Trial{trial("a", "y", "y", 1, 1), trial("b", "y", "n", 0, 1)}
	if Scalable(sat) || FitTemperature(sat) != 1 {
		t.Error("saturated probabilities must not be scaled")
	}
	pt := []Trial{{CaseID: "a", Gold: "y", Pred: "y", Probs: map[string]float64{"y": .9}, Source: decide.ProbPoint}}
	if Scalable(pt) {
		t.Error("point probabilities must not be scaled")
	}
}

func TestGateMarkdown(t *testing.T) {
	base := withModel(run(10, 1, .95, .6, time.Second, 1e-5), "m")
	md := Gate(base, base[:5], GateConfig{}).Markdown("Title")
	for _, want := range []string{"## Title: FAIL", "**FAIL**", "same cases"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
}

func TestGateNegativeMeansNone(t *testing.T) {
	if c := (GateConfig{MaxWorseCases: -1, MaxCoverageDrop: -1}).withDefaults(); c.MaxWorseCases != 0 || c.MaxCoverageDrop != 0 {
		t.Errorf("negative must mean none allowed, got %d and %v", c.MaxWorseCases, c.MaxCoverageDrop)
	}
	if c := (GateConfig{}).withDefaults(); c.MaxWorseCases != 2 || c.MaxCoverageDrop != 0.10 {
		t.Errorf("zero must take the defaults, got %d and %v", c.MaxWorseCases, c.MaxCoverageDrop)
	}
}
