package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const advAgent = `apiVersion: plugins.tessera.dev/v1alpha1
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
  messages: { denied: "Não foi aprovado.", failed: "FALHOU." }
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
metadata: { name: onerr, namespace: shop }
spec: { function: onerr, webhook: { secret: PEDIDO } }
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
metadata: { name: onerr, namespace: shop }
spec:
  description: The action fails; an approval escalation handles it.
  authorize: "has(caller.trigger)"
  action:
    act: loja/desconto
    with: { percent: 10 }
    idempotencyKey: "'desconto'"
  onError:
      - escalate:
          queue: loja
          task: answer
          message: "Aplicar 10% de desconto?"
  output: { template: "Desconto aprovado." }
`

func advLoad(t *testing.T) (*Host, journal.Store, *int) {
	t.Helper()
	h, store, calls, err := advLoadDoc(t, advAgent)
	if err != nil {
		t.Fatal(err)
	}
	return h, store, calls
}

func advLoadDoc(t *testing.T, doc string) (*Host, journal.Store, *int, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	calls := 0
	store := journal.NewMemory()
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "shop", Store: store,
		Handlers: map[string]act.Handler{"loja/desconto": func(_ context.Context, c act.Call) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if os.Getenv("ADV_FAIL") == "1" {
				return nil, &act.DefiniteError{Code: "REFUSED", Message: "refused"}
			}
			return map[string]any{}, nil
		}}})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w %v", err, issues)
	}
	return h, store, &calls, nil
}

func advToUser(ctx context.Context, store journal.Store, runID string) string {
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

// Adversarial review: a free-text approval counts only when the whole
// answer is yes ("sim", "Sim."); "sim, mas só 5%" or "ok, não aprovo" is
// unclear, and an unclear answer never approves.
func TestFreeTextApprovalIsAWholeYes(t *testing.T) {
	h, store, calls := advLoad(t)
	ctx := context.Background()
	for i, c := range []struct {
		text string
		acts int
	}{
		{"sim, mas só 5%", 0}, {"yes, but no", 0}, {"ok, não aprovo", 0}, {"s/ chance", 0}, {"não sei", 0}, {"no problem, go ahead", 0},
		{"Sim.", 1}, {"sim", 1},
	} {
		*calls = 0
		res, err := h.StartHook(ctx, "pedido", map[string]any{}, fmt.Sprint("k", i))
		if err != nil {
			t.Fatal(err)
		}
		task, _ := h.Runtime.PendingTask(ctx, res.RunID)
		if _, err := h.Runtime.Deliver(ctx, res.RunID, runtime.Reply{From: "human", Text: c.text, TaskHash: task.Hash}); err != nil {
			t.Fatal(err)
		}
		if *calls != c.acts {
			t.Errorf("%q: %d discounts (want %d), user told %q", c.text, *calls, c.acts, advToUser(ctx, store, res.RunID))
		}
	}
}

// Adversarial review: in an action's onError, an escalate asks whether
// the failed effect happened. As task: approve, the channels' Approve
// button would say "it happened" for a discount never applied: admission
// refuses it, and the admitted form (task: answer) is no channel approval.
func TestOnErrorEscalateIsNoApproval(t *testing.T) {
	approve := strings.Replace(advAgent, "          task: answer\n", "          task: approve\n", 1)
	if _, _, _, err := advLoadDoc(t, approve); err == nil || !strings.Contains(err.Error(), "spec.onError[0].escalate.task") {
		t.Fatalf("task: approve in an action's onError was admitted: %v", err)
	}
	t.Setenv("ADV_FAIL", "1")
	h, _, _ := advLoad(t)
	ctx := context.Background()
	res, err := h.StartHook(ctx, "onerr", map[string]any{}, "k1")
	if err != nil || res.Status != journal.StatusWaitingForHuman {
		t.Fatalf("%+v %v", res, err)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	if task.Kind == "approve" || !strings.Contains(task.Text, "Did it happen") {
		t.Errorf("the question is whether it happened: %s %q", task.Kind, task.Text)
	}
	if tasks, _ := h.Runtime.ChannelTasks(ctx, "shop", "slack"); len(tasks) != 0 {
		t.Errorf("no channel shows it as an approval: %v", tasks)
	}
}
