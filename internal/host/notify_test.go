package host

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const notifyDocs = `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: bistro, namespace: bistro }
spec: { timezone: America/Sao_Paulo }
---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: chat, version: 0.1.0 }
spec:
  egress: { hosts: [example.com] }
  connection: { baseUrl: "https://example.com" }
  actions:
    post:
      input: { text: string }
      output: map
      effect: external-message
      http: { method: POST, path: /post, body: { text: "${ inputs.text }" } }
    broken:
      input: { text: string }
      output: map
      effect: external-message
      http: { method: POST, path: /broken, body: { text: "${ inputs.text }" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: salao, namespace: bistro }
spec:
  sla: { remind: 1h, escalate: 2h, escalateTo: gerencia }
  notify:
    - act: chat/post
      with: { text: "${ inputs.stage + ' ' + inputs.kind + ': ' + inputs.text + ' ' + inputs.link }" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: gerencia, namespace: bistro }
spec:
  notify:
    - { act: chat/post, with: { text: "${ 'gerencia ' + inputs.stage + ': ' + inputs.text }" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: cozinha, namespace: bistro }
spec:
  notify:
    - { act: chat/broken, with: { text: "${ inputs.text }" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: pedido, namespace: bistro }
spec: { function: aprovar, webhook: { secret: PEDIDO } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: aprovar, namespace: bistro }
spec:
  description: Asks the floor to approve a table, then a discount.
  authorize: "has(caller.trigger)"
  steps:
    - { id: mesa, escalate: { queue: salao, task: approve, message: "Mesa às 21h?" } }
    - { id: desconto, escalate: { queue: salao, task: approve, message: "Desconto de 10%?" } }
  output: { template: "ok" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: prato, namespace: bistro }
spec: { function: cozinhar, webhook: { secret: PRATO } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: cozinhar, namespace: bistro }
spec:
  description: Asks the kitchen.
  authorize: "has(caller.trigger)"
  steps:
    - { id: prato, escalate: { queue: cozinha, task: answer, message: "Tem peixe?" } }
  output: { template: "ok" }
`

type chatServer struct {
	mu    sync.Mutex
	fail  bool
	posts []string
	keys  []string
	tries int // calls to the broken action
}

func (c *chatServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.URL.Path == "/broken" {
		c.tries++
		http.Error(w, "no such channel", http.StatusBadRequest)
		return
	}
	if c.fail {
		http.Error(w, "down", http.StatusBadRequest)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var m map[string]string
	_ = json.Unmarshal(b, &m)
	c.posts = append(c.posts, m["text"])
	c.keys = append(c.keys, r.Header.Get("Idempotency-Key"))
	_, _ = w.Write([]byte(`{"ok": true}`))
}

func (c *chatServer) sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.posts...)
}

