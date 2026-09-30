package metrics

import (
	"time"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Guardrail is an outcome that must not rise in any variant.
type Guardrail struct {
	Name, Milestone string
	Within          time.Duration
	MaxIncrease     float64
}

// GuardrailArm is a variant's rate of the guarded outcome.
type GuardrailArm struct {
	Variant string  `json:"variant"`
	Decided int     `json:"decided"` // had it, or past the window without it
	Hit     int     `json:"hit"`
	Rate    float64 `json:"rate"`
}

// GuardrailResult is one guardrail across the variants.
type GuardrailResult struct {
	Name string         `json:"name"`
	Arms []GuardrailArm `json:"arms"`
	// Breach names the first variant significantly worse than the control
	// by more than the allowed increase; "" when none.
	Breach     string  `json:"breach,omitempty"`
	Difference float64 `json:"difference,omitempty"`
	Evidence   float64 `json:"evidence,omitempty"`
}

// Exposure is a subject's first exposure to an experiment.
type Exposure struct {
	Variant string
	At      time.Time
}

// Exposures reads the first exposure of each subject to an experiment
// that happened while it ran: exposures after a stop (attribute stopped)
// only give the control and are not part of the comparison.
func Exposures(experiment string, ms []journal.Milestone, now time.Time) map[string]Exposure {
	name := "experiment/" + experiment
	out := map[string]Exposure{}
	for _, m := range ms {
		if m.Name != name || m.At.After(now) || m.Attributes["stopped"] == "true" {
			continue
		}
		if _, ok := out[m.ID]; !ok {
			out[m.ID] = Exposure{Variant: m.Attributes["variant"], At: m.At}
		}
	}
	return out
}

// CheckGuardrail computes a guardrail per variant (the first is the
// control). A variant breaches it when the sequential test (mSPRT, the
// same as the verdict's) finds it different from the control at level
// alpha and it is worse by more than MaxIncrease. Measured in
// docs/research/experiments-002.md: read 200 times, A/A experiments
// stopped 1 in 400 (3% base) and 4 in 400 (10%); +5 points over a 3% base
// with a 2-point margin stopped every experiment, at a median of 400
// subjects per arm; +1 point, inside the margin, 37 in 400.
func CheckGuardrail(g Guardrail, variants []string, exposed map[string]Exposure, ms []journal.Milestone, now time.Time, alpha float64) GuardrailResult {
	first := map[string]time.Time{} // subject -> first time of the outcome after its exposure
	for _, m := range ms {
		x, ok := exposed[m.ID]
		if !ok || m.Name != g.Milestone || m.At.Before(x.At) || m.At.After(now) || m.At.After(x.At.Add(g.Within)) {
			continue
		}
		if t, ok := first[m.ID]; !ok || m.At.Before(t) {
			first[m.ID] = m.At
		}
	}
	res := GuardrailResult{Name: g.Name}
	arms := map[string]*GuardrailArm{}
	for _, v := range variants {
		res.Arms = append(res.Arms, GuardrailArm{Variant: v})
	}
	for i := range res.Arms {
		arms[res.Arms[i].Variant] = &res.Arms[i]
	}
	for id, x := range exposed {
		a := arms[x.Variant]
		if a == nil {
			continue
		}
		switch _, hit := first[id]; {
		case hit:
			a.Decided++
			a.Hit++
		case now.After(x.At.Add(g.Within)):
			a.Decided++
		}
	}
	for i := range res.Arms {
		if a := &res.Arms[i]; a.Decided > 0 {
			a.Rate = float64(a.Hit) / float64(a.Decided)
		}
	}
	if len(res.Arms) < 2 {
		return res
	}
	c := res.Arms[0]
	control := Arm{Decided: c.Decided, Converted: c.Hit}
	for _, a := range res.Arms[1:] {
		ev := msprt(control, Arm{Decided: a.Decided, Converted: a.Hit})
		if d := a.Rate - c.Rate; ev >= 1/alpha && d > g.MaxIncrease && res.Breach == "" {
			res.Breach, res.Difference, res.Evidence = a.Variant, d, ev
		}
	}
	return res
}
