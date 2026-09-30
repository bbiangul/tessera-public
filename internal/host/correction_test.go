package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const agendaDocs = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: agenda, version: 0.1.0 }
spec:
  actions:
    marcar: { input: { slot: string }, output: map, effect: irreversible }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: marcar, namespace: clinica }
spec:
  description: Books a slot.
  selector: { intent: marcar }
  inputs:
    slot:
      type: string
      resolve:
        - id: pick
          ask: { text: "Terça 10h ou quarta 16h?", choices: "['terça 10h', 'quarta 16h']", minConfidence: 0.7, onLowConfidence: ask-user }
  action:
    act: agenda/marcar
    with: { slot: "${ inputs.slot }" }
    confirm: "Confirmo {{ inputs.slot }}?"
    idempotencyKey: "conversation.id + ':' + inputs.slot"
  output: { template: "Marcado: {{ inputs.slot }}." }
`

// agendaDecider routes to marcar, picks the slot the message names, says
// yes to "sim", no to "não", and a correction when the reply prefers
// another slot.
type agendaDecider struct{}

func (agendaDecider) Name() string { return "agenda" }
func (agendaDecider) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	state, _ := req.State.(map[string]any)
	text, _ := state["message"].(string)
	if r, ok := state["reply"].(string); ok {
		text = r
	}
	resp := &decide.Response{Answers: map[string]decide.Answer{}, Model: "agenda"}
	for id, q := range req.Questions {
		probs := map[string]float64{}
		if q.Kind == decide.KindNoul {
			yes := strings.Contains(text, "sim")
			if strings.Contains(q.Instructions, "something different instead") {
				yes = strings.Contains(text, "prefiro")
			}
			probs["true"], probs["false"] = 0.03, 0.97
			if yes {
				probs["true"], probs["false"] = 0.97, 0.03
			}
		} else {
			pick := "none"
			for _, o := range q.Options {
				probs[o.Key] = 0.01
				switch {
				case o.Key == "marcar" && strings.Contains(text, "marcar"),
					strings.Contains(o.Description, "terça") && strings.Contains(text, "terça"),
					strings.Contains(o.Description, "quarta") && strings.Contains(text, "quarta"):
					pick = o.Key
				}
			}
			probs[pick] = 1 - 0.01*float64(len(q.Options)-1)
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

func TestCorrectionAtTheConfirmation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(agendaDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var booked []string
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: agendaDecider{},
		Handlers: map[string]act.Handler{"agenda/marcar": func(_ context.Context, c act.Call) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			booked = append(booked, c.Args["slot"].(string))
			return map[string]any{}, nil
		}}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	say := func(text string) Reply {
		t.Helper()
		r, err := h.Handle(ctx, "c1", nil, text)
		if err != nil {
			t.Fatal(err)
		}
		for _, res := range append(r.Earlier, r.Result) {
			if n := len(res.Outbox); n > 0 && res.Outbox[n-1].Seq > 0 {
				_ = h.Runtime.Ack(ctx, res.RunID, res.Outbox[n-1].Seq)
			}
		}
		return r
	}
	if r := say("quero marcar terça"); r.Status != journal.StatusConfirming || r.Outbox[0].Text != "Confirmo terça 10h?" {
		t.Fatalf("%+v", r)
	}
	r := say("não, mudei de ideia, prefiro quarta")
	if r.Status != journal.StatusConfirming || len(r.Outbox) != 1 || r.Outbox[0].Text != "Confirmo quarta 16h?" {
		t.Fatalf("the correction starts over and confirms the new slot: %+v", r)
	}
	for _, e := range r.Earlier {
		for _, m := range e.Outbox {
			if m.Text == runtime.DefaultMessages.Cancelled || m.Text == h.opts.NotUnderstood {
				t.Errorf("no 'nothing changed' for a correction: %q", m.Text)
			}
		}
	}
	if r := say("sim"); r.Status != journal.StatusSucceeded || len(booked) != 1 || booked[0] != "quarta 16h" {
		t.Fatalf("booked %v, %+v", booked, r)
	}
}
