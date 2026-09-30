package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// The side-answer, relay and whenUnsure behaviour of agency-001..003.

// failingTurns fails the interruption question, as a provider outage.
type failingTurns struct{ turnDecider }

func (d failingTurns) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if _, ok := req.Questions["turn"]; ok {
		return nil, errors.New("typesafe: http 520")
	}
	return d.turnDecider.Decide(ctx, req)
}

// agency-001: one 520 from the classifier ended the conversation with an
// error. Now the message is neither the answer nor anything else: the
// pending question is asked again, and nothing is done.
func TestClassifierFailureAsksAgain(t *testing.T) {
	h, booked, _ := freeHost(t)
	if r := say(t, h, "quero marcar terça"); r.Status != journal.StatusConfirming {
		t.Fatalf("%+v", r)
	}
	h.opts.Decider = failingTurns{}
	r := say(t, h, "sim")
	if r.Status != journal.StatusConfirming || len(r.Outbox) != 1 || r.Outbox[0].Text != "Confirmo terça 10h?" {
		t.Fatalf("the pending question again: %+v", r)
	}
	if len(*booked) != 0 {
		t.Error("an unclassified message confirmed the booking")
	}
}

// agency-001: "ida dia 3, volta dia 17; e os documentos?" was answered as a
// side question and its dates were asked again. At a question (not a
// confirmation), a message that asks something and also answers is
// answered and delivered: the slot is taken, nothing is asked twice.
func TestAnswerWithASideQuestion(t *testing.T) {
	h, booked, _ := freeHost(t)
	if r := say(t, h, "quero marcar"); r.Status != journal.StatusWaitingForUser {
		t.Fatalf("%+v", r)
	}
	r := say(t, h, "quarta, e vocês têm estacionamento?")
	if r.Status != journal.StatusConfirming || len(r.Outbox) != 2 ||
		r.Outbox[0].Text != "Sim, temos estacionamento: conveniado ao lado." || r.Outbox[1].Text != "Confirmo quarta 16h?" {
		t.Fatalf("the side answer, then the run's next question: %+v", r.Outbox)
	}
	if len(*booked) != 0 {
		t.Error("nothing is done before the confirmation")
	}
	// At a confirmation the old rule holds: a question is never a yes.
	r = say(t, h, "sim, quarta mesmo, e vocês têm estacionamento?")
	if len(*booked) != 0 {
		t.Errorf("a yes with a question confirmed: %+v", r.Outbox)
	}
}

// An answer with a question inside, and an unsure classification of a
// message that answers: both reach the run, the question gets its answer.
func TestAnswersThatAlsoAsk(t *testing.T) {
	h, _, _ := freeHost(t)
	say(t, h, "quero marcar")
	r := say(t, h, "quarta. vocês aceitam cartão?")
	if r.Status != journal.StatusConfirming || len(r.Outbox) != 2 || r.Outbox[1].Text != "Confirmo quarta 16h?" {
		t.Fatalf("the question answered, then the run's next question: %+v", r.Outbox)
	}
	h2, _, _ := freeHost(t)
	say(t, h2, "quero marcar")
	r = say(t, h2, "hmm quarta talvez")
	if r.Status != journal.StatusConfirming || r.Outbox[len(r.Outbox)-1].Text != "Confirmo quarta 16h?" {
		t.Fatalf("an unsure turn that answers still reaches the run: %+v", r.Outbox)
	}
	h3, _, _ := freeHost(t)
	say(t, h3, "quero marcar")
	r = say(t, h3, "hmm deixa eu ver")
	if r.Status != journal.StatusWaitingForUser || len(r.Outbox) != 1 || r.Outbox[0].Text != "Terça 10h ou quarta 16h?" {
		t.Fatalf("an unsure turn that does not answer asks again: %+v", r.Outbox)
	}
}

