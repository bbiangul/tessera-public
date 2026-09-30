package decide

import (
	"errors"
	"math"
	"testing"
)

func choiceQ(keys ...string) Question {
	q := Question{Kind: KindChoice, Instructions: "pick one"}
	for _, k := range keys {
		q.Options = append(q.Options, Option{Key: k})
	}
	return q
}

func TestValidate(t *testing.T) {
	tooMany := make([]string, MaxChoiceOptions+1)
	for i := range tooMany {
		tooMany[i] = string(rune(0x100 + i))
	}
	cases := []struct {
		name string
		req  Request
		ok   bool
	}{
		{"no questions", Request{}, false},
		{"valid choice", Request{Questions: map[string]Question{"q": choiceQ("a", "b")}}, true},
		{"one option", Request{Questions: map[string]Question{"q": choiceQ("a")}}, false},
		{"too many options", Request{Questions: map[string]Question{"q": choiceQ(tooMany...)}}, false},
		{"duplicate key", Request{Questions: map[string]Question{"q": choiceQ("a", "a")}}, false},
		{"empty key", Request{Questions: map[string]Question{"q": choiceQ("a", "")}}, false},
		{"no instructions", Request{Questions: map[string]Question{"q": {Kind: KindNoul}}}, false},
		{"valid noul", Request{Questions: map[string]Question{"q": {Kind: KindNoul, Instructions: "x?"}}}, true},
		{"noul with options", Request{Questions: map[string]Question{"q": {Kind: KindNoul, Instructions: "x?", Options: []Option{{Key: "a"}}}}}, false},
		{"score one level", Request{Questions: map[string]Question{"q": {Kind: KindScore, Instructions: "x", Levels: []string{"a"}}}}, false},
		{"valid score", Request{Questions: map[string]Question{"q": {Kind: KindScore, Instructions: "x", Levels: []string{"a", "b", "c"}}}}, true},
		{"unknown kind", Request{Questions: map[string]Question{"q": {Kind: "bool", Instructions: "x"}}}, false},
		{"empty id", Request{Questions: map[string]Question{"": choiceQ("a", "b")}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.req.Validate()
			if c.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !c.ok && !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("want ErrInvalidRequest, got %v", err)
			}
		})
	}
}

func TestConfidence(t *testing.T) {
	outs := []string{"a", "b", "c", "d"}
	if got := Confidence(map[string]float64{"a": 1}, outs); math.Abs(got-1) > 1e-9 {
		t.Errorf("certain: got %v, want 1", got)
	}
	if got := Confidence(map[string]float64{"a": .25, "b": .25, "c": .25, "d": .25}, outs); math.Abs(got) > 1e-9 {
		t.Errorf("uniform: got %v, want 0", got)
	}
	mid := Confidence(map[string]float64{"a": .7, "b": .3}, outs)
	if mid <= 0 || mid >= 1 {
		t.Errorf("partial: got %v, want in (0,1)", mid)
	}
}

func TestAnswerFromProbs(t *testing.T) {
	q := choiceQ("x", "y", "z")
	a := answerFromProbs(q, map[string]float64{"x": .2, "y": .5, "z": .3}, ProbNative)
	if a.Choice != "y" {
		t.Errorf("choice = %q, want y", a.Choice)
	}
	// Ties resolve to the earliest option, so results are deterministic.
	tie := answerFromProbs(q, map[string]float64{"x": .5, "y": .5}, ProbNative)
	if tie.Choice != "x" {
		t.Errorf("tie choice = %q, want x", tie.Choice)
	}

	s := Question{Kind: KindScore, Instructions: "s", Levels: []string{"low", "mid", "high"}}
	sa := answerFromProbs(s, map[string]float64{"0": 0, "1": .5, "2": .5}, ProbNative)
	if math.Abs(sa.Score-1.5) > 1e-9 {
		t.Errorf("score = %v, want 1.5", sa.Score)
	}

	n := Question{Kind: KindNoul, Instructions: "n"}
	na := answerFromProbs(n, map[string]float64{"true": .8, "false": .2}, ProbNative)
	if na.Noul != .8 {
		t.Errorf("noul = %v, want .8", na.Noul)
	}
	if a.Calibrated || na.Calibrated {
		t.Error("answers must never start as calibrated")
	}
}

func TestNormalize(t *testing.T) {
	p, ok := normalize(map[string]float64{"a": 2, "b": 2, "junk": 10}, []string{"a", "b"})
	if !ok || p["a"] != .5 || p["b"] != .5 || len(p) != 2 {
		t.Fatalf("got %v %v", p, ok)
	}
	if _, ok := normalize(map[string]float64{"junk": 1}, []string{"a"}); ok {
		t.Fatal("want !ok when no mass on outcomes")
	}
}
