package host

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const whoDocs = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: pedir, namespace: w }
spec:
  description: d
  selector: { intent: pedir, who: "caller.role == 'patient'" }
  output: { template: "ok" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: bloquear, namespace: w }
spec:
  description: d
  selector: { intent: bloquear, who: "caller.role == 'dentist'" }
  output: { template: "ok" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horario, namespace: w }
spec:
  description: d
  selector: { intent: horario }
  output: { template: "ok" }
`

// optionsDecider records the router's options and picks none.
type optionsDecider struct {
	mu      sync.Mutex
	offered [][]string
}

func (*optionsDecider) Name() string { return "options" }
func (d *optionsDecider) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	q := req.Questions["route"]
	var keys []string
	probs := map[string]float64{}
	for _, o := range q.Options {
		keys = append(keys, o.Key)
		probs[o.Key] = 0.01
	}
	probs["none"] = 0.9
	d.mu.Lock()
	d.offered = append(d.offered, keys)
	d.mu.Unlock()
	a, err := decide.AnswerFor(q, probs, decide.ProbNative)
	if err != nil {
		return nil, err
	}
	return &decide.Response{Answers: map[string]decide.Answer{"route": a}}, nil
}

// RFC-0006: the router offers a Function only to callers its who holds
// for; a Function without who is offered to everyone.
func TestWhoFiltersTheMenu(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(whoDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &optionsDecider{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "w", Store: journal.NewMemory(), Decider: d})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	for _, c := range []struct {
		caller map[string]any
		want   string
	}{
		{map[string]any{"role": "patient"}, "horario,pedir,none"},
		{map[string]any{"role": "dentist"}, "bloquear,horario,none"},
		{map[string]any{}, "horario,none"},
	} {
		if _, err := h.Handle(context.Background(), "c", c.caller, "oi"); err != nil {
			t.Fatal(err)
		}
		got := d.offered[len(d.offered)-1]
		s := ""
		for i, k := range got {
			if i > 0 {
				s += ","
			}
			s += k
		}
		if s != c.want {
			t.Errorf("%v: offered %s, want %s", c.caller, s, c.want)
		}
	}
}
