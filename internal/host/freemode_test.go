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
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const freeDocs = agendaDocs + `---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: clinica, version: 0.1.0 }
spec:
  actions:
    info: { input: {}, output: map, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: recepcao, namespace: clinica }
spec:
  freeMode:
    instructions: "Responda dúvidas da clínica com os fatos."
    reads: [ { act: clinica/info } ]
`

// turnDecider is agendaDecider plus the interruption question: a message
// with '?' that names no slot is a question; 'tchau' a goodbye; 'cancela'
// a cancel.
type turnDecider struct{ agendaDecider }

func (d turnDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if q, ok := req.Questions["claims"]; ok {
		// A reply that says it registered something claims an action.
		state, _ := req.State.(map[string]any)
		reply, _ := state["reply"].(string)
		yes := 0.05
		if strings.Contains(reply, "registrei") {
			yes = 0.95
		}
		a, err := decide.AnswerFor(q, map[string]float64{"true": yes, "false": 1 - yes}, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		return &decide.Response{Answers: map[string]decide.Answer{"claims": a}, Model: "turns"}, nil
	}
	if q, ok := req.Questions["defers"]; ok {
		// A reply that mentions an approval defers.
		state, _ := req.State.(map[string]any)
		reply, _ := state["reply"].(string)
		yes := 0.05
		if strings.Contains(reply, "aprovação") {
			yes = 0.95
		}
		a, err := decide.AnswerFor(q, map[string]float64{"true": yes, "false": 1 - yes}, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		return &decide.Response{Answers: map[string]decide.Answer{"defers": a}, Model: "turns"}, nil
	}
	if q, ok := req.Questions["asks"]; ok {
		// An answer that asks to confirm the slot again asks the pending
		// question itself.
		state, _ := req.State.(map[string]any)
		answer, _ := state["answer"].(string)
		yes := 0.05
		if strings.Contains(answer, "confirmar terça") {
			yes = 0.95
		}
		a, err := decide.AnswerFor(q, map[string]float64{"true": yes, "false": 1 - yes}, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		return &decide.Response{Answers: map[string]decide.Answer{"asks": a}, Model: "turns"}, nil
	}
	if q, ok := req.Questions["handoff"]; ok {
		// A request for an exception goes to the team, when the facts
		// were in view.
		state, _ := req.State.(map[string]any)
		text, _ := state["message"].(string)
		_, facts := state["facts"].(map[string]any)["clinica/info"]
		pick := "answer"
		if strings.Contains(text, "exceção") && facts {
			pick = "chamar-equipe"
		}
		probs := map[string]float64{}
		for _, o := range q.Options {
			probs[o.Key] = 0.01
		}
		probs[pick] = 0.9
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		return &decide.Response{Answers: map[string]decide.Answer{"handoff": a}, Model: "turns"}, nil
	}
	if q, ok := req.Questions["later"]; ok {
		// "mando depois", "amanhã" put the pending question off.
		state, _ := req.State.(map[string]any)
		text, _ := state["message"].(string)
		yes := 0.05
		if strings.Contains(text, "depois") || strings.Contains(text, "amanhã") {
			yes = 0.95
		}
		a, err := decide.AnswerFor(q, map[string]float64{"true": yes, "false": 1 - yes}, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		return &decide.Response{Answers: map[string]decide.Answer{"later": a}, Model: "turns"}, nil
	}
	if q, ok := req.Questions["rest"]; ok {
		state, _ := req.State.(map[string]any)
		text, _ := state["message"].(string)
		yes := 0.05
		if strings.Contains(text, "estacionamento") || strings.Contains(text, "vaga") {
			yes = 0.95
		}
		a, err := decide.AnswerFor(q, map[string]float64{"true": yes, "false": 1 - yes}, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		return &decide.Response{Answers: map[string]decide.Answer{"rest": a}, Model: "turns"}, nil
	}
	if q, ok := req.Questions["alsoAsks"]; ok {
		state, _ := req.State.(map[string]any)
		text, _ := state["message"].(string)
		yes := 0.05
		if strings.Contains(text, "cartão") {
			yes = 0.95
		}
		a, err := decide.AnswerFor(q, map[string]float64{"true": yes, "false": 1 - yes}, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		return &decide.Response{Answers: map[string]decide.Answer{"alsoAsks": a}, Model: "turns"}, nil
	}
	if q, ok := req.Questions["answers"]; ok {
		// A message that names a slot also answers the slot question.
		state, _ := req.State.(map[string]any)
		text, _ := state["message"].(string)
		yes := 0.05
		if strings.Contains(text, "quarta") || strings.Contains(text, "terça") {
			yes = 0.95
		}
		a, err := decide.AnswerFor(q, map[string]float64{"true": yes, "false": 1 - yes}, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		return &decide.Response{Answers: map[string]decide.Answer{"answers": a}, Model: "turns"}, nil
	}
	q, ok := req.Questions["turn"]
	if !ok {
		return d.agendaDecider.Decide(ctx, req)
	}
	state, _ := req.State.(map[string]any)
	text, _ := state["message"].(string)
	pick := turnAnswer
	switch {
	case strings.Contains(text, "te falo depois"):
		pick = "" // unsure: the later question decides (agency-005)
	case strings.Contains(text, "tchau"):
		pick = turnGoodbye
	case strings.Contains(text, "cancela"):
		pick = turnCancel
	case strings.Contains(text, "?") && !strings.Contains(text, "terça") && !strings.Contains(text, "quarta"),
		strings.Contains(text, "estacionamento?"):
		pick = turnQuestion
	case strings.Contains(text, "hmm"):
		pick = "" // unsure
	}

	probs := map[string]float64{}
	for _, o := range q.Options {
		probs[o.Key] = 0.01
	}
	if pick == "" {
		probs[turnAnswer], probs[turnQuestion] = 0.5, 0.05 // split: under every threshold
	} else {
		probs[pick] = 0.96
	}
	a, err := decide.AnswerFor(q, probs, decide.ProbNative)
	if err != nil {
		return nil, err
	}
	return &decide.Response{Answers: map[string]decide.Answer{"turn": a}, Model: "turns"}, nil
}

// factGen answers from the facts it is given.
type factGen struct {
	mu      sync.Mutex
	states  []map[string]any
	greet   bool // open every answer with "Oi, Ana!"
	askBack bool // end every answer by asking to confirm the slot
	empty   bool // no answer at all (every draft dropped)
	instr   []string
}

func (g *factGen) Name() string { return "facts" }
func (g *factGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, _ := req.State.(map[string]any)
	g.states = append(g.states, st)
	g.instr = append(g.instr, req.Instructions)
	if g.empty {
		return &generate.Response{Text: ""}, nil
	}
	facts, _ := st["facts"].(map[string]any)
	info, _ := facts["clinica/info"].(map[string]any)
	text := "Sim, temos estacionamento: " + info["estacionamento"].(string) + "."
	if g.greet {
		text = "Oi, Ana! " + text
	}
	if g.askBack {
		text += " Posso confirmar terça 10h?"
	}
	return &generate.Response{Text: text}, nil
}

func freeHost(t *testing.T) (*Host, *[]string, *factGen) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(freeDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	var booked []string
	g := &factGen{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: g,
		Handlers: map[string]act.Handler{
			"agenda/marcar": func(_ context.Context, c act.Call) (any, error) {
				booked = append(booked, c.Args["slot"].(string))
				return map[string]any{}, nil
			},
			"clinica/info": func(context.Context, act.Call) (any, error) {
				return map[string]any{"estacionamento": "conveniado ao lado"}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h, &booked, g
}

func say(t *testing.T, h *Host, text string) Reply {
	t.Helper()
	ctx := context.Background()
	r, err := h.Handle(ctx, "c1", map[string]any{"name": "Ana"}, text)
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

// A question at the confirmation is answered from the facts, the
// confirmation is asked again word for word, and the yes after it acts
// once. It is never taken as a no.
func TestSideQuestionAtTheConfirmation(t *testing.T) {
	h, booked, _ := freeHost(t)
	if r := say(t, h, "quero marcar terça"); r.Status != journal.StatusConfirming {
		t.Fatalf("%+v", r)
	}
	r := say(t, h, "vocês têm estacionamento?")
	if r.Status != journal.StatusConfirming || len(r.Outbox) != 2 ||
		r.Outbox[0].Text != "Sim, temos estacionamento: conveniado ao lado." || r.Outbox[1].Text != "Confirmo terça 10h?" {
		t.Fatalf("answered, then the pending question again: %+v", r)
	}
	if r := say(t, h, "sim"); r.Status != journal.StatusSucceeded || len(*booked) != 1 || (*booked)[0] != "terça 10h" {
		t.Fatalf("booked %v, %+v", *booked, r)
	}
}

// Natal, 2026-09-28: a side answer that already asks for what the process
// waits on is not followed by the same question again.
func TestSideAnswerThatAsksIsNotRepeated(t *testing.T) {
	h, booked, g := freeHost(t)
	g.askBack = true
	if r := say(t, h, "quero marcar terça"); r.Status != journal.StatusConfirming {
		t.Fatalf("%+v", r)
	}
	r := say(t, h, "vocês têm estacionamento?")
	if r.Status != journal.StatusConfirming || len(r.Outbox) != 1 || !strings.HasSuffix(r.Outbox[0].Text, "Posso confirmar terça 10h?") {
		t.Fatalf("one message, the answer that asks: %+v", r.Outbox)
	}
	if r := say(t, h, "sim"); r.Status != journal.StatusSucceeded || len(*booked) != 1 {
		t.Fatalf("the process still takes the answer: booked %v, %+v", *booked, r)
	}
}

// Cancelling or saying goodbye in the middle ends the process: nothing
// is done, and no question is repeated.
func TestLeavingAProcess(t *testing.T) {
	for _, bye := range []string{"deixa, cancela", "obrigado, tchau"} {
		h, booked, _ := freeHost(t)
		say(t, h, "quero marcar terça")
		r := say(t, h, bye)
		if r.Status != journal.StatusSucceeded || len(*booked) != 0 || len(r.Outbox) != 1 {
			t.Fatalf("%q: %+v, booked %v", bye, r, *booked)
		}
		if runs, _ := h.Runtime.Store.Runs(context.Background(), journal.RunFilter{Conversation: "c1"}); runs[0].Status != journal.StatusSucceeded {
			t.Errorf("%q: the run still waits: %s", bye, runs[0].Status)
		}
	}
}

// A message no Function covers gets the free mode's answer, not "não
// entendi"; it reads the declared facts and nothing acts.
func TestFreeModeAnswersUnrouted(t *testing.T) {
	h, booked, g := freeHost(t)
	r := say(t, h, "tem estacionamento aí?")
	if !r.Unrouted || len(r.Outbox) != 1 || !strings.Contains(r.Outbox[0].Text, "conveniado ao lado") || len(*booked) != 0 {
		t.Fatalf("%+v", r)
	}
	if g.states[0]["user"] != "Ana" {
		t.Errorf("the user's name reaches the free mode: %v", g.states[0]["user"])
	}
}

// A free read's result is masked unless declared public: a clinic phone
// in a public read reaches the model; in another read it is a placeholder.
// The user's own words are masked either way.
func TestFreeModeMasksUnlessPublic(t *testing.T) {
	for _, public := range []bool{false, true} {
		dir := t.TempDir()
		docs := strings.Replace(freeDocs, "reads: [ { act: clinica/info } ]", map[bool]string{false: "reads: [ { act: clinica/info } ]", true: "reads: [ { act: clinica/info, public: true } ]"}[public], 1)
		if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(docs), 0o600); err != nil {
			t.Fatal(err)
		}
		g := &factGen{}
		h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: g,
			Handlers: map[string]act.Handler{"clinica/info": func(context.Context, act.Call) (any, error) {
				return map[string]any{"estacionamento": "sim", "telefone": "(11) 3030-4040"}, nil
			}}})
		if err != nil {
			t.Fatalf("%v %v", err, issues)
		}
		if _, err := h.Handle(context.Background(), "c1", nil, "qual o telefone? o meu é 11 98765-4321"); err != nil {
			t.Fatal(err)
		}
		st := g.states[0]
		phone := st["facts"].(map[string]any)["clinica/info"].(map[string]any)["telefone"]
		if (phone == "(11) 3030-4040") != public {
			t.Errorf("public=%v: the clinic phone reached the model as %v", public, phone)
		}
		if strings.Contains(st["message"].(string), "98765") {
			t.Errorf("the user's phone reached the model: %v", st["message"])
		}
	}
}

// Only the conversation's first message greets: a free answer that greets
// mid-conversation loses the greeting.
func TestFreeAnswerDoesNotGreetAgain(t *testing.T) {
	h, _, g := freeHost(t)
	g.greet = true
	say(t, h, "quero marcar terça")
	r := say(t, h, "vocês têm estacionamento?")
	if len(r.Outbox) != 2 || r.Outbox[0].Text != "Sim, temos estacionamento: conveniado ao lado." {
		t.Fatalf("greeted again: %+v", r.Outbox)
	}
}

const handoffDocs = `---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: equipe, namespace: clinica }
spec: { description: A equipe. }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: chamar-equipe, namespace: clinica }
spec:
  description: Pede uma decisão à equipe.
  steps:
    - id: avisar
      respond: { template: "Perguntei à equipe." }
    - id: equipe
      escalate: { queue: equipe, task: answer, message: "{{ event.message }}" }
  output: { template: "{{ steps.equipe.output }}" }
`

// The free mode starts one of its Functions when Decide, seeing the
// facts, says the message needs it; otherwise it answers. Nothing routes
// to that Function (it has no selector).
func TestFreeModeHandsOff(t *testing.T) {
	dir := t.TempDir()
	docs := strings.Replace(freeDocs, "    reads: [ { act: clinica/info } ]", "    reads: [ { act: clinica/info } ]\n    functions: [ { function: chamar-equipe, when: \"precisa de uma decisão da equipe\" } ]", 1) + handoffDocs
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(docs), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &factGen{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: g,
		Handlers: map[string]act.Handler{
			"agenda/marcar": func(context.Context, act.Call) (any, error) { return map[string]any{}, nil },
			"clinica/info": func(context.Context, act.Call) (any, error) {
				return map[string]any{"estacionamento": "conveniado ao lado"}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	r := say(t, h, "posso ter uma exceção no estacionamento?")
	if r.Routed != "chamar-equipe" || r.Status != journal.StatusWaitingForHuman || len(r.Outbox) == 0 || r.Outbox[0].Text != "Perguntei à equipe." {
		t.Fatalf("handed off: %+v", r)
	}
	if r.Handoff["chamar-equipe"] < 0.8 {
		t.Errorf("the decision is reported: %v", r.Handoff)
	}
	// Another conversation: this one waits for the team.
	r, err = h.Handle(context.Background(), "c2", map[string]any{"name": "Ana"}, "tem estacionamento")
	if err != nil {
		t.Fatal(err)
	}
	if r.Routed != "" || !r.Unrouted || r.Outbox[0].Text != "Sim, temos estacionamento: conveniado ao lado." {
		t.Fatalf("answered: %+v", r)
	}
	// The writer is told nobody else needs to decide.
	if last := g.instr[len(g.instr)-1]; !strings.Contains(last, "nobody else needs to decide") {
		t.Errorf("the answer does not know the decision: %s", last)
	}
}

// greetGen is a voice that greets in every message.
type greetGen struct{}

func (greetGen) Name() string { return "greet" }
func (greetGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	st, _ := req.State.(map[string]any)
	d, _ := st["draft"].(string)
	return &generate.Response{Text: "Oi, Ana! " + d}, nil
}

// A new run in a conversation knows the agent already spoke: only the
// conversation's first message greets (freemode-002).
func TestNoGreetingInALaterRun(t *testing.T) {
	dir := t.TempDir()
	docs := freeDocs + "  voice: { persona: \"Recepcionista.\" }\n"
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(docs), 0o600); err != nil {
		t.Fatal(err)
	}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: greetGen{},
		Handlers: map[string]act.Handler{
			"agenda/marcar": func(context.Context, act.Call) (any, error) { return map[string]any{}, nil },
			"clinica/info":  func(context.Context, act.Call) (any, error) { return map[string]any{}, nil },
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	if r := say(t, h, "quero marcar terça"); !strings.HasPrefix(r.Outbox[0].Text, "Oi, Ana!") {
		t.Fatalf("the first message greets: %+v", r.Outbox)
	}
	say(t, h, "sim")
	r := say(t, h, "quero marcar quarta")
	if len(r.Outbox) == 0 || strings.HasPrefix(r.Outbox[0].Text, "Oi") {
		t.Fatalf("a later run greeted again: %+v", r.Outbox)
	}
}

// deferGen defers in its first reply and answers in the second.
type deferGen struct{ calls int }

func (g *deferGen) Name() string { return "defer" }
func (g *deferGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	g.calls++
	if g.calls == 1 {
		return &generate.Response{Text: "Isso precisa da aprovação da equipe."}, nil
	}
	return &generate.Response{Text: "Pode sim: o estacionamento é conveniado ao lado."}, nil
}

// When the handoff decided to answer, a reply that sends the user to a
// person is written again once (escalation-003, u16).
func TestSettledAnswerDoesNotDefer(t *testing.T) {
	dir := t.TempDir()
	docs := strings.Replace(freeDocs, "    reads: [ { act: clinica/info } ]", "    reads: [ { act: clinica/info } ]\n    functions: [ { function: chamar-equipe, when: \"precisa de uma decisão da equipe\" } ]", 1) + handoffDocs
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(docs), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &deferGen{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: g,
		Handlers: map[string]act.Handler{
			"agenda/marcar": func(context.Context, act.Call) (any, error) { return map[string]any{}, nil },
			"clinica/info": func(context.Context, act.Call) (any, error) {
				return map[string]any{"estacionamento": "conveniado"}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	r := say(t, h, "posso estacionar aí")
	if g.calls != 2 || len(r.Outbox) != 1 || r.Outbox[0].Text != "Pode sim: o estacionamento é conveniado ao lado." {
		t.Fatalf("the deferring reply was sent (%d calls): %+v", g.calls, r.Outbox)
	}
}

// Free-mode exchanges are journaled: the next free answer sees them, the
// reply can be acknowledged, and follow-ups still know the last Function
// (issue #27).
func TestFreeTurnsAreJournaled(t *testing.T) {
	h, _, g := freeHost(t)
	say(t, h, "quero marcar terça")
	say(t, h, "sim")
	r1 := say(t, h, "vocês têm estacionamento")
	if r1.RunID == "" || len(r1.Outbox) != 1 || r1.Outbox[0].Seq == 0 {
		t.Fatalf("the free reply is not a journaled message: %+v", r1)
	}
	say(t, h, "e é pago")
	st := g.states[len(g.states)-1]
	var sawFirst bool
	for _, l := range st["conversation"].([]any) {
		sawFirst = sawFirst || strings.Contains(l.(string), "vocês têm estacionamento")
	}
	if !sawFirst {
		t.Errorf("the second free answer does not see the first: %v", st["conversation"])
	}
	last, ok, err := h.opts.Store.LastRun(context.Background(), "c1")
	if err != nil || !ok || last.Function != runtime.FreeRun || lastFunction(last) != "marcar" {
		t.Errorf("last run %q, follow-ups after %q", last.Function, lastFunction(last))
	}
}

// While a person handles a request, the free mode keeps answering other
// questions instead of "a person is looking at this" (issue #28).
func TestFreeModeAnswersWhileAPersonHandles(t *testing.T) {
	dir := t.TempDir()
	docs := strings.Replace(freeDocs, "    reads: [ { act: clinica/info } ]", "    reads: [ { act: clinica/info } ]\n    functions: [ { function: chamar-equipe, when: \"precisa de uma decisão da equipe\" } ]", 1) + handoffDocs
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(docs), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &factGen{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: g,
		Handlers: map[string]act.Handler{
			"agenda/marcar": func(context.Context, act.Call) (any, error) { return map[string]any{}, nil },
			"clinica/info": func(context.Context, act.Call) (any, error) {
				return map[string]any{"estacionamento": "conveniado ao lado"}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	if r := say(t, h, "posso ter uma exceção no estacionamento?"); r.Status != journal.StatusWaitingForHuman {
		t.Fatalf("handed off: %+v", r)
	}
	r := say(t, h, "tem estacionamento")
	if len(r.Outbox) != 1 || r.Outbox[0].Text != "Sim, temos estacionamento: conveniado ao lado." {
		t.Fatalf("not answered while the team handles the request: %+v", r.Outbox)
	}
	if !strings.Contains(g.instr[len(g.instr)-1], "A person is handling an earlier request") {
		t.Errorf("the free mode does not know a person handles a request")
	}
	active, ok, err := h.opts.Store.ActiveRun(context.Background(), "c1")
	if err != nil || !ok || active.Status != journal.StatusWaitingForHuman {
		t.Errorf("the team's run must still wait: %+v %v", active, err)
	}
}

// dialogue: full is the warm voice's default; marker is opt-in (voice-009).
func TestWarmDialogueDefault(t *testing.T) {
	for _, c := range []struct {
		voice string
		want  bool
	}{
		{"  voice: { persona: \"Recepcionista.\" }\n", true},
		{"  voice: { persona: \"Recepcionista.\", dialogue: marker }\n", false},
		{"  voice: { persona: \"Recepcionista.\", dialogue: full }\n", true},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(freeDocs+c.voice), 0o600); err != nil {
			t.Fatal(err)
		}
		h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: greetGen{},
			Handlers: map[string]act.Handler{
				"agenda/marcar": func(context.Context, act.Call) (any, error) { return map[string]any{}, nil },
				"clinica/info":  func(context.Context, act.Call) (any, error) { return map[string]any{}, nil },
			}})
		if err != nil {
			t.Fatalf("%v %v", err, issues)
		}
		if got := h.Runtime.Voice.WarmDialogue; got != c.want {
			t.Errorf("%q: WarmDialogue %v, want %v", c.voice, got, c.want)
		}
	}
}

// claimGen claims an action on its first draft, and answers truthfully
// when told it cannot act.
type claimGen struct{ factGen }

func (g *claimGen) Generate(ctx context.Context, req generate.Request) (*generate.Response, error) {
	if strings.Contains(req.Instructions, "You cannot act") {
		return &generate.Response{Text: "A recepção vê sua mensagem e responde por aqui."}, nil
	}
	return &generate.Response{Text: "Já registrei seu pedido para quarta às 17h."}, nil
}

// #51: a free answer that says the agent did something it did not is
// written again; one that still does is not sent.
func TestFreeAnswerNeverClaimsAnAction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(freeDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: &claimGen{},
		Handlers: map[string]act.Handler{"clinica/info": func(context.Context, act.Call) (any, error) { return map[string]any{}, nil }}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	h.Runtime.ExplainFailures = true
	r := say(t, h, "é para quarta às 17h, por favor")
	if len(r.Outbox) != 1 || strings.Contains(r.Outbox[0].Text, "registrei") || !strings.Contains(r.Outbox[0].Text, "recepção") {
		t.Errorf("want the truthful second draft: %+v", r.Outbox)
	}
}
