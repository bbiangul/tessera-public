package metrics

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func TestAssignIsStableAndWeighted(t *testing.T) {
	vs := []Variant{{"controle", 80}, {"prova-social", 20}}
	first := Assign("x", "lead-1", vs)
	for range 5 {
		if got := Assign("x", "lead-1", append([]Variant(nil), vs...)); got != first {
			t.Fatalf("the same subject must get the same variant: %s then %s", first, got)
		}
	}
	n, b := 20000, 0
	for i := 0; i < n; i++ {
		if Assign("primeira-mensagem", fmt.Sprintf("lead-%d", i), vs) == "prova-social" {
			b++
		}
	}
	if share := float64(b) / float64(n); math.Abs(share-0.2) > 0.01 {
		t.Errorf("weights 80/20 gave %.3f to the 20", share)
	}
	// Another experiment reshuffles: not the same people in the same arm.
	same := 0
	for i := 0; i < 1000; i++ {
		s := fmt.Sprintf("lead-%d", i)
		if Assign("a", s, []Variant{{"x", 50}, {"y", 50}}) == Assign("b", s, []Variant{{"x", 50}, {"y", 50}}) {
			same++
		}
	}
	if same < 430 || same > 570 {
		t.Errorf("experiments are not independent: %d of 1000 agree", same)
	}
}

func TestKnownValues(t *testing.T) {
	lo, hi := wilson(50, 100)
	if math.Abs(lo-0.4038) > 1e-3 || math.Abs(hi-0.5962) > 1e-3 {
		t.Errorf("wilson(50,100) = %.4f %.4f", lo, hi)
	}
	p := zTest(Arm{Converted: 100, Decided: 1000}, Arm{Converted: 130, Decided: 1000})
	if math.Abs(p-0.0355) > 1e-3 {
		t.Errorf("z test p = %.4f, want 0.0355", p)
	}
	v := Analyze([]Arm{{Name: "a", Converted: 10, Decided: 50}, {Name: "b", Converted: 40, Decided: 50}}, 100, 0.05)
	if v.Decision != "inconclusive" {
		t.Errorf("below the minimum sample nothing is decided: %+v", v)
	}
}

// The reason for the sequential test, measured: in A/A experiments looked
// at 20 times, the fixed-horizon z test "finds" a winner far more often
// than alpha; the mSPRT stays under it, and still finds a real 5-point
// difference.
func TestPeekingSimulation(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	run := func(pa, pb float64) (zHits, mHits int) {
		const trials, looks, perLook = 400, 20, 100
		for tr := 0; tr < trials; tr++ {
			a, b := Arm{Name: "a"}, Arm{Name: "b"}
			zHit, mHit := false, false
			for l := 0; l < looks; l++ {
				for i := 0; i < perLook; i++ {
					a.Decided++
					b.Decided++
					if r.Float64() < pa {
						a.Converted++
					}
					if r.Float64() < pb {
						b.Converted++
					}
				}
				zHit = zHit || zTest(a, b) < 0.05
				mHit = mHit || msprt(a, b) >= 1/0.05
			}
			if zHit {
				zHits++
			}
			if mHit {
				mHits++
			}
		}
		return zHits, mHits
	}
	zAA, mAA := run(0.10, 0.10)
	t.Logf("A/A, 20 looks: z test false positives %d/400, mSPRT %d/400", zAA, mAA)
	if float64(mAA)/400 > 0.05 {
		t.Errorf("mSPRT false positive rate %.3f exceeds alpha", float64(mAA)/400)
	}
	if zAA <= 2*mAA || float64(zAA)/400 < 0.10 {
		t.Errorf("the simulation should show peeking inflating the z test: z %d, mSPRT %d", zAA, mAA)
	}
	_, mAB := run(0.10, 0.15)
	t.Logf("A/B 10%% vs 15%%, up to 2000 per arm: mSPRT found it %d/400", mAB)
	if float64(mAB)/400 < 0.7 {
		t.Errorf("mSPRT power %.2f is too low for a 5-point difference at 2000 per arm", float64(mAB)/400)
	}
}

