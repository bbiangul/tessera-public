package whatsapp

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const teamOnWhatsApp = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: whatsapp, version: 0.1.0 }
spec:
  channel: { window: 24h }
  actions:
    send-text: { input: { to: string, text: string }, effect: external-message, freeText: true }
    send-buttons: { input: { to: string, text: string, buttons: list }, effect: external-message, freeText: true }
    send-task-template: { input: { to: string, text: string, buttons: list }, effect: external-message }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: loja, namespace: shop }
spec:
  channel: whatsapp
  humanInTheLoop: { queue: loja }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: shop }
spec:
  members:
    - { id: rita, name: Rita, roles: [loja], whatsapp: "5511900000001" }
    - { id: gil, name: Gil, roles: [gerencia], whatsapp: "5511900000002" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: loja, namespace: shop }
spec: { roles: [loja], channels: [whatsapp] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: gerencia, namespace: shop }
spec: { roles: [gerencia], channels: [whatsapp] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: pedido, namespace: shop }
spec: { function: aprovar, webhook: { secret: PEDIDO } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: aprovar, namespace: shop }
spec:
  description: Asks the floor to approve a discount.
  authorize: "has(caller.trigger)"
  steps:
    - { id: human, escalate: { queue: loja, task: approve, message: "Desconto de 10% para P1?" } }
  output: { value: "steps.human.output" }
