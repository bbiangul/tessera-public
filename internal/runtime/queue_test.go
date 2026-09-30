package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const slaFixture = `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: travel, namespace: travel }
spec: { timezone: America/Sao_Paulo }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: comercial, namespace: travel }
spec:
  sla: { remind: 2h, businessHours: { from: "09:00", to: "18:00" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: late, namespace: travel }
spec:
  description: A task at the end of the working week.
  steps:
    - { id: ok, escalate: { queue: comercial, task: answer, message: "Retornar ao cliente?" } }
  output: { value: "steps.ok.output" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: salao, namespace: travel }
spec:
  sla: { remind: 1h, escalate: 2h, escalateTo: gerencia, timeout: 4h, onTimeout: { answer: false } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: rigida, namespace: travel }
spec:
  sla: { timeout: 30m, onTimeout: { fail: true } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: discount, namespace: travel }
spec:
  description: A discount needs a manager's approval.
  steps:
    - { id: ok, escalate: { queue: salao, task: approve, message: "Aprovar 10% de desconto para a mesa 4?" } }
  output: { template: "aprovado: {{ steps.ok.output }}" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: strict, namespace: travel }
spec:
  description: Must be answered in 30 minutes.
  steps:
    - { id: ok, escalate: { queue: rigida, task: input, message: "Qual o número do pedido?" } }
  output: { value: "steps.ok.output" }
`

func slaHarness(t *testing.T) (*harness, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "q.yaml"), []byte(slaFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	now := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	h.rt.Clock = func() time.Time { return now }
	return h, &now
}

func queued(h *harness) []string {
	var out []string
	for _, m := range h.queues {
		out = append(out, m.To+" "+m.Text)
	}
	return out
}

// Nobody answers: a reminder at 1h, the escalation at 2h, and at 4h the
// safe default (no) as the answer. Each stage happens once.
func TestSLAStages(t *testing.T) {
	h, now := slaHarness(t)
	res := h.startFn("discount", "")
	expectStatus(t, res, journal.StatusWaitingForHuman)
	if !res.WakeAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("next SLA stage at %v", res.WakeAt)
	}
	for _, step := range []time.Duration{30 * time.Minute, 40 * time.Minute, 70 * time.Minute, 5 * time.Minute} {
		*now = now.Add(step)
		wakeAll(t, h)
	}
	got := strings.Join(queued(h), " | ")
	want := "queue:salao Aprovar 10% de desconto para a mesa 4? | queue:salao ⏰ Aprovar 10% de desconto para a mesa 4? | queue:gerencia ⤴ Aprovar 10% de desconto para a mesa 4?"
	if got != want {
		t.Fatalf("messages\n got %s\nwant %s", got, want)
	}
	if run, _ := h.store.GetRun(context.Background(), "run-1"); run.Status != journal.StatusWaitingForHuman {
		t.Fatalf("before the timeout it still waits: %s", run.Status)
	}
	*now = now.Add(2 * time.Hour) // 4h35 since the task
	wakeAll(t, h)
	run, _ := h.store.GetRun(context.Background(), "run-1")
	if run.Status != journal.StatusSucceeded {
		t.Fatalf("after the timeout the default answer continues the run: %s", run.Status)
	}
	if len(queued(h)) != 3 {
		t.Errorf("each stage happens once: %q", queued(h))
	}
	recs, _ := h.store.Records(context.Background(), "run-1")
	timedOut := false
	for _, r := range recs {
		timedOut = timedOut || (r.Kind == journal.KindResume && strings.Contains(string(r.Value), `"from":"timeout"`) && strings.Contains(string(r.Value), `"value":false`))
	}
	if !timedOut {
		t.Error("the timeout answers with onTimeout.answer (false), recorded as from the timeout")
	}
	// The safe answer denies the approval: the Function stops there (#53).
	if last := h.user[len(h.user)-1]; last != DefaultMessages.Denied {
		t.Errorf("the timeout's denial ends the Function: %q", last)
	}
}

