package host

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const liveAgent = `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: learn }
spec:
  humanInTheLoop: { queue: recepcao }
  freeMode: { instructions: "Tire dúvidas dos pacientes sobre a clínica. Para o que você não souber, indique a recepção.", self: false, knowledge: { bases: [publico, licoes] } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: publico, namespace: learn }
spec: { description: Documentos., sources: [ { files: ["DOCS/convenios.md", "DOCS/precos-particular.md", "DOCS/pagamento.md", "DOCS/cancelamento.md", "DOCS/localizacao.md", "DOCS/pos-extracao.md", "DOCS/clareamento.md"] } ] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: licoes, namespace: learn }
spec: { description: Lições., lessons: { every: 168h } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: learn }
spec: { roles: [recepcao] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: learn }
spec: { members: [ { id: rita, name: Rita, roles: [recepcao] } ] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: learn }
spec:
  description: Opening hours.
  selector: { intent: horarios, description: "O paciente quer saber o horário de funcionamento." }
  output: { template: "Segunda a sexta, das 8h às 19h." }
`

// learn-003 C5, live (TESSERA_LIVE=1, TYPESAFE_API_KEY, OPENROUTER_API_KEY):
// learn-002's phase-1 journal, a tick with real engines, every task
// approved as a member, then held-out questions to the free mode.
func TestLessonsLive(t *testing.T) {
	if os.Getenv("TESSERA_LIVE") == "" {
		t.Skip("live: set TESSERA_LIVE=1")
	}
	root, _ := filepath.Abs("../..")
	src, err := os.Open(filepath.Join(root, "bench/results/learn-002/phase1.db"))
	if err != nil {
		t.Fatal(err)
	}
	dbFile := filepath.Join(t.TempDir(), "runs.db")
	dst, _ := os.Create(dbFile)
	_, _ = io.Copy(dst, src)
	_ = src.Close()
	_ = dst.Close()
	store, err := journal.OpenSQLite(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	docs := filepath.Join(root, "bench/knowledge/docs")
	agent := liveAgent
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(strings.ReplaceAll(agent, "DOCS", docs)), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &decide.TypeSafe{APIKey: strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")), Model: "jev-1.13.0"}
	g := &generate.OpenRouter{APIKey: strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))}
	g.Model, g.Providers, _ = generate.ParseModel("deepseek/deepseek-v4.1-flash")
	now := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "learn", Store: store, Decider: d, Generator: g, DataDir: t.TempDir(),
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	if _, err := h.Tick(ctx); err != nil {
		t.Logf("tick: %v", err)
	}
	tasks, _ := h.Runtime.MemberTasks(ctx, "learn", "rita")
	t.Logf("%d task(s)", len(tasks))
	rita, _ := h.Runtime.Member("learn", "rita")
	for _, x := range tasks {
		t.Logf("TASK: %q", x.Text)
		for _, name := range []string{"Fernanda", "Diego", "Sônia", "Paulo", "Renata", "Célia", "Tiago", "Lúcia", "Márcia", "Otávio", "Beatriz", "Gustavo", "Helena", "Igor", "Nina", "Caio"} {
			if strings.Contains(x.Text, name) {
				t.Errorf("a task names %s", name)
			}
		}
		if out, err := h.Runtime.AnswerTask(ctx, x.RunID, x.Hash, rita, true); out != runtime.Recorded || err != nil {
			t.Errorf("approve: %v %v", out, err)
		}
	}
	held := map[string][]string{
		"o clareamento de vcs usa laser?":             {"LED"},
		"quanto tempo leva a limpeza?":                {"40"},
		"vcs dão nota fiscal pra eu pedir reembolso?": {"dia do pagamento", "recepção"},
		"posso ir com meu gato?":                      {"cão-guia", "cão guia"},
	}
	answered := 0
	for q, want := range held {
		res, err := h.Handle(ctx, "held:"+q, map[string]any{"channel": "whatsapp"}, q)
		if err != nil {
			t.Fatal(err)
		}
		var reply []string
		for _, o := range res.Outbox {
			reply = append(reply, o.Text)
		}
		r := strings.Join(reply, " ")
		ok := false
		for _, w := range want {
			ok = ok || strings.Contains(strings.ToLower(r), strings.ToLower(w))
		}
		if ok {
			answered++
		}
		t.Logf("HELD %q -> %q (%v)", q, r, ok)
	}
	t.Logf("RESULT tasks=%d answered=%d/4", len(tasks), answered)
	if len(tasks) < 3 || answered < 3 {
		t.Errorf("C5: tasks %d (want >= 3), answered %d of 4 (want >= 3)", len(tasks), answered)
	}
}

