package main

import (
	"github.com/bbiangul/tessera/internal/engines/decide"
	"testing"
)

func TestScopeGuardNeverTreatsUncertaintyAsUnrestricted(t *testing.T) {
	for _, policy := range []string{"A", "B"} {
		q := scopeQuestion(policy)
		if err := (decide.Request{State: "synthetic", Questions: map[string]decide.Question{"scope": q}}).Validate(); err != nil {
			t.Fatal(err)
		}
		outcomes := q.Outcomes()
		a, err := decide.AnswerFor(q, map[string]float64{outcomes[0]: .5, outcomes[1]: .5}, decide.ProbNative)
		if err != nil {
			t.Fatal(err)
		}
		answers := map[string]decide.Answer{"scope": a, "explicit": {Kind: decide.KindNoul, Noul: .01}}
		correct, unsafe, clarified := scoreScope(sample{Clarify: true}, answers, policy)
		if !correct || unsafe || !clarified {
			t.Fatal(policy, correct, unsafe, clarified)
		}
		correct, unsafe, clarified = scoreScope(sample{Allowed: []int{0, 1, 2, 3, 4, 5, 6}}, answers, policy)
		if correct || unsafe || !clarified {
			t.Fatal("unneeded clarification hidden")
		}
	}
}
