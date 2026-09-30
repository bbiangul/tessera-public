package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/metrics"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const guardDocs = `apiVersion: agents.tessera.dev/v1alpha1
kind: Subject
metadata: { name: lead, namespace: sdr }
spec: { id: "event.payload.leadId" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Funnel
metadata: { name: prosp, namespace: sdr }
spec:
  subject: lead
  stages: [ { name: contato, milestone: contatado }, { name: reuniao, milestone: reuniao-marcada, within: 14d } ]
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Experiment
metadata: { name: msg, namespace: sdr }
spec:
  subject: lead
  variants: [ { name: controle, weight: 1 }, { name: agressiva, weight: 1 } ]
  primary: { funnel: prosp, stage: reuniao }
  guardrails: [ { name: opt-out, milestone: opt-out, within: 3d, maxIncrease: 0.02 } ]
  onBreach: { function: avisar }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: novo, namespace: sdr }
spec: { function: contatar, webhook: { secret: NOVO } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: contatar, namespace: sdr }
spec:
  description: First contact, in the lead's variant.
  authorize: "has(caller.trigger)"
  steps:
    - { id: v, experiment: msg }
  output: { value: "steps.v.output" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: saiu, namespace: sdr }
spec: { function: descadastrar, webhook: { secret: SAIU } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: descadastrar, namespace: sdr }
spec:
  description: The lead asked to stop.
  authorize: "has(caller.trigger)"
  steps:
    - { id: fim, compute: { cel: "true" }, milestone: { name: opt-out, subject: lead } }
  output: { value: "true" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: avisar, namespace: sdr }
spec:
  description: Tells the team a guardrail stopped an experiment.
  authorize: "has(caller.experiment)"
  steps:
    - id: tarefa
      escalate: { queue: vendas, task: answer, message: "{{ event.payload.experiment }}: {{ event.payload.variant }} piorou {{ event.payload.guardrail }}" }
  output: { value: "true" }
`

// A variant that makes leads opt out stops the experiment: onBreach runs
// once, and every lead exposed after that gets the control, left out of
// the comparison.
func TestGuardrailStopsTheExperiment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sdr.yaml"), []byte(guardDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "sdr", Store: journal.NewMemory(), Decider: &wordDecider{},
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	variants := []metrics.Variant{{Name: "controle", Weight: 1}, {Name: "agressiva", Weight: 1}}
	optOuts := map[string]int{}
	for i := range 300 {
		id := fmt.Sprintf("lead-%d", i)
		if _, err := h.StartHook(ctx, "novo", map[string]any{"leadId": id}, id); err != nil {
			t.Fatal(err)
		}
		v := metrics.Assign("msg", id, variants)
		if v == "agressiva" && i%4 == 0 || v == "controle" && i%50 == 0 { // ~25% against ~2%
			if _, err := h.StartHook(ctx, "saiu", map[string]any{"leadId": id}, id); err != nil {
				t.Fatal(err)
			}
			optOuts[v]++
		}
	}
	if _, err := h.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if stopped, _ := h.experimentStopped(ctx, "msg"); stopped {
		t.Fatal("inside the window nothing is decided yet")
	}
	now = now.Add(4 * 24 * time.Hour)
	res, err := h.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Status != journal.StatusWaitingForHuman || len(res[0].Outbox) == 0 || res[0].Outbox[0].Text != "msg: agressiva piorou opt-out" {
		t.Fatalf("onBreach escalates once: %+v (opt-outs %v)", res, optOuts)
	}
	now = now.Add(StuckEvery)
	if res, _ := h.Tick(ctx); len(res) != 0 {
		t.Fatalf("a stopped experiment is not stopped again: %+v", res)
	}
	// After the stop: the control for everyone, even a lead hashed to the
	// other variant.
	late := ""
	for i := 1000; late == ""; i++ {
		if id := fmt.Sprintf("lead-%d", i); metrics.Assign("msg", id, variants) == "agressiva" {
			late = id
		}
	}
	r, err := h.StartHook(ctx, "novo", map[string]any{"leadId": late}, late)
	if err != nil || r.Status != journal.StatusSucceeded {
		t.Fatalf("%+v %v", r, err)
	}
	rep, err := h.ExperimentReport(ctx, "msg")
	if err != nil {
		t.Fatal(err)
	}
	if rep["stopped"] != true {
		t.Errorf("report %v", rep)
	}
	arms := rep["arms"].([]any)
	exposed := arms[0].(map[string]any)["exposed"].(int64) + arms[1].(map[string]any)["exposed"].(int64)
	if exposed != 300 {
		t.Errorf("exposures after the stop are left out: %d exposed", exposed)
	}
	ms, _ := h.Runtime.Store.Milestones(ctx, "sdr", "lead")
	for _, m := range ms {
		if m.ID == late && m.Name == "experiment/msg" && (m.Attributes["variant"] != "controle" || m.Attributes["stopped"] != "true") {
			t.Errorf("after the stop: %+v", m)
		}
	}
}
