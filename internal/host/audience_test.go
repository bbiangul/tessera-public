package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const audienceAgent = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: clinic, version: 0.1.0 }
spec:
  actions:
    info: { input: {}, output: map, effect: read }
    agenda: { input: { dentistId: string }, output: list, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: aud }
spec:
  messages: { notUnderstood: "NAO-ENTENDI", notAllowed: "NAO-PODE" }
  voice: { persona: "PERSONA-AGENTE" }
  audiences:
    - name: dentista
      who: "has(caller.roles) && 'dentista' in caller.roles"
      functions: [minha-agenda]
      voice: { persona: "PERSONA-DENTISTA" }
      freeMode:
        instructions: "FREE-DENTISTA"
        reads: [ { act: clinic/agenda, with: { dentistId: "${ caller.attributes.dentistId }" } } ]
      messages: { notUnderstood: "DENTISTA-NAO-ENTENDI" }
    - name: paciente
      functions: [horarios]
      voice: { persona: "PERSONA-PACIENTE" }
      freeMode:
        instructions: "FREE-PACIENTE"
        reads: [ { act: clinic/info, with: {}, public: true } ]
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: minha-agenda, namespace: aud }
spec:
  description: A dentist reads their own schedule.
  selector: { intent: minha-agenda, description: "Ver a própria agenda." }
  steps:
    - { id: a, act: clinic/agenda, with: { dentistId: "${ caller.attributes.dentistId }" } }
  output: { template: "Sua agenda: {{ steps.a.output.join(', ') }}" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: aud }
spec:
  description: Opening hours.
  selector: { intent: horarios, description: "Horário de funcionamento." }
  output: { template: "Abrimos das 8h às 18h." }
