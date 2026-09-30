package learn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/knowledge"
	"github.com/bbiangul/tessera/internal/mask"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

var t0 = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

// answeredRun journals a run where the customer asked question and a
// person answered answer to an escalate task of kind answer.
func answeredRun(t *testing.T, st *journal.Memory, id, question, answer string) {
	t.Helper()
	ctx := context.Background()
	scopes, _ := journal.Encode(map[string]any{"event": map[string]any{"message": question}})
	if err := st.CreateRun(ctx, journal.Run{ID: id, Namespace: "ns", Function: "perguntar", Conversation: id, Scopes: scopes, CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	sus, _ := journal.Encode(map[string]any{"to": "queue:recepcao", "task": "answer", "text": "Paciente pergunta: " + question})
	res, _ := journal.Encode(map[string]any{"from": "human", "text": answer, "by": "rita"})
	for _, r := range []journal.Record{{Site: "p", Kind: journal.KindSuspend, Value: sus, At: t0}, {Site: "p", Kind: journal.KindResume, Value: res, At: t0.Add(time.Minute)}} {
		if _, err := st.Append(ctx, id, r); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeDecider answers by keywords of the state.
type fakeDecider struct{}

func (fakeDecider) Name() string { return "fake" }
func (fakeDecider) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	b, _ := json.Marshal(req.State)
	s := string(b)
	resp := &decide.Response{Answers: map[string]decide.Answer{}}
	for id, q := range req.Questions {
		p := 0.1
		switch id {
		case "personal":
			if strings.Contains(s, "sua consulta") {
				p = 0.9
			}
		case "adds":
			if strings.Contains(s, "grátis") {
				p = 0.9
			}
		case "same":
			var st map[string]string
			_ = json.Unmarshal(b, &st)
			if topic(st["a"]) == topic(st["b"]) {
				p = 0.9
			}
		case "contradict":
			if strings.Contains(s, "Não temos") && strings.Contains(s, "Temos sim") {
				p = 0.9
			}
		case "dispute":
			if strings.Contains(s, "errada") {
				p = 0.9
			}
		}
		a, err := decide.AnswerFor(q, map[string]float64{"true": p, "false": 1 - p}, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

func topic(s string) string {
	for _, t := range []string{"estacionamento", "wifi", "criança", "desfibrilador"} {
		if strings.Contains(strings.ToLower(s), t) {
			return t
		}
	}
	return s
}

// fakeGen writes the lesson from the answer, adding "grátis" when the
// answer says "promo" (an unfaithful lesson).
type fakeGen struct{}

func (fakeGen) Name() string { return "fake" }
func (fakeGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	st, _ := req.State.(map[string]any)
	if strings.HasPrefix(req.Instructions, "Rewrite ANSWER without greetings") {
		a, _ := st["answer"].(string)
		return &generate.Response{Text: strings.TrimPrefix(a, "Oi Ana, ")}, nil
	}
	q, _ := st["question"].(string)
	a, _ := st["answer"].(string)
	fact := a
	if strings.Contains(a, "promo") {
		fact += " A consulta é grátis."
	}
	kind := "fact"
	if strings.Contains(a, "R$") {
		kind = "price"
	}
	b, _ := json.Marshal(map[string]string{"question": topic(q) + "?", "fact": fact, "kind": kind})
	return &generate.Response{Text: string(b)}, nil
}

func TestPropose(t *testing.T) {
	st := journal.NewMemory()
	answeredRun(t, st, "r1", "tem estacionamento?", "Temos o Parque Seguro, R$ 12 as duas horas. Dúvidas: ana@exemplo.com")
	answeredRun(t, st, "r2", "onde paro o carro? estacionamento", "O estacionamento conveniado é o Parque Seguro, R$ 12.")
	answeredRun(t, st, "r3", "tem wifi?", "Temos sim, a senha fica na recepção.")
	answeredRun(t, st, "r4", "wifi pra paciente?", "Não temos wi-fi, desculpe.")
	answeredRun(t, st, "r5", "minha consulta tá confirmada?", "Oi Ana, sua consulta está confirmada às 14h.")
	answeredRun(t, st, "r6", "tem desfibrilador?", "Temos um DEA na recepção.")
	answeredRun(t, st, "r7", "atende criança?", "Sim, a partir de 3 anos. promo")
	answeredRun(t, st, "r8", "atende criança pequena?", "Sim, a partir de 3 anos. promo")
	// An answer task closed by its SLA timeout: nobody answered it.
	scopes, _ := journal.Encode(map[string]any{"event": map[string]any{"message": "tem estacionamento?"}})
	ctx := context.Background()
	_ = st.CreateRun(ctx, journal.Run{ID: "r9", Namespace: "ns", Conversation: "r9", Scopes: scopes, CreatedAt: t0})
	sus, _ := journal.Encode(map[string]any{"to": "queue:recepcao", "task": "answer", "text": "?"})
	res, _ := journal.Encode(map[string]any{"from": "timeout", "text": "Temos estacionamento grátis."})
	_, _ = st.Append(ctx, "r9", journal.Record{Site: "p", Kind: journal.KindSuspend, Value: sus, At: t0})
	_, _ = st.Append(ctx, "r9", journal.Record{Site: "p", Kind: journal.KindResume, Value: res, At: t0})
	r, err := Propose(context.Background(), []Store{st}, Options{Namespace: "ns", Decider: fakeDecider{}, Generator: fakeGen{}, Mask: mask.Text})
	if err != nil {
		t.Fatal(err)
	}
	if r.Answered != 8 {
		t.Fatalf("answered %d", r.Answered)
	}
	if len(r.Proposals) != 1 || r.Proposals[0].Question != "estacionamento?" || len(r.Proposals[0].Evidence) != 2 || r.Proposals[0].Kind != "price" {
		t.Fatalf("proposals %+v", r.Proposals)
	}
	if strings.Contains(r.Proposals[0].Fact, "ana@exemplo.com") {
		t.Errorf("an e-mail reached a lesson: %s", r.Proposals[0].Fact)
	}
	if len(r.Conflicts) != 1 || !strings.Contains(r.Conflicts[0].Question, "wifi") {
		t.Errorf("conflicts %+v", r.Conflicts)
	}
	if len(r.Pending) != 1 || r.Pending[0].Run != "r6" {
		t.Errorf("below threshold %+v", r.Pending)
	}
	var personal, unfaithful int
	for _, c := range r.Rejected {
		switch {
		case strings.HasPrefix(c.Rejected, "personal"):
			personal++
		case strings.HasPrefix(c.Rejected, "unfaithful"):
			unfaithful++
		}
	}
	if personal != 1 || unfaithful != 2 {
		t.Errorf("rejected %+v", r.Rejected)
	}
	// learn-002: a rejected lesson keeps what was written, for review;
	// the personal answer was judged without the name.
	for _, c := range r.Rejected {
		if strings.HasPrefix(c.Rejected, "unfaithful") && !strings.Contains(c.Fact, "grátis") {
			t.Errorf("a rejected lesson lost its text: %+v", c)
		}
		if strings.HasPrefix(c.Rejected, "personal") && strings.Contains(c.Nameless, "Ana") {
			t.Errorf("the name reached the decision: %+v", c)
		}
	}
	// --min 1: the single case is proposed too.
	r1, _ := Propose(context.Background(), []Store{st}, Options{Namespace: "ns", Min: 1, Decider: fakeDecider{}, Generator: fakeGen{}})
	if len(r1.Proposals) != 2 {
		t.Errorf("--min 1: %d proposals", len(r1.Proposals))
	}
}

func TestAccept(t *testing.T) {
	file := filepath.Join(t.TempDir(), "licoes.md")
	ps := []Proposal{{ID: "estacionamento", Question: "Tem estacionamento?", Fact: "O conveniado é o Parque Seguro.", Kind: "price", Members: []string{"rita"}, Evidence: []Case{{Run: "r1"}, {Run: "r2"}}}}
	wrote, err := Accept(file, ps, []string{"estacionamento"}, t0)
	if err != nil || len(wrote) != 1 {
		t.Fatalf("%v %v", wrote, err)
	}
	if wrote, err = Accept(file, ps, []string{"estacionamento"}, t0); err != nil || len(wrote) != 0 {
		t.Errorf("accepted twice: %v %v", wrote, err)
	}
	if _, err := Accept(file, ps, []string{"nada"}, t0); err == nil {
		t.Errorf("an unknown id was accepted")
	}
	raw, _ := os.ReadFile(file)
	s := string(raw)
	if !strings.Contains(s, "## Tem estacionamento?") || !strings.Contains(s, "review=2026-10-27") || !strings.Contains(s, "runs=r1,r2") {
		t.Errorf("lessons file:\n%s", s)
	}
	// The provenance is metadata: the index never returns it as text.
	for _, p := range knowledge.Split(s) {
		if strings.Contains(p.Text, "lesson id=") || strings.Contains(p.Text, "runs=") {
			t.Errorf("a comment reached a passage: %+v", p)
		}
	}
}

func TestID(t *testing.T) {
	if got := id("Tem aquele gás para relaxar? Anestesia é possível"); got != "tem-aquele-gas-para-relaxar-anestesia-e" {
		t.Errorf("id %q", got)
	}
}
