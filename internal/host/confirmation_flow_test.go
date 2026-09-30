package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

type failedFreeGenerator struct{}

func (failedFreeGenerator) Name() string { return "failed" }
func (failedFreeGenerator) Generate(context.Context, generate.Request) (*generate.Response, error) {
	return nil, errors.New("unavailable")
}

type failedTurnDecider struct{ turnDecider }

func (d failedTurnDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if _, ok := req.Questions["turn"]; ok {
		return nil, errors.New("unavailable")
	}
	return d.turnDecider.Decide(ctx, req)
}

// A failure to answer or classify a side question must never deliver its
// leading yes to the pending effect confirmation.
func TestMixedConfirmationFailsClosed(t *testing.T) {
	for _, failure := range []string{"generate", "decide"} {
		t.Run(failure, func(t *testing.T) {
			h, booked, _ := freeHost(t)
			say(t, h, "quero marcar terça")
			if failure == "generate" {
				h.Runtime.Generator = failedFreeGenerator{}
			} else {
				h.opts.Decider = failedTurnDecider{}
			}
			r, err := h.Handle(context.Background(), "c1", map[string]any{"name": "Ana"}, "sim, mas antes: tem estacionamento?")
			active, ok, storeErr := h.opts.Store.ActiveRun(context.Background(), "c1")
			if len(*booked) != 0 || storeErr != nil || !ok || active.Status != journal.StatusConfirming {
				t.Fatalf("acted on an unresolved question: reply=%+v error=%v booked=%v active=%+v", r, err, *booked, active)
			}
		})
	}
}

type historyRouteDecider struct {
	turnDecider
	sawCompleted bool
	history      string
}

func (d *historyRouteDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	st, _ := req.State.(map[string]any)
	text, _ := st["message"].(string)
	if q, ok := req.Questions["route"]; ok && strings.HasPrefix(text, "Confirmo então") {
		d.history = fmt.Sprint(st["conversation"])
		d.sawCompleted = strings.Contains(d.history, "Marcado: terça 10h")
		pick := "marcar"
		if d.sawCompleted {
			pick = "none"
		}
		probs := map[string]float64{}
		for _, o := range q.Options {
			probs[o.Key] = 0.01
		}
		probs[pick] = 0.99
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		return &decide.Response{Answers: map[string]decide.Answer{"route": a}}, err
	}
	return d.turnDecider.Decide(ctx, req)
}

func TestLateConfirmationUsesHistoryAndNewRequestStillRoutes(t *testing.T) {
	h, booked, _ := freeHost(t)
	d := &historyRouteDecider{}
	h.opts.Decider = d
	say(t, h, "quero marcar terça")
	say(t, h, "sim")
	say(t, h, "tem estacionamento?")
	r := say(t, h, "Confirmo então marcar terça, obrigado!")
	if !d.sawCompleted || !r.Unrouted || len(*booked) != 1 {
		t.Fatalf("lost completed work: history=%s reply=%+v effects=%v", d.history, r, *booked)
	}
	if r = say(t, h, "quero marcar quarta"); r.Status != journal.StatusConfirming {
		t.Fatalf("a genuinely new request must still route: %+v", r)
	}
}

func TestRoutingHistoryIdentityAndMasking(t *testing.T) {
	h, _, _ := freeHost(t)
	ctx := context.Background()
	caller := map[string]any{"name": "Ana", "email": "ana@example.com"}
	if _, err := h.Handle(ctx, "private", caller, "tem estacionamento? meu email é ana@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.routingHistory(ctx, "private", map[string]any{"name": "Other"}); err == nil {
		t.Fatal("accepted another caller")
	}
	old := h.opts.Namespace
	h.opts.Namespace = "other"
	if _, err := h.routingHistory(ctx, "private", caller); err == nil {
		t.Fatal("accepted another namespace")
	}
	h.opts.Namespace = old
	lines, err := h.routingHistory(ctx, "private", caller)
	if err != nil || len(lines) == 0 {
		t.Fatalf("%v %v", lines, err)
	}
	d := &historyRouteDecider{}
	h.opts.Decider = d
	if _, err := h.route(ctx, "Confirmo então", "", "", lines, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(d.history, "ana@example.com") || !strings.Contains(d.history, "[EMAIL]") {
		t.Fatalf("history not masked: %s", d.history)
	}
}

type uncertainTurnDecider struct{ turnDecider }

func (d uncertainTurnDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if q, ok := req.Questions["turn"]; ok {
		probs := map[string]float64{}
		for _, o := range q.Options {
			probs[o.Key] = 1 / float64(len(q.Options))
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		return &decide.Response{Answers: map[string]decide.Answer{"turn": a}}, err
	}
	return d.turnDecider.Decide(ctx, req)
}

func TestUncertainConfirmationWaitsForAnotherReply(t *testing.T) {
	h, booked, _ := freeHost(t)
	say(t, h, "quero marcar terça")
	h.opts.Decider = uncertainTurnDecider{}
	r := say(t, h, "sim, mas antes: tem estacionamento?")
	// agency-007: the question is answered, then the confirmation asked
	// again; an unsure yes still books nothing.
	if r.Status != journal.StatusConfirming || len(*booked) != 0 || len(r.Outbox) != 2 ||
		r.Outbox[0].Text != "Sim, temos estacionamento: conveniado ao lado." || r.Outbox[1].Text != "Confirmo terça 10h?" {
		t.Fatalf("%+v %v", r, *booked)
	}
	h.opts.Decider = turnDecider{}
	r = say(t, h, "sim")
	if r.Status != journal.StatusSucceeded || len(*booked) != 1 {
		t.Fatalf("%+v %v", r, *booked)
	}
}
