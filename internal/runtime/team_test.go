package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const teamDocs = `apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: r }
spec:
  members:
    - { id: rita, name: Rita, roles: [recepcao] }
    - { id: gil, name: Gil, roles: [gerencia] }
    - { id: dora, name: Dora, roles: [dono] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: r }
spec: { roles: [recepcao] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: gerencia, namespace: r }
spec: { roles: [gerencia] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: dono, namespace: r }
spec: { roles: [dono] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: desconto, namespace: r }
spec:
  description: team scenario
  inputs:
    pct: { type: int, resolve: [ { id: fixed, value: "caller.pct" } ] }
  steps:
    - id: ok
      escalate:
        queue: recepcao
        task: approve
        message: "Desconto de {{ inputs.pct }}%?"
        route: [ { when: "inputs.pct > 50", queue: dono } ]
  output: { template: "ok" }
`

func teamHarness(t *testing.T, pct int64) (*harness, Result) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(teamDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	res, err := h.rt.Start(context.Background(), Start{Namespace: "r", Function: "desconto", Conversation: "c1", Caller: map[string]any{"pct": pct}})
	if err != nil {
		t.Fatal(err)
	}
	return h, h.deliver(res)
}

func (h *harness) answerAs(by string) error {
	task, err := h.rt.PendingTask(context.Background(), "run-1")
	if err != nil {
		return err
	}
	_, err = h.rt.Deliver(context.Background(), "run-1", Reply{From: "human", Text: "yes", Value: true, TaskHash: task.Hash, By: by})
	return err
}

// A queue with roles is answered only by a member who holds one.
func TestRolesDecideWhoAnswers(t *testing.T) {
	h, _ := teamHarness(t, 20)
	for _, by := range []string{"", "someone", "gil"} {
		if err := h.answerAs(by); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("%q answered: %v", by, err)
		}
	}
	if err := h.answerAs("rita"); err != nil {
		t.Fatal(err)
	}
}

// A declared rule routes the task: a large discount goes to the owner.
func TestRoutesSendByRule(t *testing.T) {
	h, _ := teamHarness(t, 60)
	task, _ := h.rt.PendingTask(context.Background(), "run-1")
	if task.Queue != "dono" {
		t.Fatalf("queue %s", task.Queue)
	}
	if err := h.answerAs("rita"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("the reception answered the owner's task: %v", err)
	}
	if err := h.answerAs("dora"); err != nil {
		t.Fatal(err)
	}
}

// A member forwards a task; the next queue's members may then answer it;
// someone outside cannot forward.
func TestForwarding(t *testing.T) {
	h, _ := teamHarness(t, 20)
	ctx := context.Background()
	if _, err := h.rt.Forward(ctx, "run-1", "gerencia", "dora"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("an outsider forwarded: %v", err)
	}
	res, err := h.rt.Forward(ctx, "run-1", "gerencia", "rita")
	if err != nil {
		t.Fatal(err)
	}
	h.deliver(res)
	task, _ := h.rt.PendingTask(ctx, "run-1")
	if task.Queue != "gerencia" || len(task.Reached) != 2 {
		t.Fatalf("task %+v", task)
	}
	last := h.queues[len(h.queues)-1]
	if last.To != "queue:gerencia" || last.Task != "approve" {
		t.Errorf("the next queue is told: %+v", last)
	}
	if err := h.answerAs("gil"); err != nil {
		t.Fatal(err)
	}
	if run, _ := h.store.GetRun(ctx, "run-1"); run.Status != journal.StatusSucceeded {
		t.Errorf("status %s", run.Status)
	}
}