// End to end on milestones: leads exposed to two first messages, the
// variant with more meetings wins once there is enough evidence, and a
// broken 50/50 split is flagged.
func TestAnalyzeExperiment(t *testing.T) {
	x := &agents.Experiment{Metadata: agents.ObjectMeta{Name: "primeira-mensagem"}, Spec: agents.ExperimentSpec{
		Subject: "lead", Variants: []agents.ExperimentVariant{{Name: "controle", Weight: 50}, {Name: "prova-social", Weight: 50}},
		Primary: agents.PrimaryMetric{Funnel: "prospeccao", Stage: "reuniao"}, MinSamplePerVariant: 200}}
	f := &agents.Funnel{Metadata: agents.ObjectMeta{Name: "prospeccao"}, Spec: agents.FunnelSpec{Subject: "lead", Cohort: "week",
		Stages: []agents.FunnelStage{{Name: "contato", Milestone: "contatado"}, {Name: "reuniao", Milestone: "reuniao-marcada", Within: "14d"}}}}
	r := rand.New(rand.NewPCG(7, 7))
	var data []journal.Milestone
	start := day(8, 3)
	for i := 0; i < 2000; i++ {
		id := fmt.Sprintf("lead-%d", i)
		at := start.Add(time.Duration(i) * 30 * time.Minute)
		v := Assign(x.Metadata.Name, id, []Variant{{"controle", 50}, {"prova-social", 50}})
		data = append(data, ms(id, "experiment/primeira-mensagem", at, "variant", v), ms(id, "contatado", at))
		rate := 0.10
		if v == "prova-social" {
			rate = 0.16
		}
		if r.Float64() < rate {
			data = append(data, ms(id, "reuniao-marcada", at.Add(3*24*time.Hour)))
		}
	}
	now := day(10, 30) // every window closed
	rep, err := AnalyzeExperiment(x, f, data, now, sp)
	if err != nil {
		t.Fatal(err)
	}
	a, b := rep.Verdict.Arms[0], rep.Verdict.Arms[1]
	t.Logf("controle %d/%d, prova-social %d/%d, evidence %.1f, SRM p %.3f", a.Converted, a.Decided, b.Converted, b.Decided, rep.Verdict.Comparisons[0].Evidence, rep.SRM)
	if a.Exposed+b.Exposed != 2000 || a.Decided != a.Exposed || rep.Verdict.Decision != "prova-social better than controle" {
		t.Errorf("verdict %+v", rep.Verdict)
	}
	if rep.SRM < 0.001 {
		t.Errorf("a hash split must pass the SRM check: %v", rep.SRM)
	}
	// Losing a third of one arm's exposures (a logging bug) is flagged.
	var broken []journal.Milestone
	dropped := 0
	for _, m := range data {
		if m.Name == "experiment/primeira-mensagem" && m.Attributes["variant"] == "controle" && dropped%3 == 0 {
			dropped++
			continue
		}
		if m.Name == "experiment/primeira-mensagem" && m.Attributes["variant"] == "controle" {
			dropped++
		}
		broken = append(broken, m)
	}
	rep, _ = AnalyzeExperiment(x, f, broken, now, sp)
	if rep.SRM >= 0.001 {
		t.Errorf("a broken split must fail the SRM check: %v", rep.SRM)
	}
}

