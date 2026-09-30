package metrics

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Variant is one arm of an experiment.
type Variant struct {
	Name   string
	Weight int
}

// Assign picks a subject's variant: a hash of the experiment and the
// subject, so the same subject always gets the same variant, on any
// process, in any order, and replays agree.
func Assign(experiment, subject string, variants []Variant) string {
	total := 0
	for _, v := range variants {
		total += v.Weight
	}
	if total <= 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(experiment + "\x00" + subject))
	x := int(binary.BigEndian.Uint64(sum[:8]) % uint64(total))
	for _, v := range variants {
		if x < v.Weight {
			return v.Name
		}
		x -= v.Weight
	}
	return variants[len(variants)-1].Name
}

// Arm is what one variant did.
type Arm struct {
	Name       string  `json:"name"`
	Exposed    int     `json:"exposed"`   // subjects that saw it
	Decided    int     `json:"decided"`   // converted or out of the window
	Converted  int     `json:"converted"` // reached the primary stage
	Rate       float64 `json:"rate"`      // Converted / Decided
	Low        float64 `json:"low"`       // 95% Wilson interval of Rate
	High       float64 `json:"high"`
	Difference float64 `json:"difference"` // Rate minus the control's
}

// Comparison is a variant against the control.
type Comparison struct {
	Variant string `json:"variant"`
	// Evidence is the mixture sequential probability ratio (mSPRT, normal
	// approximation): an always-valid test, so looking every day does not
	// raise the false positive rate. Significant when Evidence >= 1/alpha.
	Evidence    float64 `json:"evidence"`
	Significant bool    `json:"significant"`
	// PValue is the fixed-horizon two-proportion z test, for reference
	// only: it is valid only when read once, at a sample size fixed in
	// advance.
	PValue float64 `json:"pValue"`
}

// Verdict is the analysis of an experiment.
type Verdict struct {
	Arms        []Arm        `json:"arms"`
	Comparisons []Comparison `json:"comparisons"`
	// Decision: inconclusive (too few subjects, or no evidence yet),
	// <variant> better or worse than the control.
	Decision string `json:"decision"`
}

// Tau2 is the mixing variance of the mSPRT prior on the difference of
// rates: effects around ±0.1 (10 points) are what the test looks for.
const Tau2 = 0.01

// Analyze compares each variant with the first (the control) at level
// alpha, after minSample decided subjects per variant.
func Analyze(arms []Arm, minSample int, alpha float64) Verdict {
	v := Verdict{Decision: "inconclusive"}
	for i := range arms {
		a := &arms[i]
		if a.Decided > 0 {
			a.Rate = float64(a.Converted) / float64(a.Decided)
		}
		a.Low, a.High = wilson(a.Converted, a.Decided)
		if i > 0 {
			a.Difference = a.Rate - arms[0].Rate
		}
	}
	v.Arms = arms
	if len(arms) < 2 {
		return v
	}
	ready := true
	for _, a := range arms {
		ready = ready && a.Decided >= minSample
	}
	c := arms[0]
	for _, a := range arms[1:] {
		cmp := Comparison{Variant: a.Name, Evidence: msprt(c, a), PValue: zTest(c, a)}
		cmp.Significant = ready && cmp.Evidence >= 1/alpha
		v.Comparisons = append(v.Comparisons, cmp)
		if cmp.Significant && v.Decision == "inconclusive" {
			if a.Rate > c.Rate {
				v.Decision = a.Name + " better than " + c.Name
			} else {
				v.Decision = a.Name + " worse than " + c.Name
			}
		}
	}
	return v
}

func wilson(k, n int) (float64, float64) {
	if n == 0 {
		return 0, 0
	}
	const z = 1.959964
	p := float64(k) / float64(n)
	nf := float64(n)
	den := 1 + z*z/nf
	mid := (p + z*z/(2*nf)) / den
	half := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / den
	return math.Max(0, mid-half), math.Min(1, mid+half)
}

func variance(a, b Arm) float64 {
	pa, pb := float64(a.Converted)/float64(a.Decided), float64(b.Converted)/float64(b.Decided)
	return pa*(1-pa)/float64(a.Decided) + pb*(1-pb)/float64(b.Decided)
}

// msprt is the mixture likelihood ratio for the difference of two rates
// with a N(0, Tau2) mixing prior (Johari et al., "Always Valid Inference").
func msprt(a, b Arm) float64 {
	if a.Decided == 0 || b.Decided == 0 {
		return 0
	}
	v := variance(a, b)
	if v == 0 {
		return 0
	}
	d := float64(b.Converted)/float64(b.Decided) - float64(a.Converted)/float64(a.Decided)
	return math.Sqrt(v/(v+Tau2)) * math.Exp(Tau2*d*d/(2*v*(v+Tau2)))
}