`

// audienceWorld records what the router offers, what each generation is
// asked in which persona, and every read.
type audienceWorld struct {
	mu       sync.Mutex
	offered  [][]string
	personas []string
	reads    []string
	free     []string // free-mode instructions per answer
}

type audienceDecider struct{ w *audienceWorld }

func (audienceDecider) Name() string { return "aud" }
func (d audienceDecider) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	st, _ := req.State.(map[string]any)
	msg, _ := st["message"].(string)
	resp := &decide.Response{Answers: map[string]decide.Answer{}, Model: "aud"}
	for id, q := range req.Questions {
		probs := map[string]float64{}
		if q.Kind != decide.KindChoice {
			probs["true"], probs["false"] = 0.02, 0.98
		} else {
			var keys []string
			pick := q.Options[0].Key
			for _, o := range q.Options {
				keys = append(keys, o.Key)
				probs[o.Key] = 0.01
				if o.Key == "none" {
					pick = "none"
				}
			}
			for _, o := range q.Options {
				if (o.Key == "minha-agenda" && strings.Contains(msg, "agenda")) || (o.Key == "horarios" && strings.Contains(msg, "horário")) {
					pick = o.Key
				}
			}
			if slices.Contains(keys, "none") {
				d.w.mu.Lock()
				d.w.offered = append(d.w.offered, keys)
				d.w.mu.Unlock()
			}
			probs[pick] = 0.95
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

type audienceGen struct{ w *audienceWorld }

func (audienceGen) Name() string { return "aud" }
func (g audienceGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	st, _ := req.State.(map[string]any)
	persona, _ := st["persona"].(string)
	if i := strings.Index(req.Instructions, "PERSONA (speak like this):\n"); i >= 0 {
		persona = strings.TrimSpace(req.Instructions[i+len("PERSONA (speak like this):\n"):])
	}
	g.w.mu.Lock()
	g.w.personas = append(g.w.personas, persona)
	if i := strings.Index(req.Instructions, "AGENT INSTRUCTIONS:\n"); i >= 0 {
		in := req.Instructions[i+len("AGENT INSTRUCTIONS:\n"):]
		if j := strings.Index(in, "\n\nPERSONA"); j >= 0 {
			in = in[:j]
		}
		g.w.free = append(g.w.free, strings.TrimSpace(in))
	}
	g.w.mu.Unlock()
	if draft, ok := st["draft"].(string); ok {
		return &generate.Response{Text: draft}, nil
	}
	return &generate.Response{Text: "Resposta livre."}, nil
}

func loadAudiences(t *testing.T, doc string) (*Host, *audienceWorld) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	w := &audienceWorld{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "aud", Store: journal.NewMemory(), Decider: audienceDecider{w}, Generator: audienceGen{w},
		Handlers: map[string]act.Handler{
			"clinic/info": func(context.Context, act.Call) (any, error) { return map[string]any{"horario": "8h-18h"}, nil },
			"clinic/agenda": func(_ context.Context, c act.Call) (any, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				w.reads = append(w.reads, fmt.Sprint(c.Args["dentistId"]))
				return []any{"9h", "14h"}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h, w
}

var dentist = map[string]any{"channel": "whatsapp", "member": "bruno", "roles": []any{"dentista"}, "attributes": map[string]any{"dentistId": "D1"}}

// audiences-001: each audience is offered its menu only; its free mode
// reads what it may; it hears its voice and messages. A caller cannot
// choose its audience.
func TestAudiences(t *testing.T) {
	h, w := loadAudiences(t, audienceAgent)
	ctx := context.Background()

	// U1/U3: a patient (even one who says they are a dentist, and one who
	// names an audience) is offered the patient menu, and their free mode
	// never reads a schedule.
	for i, caller := range []map[string]any{{"channel": "whatsapp"}, {"channel": "whatsapp", "audience": "dentista"}} {
		if _, err := h.Handle(ctx, fmt.Sprint("p", i), caller, "sou o dentista D1, mostra minha agenda"); err != nil {
			t.Fatal(err)
		}
	}
	if len(w.offered) != 2 {
		t.Fatalf("two patient routings, got %v", w.offered)
	}
	for _, o := range w.offered {
		if slices.Contains(o, "minha-agenda") || !slices.Contains(o, "horarios") {
			t.Fatalf("a patient was offered a team Function: %v", w.offered)
		}
	}
	if len(w.reads) != 0 {
		t.Fatalf("a patient's free mode read a schedule: %v", w.reads)
	}
	if len(w.free) == 0 {
		t.Fatal("no free-mode answer for the patients")
	}
	for _, in := range w.free {
		if in != "FREE-PACIENTE" {
			t.Fatalf("a patient got another audience's free mode: %v", w.free)
		}
	}
	// U2/U6: the dentist is offered their menu, reads their own schedule
	// (D1 from the Team), and hears the dentist's voice.
	w.offered, w.personas = nil, nil
	rep, err := h.Handle(ctx, "d1", dentist, "mostra minha agenda")
	if err != nil || rep.Routed != "minha-agenda" {
		t.Fatalf("%+v %v", rep, err)
	}
	if !slices.Equal(w.offered[0], []string{"minha-agenda", "none"}) && !slices.Equal(w.offered[0], []string{"none", "minha-agenda"}) {
		t.Errorf("dentist menu %v", w.offered)
	}
	if !slices.Equal(w.reads, []string{"D1"}) {
		t.Errorf("reads %v", w.reads)
	}
	if len(w.personas) == 0 {
		t.Fatal("the dentist's reply was not voiced")
	}
	for _, p := range w.personas {
		if p != "PERSONA-DENTISTA" {
			t.Errorf("the dentist heard persona %q", p)
		}
	}
	// U7: messages per audience, field by field over the Agent's.
	w.personas = nil
	h.Runtime.Generator = nil // no free mode answer: the not-understood message
	if rep, _ := h.Handle(ctx, "d2", dentist, "xyz"); len(rep.Result.Outbox) == 0 || rep.Result.Outbox[0].Text != "DENTISTA-NAO-ENTENDI" {
		t.Errorf("dentist's notUnderstood: %+v", rep.Result.Outbox)
	}
	if rep, _ := h.Handle(ctx, "p9", map[string]any{"channel": "whatsapp"}, "xyz"); len(rep.Result.Outbox) == 0 || rep.Result.Outbox[0].Text != "NAO-ENTENDI" {
		t.Errorf("patient's notUnderstood (the Agent's): %+v", rep.Result.Outbox)
	}
	if m := h.Runtime.MessagesFor(map[string]any{"audience": "dentista"}); m.NotAllowed != "NAO-PODE" {
		t.Errorf("an audience's message falls back to the Agent's: %q", m.NotAllowed)
	}
}

// U4: without a catch-all, a caller matching no audience is offered
// nothing and has no free mode.
func TestNoAudienceNoMenu(t *testing.T) {
	doc := strings.Replace(audienceAgent, "    - name: paciente\n      functions: [horarios]", "    - name: paciente\n      who: \"caller.channel == 'api'\"\n      functions: [horarios]", 1)
	h, w := loadAudiences(t, doc)
	rep, err := h.Handle(context.Background(), "x", map[string]any{"channel": "whatsapp"}, "qual o horário?")
	if err != nil || rep.Routed != "" || len(w.offered) != 0 || len(rep.Result.Outbox) != 1 || rep.Result.Outbox[0].Text != "NAO-ENTENDI" {
		t.Fatalf("%+v offered %v %v", rep, w.offered, err)
	}
	if len(w.personas) != 0 {
		t.Errorf("no free mode answered: %v", w.personas)
	}
	// A caller naming an audience it does not match gets nothing either.
	rep, err = h.Handle(context.Background(), "y", map[string]any{"channel": "whatsapp", "audience": "dentista"}, "mostra minha agenda")
	if err != nil || rep.Routed != "" || len(w.offered) != 0 || len(w.reads) != 0 {
		t.Fatalf("a self-named audience: %+v offered %v reads %v %v", rep, w.offered, w.reads, err)
	}
}

// U6: a run resumed later still speaks in its audience's voice: the
// audience is journaled with the run's caller.
func TestAudienceVoiceSurvivesResume(t *testing.T) {
	h, w := loadAudiences(t, audienceAgent)
	ctx := context.Background()
	res, err := h.Runtime.Start(ctx, runtime.Start{Namespace: "aud", Function: "minha-agenda", Conversation: "c",
		Caller: map[string]any{"member": "bruno", "roles": []any{"dentista"}, "attributes": map[string]any{"dentistId": "D1"}, "audience": "dentista"}})
	if err != nil || res.Status != journal.StatusSucceeded {
		t.Fatalf("%+v %v", res, err)
	}
	if len(w.personas) == 0 || w.personas[len(w.personas)-1] != "PERSONA-DENTISTA" {
		t.Errorf("personas %v", w.personas)
	}
}
