package host

import (
	"context"
	"errors"
	"fmt"
	"github.com/bbiangul/tessera/internal/engines/decide/decidetest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const kbAgent = `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: kb }
spec:
  messages: { notUnderstood: "NAO-ENTENDI" }
  audiences:
    - name: equipe
      who: "has(caller.roles) && size(caller.roles) > 0"
      functions: []
      freeMode: { instructions: "Ajude a equipe.", knowledge: { bases: [publico, interno] } }
    - name: paciente
      functions: [horarios]
      freeMode: { instructions: "Ajude o paciente.", knowledge: { bases: [publico, interno], check: CHECK } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: publico, namespace: kb }
spec: { description: Políticas., sources: [ { files: [docs/estacionamento.md] } ] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: interno, namespace: kb }
spec: { description: Só da equipe., sources: [ { files: [docs/interno.md] } ], audiences: [equipe] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: kb }
spec:
  description: Opening hours.
  selector: { intent: horarios, description: "Horário de funcionamento." }
  output: { template: "8h-18h" }
`

type kbGen struct {
	mu    sync.Mutex
	docs  [][]string // FACTS.documents' sources per free answer
	reply string
	quote string // the reply to the quote fallback, when set
	calls int
}

func (*kbGen) Name() string { return "kb" }
func (g *kbGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if strings.HasPrefix(req.Instructions, "MESSAGE is a customer's question") {
		return &generate.Response{Text: "estacionamento desconto"}, nil // the rewrite
	}
	g.calls++
	if g.quote != "" && strings.HasPrefix(req.Instructions, "Reply in the language of MESSAGE, in at most two sentences") {
		return &generate.Response{Text: g.quote}, nil
	}
	st, _ := req.State.(map[string]any)
	facts, _ := st["facts"].(map[string]any)
	var got []string
	if list, ok := facts["documents"].([]any); ok {
		for _, d := range list {
			m := d.(map[string]any)
			got = append(got, m["source"].(string)+":"+m["text"].(string))
		}
	}
	g.docs = append(g.docs, got)
	return &generate.Response{Text: g.reply}, nil
}

// kbDecider routes nothing and says every reply is unsupported.
type kbDecider struct{ unsupported float64 }

func (kbDecider) Name() string { return "kb" }
func (d kbDecider) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	resp := &decide.Response{Answers: map[string]decide.Answer{}}
	for id, q := range req.Questions {
		probs := map[string]float64{}
		if q.Kind == decide.KindChoice {
			for _, o := range q.Options {
				probs[o.Key] = 0.01
			}
			probs["none"], probs["answer"] = 0.99, 0.99
		} else {
			p := 0.02
			switch {
			case id == "unsupported":
				p = d.unsupported
			case len(id) == 2 && id[0] == 'p': // relevance: every passage is relevant
				p = 0.9
			}
			probs["true"], probs["false"] = p, 1-p
		}
		for k := range probs {
			found := q.Kind != decide.KindChoice
			for _, o := range q.Options {
				found = found || o.Key == k
			}
			if !found {
				delete(probs, k)
			}
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

func loadKB(t *testing.T, check string, d kbDecider, g *kbGen) (*Host, journal.Store) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"estacionamento.md": "# Estacionamento\n\nO conveniado é o Parque Seguro: duas horas por R$ 12.\n",
		"interno.md":        "# Descontos\n\nA recepção dá até 10% de desconto no Pix.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, "docs", name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(strings.ReplaceAll(kbAgent, "CHECK", check)), 0o600); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemory()
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "kb", Store: store, Decider: d, Generator: g})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h, store
}

// knowledge-001 K6: a patient's FACTS never hold a passage of a base only
// the team may read; the team's do; the passages given are journaled.
func TestKnowledgeAudiences(t *testing.T) {
	g := &kbGen{reply: "O conveniado é o Parque Seguro."}
	h, store := loadKB(t, "false", kbDecider{}, g)
	ctx := context.Background()
	if _, err := h.Handle(ctx, "p", map[string]any{"channel": "whatsapp"}, "tem desconto no pix? e estacionamento?"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Handle(ctx, "e", map[string]any{"roles": []any{"recepcao"}}, "tem desconto no pix? e estacionamento?"); err != nil {
		t.Fatal(err)
	}
	if len(g.docs) != 2 {
		t.Fatalf("free answers %v", g.docs)
	}
	patient, staff := strings.Join(g.docs[0], "|"), strings.Join(g.docs[1], "|")
	if strings.Contains(patient, "10%") || !strings.Contains(patient, "Parque Seguro") {
		t.Errorf("patient's documents: %q", patient)
	}
	if !strings.Contains(staff, "10%") || !strings.Contains(staff, "Parque Seguro") {
		t.Errorf("staff's documents: %q", staff)
	}
	runs, err := store.Runs(ctx, journal.RunFilter{Namespace: "kb", Limit: 10})
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs %v %v", runs, err)
	}
	var journaled string
	for _, r := range runs {
		recs, _ := store.Records(ctx, r.ID)
		for _, rec := range recs {
			journaled += string(rec.Value)
		}
	}
	if !strings.Contains(journaled, `"documents"`) || !strings.Contains(journaled, "estacionamento.md") || !strings.Contains(journaled, `"hash"`) {
		t.Errorf("the passages given are not journaled: %s", journaled)
	}
}

// knowledge-001: with check, an answer Jev finds unsupported is rewritten
// once, then dropped for the fixed message.
func TestKnowledgeCheck(t *testing.T) {
	g := &kbGen{reply: "Sim, aceitamos Hapvida."}
	h, _ := loadKB(t, "true", kbDecider{unsupported: 0.9}, g)
	res, err := h.Handle(context.Background(), "p", map[string]any{"channel": "whatsapp"}, "aceitam hapvida?")
	if err != nil {
		t.Fatal(err)
	}
	// Two drafts and the quote fallback (fallback-001), which has no quote
	// here: the fixed message.
	if g.calls != 3 || len(res.Outbox) != 1 || res.Outbox[0].Text != "NAO-ENTENDI" {
		t.Errorf("calls %d, outbox %+v", g.calls, res.Outbox)
	}
	// Without check, Jev is not asked: the answer stands.
	g0 := &kbGen{reply: "Sim, aceitamos Hapvida."}
	h0, _ := loadKB(t, "false", kbDecider{unsupported: 0.9}, g0)
	if res, err := h0.Handle(context.Background(), "p", map[string]any{"channel": "whatsapp"}, "aceitam hapvida?"); err != nil || g0.calls != 1 || res.Outbox[0].Text != "Sim, aceitamos Hapvida." {
		t.Errorf("check false: calls %d %+v %v", g0.calls, res.Outbox, err)
	}
	g2 := &kbGen{reply: "O conveniado é o Parque Seguro."}
	h2, _ := loadKB(t, "true", kbDecider{unsupported: 0.1}, g2)
	res, err = h2.Handle(context.Background(), "p", map[string]any{"channel": "whatsapp"}, "estacionamento?")
	if err != nil || g2.calls != 1 || res.Outbox[0].Text != "O conveniado é o Parque Seguro." {
		t.Errorf("a supported answer: calls %d %+v %v", g2.calls, res.Outbox, err)
	}
}

// handoffSpy records the facts the handoff question sees.
type handoffSpy struct {
	kbDecider
	mu    sync.Mutex
	facts []map[string]any
}

func (s *handoffSpy) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if _, ok := req.Questions["handoff"]; ok {
		st, _ := req.State.(map[string]any)
		f, _ := st["facts"].(map[string]any)
		s.mu.Lock()
		s.facts = append(s.facts, f)
		s.mu.Unlock()
	}
	return s.kbDecider.Decide(ctx, req)
}

// learn-001: the handoff sees the documents, so it does not hand off
// what they answer.
func TestKnowledgeBeforeHandoff(t *testing.T) {
	doc := strings.Replace(kbAgent, `freeMode: { instructions: "Ajude o paciente.", knowledge: { bases: [publico, interno], check: CHECK } }`,
		`freeMode: { instructions: "Ajude o paciente.", knowledge: { bases: [publico, interno], check: CHECK }, functions: [ { function: perguntar, when: "os documentos não respondem" } ] }`, 1)
	doc += `---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: perguntar, namespace: kb }
