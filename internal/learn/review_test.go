package learn

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/knowledge"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const lessonsFile = `# Lições

## Atendem crianças?

<!-- lesson id=criancas kind=fact learned=2026-09-01 review=2027-09-01 cases=2 -->
A partir de 1 ano.

## Sedação?

<!-- lesson id=sedacao kind=price learned=2026-09-01 review=2026-10-01 cases=2 -->
R$ 250 por sessão.

## Bebedouro?

<!-- lesson id=bebedouro kind=fact learned=2026-03-01 review=2027-03-01 cases=2 -->
Temos bebedouro.

## Invisalign?

<!-- lesson id=invisalign kind=price learned=2026-06-01 review=2026-07-01 cases=2 -->
A partir de R$ 9.800.
`

var now = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

// freeTurn journals a free answer that was given lesson, then the user's
// next message in the same conversation.
func freeTurn(t *testing.T, st *journal.Memory, conv, lesson, msg, reply, next string) {
	t.Helper()
	ctx := context.Background()
	scopes, _ := journal.Encode(map[string]any{"event": map[string]any{"message": msg}})
	id := conv + "-1"
	_ = st.CreateRun(ctx, journal.Run{ID: id, Namespace: "ns", Function: "(free)", Conversation: conv, Scopes: scopes, CreatedAt: now})
	dec, _ := journal.Encode(map[string]any{"choice": "none", "documents": []any{map[string]any{"lesson": lesson, "source": "licoes.md"}}})
	send, _ := journal.Encode(map[string]any{"to": "user", "text": reply})
	_, _ = st.Append(ctx, id, journal.Record{Site: "router", Kind: journal.KindResult, Value: dec, At: now})
	_, _ = st.Append(ctx, id, journal.Record{Site: "free/0", Kind: journal.KindSend, Value: send, At: now})
	scopes2, _ := journal.Encode(map[string]any{"event": map[string]any{"message": next}, "conversation": map[string]any{"earlier": []any{"user: " + msg, "agent: " + reply}}})
	_ = st.CreateRun(ctx, journal.Run{ID: conv + "-2", Namespace: "ns", Function: "(free)", Conversation: conv, Scopes: scopes2, CreatedAt: now})
}

func TestReview(t *testing.T) {
	file := filepath.Join(t.TempDir(), "licoes.md")
	if err := os.WriteFile(file, []byte(lessonsFile), 0o600); err != nil {
		t.Fatal(err)
	}
	lessons, err := ReadLessons(file)
	if err != nil || len(lessons) != 4 || lessons[0].ID != "criancas" || lessons[0].Question != "Atendem crianças?" || lessons[0].Fact != "A partir de 1 ano." {
		t.Fatalf("lessons %+v %v", lessons, err)
	}
	st := journal.NewMemory()
	freeTurn(t, st, "c1", "criancas", "atendem bebê?", "Sim, a partir de 1 ano.", "essa informação tá errada, é a partir de 3")
	freeTurn(t, st, "c2", "criancas", "minha filha de 2 anos?", "Sim, a partir de 1 ano.", "errada, me disseram 3 anos")
	freeTurn(t, st, "c3", "criancas", "idade mínima?", "A partir de 1 ano.", "obrigada")
	freeTurn(t, st, "c4", "sedacao", "sedação?", "R$ 250.", "acho que a informação tá errada")
	freeTurn(t, st, "c5", "sedacao", "gás?", "R$ 250.", "ótimo")
	freeTurn(t, st, "c6", "sedacao", "óxido nitroso?", "R$ 250.", "obrigada")
	// Other turns of c6, before and after, that do not follow it: never
	// taken for its next message.
	for i, at := range []time.Time{now.Add(-time.Hour), now.Add(time.Hour)} {
		sc, _ := journal.Encode(map[string]any{"event": map[string]any{"message": "errada demais"}, "conversation": map[string]any{"earlier": []any{"user: outra coisa"}}})
		_ = st.CreateRun(context.Background(), journal.Run{ID: "c6-decoy" + string(rune('a'+i)), Namespace: "ns", Function: "(free)", Conversation: "c6", Scopes: sc, CreatedAt: at})
	}
	rs, err := Review(context.Background(), []Store{st}, lessons, ReviewOptions{Namespace: "ns", Now: now, Decider: fakeDecider{}})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]LessonReview{}
	for _, r := range rs {
		by[r.ID] = r
	}
	if c := by["criancas"]; !c.Harmful || c.Disputes != 2 || len(c.Uses) != 3 || c.Dead {
		t.Errorf("criancas %+v", c)
	}
	// One dispute in three uses is not harm.
	if s := by["sedacao"]; s.Harmful || s.Disputes != 1 || len(s.Uses) != 3 || !s.Due || s.Expired || s.Dead {
		t.Errorf("sedacao %+v", s)
	}
	if b := by["bebedouro"]; !b.Dead || b.Harmful {
		t.Errorf("bebedouro %+v", b)
	}
	if i := by["invisalign"]; !i.Expired || !i.Due {
		t.Errorf("invisalign %+v", i)
	}

	// Suspending, renewing and retiring edit the comment only.
	if _, err := SetMeta(file, []string{"criancas"}, map[string]string{"status": "suspended", "reason": "harmful"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Renew(file, []string{"invisalign"}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := SetMeta(file, []string{"nada"}, map[string]string{"status": "retired"}); err == nil {
		t.Errorf("an unknown lesson was changed")
	}
	raw, _ := os.ReadFile(file)
	passages := knowledge.Split(string(raw))
	active := map[string]bool{}
	for _, p := range passages {
		if strings.Contains(p.Text, "status=") || strings.Contains(p.Text, "lesson id") {
			t.Errorf("metadata reached a passage: %+v", p)
		}
		active[p.Meta["id"]] = knowledge.Active(p, now)
	}
	if active["criancas"] || !active["invisalign"] || !active["sedacao"] || !active["bebedouro"] {
		t.Errorf("active after the edits: %v\n%s", active, raw)
	}
	if !strings.Contains(string(raw), "review=2026-10-27") {
		t.Errorf("renewed price lesson:\n%s", raw)
	}
}

// Part C: a proposal on the same question as an active lesson is reported
// against it: contradicting, or reconfirming.
func TestProposeAgainstLessons(t *testing.T) {
	st := journal.NewMemory()
	answeredRun(t, st, "r1", "tem wifi?", "Não temos wi-fi, desculpe.")
	answeredRun(t, st, "r2", "wifi pra paciente?", "Não temos wi-fi para pacientes.")
	answeredRun(t, st, "r3", "tem estacionamento?", "Parque Seguro, R$ 12.")
	answeredRun(t, st, "r4", "estacionamento?", "O estacionamento é o Parque Seguro, R$ 12.")
	lessons := []Lesson{
		{ID: "wifi", Question: "Tem wifi?", Fact: "Temos sim, a senha fica na recepção.", Meta: map[string]string{}},
		{ID: "estacionamento", Question: "Tem estacionamento?", Fact: "Parque Seguro, R$ 12.", Meta: map[string]string{}},
	}
	r, err := Propose(context.Background(), []Store{st}, Options{Namespace: "ns", Decider: fakeDecider{}, Generator: fakeGen{}, Lessons: lessons})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Proposals) != 0 || len(r.Contradicts) != 1 || r.Contradicts[0].Lesson != "wifi" || len(r.Reconfirms) != 1 || r.Reconfirms[0].Lesson != "estacionamento" {
		t.Errorf("proposals %d, contradicts %+v, reconfirms %+v", len(r.Proposals), r.Contradicts, r.Reconfirms)
	}
	// A retired lesson is not compared.
	lessons[0].Meta["status"] = "retired"
	r, _ = Propose(context.Background(), []Store{st}, Options{Namespace: "ns", Decider: fakeDecider{}, Generator: fakeGen{}, Lessons: lessons})
	if len(r.Proposals) != 1 || len(r.Contradicts) != 0 {
		t.Errorf("with the wifi lesson retired: proposals %d contradicts %d", len(r.Proposals), len(r.Contradicts))
	}
}

// A new lesson on the same question supersedes the old one: both stay in
// the file, the old retired, the new active with its own id.
func TestSupersede(t *testing.T) {
	file := filepath.Join(t.TempDir(), "licoes.md")
	if err := os.WriteFile(file, []byte(lessonsFile), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Proposal{ID: "invisalign", Question: "Invisalign?", Fact: "A partir de R$ 11.500.", Kind: "price"}
	wrote, err := AcceptSuperseding(file, []Proposal{p}, []string{"invisalign"}, []string{"invisalign"}, now)
	if err != nil || len(wrote) != 1 || wrote[0] != "invisalign-20260927" {
		t.Fatalf("wrote %v %v", wrote, err)
	}
	lessons, _ := ReadLessons(file)
	status := map[string]string{}
	for _, l := range lessons {
		status[l.ID] = l.Status()
	}
	if status["invisalign"] != "retired" || status["invisalign-20260927"] != "active" {
		t.Errorf("status %v", status)
	}
	// Nothing written: nothing retired.
	if _, err := AcceptSuperseding(file, []Proposal{p}, []string{"invisalign"}, []string{"sedacao"}, now); err == nil {
		t.Errorf("superseded without writing")
	}
}
