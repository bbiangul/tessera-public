package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const stepsPlugin = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: booking, version: 1.0.0 }
spec:
  actions:
    search-flights: { input: { to: string }, output: "list<booking.Offer>", effect: read }
    book: { input: { token: string }, output: booking.Booking, effect: irreversible, verify: { mode: native } }
`

const stepsScript = `
def rank(offers, max_price = 10000):
    ok = [o for o in offers if o["price"] <= max_price]
    return sorted(ok, key = lambda o: (o["price"], o["token"]))
`

const stepsFunction = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: book-flight, namespace: travel }
spec:
  description: Finds the cheapest flight and books it.
  inputs:
    to: { type: string, resolve: [ { id: v, value: "'LIS'" } ] }
  steps:
    - id: offers
      act: booking/search-flights
      with: { to: "${ inputs.to }" }
    - id: ranked
      compute:
        starlark: { script: rank.star, function: rank, with: { offers: "${ steps.offers.output }" } }
    - id: best
      compute: { cel: "steps.ranked.output[0]" }
    - id: urgent
      decide:
        state: { message: "${ event.message }" }
        instructions: Is the trip urgent?
        yesNo: { ifTrue: travels within 48 hours }
        minConfidence: 0.8
        onLowConfidence: continue
    - id: hurry
      when: "steps.urgent.output.value == true"
      respond: { template: "Vou priorizar voos para hoje." }
    - id: summary
      branch:
        - when: "size(steps.ranked.output) == 1"
          respond: { template: "Achei um voo." }
        - else: true
          respond: { template: "Achei {{ size(steps.ranked.output) }} voos." }
  action:
    act: booking/book
    with: { token: "${ steps.best.output.token }" }
    confirm: "Reservo o voo de {{ steps.best.output.price }}?"
    idempotencyKey: "steps.best.output.token"
  output:
    template: "Reservado: {{ steps.best.output.token }} ({{ steps.hurry.status }})"
`

type booking struct {
	searches, books int
	booked          []string
}

func (b *booking) actor() *act.Registry {
	r := act.NewRegistry()
	r.Register("booking/search-flights", func(context.Context, act.Call) (any, error) {
		b.searches++
		return []any{
			map[string]any{"token": "t1", "price": 900.0},
			map[string]any{"token": "t2", "price": 700.0},
			map[string]any{"token": "t3", "price": 12000.0},
		}, nil
	})
	r.Register("booking/book", func(_ context.Context, c act.Call) (any, error) {
		b.books++
		b.booked = append(b.booked, c.IdempotencyKey)
		return map[string]any{"token": c.Args["token"]}, nil
	})
	return r
}

// yesDecider answers yes/no from the words "urgente" and "sim".
type yesDecider struct{ readerDecider }

func (d *yesDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if q, ok := req.Questions["q"]; ok && q.Kind == decide.KindNoul {
		state, _ := req.State.(map[string]any)
		msg, _ := state["message"].(string)
		p := map[string]float64{"true": 0.05, "false": 0.95}
		switch {
		case strings.Contains(msg, "urgente"):
			p = map[string]float64{"true": 0.95, "false": 0.05}
		case strings.Contains(msg, "talvez"):
			p = map[string]float64{"true": 0.5, "false": 0.5}
		}
		a, err := decide.AnswerFor(q, p, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		return &decide.Response{Answers: map[string]decide.Answer{"q": a}, Model: "yes-1"}, nil
	}
	return d.readerDecider.Decide(ctx, req)
}

func stepsHarness(t *testing.T, script string) (*harness, *booking) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"plugin.yaml": stepsPlugin, "fn.yaml": stepsFunction, "rank.star": script} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t, dir)
	b := &booking{}
	h.rt.Actor = b.actor()
	h.rt.Decider = &yesDecider{}
	return h, b
}

func (h *harness) startFn(fn, msg string) Result {
	h.t.Helper()
	res, err := h.rt.Start(context.Background(), Start{Namespace: "travel", Function: fn, Conversation: "c1", Message: msg})
	if err != nil {
		h.t.Fatal(err)
	}
	return h.deliver(res)
}

func TestStepsEndToEnd(t *testing.T) {
	h, b := stepsHarness(t, stepsScript)
	res := h.startFn("book-flight", "preciso viajar, é urgente")
	expectStatus(t, res, journal.StatusConfirming)
	res = h.reply("user", "sim", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	want := []string{
		"Vou priorizar voos para hoje.",
		"Achei 2 voos.",
		"Reservo o voo de 700?",
		"Reservado: t2 (succeeded)",
	}
	if !reflect.DeepEqual(h.user, want) {
		t.Errorf("messages\n got %q\nwant %q", h.user, want)
	}
	if b.searches != 1 || b.books != 1 || b.booked[0] != "t2" {
		t.Errorf("searches %d, books %v", b.searches, b.booked)
	}
}

func TestStepSkippedAndLowConfidence(t *testing.T) {
	h, _ := stepsHarness(t, stepsScript)
	h.startFn("book-flight", "talvez viaje") // 0.5 < 0.8: value null, hurry skipped
	res := h.reply("user", "sim", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	if h.user[0] != "Achei 2 voos." || h.user[len(h.user)-1] != "Reservado: t2 (skipped)" {
		t.Errorf("messages %q", h.user)
	}
}

func TestStarlarkFailureFailsTheRunWithoutData(t *testing.T) {
	broken := strings.Replace(stepsScript, "def rank(offers, max_price = 10000):\n",
		"def rank(offers, max_price = 10000):\n    return offers[99]\n", 1)
	h, b := stepsHarness(t, broken)
	res := h.startFn("book-flight", "oi")
	expectStatus(t, res, journal.StatusFailed)
	if !strings.Contains(res.Error, "rank.star:") || strings.Contains(res.Error, "t1") {
		t.Errorf("error %q must have the position and no data", res.Error)
	}
	if b.books != 0 || h.user[len(h.user)-1] != DefaultMessages.Failed {
		t.Errorf("books %d, messages %q", b.books, h.user)
	}
}

// A changed script is a new generation: a run started with the old one must
// not be replayed by the new program.
func TestRunsArePinnedToTheirGeneration(t *testing.T) {
	h, _ := stepsHarness(t, stepsScript)
	h.startFn("book-flight", "oi")
	h2, _ := stepsHarness(t, stepsScript+"\n# changed\n")
	if h.rt.Program.Generation() == h2.rt.Program.Generation() {
		t.Fatal("a changed script must change the generation")
	}
	h2.rt.Store = h.store
	if _, err := h2.rt.Resume(context.Background(), "run-1"); !errors.Is(err, ErrGeneration) {
		t.Errorf("want ErrGeneration, got %v", err)
	}
	h3, _ := stepsHarness(t, stepsScript)
	if h3.rt.Program.Generation() != h.rt.Program.Generation() {
		t.Error("the same files must give the same generation")
	}
}

func TestStepsReplayWithoutRepeatingCalls(t *testing.T) {
	h, b := stepsHarness(t, stepsScript)
	h.startFn("book-flight", "oi")
	for range 3 {
		if _, err := h.rt.Resume(context.Background(), "run-1"); err != nil {
			t.Fatal(err)
		}
	}
	if b.searches != 1 {
		t.Errorf("the search ran %d times", b.searches)
	}
}
