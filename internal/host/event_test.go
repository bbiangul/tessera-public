package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const eventDocs = `apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: novo-lead, namespace: sdr }
spec:
  function: cadencia
  webhook: { secret: CRM, key: "event.payload.leadId", conversation: "'wa:' + event.payload.phone" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: reuniao, namespace: sdr }
spec:
  event: { name: reuniao-marcada, key: "event.payload.leadId" }
  webhook: { secret: CRM }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: cadencia, namespace: sdr }
spec:
  description: Waits two days for a reply or the meeting, then follows up.
  authorize: "has(caller.trigger)"
  steps:
    - id: w
      wait:
        for: 48h
        orReply: true
        events: [ { name: reuniao-marcada, key: "event.payload.leadId" } ]
    - id: depois
      branch:
        - { when: "steps.w.output.event == 'reuniao-marcada'", respond: { template: "Reunião marcada para {{ steps.w.output.payload.dia }}.", to: "queue:vendas" } }
        - { when: "steps.w.output.reply != null", respond: { template: "Obrigado!", to: "queue:vendas" } }
        - { else: true, respond: { template: "Sem resposta.", to: "queue:vendas" } }
  output: { value: "true" }
`

func eventHost(t *testing.T, now *time.Time) (*Host, journal.Store) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sdr.yaml"), []byte(eventDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemory()
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "sdr", Store: store, Decider: &wordDecider{},
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return *now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h, store
}

func outboxText(r runtime.Result) []string {
	var out []string
	for _, m := range r.Outbox {
		out = append(out, m.Text)
	}
	return out
}

// The CRM reports the lead's meeting: the lead's waiting cadence gets it,
// the other lead's does not, and a second report changes nothing.
func TestEventEndsTheWait(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	h, store := eventHost(t, &now)
	ctx := context.Background()
	a, err := h.StartHook(ctx, "novo-lead", map[string]any{"leadId": "L1", "phone": "5511"}, "k1")
	if err != nil || a.Status != journal.StatusWaitingForUser {
		t.Fatalf("%+v %v", a, err)
	}
	b, _ := h.StartHook(ctx, "novo-lead", map[string]any{"leadId": "L2", "phone": "5522"}, "k2")
	if again, _ := h.StartHook(ctx, "novo-lead", map[string]any{"leadId": "L1", "phone": "5511"}, "another-key"); again.RunID != a.RunID {
		t.Fatalf("one cadence per lead, whatever the sender's key: %s and %s", a.RunID, again.RunID)
	}
	res, err := h.StartHook(ctx, "reuniao", map[string]any{"leadId": "L1", "dia": "terça"}, "e1")
	if err != nil || res.RunID != a.RunID || res.Status != journal.StatusSucceeded {
		t.Fatalf("delivered: %+v %v", res, err)
	}
	if got := outboxText(res); len(got) != 1 || got[0] != "Reunião marcada para terça." {
		t.Fatalf("the event's payload reaches the run: %q", got)
	}
	if res, _ := h.StartHook(ctx, "reuniao", map[string]any{"leadId": "L1", "dia": "quarta"}, "e2"); res.RunID != "" {
		t.Fatalf("a second report reaches nobody: %+v", res)
	}
	if run, _ := store.GetRun(ctx, b.RunID); run.Status != journal.StatusWaitingForUser {
		t.Fatalf("the other lead still waits: %s", run.Status)
	}
	// The other lead's wait times out; a late report finds nothing and the
	// stale watch is forgotten.
	now = now.Add(49 * time.Hour)
	if _, err := h.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if ws, _ := store.Watchers(ctx, "sdr", "reuniao-marcada", "L2"); len(ws) != 0 {
		t.Errorf("a finished run forgets its watches: %+v", ws)
	}
	if res, _ := h.StartHook(ctx, "reuniao", map[string]any{"leadId": "L2", "dia": "sexta"}, "e3"); res.RunID != "" {
		t.Fatalf("after the timeout: %+v", res)
	}
	if ws, _ := store.Watchers(ctx, "sdr", "reuniao-marcada", "L2"); len(ws) != 0 {
		t.Errorf("watches left: %+v", ws)
	}
	recs, _ := store.Records(ctx, b.RunID)
	for _, r := range recs {
		if r.Kind == journal.KindResume && r.Site == "w" && string(r.Value) != "" {
			if v, _ := journal.Decode(r.Value); v.(map[string]any)["event"] != nil {
				t.Errorf("the timed-out wait got an event: %v", v)
			}
		}
	}
}