// renew-001 W4, live: learn-003's four approved lessons, their review
// date yesterday, over learn-002's journal (which repeats them): renew
// tasks citing the repeated answers; approved, the lessons are used again.
func TestRenewLive(t *testing.T) {
	if os.Getenv("TESSERA_LIVE") == "" {
		t.Skip("live: set TESSERA_LIVE=1")
	}
	root, _ := filepath.Abs("../..")
	src, err := os.Open(filepath.Join(root, "bench/results/learn-002/phase1.db"))
	if err != nil {
		t.Fatal(err)
	}
	dbFile := filepath.Join(t.TempDir(), "runs.db")
	dst, _ := os.Create(dbFile)
	_, _ = io.Copy(dst, src)
	_ = src.Close()
	_ = dst.Close()
	store, err := journal.OpenSQLite(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	dir, data := t.TempDir(), t.TempDir()
	lessons := filepath.Join(data, "lessons", "learn", "licoes.md")
	_ = os.MkdirAll(filepath.Dir(lessons), 0o700)
	_ = os.WriteFile(lessons, []byte(`# Lições aprendidas

## Vocês fazem clareamento a laser?

<!-- lesson id=voces-fazem-clareamento-a-laser kind=fact learned=2025-09-27 review=2026-09-27 cases=2 -->
Não fazemos laser; o clareamento de consultório é com gel e luz LED.

## quanto tempo dura uma limpeza?

<!-- lesson id=quanto-tempo-dura-uma-limpeza kind=fact learned=2025-09-27 review=2026-09-27 cases=2 -->
Uma limpeza dura uns 40 minutos.

## vocês emitem nota fiscal?

<!-- lesson id=voces-emitem-nota-fiscal kind=policy learned=2026-03-31 review=2026-09-27 cases=2 -->
Emitimos sim: a nota sai no dia do pagamento, é só pedir na recepção.

## aceitam animais na clínica?

<!-- lesson id=aceitam-animais-na-clinica kind=policy learned=2026-03-31 review=2026-09-27 cases=2 -->
Só cão-guia, outros animais não podem entrar.
`), 0o600)
	docs := filepath.Join(root, "bench/knowledge/docs")
	agent := strings.ReplaceAll(liveAgent, "DOCS", docs)
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(agent), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &decide.TypeSafe{APIKey: strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")), Model: "jev-1.13.0"}
	g := &generate.OpenRouter{APIKey: strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))}
	g.Model, g.Providers, _ = generate.ParseModel("deepseek/deepseek-v4.1-flash")
	now := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "learn", Store: store, Decider: d, Generator: g, DataDir: data,
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	if _, err := h.Tick(ctx); err != nil {
		t.Logf("tick: %v", err)
	}
	tasks, _ := h.Runtime.MemberTasks(ctx, "learn", "rita")
	rita, _ := h.Runtime.Member("learn", "rita")
	renew, again := 0, 0
	for _, x := range tasks {
		t.Logf("TASK: %q", x.Text)
		if !strings.HasPrefix(x.Text, "Esta lição continua valendo?") {
			continue
		}
		renew++
		if strings.Contains(x.Text, "respondeu o mesmo") {
			again++
		}
		if out, err := h.Runtime.AnswerTask(ctx, x.RunID, x.Hash, rita, true); out != runtime.Recorded || err != nil {
			t.Errorf("renew: %v %v", out, err)
		}
	}
	answered := 0
	for q, want := range map[string][]string{
		"o clareamento de vcs usa laser?":             {"LED"},
		"quanto tempo leva a limpeza?":                {"40"},
		"vcs dão nota fiscal pra eu pedir reembolso?": {"dia do pagamento"},
		"posso ir com meu gato?":                      {"cão-guia", "cães-guia", "cão guia"},
	} {
		res, err := h.Handle(ctx, "held:"+q, map[string]any{"channel": "whatsapp"}, q)
		if err != nil {
			t.Fatal(err)
		}
		var r []string
		for _, o := range res.Outbox {
			r = append(r, o.Text)
		}
		reply := strings.ToLower(strings.Join(r, " "))
		ok := false
		for _, w := range want {
			ok = ok || strings.Contains(reply, strings.ToLower(w))
		}
		if ok {
			answered++
		}
		t.Logf("HELD %q -> %q (%v)", q, reply, ok)
	}
	t.Logf("RESULT renew=%d again=%d answered=%d/4", renew, again, answered)
	if renew != 4 || again < 3 || answered < 3 {
		t.Errorf("W4: renew %d (want 4), repeated %d (want >= 3), answered %d (want >= 3)", renew, again, answered)
	}
}
