package evals

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
)

type scopeRoute struct{}

func (scopeRoute) Name() string { return "fake" }
func (scopeRoute) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	answers := map[string]decide.Answer{}
	for id, q := range req.Questions {
		p := map[string]float64{}
		for _, o := range q.Options {
			p[o.Key] = 0
		}
		p["hello"] = 1
		if q.Kind == decide.KindNoul {
			// The voice's claims check: the rewrite claims nothing.
			p = map[string]float64{"true": 0, "false": 1}
		}
		a, err := decide.AnswerFor(q, p, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		answers[id] = a
	}
	return &decide.Response{Answers: answers}, nil
}

type varyingVoice struct{ n int }

func (*varyingVoice) Name() string { return "fake" }
func (g *varyingVoice) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	g.n++
	return &generate.Response{Text: fmt.Sprintf("Tudo certo, resposta %d.", g.n)}, nil
}

func TestCassetteScenarioOrderAndSelection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resources.yaml")
	cass := filepath.Join(t.TempDir(), "models.jsonl")
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: {name: agent, namespace: testing}
spec:
  voice: {persona: Cordial}
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: {name: hello, namespace: testing}
spec:
  description: Responde oi.
  selector: {intent: hello, description: Responder oi}
  output: {template: Tudo certo.}
---
apiVersion: agents.tessera.dev/v1alpha1
kind: EvalSuite
metadata: {name: scope-test, namespace: testing}
spec:
  now: "2026-10-02T08:00:00-03:00"
  scenarios:
`
	scenario := func(id string) string {
		return fmt.Sprintf("    - id: %s\n      simulator: scripted\n      turns:\n        - user: oi\n          expect: {status: Succeeded}\n", id)
	}
	write := func(order ...string) {
		t.Helper()
		s := base
		for _, id := range order {
			s += scenario(id)
		}
		if err := os.WriteFile(path, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("a", "b")
	d, err := decide.LoadCassette(cass, "decide", scopeRoute{}, decide.CassetteRecord)
	if err != nil {
		t.Fatal(err)
	}
	g, err := generate.LoadCassette(cass, "generate", &varyingVoice{}, decide.CassetteRecord)
	if err != nil {
		t.Fatal(err)
	}
	run := func(d decide.Decider, g generate.Generator, only string) map[string][]Line {
		t.Helper()
		rs, issues, err := Run(context.Background(), Options{Paths: []string{dir}, Decider: d, Generator: g, Only: only})
		if err != nil {
			t.Fatalf("%v %v", err, issues)
		}
		out := map[string][]Line{}
		for _, r := range rs {
			if !r.Pass {
				t.Fatal(r.Failures)
			}
			out[r.Scenario] = r.Transcript
		}
		return out
	}
	want := run(d, g, "")
	if reflect.DeepEqual(want["a"], want["b"]) {
		t.Fatal("fake must produce distinct recordings")
	}
	if err := d.Save(cass); err != nil {
		t.Fatal(err)
	}
	if err := g.Save(cass); err != nil {
		t.Fatal(err)
	}
	for _, order := range [][]string{{"a", "b"}, {"b", "a"}} {
		write(order...)
		for _, only := range []string{"", "scope-test/a", "scope-test/b"} {
			rd, err := decide.LoadCassette(cass, "decide", nil, decide.CassetteReplay)
			if err != nil {
				t.Fatal(err)
			}
			rg, err := generate.LoadCassette(cass, "generate", nil, decide.CassetteReplay)
			if err != nil {
				t.Fatal(err)
			}
			got := run(rd, rg, only)
			for id, tr := range got {
				if !reflect.DeepEqual(tr, want[id]) {
					t.Fatalf("%s changed: got %v want %v", id, tr, want[id])
				}
			}
		}
	}
}
