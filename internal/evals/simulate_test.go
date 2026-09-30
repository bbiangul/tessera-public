package evals

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/generate"
)

// scriptedUser plays a persona from a fixed list of messages and records
// what it was shown, so the test sees the simulator's inputs.
type scriptedUser struct {
	msgs []string
	seen [][]any
}

func (s *scriptedUser) Name() string { return "scripted-user" }
func (s *scriptedUser) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	state, _ := req.State.(map[string]any)
	conv, _ := state["conversation"].([]any)
	s.seen = append(s.seen, conv)
	if len(s.msgs) == 0 {
		return &generate.Response{Value: map[string]any{"message": nil, "done": true}, Model: "scripted"}, nil
	}
	m := s.msgs[0]
	s.msgs = s.msgs[1:]
	return &generate.Response{Value: map[string]any{"message": m, "done": false}, Model: "scripted"}, nil
}

const llmScenario = `  scenarios:
    - id: persona
      caller: { role: patient, patientId: p1, appointmentIds: [a1, a2] }
      simulator: llm
      maxTurns: 6
      persona:
        goal: "move Tuesday's appointment to Thursday 16h"
        knows: { consulta: "terça" }
      expect:
        effects: [ { action: agenda/reschedule, count: 1 } ]
`

func TestLLMSimulatorPlaysUntilDone(t *testing.T) {
	dir := copyExample(t)
	edit(t, filepath.Join(dir, "evals/reschedule.evalsuite.yaml"), "  scenarios:\n", llmScenario)
	user := &scriptedUser{msgs: []string{"oi, preciso remarcar minha consulta", "a de terça", "pode ser a das 16h", "sim"}}
	results, issues, err := Run(context.Background(), Options{Paths: []string{dir}, Decider: keywords{}, Simulator: user, MaxScenarioCostUSD: 1, Only: "reschedule/persona"})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	r := results[0]
	if !r.Pass {
		t.Fatalf("%v\n%v", r.Failures, r.Transcript)
	}
	if r.Simulator == nil || r.Simulator.Calls["generate"] != 5 {
		t.Errorf("the simulator's calls are metered apart: %+v", r.Simulator)
	}
	if last := r.Transcript[len(r.Transcript)-1]; last.Who != "simulator" || last.Text != "done" {
		t.Errorf("the scenario ends when the persona is done: %v", last)
	}
	// The simulator sees the agent's replies, never the queues or itself.
	final := user.seen[len(user.seen)-1]
	if len(final) != 8 {
		t.Errorf("the persona saw %d lines, want its 4 messages and the agent's 4 replies: %v", len(final), final)
	}
	for _, l := range final {
		who := l.(map[string]any)["who"]
		if who != "user" && who != "agent" {
			t.Errorf("the persona saw a %v line", who)
		}
	}
}

func TestLLMSimulatorStopsAtMaxTurns(t *testing.T) {
	dir := copyExample(t)
	edit(t, filepath.Join(dir, "evals/reschedule.evalsuite.yaml"), "  scenarios:\n", strings.Replace(llmScenario, "maxTurns: 6", "maxTurns: 2", 1))
	user := &scriptedUser{msgs: []string{"oi, preciso remarcar minha consulta", "a de terça", "pode ser a das 16h", "sim"}}
	results, _, err := Run(context.Background(), Options{Paths: []string{dir}, Decider: keywords{}, Simulator: user, MaxScenarioCostUSD: 1, Only: "reschedule/persona"})
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.Pass {
		t.Error("two messages cannot reschedule; the expectation must fail")
	}
	if last := r.Transcript[len(r.Transcript)-1]; !strings.Contains(last.Text, "stopped after 2") {
		t.Errorf("want the stop recorded, got %v", last)
	}
}

func TestLLMScenarioNeedsAPersona(t *testing.T) {
	dir := copyExample(t)
	edit(t, filepath.Join(dir, "evals/reschedule.evalsuite.yaml"), "  scenarios:\n", `  scenarios:
    - id: persona
      simulator: llm
`)
	_, issues, err := Run(context.Background(), Options{Paths: []string{dir}, Decider: keywords{}, MaxScenarioCostUSD: 1})
	if err == nil || !strings.Contains(fmt.Sprint(issues), "needs a persona") {
		t.Errorf("an llm scenario without a persona must be refused: %v", issues)
	}
}

func TestLLMScenarioWithoutBackendFails(t *testing.T) {
	dir := copyExample(t)
	edit(t, filepath.Join(dir, "evals/reschedule.evalsuite.yaml"), "  scenarios:\n", llmScenario)
	results, _, err := Run(context.Background(), Options{Paths: []string{dir}, Decider: keywords{}, MaxScenarioCostUSD: 1, Only: "reschedule/persona"})
	if err != nil {
		t.Fatal(err)
	}
	if r := results[0]; r.Pass || !strings.Contains(strings.Join(r.Failures, "\n"), "needs a Generate backend") {
		t.Errorf("want a clear failure without a backend: %v", r.Failures)
	}
}
