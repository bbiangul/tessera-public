//go:build integration

package decide

import (
	"context"
	"os"
	"testing"
	"time"
)

// These tests call real providers. Run with: make test-integration.
// Each one skips when its API key is missing, so CI without secrets stays green.

var confirmQ = Request{
	State: map[string]string{"assistant_asked": "Confirma sexta às 14h?", "user_reply": "pode ser"},
	Questions: map[string]Question{
		"confirmed": {Kind: KindNoul, Instructions: "Did `user_reply` agree to the proposed time?"},
		"intent": {Kind: KindChoice, Instructions: "What is the user doing?", Options: []Option{
			{Key: "confirming", Description: "Agreeing to the proposal"},
			{Key: "refusing", Description: "Declining the proposal"},
			{Key: "asking", Description: "Asking a question"},
		}},
	},
}

func checkAnswers(t *testing.T, resp *Response) {
	t.Helper()
	if len(resp.Answers) != 2 {
		t.Fatalf("answers = %d, want 2", len(resp.Answers))
	}
	for id, a := range resp.Answers {
		var sum float64
		for _, p := range a.Probabilities {
			if p < 0 || p > 1 {
				t.Errorf("%s: probability %v out of range", id, p)
			}
			sum += p
		}
		if sum < 0.98 || sum > 1.02 {
			t.Errorf("%s: probabilities sum to %v", id, sum)
		}
		if a.Calibrated {
			t.Errorf("%s: must not claim calibration without a measured profile", id)
		}
	}
	if resp.Answers["confirmed"].Noul < 0.5 || resp.Answers["intent"].Choice != "confirming" {
		t.Errorf("unexpected answers: %+v", resp.Answers)
	}
	if resp.Model == "" {
		t.Error("model not reported")
	}
}

func TestIntegrationTypeSafe(t *testing.T) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Skip("TYPESAFE_API_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := (&TypeSafe{APIKey: key, Model: "jev-latest"}).Decide(ctx, confirmQ)
	if err != nil {
		t.Fatal(err)
	}
	checkAnswers(t, resp)
	for id, a := range resp.Answers {
		if a.Source != ProbNative {
			t.Errorf("%s: source %s", id, a.Source)
		}
	}
}

func TestIntegrationDeepSeekV41FlashPinned(t *testing.T) {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	l := &LLMConstrained{APIKey: key, Model: "deepseek/deepseek-v4.1-flash", Providers: []string{"DeepSeek"}, RequireLogprobs: true}
	resp, err := l.Decide(ctx, confirmQ)
	if err != nil {
		t.Fatal(err)
	}
	checkAnswers(t, resp)
	for id, a := range resp.Answers {
		if a.Source != ProbLogprobs {
			t.Errorf("%s: source %s, want logprobs from the pinned provider", id, a.Source)
		}
	}
	if resp.Provider != "DeepSeek" {
		t.Errorf("provider = %q, want DeepSeek", resp.Provider)
	}
}
