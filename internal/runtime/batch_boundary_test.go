package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide/decidetest"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Equal independent state does not permit moving a question across a failure.
func TestBatchChangesQuestionsIssuedBeforeFailure(t *testing.T) {
	const header = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: boundary, namespace: travel }
spec:
  description: Measures the question boundary on failure.
  steps:
`
	const serial = `    - id: first
      decide:
        state: { message: "${ event.message }" }
        instructions: Is a weekend requested?
        yesNo: { ifTrue: Weekend requested }
        minConfidence: 0.7
        onLowConfidence: fail
    - id: second
      decide:
        state: { message: "${ event.message }" }
        instructions: Is morning requested?
        yesNo: { ifTrue: Morning requested }
        minConfidence: 0.7
        onLowConfidence: fail
`
	const batch = `    - id: together
      decideBatch:
        state: { message: "${ event.message }" }
        questions:
          first:
            instructions: Is a weekend requested?
            yesNo: { ifTrue: Weekend requested }
          second:
            instructions: Is morning requested?
            yesNo: { ifTrue: Morning requested }
        minConfidence: 0.7
        onLowConfidence: fail
`
	for _, failure := range []string{"uncertain", "provider"} {
		for _, arm := range []struct {
			name, steps string
			questions   int
		}{
			{"serial", serial, 1}, {"batch", batch, 2},
		} {
			t.Run(failure+"/"+arm.name, func(t *testing.T) {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "function.yaml"), []byte(header+arm.steps), 0600); err != nil {
					t.Fatal(err)
				}
				h := newHarness(t, dir)
				d := &decidetest.Scripted{Answers: map[string]map[string]float64{
					"q": {"true": .5, "false": .5}, "first": {"true": .5, "false": .5}, "second": {"true": .95, "false": .05},
				}}
				if failure == "provider" {
					d.Err = errors.New("injected provider failure")
				}
				h.rt.Decider = d
				expectStatus(t, h.startFn("boundary", "talvez"), journal.StatusFailed)
				calls := d.Calls()
				if len(calls) != 1 || len(calls[0].Questions) != arm.questions {
					t.Fatalf("unexpected calls: %+v", calls)
				}
				t.Logf("requests=%d questions=%d final=Failed", len(calls), len(calls[0].Questions))
			})
		}
	}
}
