package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const denyAgent = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: loja, version: 0.1.0 }
spec:
  actions:
    desconto: { input: { percent: int }, effect: irreversible }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: loja, namespace: shop }
spec:
  humanInTheLoop: { queue: loja }
  messages: { denied: "Não foi aprovado." }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: loja, namespace: shop }
spec: {}
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: pedido, namespace: shop }
spec: { function: aprovar, webhook: { secret: PEDIDO } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: pedido-gentil, namespace: shop }
spec: { function: aprovar-gentil, webhook: { secret: PEDIDO } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: aprovar, namespace: shop }
spec:
  description: A discount a person approves.
  authorize: "has(caller.trigger)"
  steps:
    - { id: ok, escalate: { queue: loja, task: approve, message: "Desconto de 10%?" } }
  action: { act: loja/desconto, with: { percent: 10 }, idempotencyKey: "'desconto'" }
  output: { template: "Desconto aprovado." }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: aprovar-gentil, namespace: shop }
spec:
  description: The same, with its own words on a denial.
  authorize: "has(caller.trigger)"
  steps:
    - id: ok
      escalate:
        queue: loja
        task: approve
        message: "Desconto de 10%?"
        onDeny: { template: "A loja não liberou o desconto desta vez." }
  action: { act: loja/desconto, with: { percent: 10 }, idempotencyKey: "'desconto'" }
  output: { template: "Desconto aprovado." }
`

// #53: a denied approval ends the Function. The action does not run, the
// user is not told it was approved, and hears onDeny or messages.denied.
// An answer that is neither yes nor no is no approval.
func TestADeniedApprovalStopsTheFunction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(denyAgent), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	discounts := 0
	store := journal.NewMemory()
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "shop", Store: store,
		Handlers: map[string]act.Handler{"loja/desconto": func(context.Context, act.Call) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			discounts++
			return map[string]any{}, nil
		}}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	toUser := func(runID string) string {
		recs, _ := store.Records(ctx, runID)
		var out []string
		for _, r := range recs {
			if v, err := journal.Decode(r.Value); err == nil && r.Kind == journal.KindSend {
				if m, _ := v.(map[string]any); m["to"] == "user" {
					out = append(out, m["text"].(string))
				}
			}
		}
		return strings.Join(out, " | ")
	}
	for i, c := range []struct {
		trigger string
		reply   runtime.Reply
		want    string
		acts    int
	}{
		{"pedido", runtime.Reply{From: "human", Value: false, Text: "denied"}, "Não foi aprovado.", 0},
		{"pedido-gentil", runtime.Reply{From: "human", Value: false, Text: "denied"}, "A loja não liberou o desconto desta vez.", 0},
		{"pedido", runtime.Reply{From: "human", Text: "talvez amanhã"}, "Não foi aprovado.", 0},
		{"pedido", runtime.Reply{From: "human", Value: true, Text: "approved"}, "Desconto aprovado.", 1},
	} {
		mu.Lock()
		discounts = 0
		mu.Unlock()
		res, err := h.StartHook(ctx, c.trigger, map[string]any{}, "k"+string(rune('0'+i)))
		if err != nil || res.Status != journal.StatusWaitingForHuman {
			t.Fatalf("%d: %+v %v", i, res, err)
		}
		task, _ := h.Runtime.PendingTask(ctx, res.RunID)
		c.reply.TaskHash = task.Hash
		if _, err := h.Runtime.Deliver(ctx, res.RunID, c.reply); err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		run, _ := store.GetRun(ctx, res.RunID)
		if run.Status != journal.StatusSucceeded || discounts != c.acts || toUser(res.RunID) != c.want {
			t.Errorf("%d (%s %q): status %s, %d discounts, told %q; want %d, %q", i, c.trigger, c.reply.Text, run.Status, discounts, toUser(res.RunID), c.acts, c.want)
		}
	}
}
