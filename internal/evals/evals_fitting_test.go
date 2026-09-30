package evals

import (
	"testing"

	"github.com/bbiangul/tessera/internal/runtime"
)

// agency-004: a booking's reply (a value) is not given to a question for
// the team (text), and the other way around.
func TestFittingHumanAnswer(t *testing.T) {
	humans := []any{map[string]any{"reserved": true}, "Sim, fica a 200 m."}
	if i := fitting(humans, "answer"); i != 1 {
		t.Errorf("an answer task takes text: %d", i)
	}
	if i := fitting(humans, "input"); i != 0 {
		t.Errorf("an input task takes the value: %d", i)
	}
	if i := fitting(humans[1:], "input"); i != -1 {
		t.Errorf("no value left: %d", i)
	}
	if i := fitting(humans, "approve"); i != 0 {
		t.Errorf("other kinds take the next: %d", i)
	}
}

// agency-005: in agent mode a turn's Function sends before the turn's
// result; its messages are what the turn said too.
func TestTurnSaidIncludesEarlierMessages(t *testing.T) {
	said, queues := turnSaid([]runtime.Message{{To: "user", Text: "R$ 640"}, {To: "queue:atendimento", Text: "x"}},
		[]runtime.Result{{Outbox: []runtime.Message{{To: "user", Text: "Quer reservar?"}}}})
	if len(said) != 2 || said[0] != "R$ 640" || said[1] != "Quer reservar?" || !queues["atendimento"] {
		t.Errorf("%q %v", said, queues)
	}
}