// agency-001: a user who kept talking about other things heard the pending
// question after every answer. After the first time it is not repeated;
// the run still waits, and the answer, when it comes, reaches it.
func TestPendingQuestionIsNotRepeatedTwice(t *testing.T) {
	h, _, _ := freeHost(t)
	say(t, h, "quero marcar")
	if r := say(t, h, "vocês têm estacionamento?"); len(r.Outbox) != 2 {
		t.Fatalf("the first time: the answer, then the question again: %+v", r.Outbox)
	}
	r := say(t, h, "e o estacionamento é coberto?")
	if len(r.Outbox) != 1 || r.Status != journal.StatusWaitingForUser {
		t.Fatalf("the second time: only the answer, still waiting: %+v", r)
	}
	if r := say(t, h, "quarta"); r.Status != journal.StatusConfirming {
		t.Fatalf("the answer still reaches the run: %+v", r)
	}
}

// agency-002: a request with another question in it gets the process's
// reply and the answer to the rest, the rest first.
func TestTheRestOfTheMessage(t *testing.T) {
	h, _, g := freeHost(t)
	r := say(t, h, "quero marcar, e vocês têm estacionamento?")
	if r.Routed != "marcar" || len(r.Outbox) != 2 || r.Outbox[0].Text != "Sim, temos estacionamento: conveniado ao lado." || r.Outbox[1].Text != "Terça 10h ou quarta 16h?" {
		t.Fatalf("the rest answered first, then the process's question: %+v", r.Outbox)
	}
	if !strings.Contains(g.instr[len(g.instr)-1], "ALREADY") {
		t.Error("the free answer is told what the process answers")
	}
	h2, _, _ := freeHost(t)
	if r := say(t, h2, "quero marcar"); len(r.Outbox) != 1 {
		t.Fatalf("a request alone gets only the process's reply: %+v", r.Outbox)
	}
}

// agency-003: a customer who answers the team with a question ("dá pra
// pagar no Pix?") was answered by the free mode, and the team never saw
// it. A relay wait takes every reply as written.
func TestRelayWaitTakesQuestions(t *testing.T) {
	for _, relay := range []bool{true, false} {
		dir := t.TempDir()
		fn := fmt.Sprintf(`apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: repassar, namespace: clinica }
spec:
  description: Relays a reply.
  steps:
    - { id: q, respond: { template: "A equipe pergunta: confirma?" } }
    - { id: w, wait: { for: 24h, orReply: true, relay: %v } }
  output: { template: "recebi: {{ steps.w.output.reply }}" }
`, relay)
		if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(freeDocs+"---\n"+fn), 0o600); err != nil {
			t.Fatal(err)
		}
		h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: &factGen{},
			Handlers: map[string]act.Handler{"clinica/info": func(context.Context, act.Call) (any, error) {
				return map[string]any{"estacionamento": "conveniado ao lado"}, nil
			}}})
		if err != nil {
			t.Fatalf("%v %v", err, issues)
		}
		if _, err := h.Runtime.Start(context.Background(), runtime.Start{Namespace: "clinica", Function: "repassar", Conversation: "c1", Caller: map[string]any{"name": "Ana"}}); err != nil {
			t.Fatal(err)
		}
		r := say(t, h, "vocês têm estacionamento?")
		got := r.Outbox[len(r.Outbox)-1].Text
		if relay && got != "recebi: vocês têm estacionamento?" {
			t.Errorf("relay: the question reaches the run: %+v", r.Outbox)
		}
		if !relay && got == "recebi: vocês têm estacionamento?" {
			t.Errorf("without relay the question is a side question: %+v", r.Outbox)
		}
	}
}

// agency-003: an answer with a question the run's next message does not
// answer ("quarta sim, e tem vaga pra carro?") gets it answered, before
// the run's confirmation.
func TestTheRestOfAReply(t *testing.T) {
	h, booked, _ := freeHost(t)
	say(t, h, "quero marcar")
	r := say(t, h, "quarta sim, e tem vaga pra carro?")
	if r.Status != journal.StatusConfirming || len(r.Outbox) != 2 || r.Outbox[0].Text != "Sim, temos estacionamento: conveniado ao lado." || r.Outbox[1].Text != "Confirmo quarta 16h?" {
		t.Fatalf("the rest, then the confirmation: %+v", r.Outbox)
	}
	if len(*booked) != 0 {
		t.Error("nothing is done before the yes")
	}
}

