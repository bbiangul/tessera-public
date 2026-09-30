package bench

import (
	"bytes"
	"math"
	"sort"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
)

// Trial is one answer to one case by one backend.
type Trial struct {
	Target   string             `json:"target"` // backend:model label
	CaseID   string             `json:"caseId"`
	Task     string             `json:"task"`
	Repeat   int                `json:"repeat"`
	Gold     string             `json:"gold"`
	Pred     string             `json:"pred,omitempty"`
	Probs    map[string]float64 `json:"probs,omitempty"`
	Source   decide.ProbSource  `json:"source,omitempty"`
	Model    string             `json:"model,omitempty"`
	Provider string             `json:"provider,omitempty"`
	Latency  time.Duration      `json:"latency"`
	CostUSD  float64            `json:"costUSD"`
	Err      string             `json:"err,omitempty"`
}

// Correct reports whether the trial predicted the gold outcome.
func (t Trial) Correct() bool { return t.Err == "" && t.Pred == t.Gold }

// PTop is the probability of the predicted outcome.
func (t Trial) PTop() float64 { return t.Probs[t.Pred] }

// Selective is accuracy when only answers with PTop >= Threshold are acted on.
type Selective struct {
	Threshold float64 `json:"threshold"`
	Coverage  float64 `json:"coverage"` // share of answered trials at or above threshold
	Accuracy  float64 `json:"accuracy"` // accuracy among those
}

// Summary aggregates the trials of one target.
type Summary struct {
	Target string `json:"target"`
	Trials int    `json:"trials"`
	Cases  int    `json:"cases"`
	Errors int    `json:"errors"`

	TrialAccuracy float64 `json:"trialAccuracy"`
	// CaseAccuracy uses the majority prediction per case; CI is Wilson 95%.
	CaseAccuracy float64    `json:"caseAccuracy"`
	CaseCI       [2]float64 `json:"caseCI"`
	Consistency  float64    `json:"consistency"` // cases with the same prediction in every repeat

	Brier float64 `json:"brier"`
	// LogLoss clips the probability of the gold answer at LogLossFloor, so
	// for saturated models (p exactly 0 or 1) it mostly reflects the floor:
	// read it with Clipped and ConfidentlyWrong.
	LogLoss float64 `json:"logLoss"`
	// Clipped counts answered trials whose gold probability was below the floor.
	Clipped int `json:"clipped"`
	// ConfidentlyWrong is the share of answered trials that were wrong with
	// top probability ≥ 0.9: the errors a confidence threshold cannot catch.
	ConfidentlyWrong float64 `json:"confidentlyWrong"`
	ECE              float64 `json:"ece"` // 10 equal-width bins over PTop
	// AUROC: does confidence separate right from wrong? -1 when undefined.
	AUROC  float64     `json:"auroc"`
	Select []Selective `json:"selective"`

	LatencyP50 time.Duration `json:"latencyP50"`
	LatencyP95 time.Duration `json:"latencyP95"`
	CostPerQ   float64       `json:"costPerQuestion"`

	Sources map[decide.ProbSource]int `json:"sources"`
	ByTask  map[string]float64        `json:"accuracyByTask"`
}

// Summarize computes metrics for trials that all share one target.
func Summarize(target string, trials []Trial) Summary {
	s := Summary{Target: target, Trials: len(trials), Sources: map[decide.ProbSource]int{}, ByTask: map[string]float64{}}
	var answered []Trial
	var lat []float64
	var cost float64
	byCase := map[string][]Trial{}
	taskN, taskOK := map[string]int{}, map[string]int{}
	for _, t := range trials {
		byCase[t.CaseID] = append(byCase[t.CaseID], t)
		cost += t.CostUSD
		taskN[t.Task]++
		if t.Correct() {
			taskOK[t.Task]++
		}
		if t.Err != "" {
			s.Errors++
			continue
		}
		answered = append(answered, t)
		lat = append(lat, float64(t.Latency))
		s.Sources[t.Source]++
	}
	s.Cases = len(byCase)
	if len(trials) > 0 {
		s.CostPerQ = cost / float64(len(trials))
	}
	for task, n := range taskN {
		s.ByTask[task] = float64(taskOK[task]) / float64(n)
	}

	var ok int
	for _, t := range trials {
		if t.Correct() {
			ok++
		}
	}
	if len(trials) > 0 {
		s.TrialAccuracy = float64(ok) / float64(len(trials))
	}

	var caseOK, consistent int
	for _, ts := range byCase {
		pred, agree := majority(ts)
		if pred == ts[0].Gold {
			caseOK++
		}
		if agree {
			consistent++
		}
	}
	if s.Cases > 0 {
		s.CaseAccuracy = float64(caseOK) / float64(s.Cases)
		s.Consistency = float64(consistent) / float64(s.Cases)
		s.CaseCI = Wilson(caseOK, s.Cases, 1.96)
	}

	s.Brier, s.LogLoss, s.ECE = proper(answered)
	s.AUROC = AUROC(answered)
	s.ConfidentlyWrong = confidentlyWrong(answered, 0.9)
	s.Clipped = clipped(answered)
	for _, th := range []float64{0.5, 0.7, 0.9} {
		s.Select = append(s.Select, selective(trials, th))
	}
	s.LatencyP50 = time.Duration(Percentile(lat, 0.50))
	s.LatencyP95 = time.Duration(Percentile(lat, 0.95))
	return s
}

