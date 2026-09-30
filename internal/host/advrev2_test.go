package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const advOpenThenRoled = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: slack, version: 0.1.0 }
spec:
  actions:
    post-task: { input: { to: string, text: string, buttons: list }, effect: external-message }
    update-task: { input: { channel: string, ts: string, text: string }, effect: external-message }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: loja, namespace: shop }
spec:
  humanInTheLoop: { queue: balcao }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: shop }
spec:
  members:
    - { id: rita, name: Rita, roles: [estagiario], slack: U0RITA }
    - { id: gil, name: Gil, roles: [gerencia], slack: U0GIL }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: balcao, namespace: shop }
spec: {}
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: gerencia, namespace: shop }
spec: { roles: [gerencia], channels: [slack] }
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
  description: Asks the counter; big ones go to management.
  authorize: "has(caller.trigger)"
  steps:
    - { id: human, escalate: { queue: balcao, task: approve, message: "Desconto de 60%?" } }
  output: { template: "aprovado" }
`

// A3: a task that starts in an open queue stays open after it reaches a
// roled, Slack-enabled queue: the managers never get it on Slack, and a
// member without the role answers it.
func TestAdvOpenQueueForwardedToRoledQueue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(advOpenThenRoled), 0o600); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemory()
	ok := func(context.Context, act.Call) (any, error) { return map[string]any{}, nil }
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "shop", Store: store,
		Handlers: map[string]act.Handler{"slack/post-task": ok, "slack/update-task": ok}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	res, err := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Runtime.Forward(ctx, res.RunID, "gerencia", "anyone at the counter"); err != nil {
		t.Fatal(err)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	members, open, _ := h.Runtime.Answerers(ctx, res.RunID)
	ct, _ := h.Runtime.ChannelTasks(ctx, "shop", "slack")
	t.Logf("task now in %v; open=%v answerers=%d; slack channel tasks=%d", task.Reached, open, len(members), len(ct))
	rita, _ := h.Runtime.MemberBySlack("shop", "U0RITA")
	out, err := h.Runtime.AnswerTask(ctx, res.RunID, task.Hash, rita, true)
	run, _ := store.GetRun(ctx, res.RunID)
	t.Logf("rita (role estagiario, not gerencia) answers: outcome=%v err=%v run=%s", out, err, run.Status)
	if len(ct) == 0 || out == 0 {
		t.Errorf("forwarded to gerencia, but slack tasks=%d and a non-gerencia member's approval outcome=%v (0=Recorded)", len(ct), out)
	}
}
