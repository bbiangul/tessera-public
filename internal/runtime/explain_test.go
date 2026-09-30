package runtime

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// explainGen passes SYSTEM on as its explanation, or answers what it is
// told to.
type explainGen struct {
	mu     sync.Mutex
	reply  string
	system []string
}

func (*explainGen) Name() string { return "explain" }
func (g *explainGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, _ := req.State.(map[string]any)
	s, _ := st["system"].(string)
	g.system = append(g.system, s)
	if g.reply != "" {
		return &generate.Response{Text: g.reply}, nil
	}
	return &generate.Response{Text: "O sistema da clínica informou: " + s}, nil
}

// claimDecider says an explanation claims the action when it has "feito".
type claimDecider struct{ Decider decide.Decider }

func (claimDecider) Name() string { return "claims" }
func (d claimDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	q, ok := req.Questions["claims"]
	if !ok {
		return d.Decider.Decide(ctx, req)
	}
	st, _ := req.State.(map[string]any)
	e, _ := st["reply"].(string)
	yes := 0.02
	if strings.Contains(e, "feito") {
		yes = 0.9
	}
	a, err := decide.AnswerFor(q, map[string]float64{"true": yes, "false": 1 - yes}, decide.ProbNative)
	if err != nil {
		return nil, err
	}
	return &decide.Response{Answers: map[string]decide.Answer{"claims": a}}, nil
}

func explainHarness(t *testing.T, g *explainGen, fail error) *harness {
	t.Helper()
	h, p := reviewHarness(t, fn("f", "p/send", `[ {respond: {template: "Não consegui remarcar."}} ]`))
	p.on["send"] = func(act.Call) (any, error) { return nil, fail }
	h.rt.Generator, h.rt.ExplainFailures = g, true
	h.rt.Decider = claimDecider{Decider: h.rt.Decider}
	return h
}

// drift-002: a definite refusal's words follow the failure's message; a
// public detail keeps the clinic's phone, a private one is masked first.
func TestDefiniteFailuresAreExplained(t *testing.T) {
	g := &explainGen{}
	h := explainHarness(t, g, &act.DefiniteError{Code: "FORBIDDEN", Message: "server answered 403", Detail: "online rescheduling is suspended: call (11) 3030-4040", Public: true})
	res := h.startR("f")
	expectStatus(t, res, journal.StatusFailed)
	last := h.user[len(h.user)-1]
	if last == "Não consegui remarcar." || !strings.Contains(last, "3030-4040") {
		t.Errorf("want the message rewritten with the system's words: %q", last)
	}

	g = &explainGen{}
	h = explainHarness(t, g, &act.DefiniteError{Code: "INVALID", Message: "server answered 422", Detail: "patient ana@example.com has a debt"})
	h.startR("f")
	if len(g.system) != 1 || strings.Contains(g.system[0], "ana@example.com") {
		t.Errorf("a private detail reaches the model masked: %v", g.system)
	}
	if last := h.user[len(h.user)-1]; last != "Não consegui remarcar." {
		t.Errorf("an explanation with a masked placeholder is dropped: %q", last)
	}
}

// The message stands when there is nothing to pass on, when the
// rewrite claims the action or loses one of the system's numbers, when
// the system said nothing, or when explaining is off.
func TestExplanationsAreDroppedOnDoubt(t *testing.T) {
	refusal := &act.DefiniteError{Code: "FORBIDDEN", Message: "server answered 403", Detail: "call (11) 3030-4040", Public: true}
	for name, c := range map[string]struct {
		g    *explainGen
		fail error
		off  bool
	}{
		"none":       {&explainGen{reply: "NONE"}, refusal, false},
		"claims":     {&explainGen{reply: "Pronto, está feito."}, refusal, false},
		"lost phone": {&explainGen{reply: "Não consegui remarcar: ligue para a clínica."}, refusal, false},
		"no detail":  {&explainGen{}, &act.DefiniteError{Code: "INVALID", Message: "server answered 400"}, false},
		"off":        {&explainGen{}, refusal, true},
	} {
		h := explainHarness(t, c.g, c.fail)
		h.rt.ExplainFailures = !c.off
		h.startR("f")
		if last := h.user[len(h.user)-1]; last != "Não consegui remarcar." {
			t.Errorf("%s: %q", name, last)
		}
	}
}
