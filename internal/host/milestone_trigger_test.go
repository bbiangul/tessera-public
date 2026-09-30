package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const openingDocs = `apiVersion: agents.tessera.dev/v1alpha1
kind: Subject
metadata: { name: loja, namespace: fq }
spec: { id: "event.payload.unidade" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: etapa, namespace: fq }
spec:
  function: registrar
  webhook: { secret: ERP, conversation: "'erp:' + event.payload.unidade" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: registrar, namespace: fq }
spec:
  description: Records a finished step.
  authorize: "has(caller.trigger)"
  steps:
    - { id: aberta, when: "event.payload.etapa == 'inauguracao'", compute: { cel: "true" }, milestone: { name: loja-aberta, subject: loja } }
    - { id: obra, when: "event.payload.etapa == 'obra'", compute: { cel: "true" }, milestone: { name: obra-concluida, subject: loja } }
  output: { value: "true" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: inaugurou, namespace: fq }
spec:
  function: parabens
  milestone: { subject: loja, name: loja-aberta }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: parabens, namespace: fq }
spec:
  description: Tells the board a store opened.
  authorize: "has(caller.trigger)"
  steps:
    - { id: nota, respond: { template: "A loja {{ event.payload.id }} inaugurou.", to: "queue:diretoria" } }
  output: { value: "true" }
`

func openingHost(t *testing.T, dir string, store journal.Store, now *time.Time) *Host {
	t.Helper()
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "fq", Store: store, Decider: &wordDecider{},
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return *now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h
}

func TestMilestoneTrigger(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fq.yaml"), []byte(openingDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	// History: a store that opened before the trigger existed.
	store := journal.NewMemory()
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	old := withoutTrigger(openingDocs)
	if err := os.WriteFile(filepath.Join(dir, "fq.yaml"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	h0 := openingHost(t, dir, store, &now)
	ctx := context.Background()
	if _, err := h0.StartHook(ctx, "etapa", map[string]any{"unidade": "7", "etapa": "inauguracao"}, "old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fq.yaml"), []byte(openingDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	a, b := openingHost(t, dir, store, &now), openingHost(t, dir, store, &now) // two schedulers
	if res, _ := a.Tick(ctx); len(res) != 0 {
		t.Fatalf("history does not start the trigger: %+v", res)
	}
	for _, e := range []struct{ unidade, etapa string }{{"12", "obra"}, {"12", "inauguracao"}} {
		if _, err := a.StartHook(ctx, "etapa", map[string]any{"unidade": e.unidade, "etapa": e.etapa}, e.unidade+e.etapa); err != nil {
			t.Fatal(err)
		}
	}
	var started []runtime.Result
	for range 2 {
		for _, h := range []*Host{a, b} {
			res, err := h.Tick(ctx)
			if err != nil {
				t.Fatal(err)
			}
			started = append(started, res...)
		}
	}
	if len(started) != 1 || len(started[0].Outbox) != 1 || started[0].Outbox[0].Text != "A loja 12 inaugurou." {
		t.Fatalf("once, for the new opening only: %+v", started)
	}
	// The cursor moved past what was handled, so the next tick reads only
	// new milestones (a trigger never rereads a page of others' forever).
	last, _ := store.LastMilestone(ctx, "fq")
	if cur, ok, _ := store.Cursor(ctx, a.cursorName(a.milestoneTriggers[0])); !ok || cur != last {
		t.Errorf("cursor %d, last milestone %d", cur, last)
	}
	// A crash after the start, before the cursor moved: the next tick
	// reads the milestone again and starts nothing.
	_ = store.SetCursor(ctx, a.cursorName(a.milestoneTriggers[0]), last-1)
	if res, err := b.Tick(ctx); err != nil || len(res) != 0 {
		t.Fatalf("after a crash: %+v %v", res, err)
	}
	run, _ := store.GetRun(ctx, started[0].RunID)
	if run.Conversation != "erp:12" {
		t.Errorf("in the conversation of the run that recorded it: %q", run.Conversation)
	}
}

// withoutTrigger is the resources before the milestone trigger (and its
// Function) was added.
func withoutTrigger(docs string) string {
	sep := "---\napiVersion: agents.tessera.dev/v1alpha1\n"
	i := strings.Index(docs, sep+"kind: Trigger\nmetadata: { name: inaugurou")
	return docs[:i]
}