`

type phone struct {
	mu        sync.Mutex
	buttons   []string // "to: text [ids]"
	texts     []string
	templates []string        // "to: text [ids]"
	closed    map[string]bool // numbers outside their 24h window
}

func teamSetup(t *testing.T) (*Adapter, *httptest.Server, *phone, *host.Host, journal.Store) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(teamOnWhatsApp), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &phone{closed: map[string]bool{}}
	store := journal.NewMemory()
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "shop", Store: store,
		Handlers: map[string]act.Handler{
			"whatsapp/send-text": func(_ context.Context, c act.Call) (any, error) {
				p.mu.Lock()
				defer p.mu.Unlock()
				p.texts = append(p.texts, fmt.Sprint(c.Args["to"], ": ", c.Args["text"]))
				return map[string]any{}, nil
			},
			"whatsapp/send-task-template": func(_ context.Context, c act.Call) (any, error) {
				p.mu.Lock()
				defer p.mu.Unlock()
				var ids []string
				for _, b := range c.Args["buttons"].([]any) {
					ids = append(ids, b.(map[string]any)["id"].(string))
				}
				p.templates = append(p.templates, fmt.Sprint(c.Args["to"], ": ", c.Args["text"], " ", ids))
				return map[string]any{}, nil
			},
			"whatsapp/send-buttons": func(_ context.Context, c act.Call) (any, error) {
				p.mu.Lock()
				defer p.mu.Unlock()
				if p.closed[fmt.Sprint(c.Args["to"])] {
					return nil, &act.DefiniteError{Code: "INVALID", Message: "send-buttons: server answered 400", Detail: "(#131047) Re-engagement message"}
				}
				var ids []string
				for _, b := range c.Args["buttons"].([]any) {
					ids = append(ids, b.(map[string]any)["id"].(string))
				}
				p.buttons = append(p.buttons, fmt.Sprint(c.Args["to"], ": ", c.Args["text"], " ", ids))
				return map[string]any{}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	a := &Adapter{Host: h, VerifyToken: "vt", AppSecret: "app-secret", Plugin: "whatsapp"}
	if err := a.Open(":memory:"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	ts := httptest.NewServer(a.Handler())
	t.Cleanup(ts.Close)
	return a, ts, p, h, store
}

func tap(id, from, button string) []byte {
	return []byte(fmt.Sprintf(`{"entry":[{"changes":[{"value":{"contacts":[{"wa_id":%q,"profile":{"name":"x"}}],
		"messages":[{"id":%q,"from":%q,"type":"interactive","interactive":{"type":"button_reply","button_reply":{"id":%q,"title":"Aprovar"}}}]}}]}]}`, from, id, from, button))
}

// backoffice-002: the task reaches the member's WhatsApp with buttons; only
// that member's tap, from their own number, answers it, once.
func TestApprovingOnWhatsApp(t *testing.T) {
	a, ts, p, h, store := teamSetup(t)
	ctx := context.Background()
	res, err := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if err != nil || res.Status != journal.StatusWaitingForHuman {
		t.Fatalf("%+v %v", res, err)
	}
	if n, err := a.DeliverTasks(ctx); err != nil || n != 1 {
		t.Fatalf("sent %d %v", n, err)
	}
	if n, _ := a.DeliverTasks(ctx); n != 0 {
		t.Error("a task goes to a member once")
	}
	if len(p.buttons) != 1 || !strings.HasPrefix(p.buttons[0], "5511900000001: Desconto de 10% para P1?") {
		t.Fatalf("only Rita gets the buttons: %v", p.buttons)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	approve := "task:" + task.Hash + ":approve"

	// Not a member's number; a member without the role; free text.
	post(t, ts, tap("m1", "5599999999999", approve), "app-secret")
	post(t, ts, tap("m2", "5511900000002", approve), "app-secret")
	post(t, ts, text("m3", "5511900000001", "pode seguir"), "app-secret")
	if run, _ := store.GetRun(ctx, res.RunID); run.Status != journal.StatusWaitingForHuman {
		t.Fatalf("answered by the wrong person or by text: %s", run.Status)
	}
	// A forged signature is refused before anything.
	if code := post(t, ts, tap("m4", "5511900000001", approve), "wrong"); code != 401 {
		t.Errorf("unsigned tap: %d", code)
	}
	// Rita's tap answers; the same tap again does not.
	post(t, ts, tap("m5", "5511900000001", approve), "app-secret")
	run, _ := store.GetRun(ctx, res.RunID)
	if run.Status != journal.StatusSucceeded {
		t.Fatalf("Rita's tap did not answer: %s", run.Status)
	}
	recs, _ := store.Records(ctx, res.RunID)
	var byRita bool
	for _, r := range recs {
		byRita = byRita || (r.Kind == journal.KindResume && strings.Contains(string(r.Value), `"by":"rita"`))
	}
	if !byRita {
		t.Error("the answer is Rita's")
	}
	post(t, ts, tap("m6", "5511900000001", approve), "app-secret")
	joined := strings.Join(p.texts, "\n")
	for _, want := range []string{"5599999999999: Este número não está na equipe", "5511900000002: Você não pode responder", "5511900000001: Registrado: aprovado", "5511900000001: Esta tarefa já foi respondida"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing reply %q in %s", want, joined)
		}
	}
}

// Forwarded to the manager's queue, the task reaches the manager's
// WhatsApp, and the manager's tap answers it.
func TestAForwardedTaskReachesTheNextQueue(t *testing.T) {
	a, ts, p, h, store := teamSetup(t)
	ctx := context.Background()
	res, _ := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if _, err := a.DeliverTasks(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Runtime.Forward(ctx, res.RunID, "gerencia", "rita"); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.DeliverTasks(ctx); n != 1 || !strings.HasPrefix(p.buttons[len(p.buttons)-1], "5511900000002:") {
		t.Fatalf("the manager gets it: %d %v", n, p.buttons)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	post(t, ts, tap("g1", "5511900000002", "task:"+task.Hash+":approve"), "app-secret")
	if run, _ := store.GetRun(ctx, res.RunID); run.Status != journal.StatusSucceeded {
		t.Errorf("status %s", run.Status)
	}
}

func quickReply(id, from, payload string) []byte {
	return []byte(fmt.Sprintf(`{"entry":[{"changes":[{"value":{"messages":[{"id":%q,"from":%q,"type":"button","button":{"text":"Aprovar","payload":%q}}]}}]}]}`, id, from, payload))
}

// Outside the member's 24h window the task goes as an approved template
// with quick-reply buttons, and its quick reply answers the same way.
func TestOutsideTheWindowTheTemplateCarriesTheTask(t *testing.T) {
	a, ts, p, h, store := teamSetup(t)
	p.closed["5511900000001"] = true
	ctx := context.Background()
	res, _ := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if n, err := a.DeliverTasks(ctx); err != nil || n != 1 {
		t.Fatalf("sent %d %v", n, err)
	}
	if len(p.buttons) != 0 || len(p.templates) != 1 || !strings.HasPrefix(p.templates[0], "5511900000001: Desconto de 10% para P1?") {
		t.Fatalf("the template carries it: buttons %v templates %v", p.buttons, p.templates)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	post(t, ts, quickReply("q1", "5511900000001", "task:"+task.Hash+":approve"), "app-secret")
	if run, _ := store.GetRun(ctx, res.RunID); run.Status != journal.StatusSucceeded {
		t.Errorf("the quick reply did not answer: %s", run.Status)
	}
}
