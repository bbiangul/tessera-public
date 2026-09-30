package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const retrieveAgent = `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: rt }
spec:
  audiences:
    - { name: equipe, who: "has(caller.roles) && size(caller.roles) > 0", functions: [consulta] }
    - { name: paciente, functions: [consulta] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: publico, namespace: rt }
spec: { description: Público., sources: [ { files: [docs/precos.md, docs/licoes.md] } ] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: interno, namespace: rt }
spec: { description: Equipe., sources: [ { files: [docs/interno.md] } ], audiences: [equipe] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: consulta, namespace: rt }
spec:
  description: Answers a price question from the documents.
  selector: { intent: consulta, description: "Pergunta de preço." }
  steps:
    - { id: docs, retrieve: { bases: [publico, interno], query: "event.message", k: 10 } }
    - { id: ok, ask: { text: "Confirma?", choices: "['sim', 'não']", minConfidence: 0.5, onLowConfidence: ask-user } }
  output: { template: "{{ steps.docs.output.map(p, p.text).join(' | ') }}" }
`

// routeDecider routes to consulta, picks the first choice of an ask, and
// finds every passage relevant.
type routeDecider struct{}

func (routeDecider) Name() string { return "route" }
func (routeDecider) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	resp := &decide.Response{Answers: map[string]decide.Answer{}}
	st, _ := req.State.(map[string]any)
	said := strings.TrimSpace(fmt.Sprint(st["message"]))
	if r, ok := st["reply"]; ok {
		said = strings.TrimSpace(fmt.Sprint(r))
	}
	for id, q := range req.Questions {
		probs := map[string]float64{}
		if q.Kind == decide.KindChoice {
			for i, o := range q.Options {
				probs[o.Key] = 0.01
				if i == 0 {
					probs[o.Key] = 0.9
				}
				if o.Key == "consulta" {
					probs[o.Key] = 0.99
				}
				if o.Key == "o1" || o.Key == "none" { // an ask: yes only to "sim"
					probs[o.Key] = 0.2
					if o.Key == "o1" && said == "sim" {
						probs[o.Key] = 0.95
					}
				}
			}
		} else {
			p := 0.02
			if len(id) == 2 && id[0] == 'p' {
				p = 0.9
			}
			probs["true"], probs["false"] = p, 1-p
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

// retrieve-001: a retrieve step returns the passages the caller's
// audience may read, never an expired lesson, and a resumed run replays
// what it first retrieved even after the documents changed.
func TestRetrieveStep(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "docs"), 0o700)
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/precos.md", "# Preços\n\nA limpeza custa R$ 180.\n")
	write("docs/interno.md", "# Descontos da limpeza\n\nNa limpeza, a recepção dá até 10%.\n")
	write("docs/licoes.md", "# Lições\n\n## Limpeza antiga\n\n<!-- lesson id=velha kind=price review=2026-01-01 -->\nA limpeza custava R$ 90.\n")
	write("a.yaml", retrieveAgent)
	store := journal.NewMemory()
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "rt", Store: store, Decider: routeDecider{}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	answer := func(conv string, caller map[string]any, msgs ...string) string {
		var last string
		for _, m := range msgs {
			res, err := h.Handle(ctx, conv, caller, m)
			if err != nil {
				t.Fatal(err)
			}
			for _, o := range res.Outbox {
				last = o.Text
			}
		}
		return last
	}
	patient := answer("p", map[string]any{"channel": "whatsapp"}, "quanto custa a limpeza?", "sim")
	if !strings.Contains(patient, "R$ 180") || strings.Contains(patient, "10%") || strings.Contains(patient, "R$ 90") {
		t.Errorf("patient: %q", patient)
	}
	staff := answer("s", map[string]any{"roles": []any{"recepcao"}}, "quanto custa a limpeza?", "sim")
	if !strings.Contains(staff, "R$ 180") || !strings.Contains(staff, "10%") {
		t.Errorf("staff: %q", staff)
	}

	// A run waiting at its ask when the price changes replays the old one.
	if out := answer("r", map[string]any{"channel": "whatsapp"}, "quanto custa a limpeza?"); out != "Confirma?" {
		t.Fatalf("not waiting: %q", out)
	}
	write("docs/precos.md", "# Preços\n\nA limpeza custa R$ 250.\n")
	if err := h.rebuild(); err != nil {
		t.Fatal(err)
	}
	if out := answer("r", map[string]any{"channel": "whatsapp"}, "sim"); !strings.Contains(out, "R$ 180") || strings.Contains(out, "R$ 250") {
		t.Errorf("resumed run: %q", out)
	}
	if out := answer("n", map[string]any{"channel": "whatsapp"}, "quanto custa a limpeza?", "sim"); !strings.Contains(out, "R$ 250") {
		t.Errorf("a new run: %q", out)
	}
}

// A failed run journals why, masked, even with no queue to tell.
func TestFailureReasonJournaled(t *testing.T) {
	dir := t.TempDir()
	agent := `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: fr }
spec: { messages: { failed: "FALHOU" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: consulta, namespace: fr }
spec:
  description: Breaks on its output.
  selector: { intent: consulta, description: "Qualquer coisa." }
  steps:
    - { id: x, compute: { cel: "'ana@exemplo.com'" } }
  output: { template: "{{ steps.x.output.value }}" }
`
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(agent), 0o600); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemory()
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "fr", Store: store, Decider: routeDecider{}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	res, err := h.Handle(context.Background(), "c", map[string]any{}, "oi")
	if err != nil || len(res.Outbox) != 1 || res.Outbox[0].Text != "FALHOU" {
		t.Fatalf("%+v %v", res, err)
	}
	recs, _ := store.Records(context.Background(), res.RunID)
	var reason string
	for _, r := range recs {
		if r.Site == "failed/reason" {
			reason = string(r.Value)
		}
	}
	if !strings.Contains(reason, "spec.output") {
		t.Errorf("reason not journaled: %q", reason)
	}
}
