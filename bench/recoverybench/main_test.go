package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/decide/decidetest"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

type scriptedRecovery struct{}

func (scriptedRecovery) Name() string { return "scripted-recovery" }
func (scriptedRecovery) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	answers := map[string]map[string]float64{}
	for id := range req.Questions {
		switch id {
		case "route":
			if strings.Contains(req.State.(map[string]any)["message"].(string), "sei") {
				answers[id] = map[string]float64{"none": 1}
			} else {
				answers[id] = map[string]float64{"filter-days": 1}
			}
		case "explicit":
			if _, reply := req.Questions["resolved"]; reply {
				answers[id] = map[string]float64{"true": 1}
			} else {
				answers[id] = map[string]float64{"true": .5, "false": .5}
			}
		case "resolved":
			state := req.State.(map[string]any)
			p := 1.0
			if strings.Contains(state["message"].(string), "sei") {
				p = 0
			}
			answers[id] = map[string]float64{"true": p, "false": 1 - p}
		case "6":
			answers[id] = map[string]float64{"true": 1}
		default:
			answers[id] = map[string]float64{"false": 1}
		}
	}
	return (&decidetest.Scripted{Answers: answers}).Decide(ctx, req)
}
func TestDurableClarificationAndBoundedStop(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply *string
		want  []int
	}{
		{"resolved", ptr("Só sábado."), []int{6}},
		{"unresolved", ptr("Não sei ainda."), nil},
		{"timeout", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := run(context.Background(), "../results/clarification-001/defs/R", dialogue{ID: tc.name, Message: "initial", Reply: tc.reply, Allowed: tc.want}, scriptedRecovery{})
			if !r.Correct || !r.SameRun || !r.Asked || r.Error != "" {
				t.Fatalf("%+v", r)
			}
		})
	}
}
func TestAssessmentRejectsEarlierUnsafeOffer(t *testing.T) {
	r := result{Masks: [][]int{{0, 6}, {6}}, FinalStatus: journal.StatusSucceeded}
	assess(dialogue{Allowed: []int{6}}, &r)
	if r.Correct || !r.Unsafe {
		t.Fatal(r)
	}
	r = result{Masks: [][]int{{6}}, FinalStatus: journal.StatusWaitingForTime}
	assess(dialogue{Allowed: []int{6}}, &r)
	if r.Correct {
		t.Fatal("unfinished run counted complete")
	}
}
func ptr(s string) *string { return &s }

func TestUnroutedReplyHasNoRunToAcknowledge(t *testing.T) {
	r := run(context.Background(), "../results/clarification-001/defs/B", dialogue{ID: "unrouted", Message: "initial", Reply: ptr("Não sei ainda.")}, scriptedRecovery{})
	if r.Error != "" || r.SameRun || r.Correct {
		t.Fatalf("%+v", r)
	}
}