func zTest(a, b Arm) float64 {
	if a.Decided == 0 || b.Decided == 0 {
		return 1
	}
	pooled := float64(a.Converted+b.Converted) / float64(a.Decided+b.Decided)
	se := math.Sqrt(pooled * (1 - pooled) * (1/float64(a.Decided) + 1/float64(b.Decided)))
	if se == 0 {
		return 1
	}
	z := (float64(b.Converted)/float64(b.Decided) - float64(a.Converted)/float64(a.Decided)) / se
	return math.Erfc(math.Abs(z) / math.Sqrt2)
}

// ExperimentReport is an experiment's arms and verdict.
type ExperimentReport struct {
	Experiment string  `json:"experiment"`
	Verdict    Verdict `json:"verdict"`
	// SRM is the p-value of a sample ratio mismatch test (exposures
	// against the weights; two variants). Below 0.001 the assignment or the
	// exposure logging is broken and the verdict must not be trusted.
	SRM float64 `json:"srm"`
	// Guardrails, when the experiment has any.
	Guardrails []GuardrailResult `json:"guardrails,omitempty"`
	// Stopped is set when a guardrail stopped the experiment: subjects
	// exposed since get the control and are left out of the comparison.
	Stopped bool `json:"stopped,omitempty"`
}

// AnalyzeExperiment reads the exposures (the milestone
// "experiment/<name>", first per subject) and, per variant, the funnel's
// primary stage over the exposed subjects.
func AnalyzeExperiment(x *agents.Experiment, f *agents.Funnel, ms []journal.Milestone, now time.Time, loc *time.Location) (ExperimentReport, error) {
	exposed := Exposures(x.Metadata.Name, ms, now)
	variant := map[string]string{} // subject -> first variant seen
	for id, e := range exposed {
		variant[id] = e.Variant
	}
	stage := -1
	for i, st := range f.Spec.Stages {
		if st.Name == x.Spec.Primary.Stage {
			stage = i - 1 // Stages in a cohort start at the second stage
		}
	}
	if stage < 0 {
		return ExperimentReport{}, fmt.Errorf("funnel %s has no stage %q after the entry", f.Metadata.Name, x.Spec.Primary.Stage)
	}
	var arms []Arm
	for _, v := range x.Spec.Variants {
		var sub []journal.Milestone
		exposed := 0
		for _, m := range ms {
			if variant[m.ID] == v.Name {
				sub = append(sub, m)
			}
		}
		for _, name := range variant {
			if name == v.Name {
				exposed++
			}
		}
		rep, err := Compute(f, sub, now, loc, "")
		if err != nil {
			return ExperimentReport{}, err
		}
		a := Arm{Name: v.Name, Exposed: exposed}
		for _, c := range rep.Cohorts {
			a.Converted += c.Stages[stage].Reached
			a.Decided += c.Stages[stage].Reached + c.Stages[stage].Lost
		}
		arms = append(arms, a)
	}
	minSample, alpha := x.Spec.MinSamplePerVariant, x.Spec.Alpha
	if minSample == 0 {
		minSample = 100
	}
	if alpha == 0 {
		alpha = 0.05
	}
	out := ExperimentReport{Experiment: x.Metadata.Name, Verdict: Analyze(arms, minSample, alpha), SRM: 1}
	if len(arms) == 2 {
		out.SRM = srm(arms[0].Exposed, arms[1].Exposed, x.Spec.Variants[0].Weight, x.Spec.Variants[1].Weight)
	}
	names := make([]string, len(x.Spec.Variants))
	for i, v := range x.Spec.Variants {
		names[i] = v.Name
	}
	for _, g := range x.Spec.Guardrails {
		within, err := ParseDuration(g.Within)
		if err != nil {
			return ExperimentReport{}, fmt.Errorf("guardrail %s: %w", g.Name, err)
		}
		out.Guardrails = append(out.Guardrails, CheckGuardrail(Guardrail{Name: g.Name, Milestone: g.Milestone, Within: within, MaxIncrease: g.MaxIncrease},
			names, exposed, ms, now, alpha))
	}
	return out, nil
}

// srm is the chi-square test (1 degree of freedom) of two exposure counts
// against their weights.
func srm(a, b, wa, wb int) float64 {
	n := float64(a + b)
	if n == 0 {
		return 1
	}
	ea := n * float64(wa) / float64(wa+wb)
	eb := n - ea
	chi := (float64(a)-ea)*(float64(a)-ea)/ea + (float64(b)-eb)*(float64(b)-eb)/eb
	return math.Erfc(math.Sqrt(chi / 2))
}
