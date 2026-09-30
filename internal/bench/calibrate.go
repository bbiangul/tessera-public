package bench

import (
	"math"
	"sort"
)

// AUROC measures whether confidence separates right answers from wrong ones:
// the probability that a random correct trial has a higher PTop than a random
// wrong one (ties count half). 0.5 is no separation, 1.0 is perfect. It is
// independent of any threshold. Returns -1 when there are no right or no
// wrong answers to compare.
func AUROC(ts []Trial) float64 {
	var pos, neg []float64
	for _, t := range ts {
		if t.Err != "" {
			continue
		}
		if t.Correct() {
			pos = append(pos, t.PTop())
		} else {
			neg = append(neg, t.PTop())
		}
	}
	if len(pos) == 0 || len(neg) == 0 {
		return -1
	}
	var wins float64
	for _, p := range pos {
		for _, n := range neg {
			switch {
			case p > n:
				wins++
			case p == n:
				wins += 0.5
			}
		}
	}
	return wins / float64(len(pos)*len(neg))
}

// Temperature scaling rescales a distribution as p_i^(1/T) / sum_j p_j^(1/T).
// T > 1 softens overconfident probabilities; it never changes which option
// wins, so it can fix calibration but not separation (AUROC).
func scale(probs map[string]float64, temp float64) map[string]float64 {
	out := make(map[string]float64, len(probs))
	var z float64
	for k, p := range probs {
		v := math.Pow(math.Max(p, 1e-9), 1/temp)
		out[k] = v
		z += v
	}
	for k := range out {
		out[k] /= z
	}
	return out
}

// Scaled returns trials with probabilities rescaled by temp.
func Scaled(ts []Trial, temp float64) []Trial {
	out := make([]Trial, len(ts))
	for i, t := range ts {
		out[i] = t
		if t.Err == "" && len(t.Probs) > 0 {
			out[i].Probs = scale(t.Probs, temp)
		}
	}
	return out
}

// Temperature grid bounds; a fitted value at a bound means the data wanted
// more than the grid allows, which CalibrationResult reports.
const (
	minTemperature = 0.25
	maxTemperature = 50
)

// temperatureGrid is log-spaced from minTemperature to maxTemperature,
// both included.
func temperatureGrid() []float64 {
	const steps = 160
	g := make([]float64, 0, steps+1)
	lo, hi := math.Log(minTemperature), math.Log(maxTemperature)
	for i := 0; i <= steps; i++ {
		g = append(g, math.Exp(lo+(hi-lo)*float64(i)/steps))
	}
	return g
}

// Scalable reports whether temperature scaling means anything for trials.
// Point and vote probabilities are counts, not a distribution a temperature
// can reshape. Saturated probabilities (every answer at exactly 1.0, as the
// official DeepSeek provider returns) put zero on the other options, so the
// fitted temperature is set by the floor that replaces zero, not by the data.
// Constant but unsaturated confidence (always 0.99) is scalable.
func Scalable(ts []Trial) bool {
	unsaturated := false
	for _, t := range ts {
		if t.Err != "" {
			continue
		}
		if t.Source == "point" || t.Source == "vote" {
			return false // any of these makes the whole target unscalable
		}
		if t.PTop() < 1-1e-9 {
			unsaturated = true
		}
	}
	return unsaturated
}

// FitTemperature returns the temperature that minimizes log loss on ts. It
// returns 1 (no scaling) for sources that cannot be scaled and when the loss
// is flat, so a constant never looks like a finding.
func FitTemperature(ts []Trial) float64 {
	if !Scalable(ts) {
		return 1
	}
	_, base, _ := proper(answeredOnly(ts))
	best, bestLoss := 1.0, base
	for _, temp := range temperatureGrid() {
		_, loss, _ := proper(answeredOnly(Scaled(ts, temp)))
		if loss < bestLoss-1e-9 {
			best, bestLoss = temp, loss
		}
	}
	return best
}

// CalibrationResult compares raw probabilities with temperature scaling
// fitted by k-fold cross-validation over cases, so every trial is scored by a
// temperature fitted without it.
type CalibrationResult struct {
	Target      string  `json:"target"`
	Folds       int     `json:"folds"`
	Temperature float64 `json:"temperature"` // fitted on all trials, for use as a profile
	// Scalable is false for point and vote sources, which are left as-is.
	Scalable bool `json:"scalable"`
	// AtBound is true when a fitted temperature sits on the grid's edge.
	AtBound   bool      `json:"atBound"`
	FoldTemps []float64 `json:"foldTemperatures"`
	Raw       Summary   `json:"raw"`
	CV        Summary   `json:"crossValidated"`
}

// CrossValidateTemperature fits temperature scaling with k folds split by case.
func CrossValidateTemperature(target string, ts []Trial, k int) CalibrationResult {
	if k < 2 {
		k = 5
	}
	ids := map[string]bool{}
	for _, t := range ts {
		ids[t.CaseID] = true
	}
	cases := make([]string, 0, len(ids))
	for id := range ids {
		cases = append(cases, id)
	}
	sort.Strings(cases) // deterministic folds
	fold := map[string]int{}
	for i, id := range cases {
		fold[id] = i % k
	}

	res := CalibrationResult{Target: target, Folds: k, Temperature: FitTemperature(ts), Raw: Summarize(target, ts), Scalable: Scalable(ts)}
	var cv []Trial
	for f := range k {
		var train, test []Trial
		for _, t := range ts {
			if fold[t.CaseID] == f {
				test = append(test, t)
			} else {
				train = append(train, t)
			}
		}
		temp := FitTemperature(train)
		res.FoldTemps = append(res.FoldTemps, temp)
		if temp <= minTemperature*1.0001 || temp >= maxTemperature*0.9999 {
			res.AtBound = true
		}
		cv = append(cv, Scaled(test, temp)...)
	}
	res.CV = Summarize(target+" (temperature-scaled, CV)", cv)
	return res
}

func answeredOnly(ts []Trial) []Trial {
	var out []Trial
	for _, t := range ts {
		if t.Err == "" {
			out = append(out, t)
		}
	}
	return out
}