// Business hours: a task on Friday at 17h (São Paulo) with a 2h reminder
// is reminded on Monday at 10h, not on Friday night.
func TestSLAInBusinessHours(t *testing.T) {
	h, now := slaHarness(t)
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	*now = time.Date(2026, 9, 25, 17, 0, 0, 0, loc)
	res := h.startFn("late", "")
	if want := time.Date(2026, 9, 28, 10, 0, 0, 0, loc); !res.WakeAt.Equal(want) {
		t.Fatalf("reminder at %v, want %v", res.WakeAt.In(loc), want)
	}
	*now = time.Date(2026, 9, 26, 12, 0, 0, 0, loc)
	wakeAll(t, h)
	if len(queued(h)) != 1 {
		t.Fatalf("no reminder on Saturday: %q", queued(h))
	}
}

// Every human answer names its task, not only approvals: a late or
// retried answer must not answer the run's next task.
func TestEveryAnswerNamesItsTask(t *testing.T) {
	h, _ := slaHarness(t)
	h.startFn("strict", "")
	ctx := context.Background()
	if _, err := h.rt.Deliver(ctx, "run-1", Reply{From: "human", Text: "123"}); !errors.Is(err, ErrTaskChanged) {
		t.Fatalf("an answer without the task's hash: %v", err)
	}
	task, _ := h.rt.PendingTask(ctx, "run-1")
	if res, err := h.rt.Deliver(ctx, "run-1", Reply{From: "human", Text: "123", TaskHash: task.Hash}); err != nil || res.Status != journal.StatusSucceeded {
		t.Fatalf("%+v %v", res, err)
	}
}

// An answer before the deadline stops the SLA; an approval must carry the
// task's hash, and a hash of another task is refused.
func TestApprovalIsBoundToTheTask(t *testing.T) {
	h, now := slaHarness(t)
	h.startFn("discount", "")
	ctx := context.Background()
	if _, err := h.rt.Deliver(ctx, "run-1", Reply{From: "human", Text: "sim", Value: true}); !errors.Is(err, ErrTaskChanged) {
		t.Fatalf("an approval without the task's hash must be refused: %v", err)
	}
	if _, err := h.rt.Deliver(ctx, "run-1", Reply{From: "human", Value: true, TaskHash: "0000"}); !errors.Is(err, ErrTaskChanged) {
		t.Fatalf("a hash of another task must be refused: %v", err)
	}
	task, err := h.rt.PendingTask(ctx, "run-1")
	if err != nil || task.Kind != "approve" || task.Queue != "salao" || task.Hash == "" {
		t.Fatalf("task %+v %v", task, err)
	}
	*now = now.Add(20 * time.Minute)
	res, err := h.rt.Deliver(ctx, "run-1", Reply{From: "human", Value: true, TaskHash: task.Hash, By: "gerente@loja"})
	if err != nil || res.Status != journal.StatusSucceeded {
		t.Fatalf("%+v %v", res, err)
	}
	*now = now.Add(5 * time.Hour)
	if got := wakeAll(t, h); len(got) != 0 {
		t.Errorf("an answered task has no SLA left: %+v", got)
	}
	recs, _ := h.store.Records(ctx, "run-1")
	found := false
	for _, r := range recs {
		found = found || (r.Kind == journal.KindResume && strings.Contains(string(r.Value), `"by":"gerente@loja"`))
	}
	if !found {
		t.Error("who approved is recorded")
	}
}

func TestSLATimeoutFails(t *testing.T) {
	h, now := slaHarness(t)
	h.startFn("strict", "")
	*now = now.Add(31 * time.Minute)
	wakeAll(t, h)
	if run, _ := h.store.GetRun(context.Background(), "run-1"); run.Status != journal.StatusFailed {
		t.Errorf("status %s", run.Status)
	}
}

// A reply delivered again after its execution failed finishes that
// execution only if it is the same reply: same text and same value.
func TestRepeatedReplyMustBeTheSame(t *testing.T) {
	h, _ := slaHarness(t)
	h.startFn("discount", "")
	ctx := context.Background()
	task, _ := h.rt.PendingTask(ctx, "run-1")
	// An approval recorded, then the process stopped before continuing.
	v, _ := journal.Encode(map[string]any{"text": "", "value": true, "from": "human"})
	if _, err := h.store.Append(ctx, "run-1", journal.Record{Site: task.Site, Kind: journal.KindResume, Value: v, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.rt.Deliver(ctx, "run-1", Reply{From: "human", Value: false, TaskHash: task.Hash}); !errors.Is(err, ErrNotWaiting) {
		t.Fatalf("a denial after a recorded approval is not the same reply: %v", err)
	}
}
