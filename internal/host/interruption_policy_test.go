package host

import (
	"context"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime/journal"
	"testing"
)

type splitSideDecider struct {
	turnDecider
	probs map[string]float64
}

func (d splitSideDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if q, ok := req.Questions["turn"]; ok {
		a, err := decide.AnswerFor(q, d.probs, decide.ProbNative)
		return &decide.Response{Answers: map[string]decide.Answer{"turn": a}}, err
	}
	return d.turnDecider.Decide(ctx, req)
}

func TestSplitSideRequestAnswersBeforeConfirming(t *testing.T) {
	h, booked, _ := freeHost(t)
	say(t, h, "quero marcar terça")
	h.opts.Decider = splitSideDecider{probs: map[string]float64{"answer": 0, "question": .43, "request": .57, "cancel": 0, "goodbye": 0}}
	r := say(t, h, "antes de confirmar: tem estacionamento?")
	if r.Status != journal.StatusConfirming || len(*booked) != 0 || len(r.Outbox) != 2 {
		t.Fatalf("side request ignored or acted: %+v booked=%v", r, *booked)
	}
	h.opts.Decider = turnDecider{}
	r = say(t, h, "sim")
	if r.Status != journal.StatusSucceeded || len(*booked) != 1 {
		t.Fatalf("fresh confirmation failed: %+v booked=%v", r, *booked)
	}
}

func TestInterruptionNeverAddsPermissionToSideMass(t *testing.T) {
	for _, tc := range []struct {
		p    map[string]float64
		want string
	}{
		{map[string]float64{"answer": .4, "question": .3, "request": .2, "cancel": .1, "goodbye": 0}, ""},
		{map[string]float64{"answer": .7, "question": .1, "request": .2, "cancel": 0, "goodbye": 0}, turnAnswer},
		{map[string]float64{"answer": .1, "question": .05, "request": .05, "cancel": .8, "goodbye": 0}, turnCancel},
	} {
		h, _, _ := freeHost(t)
		h.opts.Decider = splitSideDecider{probs: tc.p}
		got, err := h.interruption(context.Background(), "Posso remarcar?", "resposta")
		if err != nil || got != tc.want {
			t.Fatalf("got %q, want %q: %v", got, tc.want, err)
		}
	}
}