// majority returns the most frequent prediction (errors count as "") and
// whether all trials agree. A tie between predictions has no majority and
// returns "", which never matches a gold answer: a coin flip is not right.
func majority(ts []Trial) (string, bool) {
	count := map[string]int{}
	for _, t := range ts {
		p := t.Pred
		if t.Err != "" {
			p = ""
		}
		count[p]++
	}
	best, bestN, tied := "", -1, false
	for k, n := range count {
		switch {
		case n > bestN:
			best, bestN, tied = k, n, false
		case n == bestN:
			tied = true
		}
	}
	if tied {
		return "", false
	}
	return best, len(count) == 1
}

// LogLossFloor is the smallest gold probability log loss uses.
const LogLossFloor = 1e-6

func clipped(ts []Trial) int {
	n := 0
	for _, t := range ts {
		if t.Probs[t.Gold] < LogLossFloor {
			n++
		}
	}
	return n
}

func confidentlyWrong(ts []Trial, th float64) float64 {
	if len(ts) == 0 {
		return 0
	}
	n := 0
	for _, t := range ts {
		if !t.Correct() && t.PTop() >= th {
			n++
		}
	}
	return float64(n) / float64(len(ts))
}

// proper computes Brier score, log loss and expected calibration error.
func proper(ts []Trial) (brier, logloss, ece float64) {
	if len(ts) == 0 {
		return 0, 0, 0
	}
	const bins = 10
	var binN [bins]int
	var binConf, binAcc [bins]float64
	for _, t := range ts {
		for k, p := range t.Probs {
			y := 0.0
			if k == t.Gold {
				y = 1
			}
			brier += (p - y) * (p - y)
		}
		if _, ok := t.Probs[t.Gold]; !ok { // gold had zero mass
			brier += 1
		}
		logloss -= math.Log(math.Max(t.Probs[t.Gold], LogLossFloor))

		c := t.PTop()
		b := min(int(c*bins), bins-1)
		binN[b]++
		binConf[b] += c
		if t.Correct() {
			binAcc[b]++
		}
	}
	n := float64(len(ts))
	for b := range bins {
		if binN[b] == 0 {
			continue
		}
		k := float64(binN[b])
		ece += k / n * math.Abs(binAcc[b]/k-binConf[b]/k)
	}
	return brier / n, logloss / n, ece
}

func selective(ts []Trial, th float64) Selective {
	s := Selective{Threshold: th}
	var kept, ok int
	for _, t := range ts {
		if t.Err == "" && t.PTop() >= th {
			kept++
			if t.Correct() {
				ok++
			}
		}
	}
	if len(ts) > 0 {
		s.Coverage = float64(kept) / float64(len(ts))
	}
	if kept > 0 {
		s.Accuracy = float64(ok) / float64(kept)
	}
	return s
}

// Wilson returns the Wilson score interval for k successes in n trials.
func Wilson(k, n int, z float64) [2]float64 {
	if n == 0 {
		return [2]float64{0, 0}
	}
	p := float64(k) / float64(n)
	nf := float64(n)
	den := 1 + z*z/nf
	center := (p + z*z/(2*nf)) / den
	half := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / den
	return [2]float64{math.Max(0, center-half), math.Min(1, center+half)}
}

// Percentile uses linear interpolation between closest ranks (the method
// numpy calls "linear"). The v0.1 benchmark rounded down, understating p95.
func Percentile(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	pos := q * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
