package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide/decidetest"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const helloAgent = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: hello, namespace: demo }
spec:
  description: Greets the user.
  selector: { intent: greeting }
  steps:
    - id: ok
      escalate: { queue: team, task: approve, message: "Posso cumprimentar?" }
  output: { template: "Olá! ({{ steps.ok.output }})" }
`

func TestChatLoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(helloAgent), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &decidetest.Scripted{Answers: map[string]map[string]float64{"route": {"hello": 0.9, "none": 0.1}}}
	h, _, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "demo", Store: journal.NewMemory(), Decider: d})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	in := strings.NewReader("/human sim\noi\noi de novo\n/human true\n/quit\n")
	if err := chatLoop(context.Background(), h, in, &out, "c", nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"error: no run is waiting",
		"[started hello]",
		"[queue:team, task approve] Posso cumprimentar?",
		"[waiting for a person",
		"agent> A person from the team",
		"agent> Olá! (true)",
		"[run Succeeded]",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
}

func TestChatRequiresABudget(t *testing.T) {
	if _, err := run(t, "chat", "x", "--namespace", "demo", "--max-cost-usd", "0"); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Errorf("want a budget error, got %v", err)
	}
}
