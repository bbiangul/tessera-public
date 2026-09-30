package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide/decidetest"
)

func TestPreferenceBatchPolicy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		info, stop float64
		side       []string
		want       string
		count      int
	}{
		{"mixed FAQ", .95, .05, []string{}, turnQuestion, 3},
		{"current continuation", .05, .05, []string{"Saturday; parking?"}, turnAnswer, 3},
		{"withdrawal", .05, .95, []string{}, turnCancel, 3},
		{"ordinary wait", .95, .95, nil, turnAnswer, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := freeHost(t)
			d := &decidetest.Scripted{Answers: map[string]map[string]float64{
				"turn":        {turnAnswer: .96, turnQuestion: .01, turnRequest: .01, turnCancel: .01, turnGoodbye: .01},
				"information": {"true": tc.info, "false": 1 - tc.info}, "stopCurrent": {"true": tc.stop, "false": 1 - tc.stop},
			}}
			h.opts.Decider = d
			got, err := h.interruptionWithContext(context.Background(), "Which day?", "Proceed", tc.side)
			if err != nil || got != tc.want {
				t.Fatalf("got %q %v", got, err)
			}
			calls := d.Calls()
			if len(calls) != 1 || len(calls[0].Questions) != tc.count {
				t.Fatalf("must be one request: %+v", calls)
			}
		})
	}
}

func TestPreferenceContextMaskedAndFailuresPropagate(t *testing.T) {
	h, _, _ := freeHost(t)
	d := &decidetest.Scripted{Err: errors.New("provider unavailable")}
	h.opts.Decider = d
	got, err := h.interruptionWithContext(context.Background(), "Day?", "yes", []string{"email maria@example.com"})
	if err == nil || got != "" {
		t.Fatalf("provider error advanced: %q %v", got, err)
	}
	state := d.Calls()[0].State.(map[string]any)
	if strings.Contains(fmt.Sprint(state["sideMessages"]), "maria@example.com") {
		t.Fatal("unmasked side context")
	}
	d.Err = nil
	d.Answers = map[string]map[string]float64{}
	if _, err := h.interruptionWithContext(context.Background(), "Day?", "yes", []string{}); err == nil {
		t.Fatal("missing answer advanced")
	}
}

func TestPreferenceQuestionsMatchMeasuredK(t *testing.T) {
	b, err := os.ReadFile("../../bench/results/preference-001/classifier-k-replication/runs.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var row struct {
		Request decide.Request `json:"request"`
	}
	if err := json.Unmarshal(bytes.SplitN(b, []byte("\n"), 2)[0], &row); err != nil {
		t.Fatal(err)
	}
	h, _, _ := freeHost(t)
	d := &decidetest.Scripted{Err: errors.New("capture only")}
	h.opts.Decider = d
	_, _ = h.interruptionWithContext(context.Background(), "Day?", "yes", []string{})
	if !reflect.DeepEqual(d.Calls()[0].Questions, row.Request.Questions) {
		t.Fatal("production questions differ from measured K")
	}
}