// agency-003: "fulana@x.com. E vocês têm estacionamento?" answering the
// e-mail question was taken as a side question and the e-mail was asked
// again. A reply holding the pattern the question asks for is its answer.
func TestFormatReplyIsTheAnswer(t *testing.T) {
	dir := t.TempDir()
	fn := `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: contato, namespace: clinica }
spec:
  description: Takes an e-mail.
  inputs:
    email:
      type: string
      format: email
      resolve:
        - { id: asked, ask: { text: "Qual o seu e-mail?", minConfidence: 0.7, onLowConfidence: ask-user } }
  output: { template: "Anotado: {{ inputs.email }}" }
`
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(freeDocs+"---\n"+fn), 0o600); err != nil {
		t.Fatal(err)
	}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: &factGen{},
		Handlers: map[string]act.Handler{"clinica/info": func(context.Context, act.Call) (any, error) {
			return map[string]any{"estacionamento": "conveniado ao lado"}, nil
		}}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	res, err := h.Runtime.Start(context.Background(), runtime.Start{Namespace: "clinica", Function: "contato", Conversation: "c1", Caller: map[string]any{"name": "Ana"}})
	if err != nil || res.Status != journal.StatusWaitingForUser {
		t.Fatalf("%v %+v", err, res)
	}
	if err := h.Runtime.Ack(context.Background(), res.RunID, res.Outbox[len(res.Outbox)-1].Seq); err != nil {
		t.Fatal(err)
	}
	r := say(t, h, "ana@example.com. E vocês têm estacionamento?")
	if r.Status != journal.StatusSucceeded || len(r.Outbox) != 2 || r.Outbox[1].Text != "Anotado: ana@example.com" || !strings.Contains(r.Outbox[0].Text, "estacionamento") {
		t.Fatalf("the e-mail taken, the question answered: %+v", r.Outbox)
	}
}