spec:
  description: Asks the reception.
  output: { template: "ok" }
`
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "docs"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "docs", "estacionamento.md"), []byte("# Estacionamento\n\nParque Seguro.\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "docs", "interno.md"), []byte("# Descontos\n\n10% no Pix.\n"), 0o600)
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(strings.ReplaceAll(doc, "CHECK", "false")), 0o600); err != nil {
		t.Fatal(err)
	}
	spy := &handoffSpy{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "kb", Store: journal.NewMemory(), Decider: spy, Generator: &kbGen{reply: "Parque Seguro."}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	if _, err := h.Handle(context.Background(), "p", map[string]any{"channel": "whatsapp"}, "tem estacionamento?"); err != nil {
		t.Fatal(err)
	}
	if len(spy.facts) != 1 {
		t.Fatalf("handoff asked %d times", len(spy.facts))
	}
	docs, _ := spy.facts[0]["documents"].([]any)
	if len(docs) == 0 || !strings.Contains(fmt.Sprint(docs), "Parque Seguro") || strings.Contains(fmt.Sprint(docs), "10%") {
		t.Errorf("the handoff's documents: %v", docs)
	}
}

// relevanceDecider says passage p1 is relevant and every other not; with
// fail, the relevance request errors.
type relevanceDecider struct {
	kbDecider
	fail bool
}

func (d relevanceDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if _, ok := req.Questions["p0"]; ok {
		if d.fail {
			return nil, fmt.Errorf("down")
		}
		resp := &decide.Response{Answers: map[string]decide.Answer{}}
		for id, q := range req.Questions {
			p := 0.1
			if id == "p1" {
				p = 0.9
			}
			a, _ := decide.AnswerFor(q, map[string]float64{"true": p, "false": 1 - p}, decide.ProbNative)
			resp.Answers[id] = a
		}
		return resp, nil
	}
	return d.kbDecider.Decide(ctx, req)
}

// knowledge-002: only passages Jev finds relevant reach the answer (on by
// default); unsure, all of them.
func TestKnowledgeRelevance(t *testing.T) {
	for _, c := range []struct {
		name string
		fail bool
		want int
	}{{"filtered", false, 1}, {"unsure", true, 2}} {
		g := &kbGen{reply: "ok"}
		dir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(dir, "docs"), 0o700)
		_ = os.WriteFile(filepath.Join(dir, "docs", "estacionamento.md"), []byte("# Estacionamento\n\nParque Seguro, estacionamento conveniado.\n\n# Moto\n\nNão há vaga de moto no estacionamento.\n"), 0o600)
		_ = os.WriteFile(filepath.Join(dir, "docs", "interno.md"), []byte("# Descontos\n\n10%.\n"), 0o600)
		doc := strings.ReplaceAll(kbAgent, ", check: CHECK", "")
		_ = os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(doc), 0o600)
		h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "kb", Store: journal.NewMemory(), Decider: relevanceDecider{fail: c.fail}, Generator: g})
		if err != nil {
			t.Fatalf("%v %v", err, issues)
		}
		if _, err := h.Handle(context.Background(), "p", map[string]any{"channel": "whatsapp"}, "estacionamento de moto?"); err != nil {
			t.Fatal(err)
		}
		if len(g.docs) != 1 || len(g.docs[0]) != c.want {
			t.Errorf("%s: documents %v", c.name, g.docs)
		}
	}
}

// disputeDecider says the user disputes when NEXT says "errado".
type disputeDecider struct{ kbDecider }

func (d disputeDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if q, ok := req.Questions["dispute"]; ok {
		st, _ := req.State.(map[string]any)
		p := 0.1
		if strings.Contains(fmt.Sprint(st["next"]), "errado") {
			p = 0.9
		}
		a, _ := decide.AnswerFor(q, map[string]float64{"true": p, "false": 1 - p}, decide.ProbNative)
		return &decide.Response{Answers: map[string]decide.Answer{"dispute": a}}, nil
	}
	return d.kbDecider.Decide(ctx, req)
}

// dispute-001: a lesson the user disputes is not given again, the turn
// says so in FACTS.disputed and journals the lesson; a benign follow-up
// changes nothing.
func TestKnowledgeDisputedLesson(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "docs"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "docs", "estacionamento.md"), []byte("# Estacionamento\n\nParque Seguro.\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "docs", "interno.md"), []byte("# Descontos\n\n10%.\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "docs", "licoes.md"), []byte("# Lições\n\n## Atendem crianças?\n\n<!-- lesson id=criancas kind=fact review=2027-01-01 -->\nAtendemos crianças a partir de 1 ano.\n"), 0o600)
	doc := strings.ReplaceAll(kbAgent, ", check: CHECK", "")
	doc = strings.Replace(doc, "sources: [ { files: [docs/estacionamento.md] } ]", "sources: [ { files: [docs/estacionamento.md, docs/licoes.md] } ]", 1)
	_ = os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(doc), 0o600)
	g := &kbGen{reply: "Atendemos crianças a partir de 1 ano."}
	store := journal.NewMemory()
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "kb", Store: store, Decider: disputeDecider{}, Generator: g})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	patient := map[string]any{"channel": "whatsapp"}
	for _, m := range []string{"atendem crianças?", "isso está errado, me disseram que crianças só a partir de 3 anos"} {
		if _, err := h.Handle(ctx, "c", patient, m); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []string{"atendem crianças?", "e crianças de 2 anos também?"} {
		if _, err := h.Handle(ctx, "b", patient, m); err != nil {
			t.Fatal(err)
		}
	}
	if len(g.docs) != 4 {
		t.Fatalf("free answers %d", len(g.docs))
	}
	has := func(i int) bool { return strings.Contains(strings.Join(g.docs[i], "|"), "1 ano") }
	if !has(0) || has(1) || !has(2) || !has(3) {
		t.Errorf("the lesson given per turn: %v %v %v %v", has(0), has(1), has(2), has(3))
	}
	runs, _ := store.Runs(ctx, journal.RunFilter{Namespace: "kb", Limit: 10})
	var journaled string
	for _, r := range runs {
		recs, _ := store.Records(ctx, r.ID)
		for _, rec := range recs {
			journaled += string(rec.Value)
		}
	}
	if strings.Count(journaled, `"disputedLessons":["criancas"]`) != 1 {
		t.Errorf("the dispute is not journaled once: %s", journaled)
	}

	// With a free-mode handoff, the dispute goes to it and is journaled
	// with its route too.
	doc2 := strings.Replace(doc, `freeMode: { instructions: "Ajude o paciente.", knowledge: { bases: [publico, interno] } }`,
		`freeMode: { instructions: "Ajude o paciente.", knowledge: { bases: [publico, interno] }, functions: [ { function: perguntar, when: "uma pessoa precisa responder" } ] }`, 1)
	if doc2 == doc {
		t.Fatal("fixture unchanged")
	}
	doc2 += "---\napiVersion: agents.tessera.dev/v1alpha1\nkind: Function\nmetadata: { name: perguntar, namespace: kb }\nspec:\n  description: Asks a person.\n  output: { template: \"ok\" }\n"
	_ = os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(doc2), 0o600)
	store2 := journal.NewMemory()
	h2, issues, err := Load(Options{Paths: []string{dir}, Namespace: "kb", Store: store2, Decider: handoffOnDispute{}, Generator: &kbGen{reply: "Atendemos crianças a partir de 1 ano."}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	for _, m := range []string{"atendem crianças?", "isso está errado, me disseram que crianças só a partir de 3 anos"} {
		if _, err := h2.Handle(ctx, "h", patient, m); err != nil {
			t.Fatal(err)
		}
	}
	runs2, _ := store2.Runs(ctx, journal.RunFilter{Namespace: "kb", Limit: 10})
	found := false
	for _, r := range runs2 {
		if r.Function != "perguntar" {
			continue
		}
		recs, _ := store2.Records(ctx, r.ID)
		for _, rec := range recs {
			found = found || strings.Contains(string(rec.Value), `"disputedLessons":["criancas"]`)
		}
	}
	if !found {
		t.Errorf("a handed-off dispute is not journaled")
	}
}

// handoffOnDispute hands off when FACTS.disputed is present.
type handoffOnDispute struct{ disputeDecider }

func (d handoffOnDispute) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if q, ok := req.Questions["handoff"]; ok {
		st, _ := req.State.(map[string]any)
		f, _ := st["facts"].(map[string]any)
		probs := map[string]float64{"answer": 0.9, "perguntar": 0.1}
		if _, disputed := f["disputed"]; disputed {
			probs = map[string]float64{"answer": 0.1, "perguntar": 0.9}
		}
		a, _ := decide.AnswerFor(q, probs, decide.ProbNative)
		return &decide.Response{Answers: map[string]decide.Answer{"handoff": a}}, nil
	}
	return d.disputeDecider.Decide(ctx, req)
}

// fallback-001: after two rejected drafts, a quote found word for word in
// a passage is sent (and journaled as the fallback); an invented one is
// not.
func TestKnowledgeQuoteFallback(t *testing.T) {
	for _, c := range []struct {
		name, quote, want string
	}{
		{"real", `Pela nossa política, "O conveniado é o Parque Seguro: duas horas por R$ 12". A recepção confirma o seu caso.`, "Parque Seguro"},
		{"invented", `Pela nossa política, "O estacionamento é gratuito para pacientes". A recepção confirma.`, "NAO-ENTENDI"},
	} {
		g := &kbGen{reply: "Sim, é grátis.", quote: c.quote}
		h, store := loadKB(t, "true", kbDecider{unsupported: 0.9}, g)
		res, err := h.Handle(context.Background(), "p", map[string]any{"channel": "whatsapp"}, "o estacionamento é de graça?")
		if err != nil || len(res.Outbox) != 1 || !strings.Contains(res.Outbox[0].Text, c.want) {
			t.Fatalf("%s: %+v %v", c.name, res.Outbox, err)
		}
		recs, _ := store.Records(context.Background(), res.RunID)
		marked := false
		for _, r := range recs {
			marked = marked || strings.Contains(string(r.Value), `"fallback":"quote"`)
		}
		if marked != (c.name == "real") {
			t.Errorf("%s: journaled as fallback %v", c.name, marked)
		}
	}
}

// The grounding question is the measured one (knowledge-001/003) for a
// single message; what the agent already said and the web join it only
// when present (agency-001: with them always in the text, overreach on
// the fallback set's traps rose from 1 to 4 of 27).
func TestUnsupportedQuestionIsTheMeasuredOne(t *testing.T) {
	h, _, _ := freeHost(t)
	d := &decidetest.Scripted{Err: errors.New("capture only")}
	h.opts.Decider = d
	h.unsupported(context.Background(), map[string]any{}, nil, "posso?", "pode")
	first := d.Calls()[0].Questions["unsupported"].Instructions
	if strings.Contains(first, "AGENTSAID") || strings.Contains(first, "FACTS.web") {
		t.Errorf("a single message gets the measured question: %s", first)
	}
	if _, ok := d.Calls()[0].State.(map[string]any)["agentSaid"]; ok {
		t.Error("no agentSaid without earlier agent messages")
	}
	h.unsupported(context.Background(), map[string]any{"web": []any{"x"}}, []string{"user: oi", "agent: Hotel em Ponta Negra, R$ 1.820"}, "é perto da praia?", "fica em Ponta Negra")
	second := d.Calls()[1]
	if !strings.Contains(second.Questions["unsupported"].Instructions, "AGENTSAID") || !strings.Contains(second.Questions["unsupported"].Instructions, "FACTS.web") {
		t.Errorf("the agent's earlier messages and the web are named when present")
	}
	said, _ := second.State.(map[string]any)["agentSaid"].([]any)
	if len(said) != 1 || said[0] != "Hotel em Ponta Negra, R$ 1.820" {
		t.Errorf("only the agent's lines support a reply: %v", said)
	}
}

type fixedText struct{ text string }

func (fixedText) Name() string { return "fixed" }
func (g fixedText) Generate(context.Context, generate.Request) (*generate.Response, error) {
	return &generate.Response{Text: g.text}, nil
}

// agency-002: with no documents, a right answer from the web dropped twice
// becomes the page's own sentence, with its site, checked word for word.
func TestWebQuoteFallback(t *testing.T) {
	h, _, _ := freeHost(t)
	h.opts.Decider = onSubject{}
	facts := map[string]any{"web": []any{map[string]any{"site": "vistos.mne.gov.pt", "snippet": "Brasileiros não precisam de visto para turismo por até 90 dias. Passaporte válido."},
		map[string]any{"site": "localiza.com", "snippet": "Diárias a partir de R$ 99 no aeroporto de Natal."}}}
	for _, c := range []struct {
		reply string
		sent  bool
	}{
		{`Segundo o site vistos.mne.gov.pt, "Brasileiros não precisam de visto para turismo por até 90 dias". Confirme com o consulado.`, true},
		{`Segundo o site vistos.mne.gov.pt, "Brasileiros nunca precisam de visto". Confirme com o consulado.`, false},
		{`Segundo um site, "Brasileiros não precisam de visto para turismo por até 90 dias". Confirme com o consulado.`, false},
		// agency-006: a page's own words about something else.
		{`Segundo o site localiza.com, "Diárias a partir de R$ 99 no aeroporto de Natal". Confirme com a locadora.`, false},
	} {
		h.Runtime.Generator = fixedText{c.reply}
		got := h.quoteFallback(context.Background(), facts, "preciso de visto pra Portugal?")
		if (got != "") != c.sent {
			t.Errorf("%q: sent %v, want %v", c.reply, got != "", c.sent)
		}
	}
}

// onSubject answers quoteAnswers: a quote about visas answers a message
// about visas.
type onSubject struct{ turnDecider }

func (d onSubject) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	q, ok := req.Questions["answers"]
	if !ok {
		return d.turnDecider.Decide(ctx, req)
	}
	state, _ := req.State.(map[string]any)
	quote, _ := state["quote"].(string)
	yes := 0.05
	if strings.Contains(quote, "visto") {
		yes = 0.95
	}
	a, err := decide.AnswerFor(q, map[string]float64{"true": yes, "false": 1 - yes}, decide.ProbNative)
	if err != nil {
		return nil, err
	}
	return &decide.Response{Answers: map[string]decide.Answer{"answers": a}, Model: "subject"}, nil
}
