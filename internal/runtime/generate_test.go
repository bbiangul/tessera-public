package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const extractFunction = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: plan, namespace: travel }
spec:
  description: Understands a trip request.
  steps:
    - id: trip
      extract:
        state: { message: "${ event.message }", today: "${ formatDate(event.time, 'America/Sao_Paulo') }" }
        instructions: Extract the trip.
        schema:
          type: object
          required: [destination, departDate]
          properties:
            destination: { type: string }
            departDate: { type: string, pattern: "^\\d{4}-\\d{2}-\\d{2}$" }
      onError:
        - respond: { template: "Não entendi a viagem. Para onde e quando?" }
    - id: future
      compute: { cel: "date(steps.trip.output.departDate, 'America/Sao_Paulo') > event.time" }
  output:
    template: "{{ steps.trip.output.destination }} em {{ steps.trip.output.departDate }} ({{ steps.future.output }})"
`

// scriptedGen returns fixed replies and counts calls.
type scriptedGen struct {
	value any
	err   error
	calls int
	last  generate.Request
}

func (g *scriptedGen) Name() string { return "scripted" }
func (g *scriptedGen) Generate(_ context.Context, r generate.Request) (*generate.Response, error) {
	g.calls++
	g.last = r
	if g.err != nil {
		return &generate.Response{Attempts: 2}, g.err
	}
	return &generate.Response{Value: g.value, Model: "scripted-1", Attempts: 1}, nil
}

func extractHarness(t *testing.T, g *scriptedGen) *harness {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fn.yaml"), []byte(extractFunction), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	h.rt.Generator = g
	return h
}

func TestExtractStep(t *testing.T) {
	g := &scriptedGen{value: map[string]any{"destination": "Lisboa", "departDate": "2026-10-12"}}
	h := extractHarness(t, g)
	res := h.startFn("plan", "Lisboa dia 12 de outubro, email ana@x.com")
	expectStatus(t, res, journal.StatusSucceeded)
	if h.user[0] != "Lisboa em 2026-10-12 (true)" {
		t.Errorf("messages %q", h.user)
	}
	state := g.last.State.(map[string]any)
	if state["today"] != "2026-09-23" || state["message"] != "Lisboa dia 12 de outubro, email [EMAIL]" {
		t.Errorf("state sent to the model: %v (must be masked)", state)
	}
	if _, err := h.rt.Resume(context.Background(), "run-1"); err != nil || g.calls != 1 {
		t.Errorf("replay called the model again: %d calls, %v", g.calls, err)
	}
}

func TestRefusedExtractionGoesToOnError(t *testing.T) {
	g := &scriptedGen{err: generate.ErrInvalidOutput}
	h := extractHarness(t, g)
	res := h.startFn("plan", "hmm")
	expectStatus(t, res, journal.StatusFailed)
	if len(h.user) != 1 || h.user[0] != "Não entendi a viagem. Para onde e quando?" {
		t.Errorf("messages %q", h.user)
	}
}

func TestExtractWithoutAGenerator(t *testing.T) {
	h := extractHarness(t, &scriptedGen{})
	h.rt.Generator = nil
	res := h.startFn("plan", "Lisboa")
	expectStatus(t, res, journal.StatusFailed) // onError still answers the user
}
