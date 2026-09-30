package main

import (
	"github.com/bbiangul/tessera/internal/engines/decide"
	"reflect"
	"testing"
)

func operandAnswers(t *testing.T, policy, base string, roles map[string]string) map[string]decide.Answer {
	t.Helper()
	qs := structuredQuestions(policy)
	if err := (decide.Request{State: "synthetic", Questions: qs}).Validate(); err != nil {
		t.Fatal(err)
	}
	out := map[string]decide.Answer{}
	for id, q := range qs {
		key := "false"
		if id == "base" {
			key = base
		} else if policy == "G" && id != "scope" {
			key = "none"
			if role, ok := roles[id]; ok {
				key = role
			}
		} else if policy == "H" && id != "scope" {
			for day, role := range roles {
				if id == role+day {
					key = "true"
				}
			}
		}
		a, err := decide.AnswerFor(q, map[string]float64{key: 1}, decide.ProbNative)
		if err != nil {
			t.Fatal(err)
		}
		out[id] = a
	}
	return out
}

func TestStructuredSetAlgebra(t *testing.T) {
	for _, policy := range []string{"G", "H"} {
		for _, tc := range []struct {
			name, base string
			roles      map[string]string
			want       []int
		}{
			{"weekend minus Sunday", "weekend", map[string]string{"0": "exclude"}, []int{6}},
			{"any except Saturday", "all", map[string]string{"6": "exclude"}, []int{0, 1, 2, 3, 4, 5}},
			{"weekdays plus Saturday", "weekdays", map[string]string{"6": "include"}, []int{1, 2, 3, 4, 5, 6}},
			{"named range minus day", "explicit", map[string]string{"1": "include", "2": "exclude", "3": "include"}, []int{1, 3}},
			{"corrected preference operands", "explicit", map[string]string{"5": "include"}, []int{5}},
		} {
			t.Run(policy+"/"+tc.name, func(t *testing.T) {
				out, known := structuredMask(operandAnswers(t, policy, tc.base, tc.roles), policy)
				if !known || !reflect.DeepEqual(out, tc.want) {
					t.Fatal(out, known)
				}
			})
		}
	}
}

func TestStructuredUnknownOperandsNeverGrantDays(t *testing.T) {
	for _, policy := range []string{"G", "H"} {
		qs := structuredQuestions(policy)
		for id, q := range qs {
			answers := operandAnswers(t, policy, "all", nil)
			delete(answers, id)
			if _, known := structuredMask(answers, policy); known {
				t.Fatal("missing operand accepted", policy, id)
			}
			probabilities := map[string]float64{}
			for _, k := range q.Outcomes() {
				probabilities[k] = 1 / float64(len(q.Outcomes()))
			}
			uncertain, err := decide.AnswerFor(q, probabilities, decide.ProbNative)
			if err != nil {
				t.Fatal(err)
			}
			answers[id] = uncertain
			if _, known := structuredMask(answers, policy); known {
				t.Fatal("uncertain operand accepted", policy, id)
			}
		}
	}
	answers := operandAnswers(t, "H", "weekend", map[string]string{"6": "include"})
	answers["exclude6"] = decide.Answer{Kind: decide.KindNoul, Noul: 1}
	if _, known := structuredMask(answers, "H"); known {
		t.Fatal("contradictory operands accepted")
	}
}
