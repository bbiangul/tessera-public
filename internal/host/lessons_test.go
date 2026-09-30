package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const lessonsAgent = `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: ls }
spec:
  humanInTheLoop: { queue: recepcao }
  freeMode: { instructions: "Ajude.", knowledge: { bases: [licoes] } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: licoes, namespace: ls }
spec: { description: Lições., lessons: { every: 24h } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: ls }
spec: { roles: [recepcao] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: ls }
spec:
  members:
    - { id: rita, name: Rita, roles: [recepcao] }
    - { id: gil, name: Gil, roles: [gerencia] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: gerencia, namespace: ls }
spec: { roles: [gerencia] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: ls }
spec:
  description: Opening hours.
  selector: { intent: horarios, description: "Horário." }
  output: { template: "8h" }
`

// lessonEngines fake learn's questions by keywords, and record the free
// mode's documents.
type lessonEngines struct{ docs []string }

func topicOf(s string) string {
	for _, t := range []string{"estacionamento", "wifi", "café"} {
		if strings.Contains(strings.ToLower(s), t) {
			return t
		}
	}
	return s
}

func (lessonEngines) Name() string { return "fake" }
func (e *lessonEngines) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	b, _ := json.Marshal(req.State)
	s := string(b)
	resp := &decide.Response{Answers: map[string]decide.Answer{}}
	for id, q := range req.Questions {
		probs := map[string]float64{}
		if q.Kind == decide.KindChoice {
			for _, o := range q.Options {
				probs[o.Key] = 0.01
			}
			probs["none"], probs["answer"] = 0.99, 0.99
			for k := range probs {
				ok := false
				for _, o := range q.Options {
					ok = ok || o.Key == k
				}
				if !ok {
					delete(probs, k)
				}
			}
		} else {
			p := 0.1
			switch {
			case id == "same":
				var st map[string]any
				_ = json.Unmarshal(b, &st)
				if topicOf(st["a"].(string)) == topicOf(st["b"].(string)) {
					p = 0.9
				}
			case id == "contradict":
				if strings.Contains(s, "Não temos") && strings.Contains(s, "Temos sim") {
					p = 0.9
				}
			case len(id) == 2 && id[0] == 'p':
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

func (e *lessonEngines) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	st, _ := req.State.(map[string]any)
	switch {
	case strings.HasPrefix(req.Instructions, "Rewrite ANSWER"):
		return &generate.Response{Text: st["answer"].(string)}, nil
	case strings.Contains(req.Instructions, "Write the lesson"):
		b, _ := json.Marshal(map[string]string{"question": "Tem " + topicOf(st["question"].(string)) + "?", "fact": st["answer"].(string), "kind": "fact"})
		return &generate.Response{Text: string(b)}, nil
	case strings.HasPrefix(req.Instructions, "MESSAGE is a customer's question"):
		return &generate.Response{Text: ""}, nil
	}
	facts, _ := st["facts"].(map[string]any)
	d, _ := json.Marshal(facts["documents"])
	e.docs = append(e.docs, string(d))
	return &generate.Response{Text: "ok"}, nil
}

func answered(t *testing.T, st journal.Store, id, question, answer string, at time.Time) {
	t.Helper()
	ctx := context.Background()
	scopes, _ := journal.Encode(map[string]any{"event": map[string]any{"message": question}})
	if err := st.CreateRun(ctx, journal.Run{ID: id, Namespace: "ls", Function: "perguntar", Conversation: id, Scopes: scopes, CreatedAt: at}); err != nil {
		t.Fatal(err)
	}
	sus, _ := journal.Encode(map[string]any{"to": "queue:recepcao", "task": "answer", "text": question})
	res, _ := journal.Encode(map[string]any{"from": "human", "text": answer, "by": "rita"})
	_, _ = st.Append(ctx, id, journal.Record{Site: "p", Kind: journal.KindSuspend, Value: sus, At: at})
	_, _ = st.Append(ctx, id, journal.Record{Site: "p", Kind: journal.KindResume, Value: res, At: at})
}

// learn-003 C1-C4: proposals become approval tasks once; approved
// lessons are written with who approved them and used; denials and roles
// hold; a contradiction supersedes the lesson it replaces.
func TestLessonsInTheChannel(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(lessonsAgent), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(data, "lessons", "ls", "licoes.md")
	_ = os.MkdirAll(filepath.Dir(file), 0o700)
	_ = os.WriteFile(file, []byte("# Lições\n\n## Tem wifi?\n\n<!-- lesson id=wifi kind=fact review=2027-01-01 cases=2 -->\nTemos sim, a senha fica na recepção.\n"), 0o600)
	store := journal.NewMemory()
	now := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	answered(t, store, "r1", "tem estacionamento?", "O conveniado é o Parque Seguro.", now.Add(-time.Hour))
	answered(t, store, "r2", "onde paro? estacionamento", "O estacionamento conveniado é o Parque Seguro.", now.Add(-time.Hour))
	answered(t, store, "r3", "tem wifi?", "Não temos wi-fi.", now.Add(-time.Hour))
	answered(t, store, "r4", "wifi pra paciente?", "Não temos wi-fi para pacientes.", now.Add(-time.Hour))
	answered(t, store, "r5", "tem café?", "Tem café na recepção.", now.Add(-time.Hour))
	answered(t, store, "r6", "café na espera?", "Temos café na recepção.", now.Add(-time.Hour))
	eng := &lessonEngines{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "ls", Store: store, Decider: eng, Generator: eng, DataDir: data,
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	tasks := func() map[string]runtime.Task {
		ts, err := h.Runtime.MemberTasks(ctx, "ls", "rita")
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]runtime.Task{}
		for _, x := range ts {
			out[topicOf(x.Text)] = x
		}
		return out
	}
	if _, err := h.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	ts := tasks()
	if len(ts) != 3 || !strings.Contains(ts["estacionamento"].Text, "Resposta: O ") || !strings.Contains(ts["wifi"].Text, "Substitui: Temos sim") || ts["café"].Kind != "approve" {
		t.Fatalf("tasks after the first tick: %+v", ts)
	}
	now = now.Add(time.Hour)
	_, _ = h.Tick(ctx)
	now = now.Add(24 * time.Hour)
	_, _ = h.Tick(ctx)
	if n := len(tasks()); n != 3 {
		t.Errorf("tasks after more ticks: %d (proposed again)", n)
	}

	// C3: a member without the queue's role cannot approve.
	gil, _ := h.Runtime.Member("ls", "gil")
	if out, err := h.Runtime.AnswerTask(ctx, ts["estacionamento"].RunID, ts["estacionamento"].Hash, gil, true); out != runtime.NotAllowed || err != nil {
		t.Errorf("gil approved: %v %v", out, err)
	}
	rita, _ := h.Runtime.Member("ls", "rita")
	// C2: approved, written with who approved, and used.
	if out, err := h.Runtime.AnswerTask(ctx, ts["estacionamento"].RunID, ts["estacionamento"].Hash, rita, true); out != runtime.Recorded || err != nil {
		t.Fatalf("estacionamento: %v %v", out, err)
	}
	// C4: the contradiction, approved, supersedes.
	if out, err := h.Runtime.AnswerTask(ctx, ts["wifi"].RunID, ts["wifi"].Hash, rita, true); out != runtime.Recorded || err != nil {
		t.Fatalf("wifi: %v %v", out, err)
	}
	// C3: denied, nothing written.
	if out, err := h.Runtime.AnswerTask(ctx, ts["café"].RunID, ts["café"].Hash, rita, false); out != runtime.Recorded || err != nil {
		t.Fatalf("café: %v %v", out, err)
	}
	raw, _ := os.ReadFile(file)
	s := string(raw)
	if !strings.Contains(s, "Parque Seguro") || !strings.Contains(s, "approvedBy=rita") || strings.Contains(s, "café") {
		t.Errorf("lessons file:\n%s", s)
	}
	if !strings.Contains(s, "id=wifi kind=fact review=2027-01-01 cases=2 ") || !strings.Contains(s, " status=retired") || !strings.Contains(s, " reason=superseded") || !strings.Contains(s, "Não temos wi-fi") {
		t.Errorf("supersede:\n%s", s)
	}
	if _, err := h.Handle(ctx, "p", map[string]any{"channel": "whatsapp"}, "tem estacionamento?"); err != nil {
		t.Fatal(err)
	}
	if len(eng.docs) == 0 || !strings.Contains(eng.docs[len(eng.docs)-1], "Parque Seguro") {
		t.Errorf("the approved lesson was not used: %v", eng.docs)
	}
	// A retried accept (same approval) writes nothing and does not fail,
	// even when it superseded.
	retry := act.Call{Action: "lessons/accept", IdempotencyKey: "retry-1", Args: map[string]any{"base": "licoes", "id": "wifi-2", "question": "Tem wifi?", "fact": "Agora temos wifi.", "kind": "fact", "cases": int64(2), "members": "rita", "runs": "x,y", "supersede": "tem-wifi"}}
	for i := 0; i < 2; i++ {
		if _, err := h.Runtime.Actor.Call(ctx, retry); err != nil {
			t.Fatalf("accept %d: %v", i, err)
		}
	}
	if raw, _ := os.ReadFile(file); strings.Count(string(raw), "Agora temos wifi.") != 1 {
		t.Errorf("a retried accept wrote twice:\n%s", raw)
	}
	// Denied, and reworded later: not proposed again.
	now = now.Add(25 * time.Hour)
	_, _ = h.Tick(ctx)
	if ts := tasks(); len(ts) != 0 {
		t.Errorf("proposed again after decisions: %+v", ts)
	}
}
