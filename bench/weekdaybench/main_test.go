package main

import (
	"github.com/bbiangul/tessera/internal/engines/decide"
	"testing"
)

func TestWeekdayScoring(t *testing.T) {
	q := questions()
	if len(q) != 8 {
		t.Fatal(len(q))
	}
	if e := (decide.Request{State: "test", Questions: q}).Validate(); e != nil {
		t.Fatal(e)
	}
	a := map[string]decide.Answer{}
	for k := range q {
		v := "false"
		if k == "6" || k == "explicit" {
			v = "true"
		}
		n := .1
		if v == "true" {
			n = .9
		}
		a[k] = decide.Answer{Kind: decide.KindNoul, Noul: n, Probabilities: map[string]float64{v: .9}}
	}
	s := sample{Allowed: []int{6}}
	if ok, bad := score(s, a); !ok || bad {
		t.Fatal(ok, bad)
	}
	a["1"] = decide.Answer{Kind: decide.KindNoul, Noul: .9, Probabilities: map[string]float64{"true": .9}}
	if ok, bad := score(s, a); ok || !bad {
		t.Fatal(ok, bad)
	}
	delete(a, "1")
	if ok, _ := score(s, a); ok {
		t.Fatal("missing accepted")
	}
}