// Guardrails (docs/research/experiments-002.md): a variant is stopped when
// the mSPRT finds it different from the control and it is worse by more
// than the margin. Read at every look, with a 2-point margin.
func TestGuardrailSimulation(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	const margin, alpha = 0.02, 0.05
	run := func(pa, pb float64, looks, perLook int) (stops int, medianN int) {
		var at []int
		for tr := 0; tr < 400; tr++ {
			a, b := Arm{}, Arm{}
			for l := 0; l < looks; l++ {
				for i := 0; i < perLook; i++ {
					a.Decided++
					b.Decided++
					if r.Float64() < pa {
						a.Converted++
					}
					if r.Float64() < pb {
						b.Converted++
					}
				}
				d := float64(b.Converted)/float64(b.Decided) - float64(a.Converted)/float64(a.Decided)
				if msprt(a, b) >= 1/alpha && d > margin {
					at = append(at, a.Decided)
					break
				}
			}
		}
		if len(at) == 0 {
			return 0, 0
		}
		slices.Sort(at)
		return len(at), at[len(at)/2]
	}
	for _, c := range []struct {
		name          string
		pa, pb        float64
		looks, per    int
		atLeast, most int // stops out of 400
	}{
		{"A/A 3%, 20 looks", 0.03, 0.03, 20, 100, 0, 8},
		{"A/A 3%, 200 looks", 0.03, 0.03, 200, 10, 0, 8},
		{"A/A 10%, 200 looks", 0.10, 0.10, 200, 10, 0, 20},
		{"+1 point (inside the margin)", 0.03, 0.04, 20, 100, 0, 60},
		{"+5 points", 0.03, 0.08, 20, 100, 380, 400},
		{"+10 points", 0.03, 0.13, 20, 100, 400, 400},
	} {
		stops, n := run(c.pa, c.pb, c.looks, c.per)
		t.Logf("%s: stopped %d/400, median %d subjects per arm", c.name, stops, n)
		if stops < c.atLeast || stops > c.most {
			t.Errorf("%s: %d stops, want %d..%d", c.name, stops, c.atLeast, c.most)
		}
	}
}

func TestCheckGuardrail(t *testing.T) {
	day := 24 * time.Hour
	t0 := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	var ms []journal.Milestone
	add := func(id, name, variant string, at time.Time, stopped bool) {
		attrs := map[string]string{}
		if variant != "" {
			attrs["variant"] = variant
		}
		if stopped {
			attrs["stopped"] = "true"
		}
		ms = append(ms, journal.Milestone{ID: id, Name: name, At: at, Attributes: attrs})
	}
	for i := range 200 {
		a, b := fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i)
		add(a, "experiment/x", "control", t0, false)
		add(b, "experiment/x", "rude", t0, false)
		if i < 6 { // 3%
			add(a, "opt-out", "", t0.Add(day), false)
		}
		if i < 40 { // 20%
			add(b, "opt-out", "", t0.Add(day), false)
		}
	}
	add("b0", "opt-out", "", t0.Add(30*day), false)  // outside the window: not counted again
	add("late", "experiment/x", "control", t0, true) // after a stop: left out
	g := Guardrail{Name: "opt-out", Milestone: "opt-out", Within: 14 * day, MaxIncrease: 0.02}
	early := CheckGuardrail(g, []string{"control", "rude"}, Exposures("x", ms, t0.Add(12*time.Hour)), ms, t0.Add(12*time.Hour), 0.05)
	if early.Breach != "" || early.Arms[0].Decided != 0 {
		t.Fatalf("inside the window everyone is pending: %+v", early)
	}
	now := t0.Add(15 * day)
	res := CheckGuardrail(g, []string{"control", "rude"}, Exposures("x", ms, now), ms, now, 0.05)
	if res.Arms[0].Decided != 200 || res.Arms[0].Hit != 6 || res.Arms[1].Hit != 40 {
		t.Fatalf("arms %+v", res.Arms)
	}
	if res.Breach != "rude" || res.Difference < 0.16 {
		t.Fatalf("a 17-point increase breaches a 2-point guardrail: %+v", res)
	}
	g.MaxIncrease = 0.2
	if res := CheckGuardrail(g, []string{"control", "rude"}, Exposures("x", ms, now), ms, now, 0.05); res.Breach != "" {
		t.Errorf("inside the margin: %+v", res)
	}
}
