package bench

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// GateConfig holds the regression gates of RFC-0002 (level 3). Zero values
// take the defaults.
type GateConfig struct {
	// Alpha for the one-sided McNemar test on cases that flipped; default 0.05.
	Alpha float64
	// RoutingThreshold is where selective accuracy is checked; default 0.7.
	RoutingThreshold float64
	// MinSelectiveAccuracy at RoutingThreshold; default 0.99.
	MinSelectiveAccuracy float64
	// MinAUROC; default 0.9. Skipped when AUROC is undefined (no errors at all).
	MinAUROC float64
	// MaxLatencyRatio of candidate p95 to baseline p95; default 1.5.
	MaxLatencyRatio float64
	// MaxCostRatio of candidate to baseline cost per question; default 1.2.
	MaxCostRatio float64
	// MaxErrors allowed in the candidate; default 0.
	MaxErrors int
	// MaxWorseCases is the most cases allowed to flip from right to wrong,
	// whatever McNemar says; default 2. Small suites need both: McNemar
	// alone cannot flag fewer than 5 flips at alpha 0.05. Negative means
	// none (zero is taken as "use the default").
	MaxWorseCases int
	// MaxCoverageDrop is the largest allowed drop in coverage at the routing
	// threshold, as a fraction; default 0.10. Negative means none.
	MaxCoverageDrop float64
	// SkipLatency disables the latency gate, for candidates measured in a
	// different environment than the baseline (e.g. CI runners vs a laptop).
	SkipLatency bool
}

func (c GateConfig) withDefaults() GateConfig {
	if c.Alpha == 0 {
		c.Alpha = 0.05
	}
	if c.RoutingThreshold == 0 {
		c.RoutingThreshold = 0.7
	}
	if c.MinSelectiveAccuracy == 0 {
		c.MinSelectiveAccuracy = 0.99
	}
	if c.MinAUROC == 0 {
		c.MinAUROC = 0.9
	}
	if c.MaxLatencyRatio == 0 {
		c.MaxLatencyRatio = 1.5
	}
	if c.MaxCostRatio == 0 {
		c.MaxCostRatio = 1.2
	}
	if c.MaxWorseCases == 0 {
		c.MaxWorseCases = 2
	}
	if c.MaxCoverageDrop == 0 {
		c.MaxCoverageDrop = 0.10
	}
	c.MaxWorseCases = max(c.MaxWorseCases, 0)
	c.MaxCoverageDrop = max(c.MaxCoverageDrop, 0)
	return c
}

// Check is one gate's outcome.
type Check struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

// GateResult is the outcome of comparing a candidate run with a baseline.
type GateResult struct {
	Baseline  Summary `json:"baseline"`
	Candidate Summary `json:"candidate"`
	Checks    []Check `json:"checks"`
	// Worse and Better list cases whose majority answer flipped.
	Worse  []string `json:"worse"`
	Better []string `json:"better"`
}

// Pass reports whether every check passed.
func (g GateResult) Pass() bool {
	for _, c := range g.Checks {
		if !c.Pass {
			return false
		}
	}
	return true
}

// Gate compares candidate trials with baseline trials of the same dataset.
func Gate(baseline, candidate []Trial, cfg GateConfig) GateResult {
	cfg = cfg.withDefaults()
	bs, cs := Summarize("baseline", baseline), Summarize("candidate", candidate)
	g := GateResult{Baseline: bs, Candidate: cs}

	// The candidate must answer exactly the baseline's cases, the same number
	// of times; a truncated or wrong-dataset run must never pass.
	g.Checks = append(g.Checks, sameCases(baseline, candidate))

	bRight, cRight := caseCorrectness(baseline), caseCorrectness(candidate)
	for id, br := range bRight {
		cr, ok := cRight[id]
		switch {
		case !ok:
			g.Worse = append(g.Worse, id) // a missing case counts as lost
		case br && !cr:
			g.Worse = append(g.Worse, id)
		case !br && cr:
			g.Better = append(g.Better, id)
		}
	}
	sort.Strings(g.Worse)
	sort.Strings(g.Better)
	p := McNemarWorse(len(g.Worse), len(g.Better))
	g.Checks = append(g.Checks, Check{
		Name:   "no significant accuracy regression (McNemar)",
		Pass:   !(len(g.Worse) > len(g.Better) && p < cfg.Alpha),
		Detail: fmt.Sprintf("%d cases worse, %d better, one-sided p = %.3f (alpha %.2f); case accuracy %.1f%% → %.1f%%", len(g.Worse), len(g.Better), p, cfg.Alpha, 100*bs.CaseAccuracy, 100*cs.CaseAccuracy),
	})
	g.Checks = append(g.Checks, Check{
		Name:   "at most a few cases lost",
		Pass:   len(g.Worse)-len(g.Better) <= cfg.MaxWorseCases,
		Detail: fmt.Sprintf("net %d cases lost (max %d)", len(g.Worse)-len(g.Better), cfg.MaxWorseCases),
	})

	bsel := selective(baseline, cfg.RoutingThreshold)
	sel := selective(candidate, cfg.RoutingThreshold)
	g.Checks = append(g.Checks, Check{
		Name:   fmt.Sprintf("selective accuracy at p ≥ %.2f", cfg.RoutingThreshold),
		Pass:   sel.Coverage > 0 && sel.Accuracy >= cfg.MinSelectiveAccuracy,
		Detail: fmt.Sprintf("%.1f%% on %.0f%% of answers (min %.1f%%)", 100*sel.Accuracy, 100*sel.Coverage, 100*cfg.MinSelectiveAccuracy),
	})
	g.Checks = append(g.Checks, Check{
		Name:   fmt.Sprintf("coverage at p ≥ %.2f", cfg.RoutingThreshold),
		Pass:   sel.Coverage >= bsel.Coverage-cfg.MaxCoverageDrop,
		Detail: fmt.Sprintf("%.0f%% → %.0f%% of answers acted on (max drop %.0f points)", 100*bsel.Coverage, 100*sel.Coverage, 100*cfg.MaxCoverageDrop),
	})

	aurocOK := cs.AUROC >= cfg.MinAUROC || (cs.AUROC < 0 && bs.AUROC < 0)
	g.Checks = append(g.Checks, Check{
		Name:   "confidence separates right from wrong (AUROC)",
		Pass:   aurocOK,
		Detail: fmt.Sprintf("%s → %s (min %.2f; undefined passes only if it was undefined before)", auroc(bs.AUROC), auroc(cs.AUROC), cfg.MinAUROC),
	})

	g.Checks = append(g.Checks, sameModels(baseline, candidate))

	if !cfg.SkipLatency {
		g.Checks = append(g.Checks, ratioCheck("p95 latency", float64(bs.LatencyP95), float64(cs.LatencyP95), cfg.MaxLatencyRatio,
			fmt.Sprintf("%s → %s", ms(bs.LatencyP95), ms(cs.LatencyP95))))
	}
	g.Checks = append(g.Checks, ratioCheck("cost per question", bs.CostPerQ, cs.CostPerQ, cfg.MaxCostRatio,
		fmt.Sprintf("$%.7f → $%.7f", bs.CostPerQ, cs.CostPerQ)))

	g.Checks = append(g.Checks, Check{
		Name:   "errors",
		Pass:   cs.Errors <= cfg.MaxErrors,
		Detail: fmt.Sprintf("%d (max %d)", cs.Errors, cfg.MaxErrors),
	})
	return g
}

