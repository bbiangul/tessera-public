package decide

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression tests for the adversarial review of 2026-09-23.

type fixed struct{ p float64 }

func (f fixed) Name() string { return "fixed" }
func (f fixed) Decide(_ context.Context, req Request) (*Response, error) {
	r := &Response{Answers: map[string]Answer{}, Model: "m"}
	for id, q := range req.Questions {
		a, err := AnswerFor(q, map[string]float64{"true": f.p, "false": 1 - f.p}, ProbNative)
		if err != nil {
			return nil, err
		}
		r.Answers[id] = a
	}
	return r, nil
}

var cpfReq = Request{
	State:     map[string]string{"cpf": "123.456.789-00"},
	Questions: map[string]Question{"ok": {Kind: KindNoul, Instructions: "Is the document valid?"}},
}

func TestCassetteMissCarriesNoState(t *testing.T) {
	c, _ := LoadCassette(filepath.Join(t.TempDir(), "none.jsonl"), "t", nil, CassetteReplay)
	_, err := c.Decide(context.Background(), cpfReq)
	if !errors.Is(err, ErrCassetteMiss) || strings.Contains(err.Error(), "123.456") {
		t.Errorf("the miss must not print the state: %v", err)
	}
	if !strings.Contains(err.Error(), "Is the document valid?") {
		t.Errorf("the miss must name the question: %v", err)
	}
}

func TestCassetteKeepsOtherTargetsAndReplacesOnRerecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	record := func(target string, p float64) {
		c, err := LoadCassette(path, target, fixed{p}, CassetteRecord)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Decide(context.Background(), cpfReq); err != nil {
			t.Fatal(err)
		}
		if err := c.Save(path); err != nil {
			t.Fatal(err)
		}
	}
	record("A", 0.9)
	record("B", 0.1)
	record("A", 0.7) // re-record A: must replace, not accumulate
	data, _ := os.ReadFile(path)
	if n := strings.Count(string(data), "\n"); n != 2 {
		t.Errorf("want 2 lines (one per target), got %d:\n%s", n, data)
	}
	for target, want := range map[string]float64{"A": 0.7, "B": 0.1} {
		c, _ := LoadCassette(path, target, nil, CassetteReplay)
		r, err := c.Decide(context.Background(), cpfReq)
		if err != nil || r.Answers["ok"].Noul != want {
			t.Errorf("%s: got %+v %v, want %v", target, r, err, want)
		}
	}
}

func TestVerbalEdgeCases(t *testing.T) {
	q := choiceQ("x", "y", "z")
	cases := map[string]bool{ // reply -> accepted
		"a 0.9":              true,  // lowercase label
		"A 0.3":              false, // below 1/3 would change the answer
		"A 0.9 wait, B 0.99": false, // trailing text
		"A: 85%":             false,
		"B 0,95":             true,
	}
	for reply, ok := range cases {
		content := reply
		_, err := parseVerbal(q, chatResponse{Choices: []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
			Logprobs *struct {
				Content []struct {
					Token       string `json:"token"`
					TopLogprobs []struct {
						Token   string  `json:"token"`
						Logprob float64 `json:"logprob"`
					} `json:"top_logprobs"`
				} `json:"content"`
			} `json:"logprobs"`
		}{{Message: struct {
			Content *string `json:"content"`
		}{Content: &content}}}})
		if (err == nil) != ok {
			t.Errorf("%q: accepted=%v, want %v (err %v)", reply, err == nil, ok, err)
		}
	}
}

func TestLowLabelMassFallsBackToPoint(t *testing.T) {
	srv := fakeServer(t, func(map[string]any) (int, any) {
		return 200, chatReply("A", map[string]float64{"A": .6, "Hmm": .4}) // 60% < 80%
	})
	l := &LLMConstrained{APIKey: "k", BaseURL: srv.URL}
	resp, err := l.Decide(context.Background(), Request{Questions: map[string]Question{"q": choiceQ("x", "y")}})
	if err != nil {
		t.Fatal(err)
	}
	if a := resp.Answers["q"]; a.Source != ProbPoint {
		t.Errorf("60%% label mass must not be renormalized into a distribution: %+v", a)
	}
	if cleanLabel(" a") != "A" {
		t.Error("lowercase labels must match")
	}
}

func TestBilledFailuresReturnUsage(t *testing.T) {
	srv := fakeServer(t, func(map[string]any) (int, any) { return 200, chatReply("not a label", nil) })
	l := &LLMConstrained{APIKey: "k", BaseURL: srv.URL}
	resp, err := l.Decide(context.Background(), Request{Questions: map[string]Question{"q": choiceQ("x", "y")}})
	if err == nil || resp == nil || resp.Usage.CostUSD == 0 {
		t.Errorf("a billed failure must return its usage: resp=%+v err=%v", resp, err)
	}
}