func notifyHost(t *testing.T, chat *chatServer, now *time.Time) *Host {
	t.Helper()
	srv := httptest.NewTLSServer(chat)
	t.Cleanup(srv.Close)
	// example.com is in the test server's certificate; dial the server.
	client := srv.Client()
	tr := client.Transport.(*http.Transport).Clone()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	client.Transport = tr
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bistro.yaml"), []byte(notifyDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: &wordDecider{},
		AllowPrivateNetworks: true, HTTPClient: client, Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return *now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func TestNotifyTasks(t *testing.T) {
	chat := &chatServer{fail: true}
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	h := notifyHost(t, chat, &now)
	ctx := context.Background()
	res, err := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if err != nil || res.Status != journal.StatusWaitingForHuman {
		t.Fatalf("%+v %v", res, err)
	}
	link := func(task runtime.Task) string { return "https://x/tasks/" + task.Hash[:8] }

	if _, err := h.Notify(ctx, link); err == nil {
		t.Fatal("a failing notification must report its error")
	}
	chat.mu.Lock()
	chat.fail = false
	chat.mu.Unlock()
	if n, _ := h.Notify(ctx, link); n != 0 {
		t.Fatal("a failed notification waits before it is tried again")
	}
	now = now.Add(61 * time.Second)
	n, err := h.Notify(ctx, link)
	if err != nil || n != 1 {
		t.Fatalf("the failed notification is tried again: %d %v", n, err)
	}
	if n, _ := h.Notify(ctx, link); n != 0 {
		t.Fatalf("once per task message, got %d more", n)
	}
	task, err := h.Runtime.PendingTask(ctx, res.RunID)
	if err != nil {
		t.Fatal(err)
	}
	posts := chat.sent()
	if want := "created approve: Mesa às 21h? https://x/tasks/" + task.Hash[:8]; len(posts) != 1 || posts[0] != want {
		t.Fatalf("posts %q, want %q", posts, want)
	}
	if chat.keys[0] == "" || !strings.HasPrefix(chat.keys[0], res.RunID+"/") {
		t.Errorf("idempotency key %q", chat.keys[0])
	}

	now = now.Add(61 * time.Minute) // the SLA reminder is another message
	if _, err := h.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := h.Notify(ctx, link); err != nil || n != 1 || !strings.HasPrefix(chat.sent()[1], "remind approve: ⏰ Mesa") {
		t.Fatalf("reminder: %d %v %q", n, err, chat.sent())
	}
	now = now.Add(time.Hour) // the escalation goes to the other queue's actions
	if _, err := h.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := h.Notify(ctx, link); err != nil || n != 1 || chat.sent()[2] != "gerencia escalate: ⤴ Mesa às 21h?" {
		t.Fatalf("escalation: %d %v %q", n, err, chat.sent())
	}

	// The next task: only its own message, never the answered one's.
	if _, err := h.Runtime.Deliver(ctx, res.RunID, runtime.Reply{From: "human", Value: true, Text: "ok", TaskHash: task.Hash, By: "ana"}); err != nil {
		t.Fatal(err)
	}
	next, err := h.Runtime.PendingTask(ctx, res.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := h.Notify(ctx, link); err != nil || n != 1 {
		t.Fatalf("the next task: %d %v", n, err)
	}
	if got := chat.sent()[3:]; len(got) != 1 || got[0] != "created approve: Desconto de 10%? https://x/tasks/"+next.Hash[:8] {
		t.Fatalf("an answered task is not notified again: %q", got)
	}
}

func TestNotifyFailureDoesNotBlockOthers(t *testing.T) {
	chat := &chatServer{}
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	h := notifyHost(t, chat, &now)
	ctx := context.Background()
	if _, err := h.StartHook(ctx, "pedido", map[string]any{}, "a"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second) // the broken queue's run is listed first (newest)
	if _, err := h.StartHook(ctx, "prato", map[string]any{}, "b"); err != nil {
		t.Fatal(err)
	}
	n, err := h.Notify(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "chat/broken") || n != 1 {
		t.Fatalf("the working queue is notified and the broken one reported: %d %v", n, err)
	}
	for range 5 {
		_, _ = h.Notify(ctx, nil)
	}
	if chat.tries != 1 {
		t.Errorf("a failing action is not called every tick: %d calls", chat.tries)
	}
}

// The first task answered before any notification went out: only the next
// task is notified, never the answered one with the next one's link.
func TestNotifyOnlyThePendingTask(t *testing.T) {
	chat := &chatServer{}
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	h := notifyHost(t, chat, &now)
	ctx := context.Background()
	res, err := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := h.Runtime.PendingTask(ctx, res.RunID)
	if _, err := h.Runtime.Deliver(ctx, res.RunID, runtime.Reply{From: "human", Value: true, Text: "ok", TaskHash: first.Hash}); err != nil {
		t.Fatal(err)
	}
	if n, err := h.Notify(ctx, nil); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if got := chat.sent(); len(got) != 1 || !strings.Contains(got[0], "Desconto") {
		t.Fatalf("only the pending task: %q", got)
	}
}