// agency-003: "não consigo confirmar; se quiser, peço para a equipe"
// promised what happened only if the next turn was classified as asking
// for a person. With whenUnsure, no answer standing starts that Function.
func TestWhenUnsureAsksTheTeam(t *testing.T) {
	dir := t.TempDir()
	docs := strings.Replace(freeDocs, "    reads: [ { act: clinica/info } ]\n", "    reads: [ { act: clinica/info } ]\n    whenUnsure: perguntar\n    knowledge: { bases: [kb] }\n", 1) + `---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: perguntar, namespace: clinica }
spec:
  description: Asks the team.
  steps:
    - { id: r, escalate: { queue: recepcao, task: answer, message: "Pergunta: {{ event.message }}" } }
  output: { template: "{{ steps.r.output }}" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: clinica }
spec: {}
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: kb, namespace: clinica }
spec:
  description: Nada.
  sources: [ { files: ["kb.md"] } ]
`
	if docs == freeDocs {
		t.Fatal("test setup")
	}
	if err := os.WriteFile(filepath.Join(dir, "kb.md"), []byte("# Horário\n\nAbrimos às 8h.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(docs), 0o600); err != nil {
		t.Fatal(err)
	}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: unsupportedAlways{turnDecider{}}, Generator: fixedText{"Abrimos domingo sim."},
		Handlers: map[string]act.Handler{"clinica/info": func(context.Context, act.Call) (any, error) {
			return map[string]any{"estacionamento": "conveniado ao lado"}, nil
		}}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	r := say(t, h, "vocês atendem domingo?")
	if r.Routed != "perguntar" || r.Status != journal.StatusWaitingForHuman {
		t.Fatalf("no answer stands: the team is asked: %+v", r)
	}
	// agency-007: the same while a process waits. It stops (it did nothing
	// yet) and the team gets the question; one run waits at a time.
	h.opts.Store = journal.NewMemory()
	h.Runtime.Store = h.opts.Store
	if r := say(t, h, "quero marcar terça"); r.Status != journal.StatusConfirming {
		t.Fatalf("%+v", r)
	}
	r = say(t, h, "vocês atendem domingo?")
	if r.Routed != "perguntar" || r.Status != journal.StatusWaitingForHuman {
		t.Fatalf("the waiting run stops, the team is asked: %+v", r)
	}
	if active, ok, _ := h.opts.Store.ActiveRun(context.Background(), "c1"); !ok || active.Function != "perguntar" {
		t.Errorf("one run waits, the team's: %+v", active)
	}
}

// unsupportedAlways says every free answer states something unsupported.
type unsupportedAlways struct{ turnDecider }

func (d unsupportedAlways) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if q, ok := req.Questions["unsupported"]; ok {
		a, err := decide.AnswerFor(q, map[string]float64{"true": 0.95, "false": 0.05}, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		return &decide.Response{Answers: map[string]decide.Answer{"unsupported": a}}, nil
	}
	return d.turnDecider.Decide(ctx, req)
}

// agency-005: "te falo depois" is not the pending question's answer. The
// run waits, the question is not asked again, what the message asks is
// answered, and the answer when it comes still reaches the run.
func TestLaterKeepsTheProcessWaiting(t *testing.T) {
	h, booked, _ := freeHost(t)
	say(t, h, "quero marcar")
	r := say(t, h, "ainda não sei, te falo depois. e tem vaga?")
	if r.Result.Status != journal.StatusWaitingForUser || len(r.Outbox) != 2 ||
		r.Outbox[0].Text != "Sim, temos estacionamento: conveniado ao lado." || r.Outbox[1].Text != defaultLater {
		t.Fatalf("the answer, then the hold, no question again: %+v", r.Outbox)
	}
	if r := say(t, h, "mando amanhã"); len(r.Outbox) != 1 || r.Outbox[0].Text != defaultLater {
		t.Fatalf("only the hold: %+v", r.Outbox)
	}
	if r := say(t, h, "quarta"); r.Status != journal.StatusConfirming {
		t.Fatalf("the answer still reaches the run: %+v", r)
	}
	// What a message gives is not held with what it puts off.
	h2, _, _ := freeHost(t)
	say(t, h2, "quero marcar")
	if r := say(t, h2, "quarta sim, o resto te falo depois"); r.Status != journal.StatusConfirming {
		t.Fatalf("the part it gives reaches the run: %+v", r)
	}
	// At a confirmation, putting it off confirms nothing.
	if r := say(t, h, "vou ver com minha esposa e confirmo amanhã"); r.Status != journal.StatusConfirming || len(*booked) != 0 {
		t.Fatalf("nothing booked: %+v %v", r, *booked)
	}
	if r := say(t, h, "sim"); r.Status != journal.StatusSucceeded || len(*booked) != 1 {
		t.Fatalf("the yes books once: %v", *booked)
	}
}

// agency-007 (agency-006 p11): an unsure message that asks something is
// answered, not met with the pending question alone; an answer the checks
// drop is said to be unsure, never skipped.
func TestPendingNeverSkipsAQuestion(t *testing.T) {
	h, _, _ := freeHost(t)
	say(t, h, "quero marcar terça")
	h.opts.Decider = uncertainTurnDecider{}
	r := say(t, h, "não sei bem ainda, vocês têm estacionamento?")
	if r.Status != journal.StatusConfirming || len(r.Outbox) != 2 || r.Outbox[0].Text != "Sim, temos estacionamento: conveniado ao lado." {
		t.Fatalf("answered, the run still waits: %+v", r.Outbox)
	}
	h2, _, g := freeHost(t)
	say(t, h2, "quero marcar terça")
	g.empty = true
	r = say(t, h2, "vocês têm estacionamento?")
	if len(r.Outbox) != 2 || r.Outbox[0].Text != h2.unsure(map[string]any{}) || r.Outbox[1].Text != "Confirmo terça 10h?" {
		t.Fatalf("unsure, then the pending question: %+v", r.Outbox)
	}
}