// sameCases checks that the candidate answered every baseline case as many
// times as the baseline did, and nothing else.
func sameCases(baseline, candidate []Trial) Check {
	count := func(ts []Trial) map[string]int {
		m := map[string]int{}
		for _, t := range ts {
			m[t.CaseID]++
		}
		return m
	}
	b, c := count(baseline), count(candidate)
	var missing, extra, uneven int
	for id, n := range b {
		switch cn, ok := c[id]; {
		case !ok:
			missing++
		case cn != n:
			uneven++
		}
	}
	for id := range c {
		if _, ok := b[id]; !ok {
			extra++
		}
	}
	return Check{
		Name:   "same cases and repeats as the baseline",
		Pass:   missing == 0 && extra == 0 && uneven == 0,
		Detail: fmt.Sprintf("%d baseline cases; %d missing, %d not in the baseline, %d with a different number of repeats", len(b), missing, extra, uneven),
	}
}

// sameModels checks that every model and provider that answered the
// candidate also answered the baseline: a silent alias move to a new model
// version is a change that needs its own baseline.
func sameModels(baseline, candidate []Trial) Check {
	seen := map[string]bool{}
	for _, t := range baseline {
		seen[t.Model+"@"+t.Provider] = true
	}
	var unknown []string
	for _, t := range candidate {
		k := t.Model + "@" + t.Provider
		if t.Err == "" && !seen[k] {
			seen[k] = true
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	d := "every model/provider was in the baseline"
	if len(unknown) > 0 {
		d = "not in the baseline: " + strings.Join(unknown, ", ")
	}
	return Check{Name: "same model versions as the baseline", Pass: len(unknown) == 0, Detail: d}
}

func ratioCheck(name string, base, cand, maxRatio float64, detail string) Check {
	c := Check{Name: name, Pass: true, Detail: detail}
	if base > 0 {
		r := cand / base
		c.Pass = r <= maxRatio
		c.Detail += fmt.Sprintf(" (×%.2f, max ×%.2f)", r, maxRatio)
	}
	return c
}

// caseCorrectness returns, per case, whether the majority answer was right.
func caseCorrectness(ts []Trial) map[string]bool {
	byCase := map[string][]Trial{}
	for _, t := range ts {
		byCase[t.CaseID] = append(byCase[t.CaseID], t)
	}
	out := make(map[string]bool, len(byCase))
	for id, cts := range byCase {
		pred, _ := majority(cts)
		out[id] = pred == cts[0].Gold
	}
	return out
}

// McNemarWorse is the exact one-sided McNemar p-value that the candidate is
// worse: P(X >= worse) for X ~ Binomial(worse+better, 1/2).
func McNemarWorse(worse, better int) float64 {
	n := worse + better
	if n == 0 {
		return 1
	}
	var p float64
	for k := worse; k <= n; k++ {
		p += math.Exp(logChoose(n, k) - float64(n)*math.Ln2)
	}
	return math.Min(1, p)
}

func logChoose(n, k int) float64 {
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}

// Markdown renders a gate result.
func (g GateResult) Markdown(title string) string {
	var b strings.Builder
	verdict := "PASS"
	if !g.Pass() {
		verdict = "FAIL"
	}
	fmt.Fprintf(&b, "## %s: %s\n\n| Gate | Result | Detail |\n| --- | --- | --- |\n", title, verdict)
	for _, c := range g.Checks {
		mark := "pass"
		if !c.Pass {
			mark = "**FAIL**"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", c.Name, mark, c.Detail)
	}
	if len(g.Worse) > 0 {
		fmt.Fprintf(&b, "\nCases that got worse: %s\n", strings.Join(g.Worse, ", "))
	}
	if len(g.Better) > 0 {
		fmt.Fprintf(&b, "\nCases that got better: %s\n", strings.Join(g.Better, ", "))
	}
	return b.String()
}
