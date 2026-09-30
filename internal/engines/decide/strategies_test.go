package decide

import (
	"context"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestVoteCountsSamples(t *testing.T) {
	replies := []string{"A", "A", "B", "A", "nonsense"}
	var i atomic.Int32
	var mu sync.Mutex
	var sawTemp float64
	srv := fakeServer(t, func(body map[string]any) (int, any) {
		mu.Lock()
		sawTemp, _ = body["temperature"].(float64)
		mu.Unlock()
		if _, has := body["logprobs"]; has {
			t.Error("vote must not request logprobs")
		}
		n := int(i.Add(1)) - 1
		return 200, chatReply(replies[n%len(replies)], nil)
	})
	l := &LLMConstrained{APIKey: "k", BaseURL: srv.URL, Strategy: StrategyVote, Samples: 5, SampleTemperature: 0.7}
	resp, err := l.Decide(context.Background(), Request{Questions: map[string]Question{"q": choiceQ("x", "y")}})
	if err != nil {
		t.Fatal(err)
	}
	a := resp.Answers["q"]
	// 3 votes x, 1 vote y, 1 abstention: probabilities over the 4 valid votes.
	if a.Source != ProbVote || a.Choice != "x" || math.Abs(a.Probabilities["x"]-0.75) > 1e-9 {
		t.Errorf("answer = %+v", a)
	}
	mu.Lock()
	defer mu.Unlock()
	if sawTemp != 0.7 || i.Load() != 5 {
		t.Errorf("temperature %v, calls %d", sawTemp, i.Load())
	}
	if math.Abs(resp.Usage.CostUSD-5*0.00002) > 1e-12 {
		t.Errorf("cost %v should sum all samples", resp.Usage.CostUSD)
	}
}

func TestVoteFailsWhenAllSamplesFail(t *testing.T) {
	srv := fakeServer(t, func(map[string]any) (int, any) { return 200, chatReply("hmm", nil) })
	l := &LLMConstrained{APIKey: "k", BaseURL: srv.URL, Strategy: StrategyVote, Samples: 3}
	if _, err := l.Decide(context.Background(), Request{Questions: map[string]Question{"q": choiceQ("x", "y")}}); err == nil {
		t.Fatal("want error")
	}
}

func TestVerbal(t *testing.T) {
	cases := map[string]struct {
		reply  string
		choice string
		p      float64
		ok     bool
	}{
		"plain":        {"B 0.85", "y", 0.85, true},
		"comma":        {"B 0,8", "y", 0.8, true},
		"punctuated":   {"**A** (0.6)", "x", 0.6, true},
		"one":          {"C 1", "z", 1, true},
		"no prob":      {"B", "", 0, false},
		"out of range": {"A 1.7", "", 0, false},
		"bad label":    {"Q 0.5", "", 0, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fakeServer(t, func(body map[string]any) (int, any) {
				msgs := body["messages"].([]any)
				if !strings.Contains(msgs[0].(map[string]any)["content"].(string), "probability") {
					t.Error("verbal system prompt not used")
				}
				return 200, chatReply(c.reply, nil)
			})
			l := &LLMConstrained{APIKey: "k", BaseURL: srv.URL, Strategy: StrategyVerbal}
			resp, err := l.Decide(context.Background(), Request{Questions: map[string]Question{"q": choiceQ("x", "y", "z")}})
			if !c.ok {
				if err == nil {
					t.Fatalf("want error for %q", c.reply)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			a := resp.Answers["q"]
			if a.Source != ProbVerbal || a.Choice != c.choice || math.Abs(a.Probabilities[c.choice]-c.p) > 1e-9 {
				t.Errorf("answer = %+v", a)
			}
			var sum float64
			for _, p := range a.Probabilities {
				sum += p
			}
			if math.Abs(sum-1) > 1e-9 {
				t.Errorf("probabilities sum to %v", sum)
			}
		})
	}
}

func TestUnknownStrategy(t *testing.T) {
	srv := fakeServer(t, func(map[string]any) (int, any) { return 200, chatReply("A", nil) })
	l := &LLMConstrained{APIKey: "k", BaseURL: srv.URL, Strategy: "magic"}
	if _, err := l.Decide(context.Background(), Request{Questions: map[string]Question{"q": choiceQ("x", "y")}}); err == nil {
		t.Fatal("want error")
	}
}
