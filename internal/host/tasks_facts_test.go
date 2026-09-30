package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const tasksAgent = `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: loja, namespace: tk }
spec:
  messages: { notUnderstood: "NAO-ENTENDI" }
  audiences:
    - name: equipe
      who: "has(caller.roles) && size(caller.roles) > 0"
      functions: []
      freeMode: { instructions: "Ajude a equipe.", tasks: true }
    - { name: outros, functions: [], freeMode: { instructions: "Ajude.", tasks: true } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: tk }
spec:
  members:
    - { id: rita, name: Rita, roles: [loja] }
    - { id: dora, name: Dora, roles: [dono] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: loja, namespace: tk }
spec: { roles: [loja] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: dono, namespace: tk }
spec: { roles: [dono] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: pedido, namespace: tk }
spec: { function: aprovar, webhook: { secret: PEDIDO } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: aprovar, namespace: tk }
spec:
  description: A discount a person approves.
  authorize: "has(caller.trigger)"
  inputs:
    percent: { type: int, description: percent, resolve: [ { id: e, value: "event.payload.percent" } ] }
    email: { type: string, description: e-mail, resolve: [ { id: e, value: "event.payload.email" } ] }
  steps:
    - id: ok
      escalate:
        queue: loja
        task: approve
        message: "Desconto de {{ inputs.percent }}% para {{ inputs.email }}?"
        route: [ { when: "inputs.percent > 50", queue: dono } ]
  output: { value: "steps.ok.output" }
`

type factsGen struct {
	mu    sync.Mutex
	tasks [][]string
}

func (*factsGen) Name() string { return "facts" }
func (g *factsGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	st, _ := req.State.(map[string]any)
	facts, _ := st["facts"].(map[string]any)
	g.mu.Lock()
	defer g.mu.Unlock()
	var got []string
	if list, ok := facts["myTasks"].([]any); ok {
		for _, t := range list {
			m, _ := t.(map[string]any)
			got = append(got, m["task"].(string))
		}
		g.tasks = append(g.tasks, got)
	} else {
		g.tasks = append(g.tasks, nil)
	}
	return &generate.Response{Text: "Você tem tarefas."}, nil
}

// backoffice-006: freeMode.tasks gives a member exactly the tasks they
// may answer, masked; a caller who is no member gets none.
func TestFreeModeTasks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(tasksAgent), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &factsGen{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "tk", Store: journal.NewMemory(), Decider: audienceDecider{&audienceWorld{}}, Generator: g})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	for i, ev := range []map[string]any{{"percent": 10, "email": "ana@exemplo.com"}, {"percent": 20, "email": "5511911112222"}, {"percent": 60, "email": "caio@exemplo.com"}} {
		if res, err := h.StartHook(ctx, "pedido", ev, string(rune('a'+i))); err != nil || res.Status != journal.StatusWaitingForHuman {
			t.Fatalf("%+v %v", res, err)
		}
	}
	rita := map[string]any{"member": "rita", "roles": []any{"loja"}}
	if _, err := h.Handle(ctx, "r", rita, "tenho algo para aprovar?"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Handle(ctx, "x", map[string]any{"channel": "slack"}, "tenho algo para aprovar?"); err != nil {
		t.Fatal(err)
	}
	if len(g.tasks) != 2 {
		t.Fatalf("free answers %v", g.tasks)
	}
	got := strings.Join(g.tasks[0], " | ")
	if len(g.tasks[0]) != 2 || !strings.Contains(got, "10%") || !strings.Contains(got, "20%") || strings.Contains(got, "60%") {
		t.Errorf("Rita's tasks: %q", got)
	}
	if strings.Contains(got, "@exemplo.com") || !strings.Contains(got, "[EMAIL]") {
		t.Errorf("tasks reach the model masked: %q", got)
	}
	// #55: a WhatsApp ID in a task's text (bare digits) is masked too.
	if strings.Contains(got, "5511911112222") || !strings.Contains(got, "[PHONE]") {
		t.Errorf("a bare WhatsApp number reached the model: %q", got)
	}
	if g.tasks[1] != nil {
		t.Errorf("a non-member got tasks: %v", g.tasks[1])
	}
}
