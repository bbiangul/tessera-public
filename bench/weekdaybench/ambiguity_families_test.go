package main

import (
	"github.com/bbiangul/tessera/internal/engines/decide"
	"testing"
)

func TestFamilyGuardsRequireAllAnswersConfidentlyClear(t *testing.T) {
	for _, policy := range []string{"C", "D", "E", "F"} {
		qs := familyScopeQuestions(policy)
		if err := (decide.Request{State: "synthetic", Questions: qs}).Validate(); err != nil {
			t.Fatal(err)
		}
		answers := map[string]decide.Answer{"explicit": {Kind: decide.KindNoul, Noul: .05}}
		for id := range qs {
			answers[id] = decide.Answer{Kind: decide.KindNoul, Noul: .05}
		}
		s := sample{Allowed: []int{0, 1, 2, 3, 4, 5, 6}}
		good, unsafe, clarified := scoreScope(s, answers, policy)
		if !good || unsafe || clarified {
			t.Fatal("clear unrestricted request blocked", policy)
		}
		for id := range qs {
			for _, p := range []float64{.5, .95} {
				answers[id] = decide.Answer{Kind: decide.KindNoul, Noul: p}
				good, unsafe, clarified = scoreScope(sample{Clarify: true}, answers, policy)
				if !good || unsafe || !clarified {
					t.Fatal("unsafe guard bypass", policy, id, p)
				}
			}
			delete(answers, id)
			if scopeClear(answers, policy) {
				t.Fatal("missing guard accepted", policy, id)
			}
			answers[id] = decide.Answer{Kind: decide.KindNoul, Noul: .05}
		}
	}
}
