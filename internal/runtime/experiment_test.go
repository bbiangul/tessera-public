package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bbiangul/tessera/internal/metrics"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const abTest = `apiVersion: agents.tessera.dev/v1alpha1
kind: Subject
metadata: { name: lead, namespace: travel }
spec: { id: "conversation.id" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Funnel
metadata: { name: prospeccao, namespace: travel }
spec:
  subject: lead
  stages: [ { name: contato, milestone: contatado }, { name: reuniao, milestone: reuniao-marcada, within: 14d } ]
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Experiment
metadata: { name: primeira-mensagem, namespace: travel }
spec:
  subject: lead
  variants: [ { name: controle, weight: 50 }, { name: prova-social, weight: 50 } ]
  primary: { funnel: prospeccao, stage: reuniao }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: greet, namespace: travel }
spec:
  description: First message, in two versions.
  steps:
    - { id: v, experiment: primeira-mensagem }
    - id: say
      branch:
        - { when: "steps.v.output == 'prova-social'", respond: { template: "Mais de 500 empresas já usam. Posso ajudar?" } }
        - { else: true, respond: { template: "Olá! Posso ajudar?" } }
  output: { value: "steps.v.output" }
`

// The step gives the subject its hashed variant, the message follows it,
// and the exposure is recorded once, even when the run is resumed after a
// failure at every write.
func TestExperimentStep(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ab.yaml"), []byte(abTest), 0o600); err != nil {
		t.Fatal(err)
	}
	want := metrics.Assign("primeira-mensagem", "c1", []metrics.Variant{{Name: "controle", Weight: 50}, {Name: "prova-social", Weight: 50}})
	for k := 0; k <= 4; k++ {
		h := newHarness(t, dir)
		h.store.FailAppendAfter = k
		ctx := context.Background()
		res, err := h.rt.Start(ctx, Start{Namespace: "travel", Function: "greet", Conversation: "c1"})
		for tries := 0; err != nil; tries++ {
			if tries > 3 {
				t.Fatalf("k=%d: %v", k, err)
			}
			res, err = h.rt.Resume(ctx, "run-1")
		}
		h.deliver(res)
		ms, _ := h.store.Milestones(ctx, "travel", "lead")
		if len(ms) != 1 || ms[0].Name != "experiment/primeira-mensagem" || ms[0].Attributes["variant"] != want {
			t.Fatalf("k=%d: exposures %+v", k, ms)
		}
		text := "Olá! Posso ajudar?"
		if want == "prova-social" {
			text = "Mais de 500 empresas já usam. Posso ajudar?"
		}
		if len(h.user) != 1 || h.user[0] != text {
			t.Errorf("k=%d: messages %q for variant %s", k, h.user, want)
		}
		if run, _ := h.store.GetRun(ctx, "run-1"); run.Status != journal.StatusSucceeded {
			t.Errorf("k=%d: %s", k, run.Status)
		}
	}
}
