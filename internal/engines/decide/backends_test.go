package decide

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bbiangul/tessera/internal/httpx"
)

// lastRawBody holds the raw JSON of the most recent request, for tests that
// check key order (which decoding into a Go map would lose).
var lastRawBody atomic.Value

func fakeServer(t *testing.T, handler func(body map[string]any) (int, any)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		lastRawBody.Store(string(raw))
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("bad request json: %v", err)
		}
		status, resp := handler(body)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTypeSafeDecide(t *testing.T) {
	srv := fakeServer(t, func(map[string]any) (int, any) {
		return 200, map[string]any{
			"model": "jev-1.13.0",
			"answers": map[string]any{
				"dept":   map[string]any{"type": "choice", "choice": "billing", "confidence": .8, "probabilities": map[string]float64{"billing": .9, "tech": .1}},
				"urgent": map[string]any{"type": "noul", "noul": .95},
				"mood":   map[string]any{"type": "score", "score": 1.05, "probabilities": map[string]float64{"0": 0, "1": .95, "2": .05}},
			},
			"usage": map[string]int{"input_tokens": 1000, "output_tokens": 20},
		}
	})
	ts := &TypeSafe{APIKey: "k", BaseURL: srv.URL, Model: "jev-1.13.0"}
	resp, err := ts.Decide(context.Background(), Request{
		State: "payouts failing",
		Questions: map[string]Question{
			"dept":   {Kind: KindChoice, Instructions: "team?", Options: []Option{{Key: "tech", Description: "bugs"}, {Key: "billing"}}},
			"urgent": {Kind: KindNoul, Instructions: "urgent?"},
			"mood":   {Kind: KindScore, Instructions: "mood", Levels: []string{"calm", "annoyed", "angry"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if raw := lastRawBody.Load().(string); !strings.Contains(raw, `"criteria":{"tech":"bugs","billing":null}`) {
		t.Errorf("criteria order/shape not preserved: %s", raw)
	}
	if got := resp.Answers["dept"].Choice; got != "billing" {
		t.Errorf("dept = %q", got)
	}
	if got := resp.Answers["urgent"]; got.Noul != .95 || math.Abs(got.Probabilities["false"]-.05) > 1e-9 {
		t.Errorf("urgent = %+v", got)
	}
	if got := resp.Answers["mood"].Score; got != 1.05 {
		t.Errorf("mood = %v", got)
	}
	if resp.Usage.CostUSD != 1000*typesafeDefaultPrice || resp.Model != "jev-1.13.0" {
		t.Errorf("usage/model = %+v %q", resp.Usage, resp.Model)
	}
	for _, a := range resp.Answers {
		if a.Source != ProbNative || a.Calibrated {
			t.Errorf("source=%s calibrated=%v", a.Source, a.Calibrated)
		}
	}
}

func TestTypeSafeRejectsMalformedAnswers(t *testing.T) {
	cases := map[string]map[string]any{
		"wrong type":   {"type": "noul", "noul": .5},
		"no mass":      {"type": "choice", "choice": "zzz", "probabilities": map[string]float64{"zzz": 1}},
		"choice lies":  {"type": "choice", "choice": "a", "probabilities": map[string]float64{"a": .1, "b": .9}},
		"missing noul": {"type": "choice"},
	}
	for name, ans := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fakeServer(t, func(map[string]any) (int, any) {
				return 200, map[string]any{"model": "m", "answers": map[string]any{"q": ans}}
			})
			ts := &TypeSafe{APIKey: "k", BaseURL: srv.URL}
			_, err := ts.Decide(context.Background(), Request{Questions: map[string]Question{"q": choiceQ("a", "b")}})
			if err == nil {
				t.Fatal("want error for malformed answer")
			}
		})
	}
}

func TestTypeSafeRetriesOverload(t *testing.T) {
	var calls atomic.Int32
	srv := fakeServer(t, func(map[string]any) (int, any) {
		if calls.Add(1) == 1 {
			return 529, map[string]string{"error": "overloaded"}
		}
		return 200, map[string]any{"model": "m", "answers": map[string]any{"q": map[string]any{"type": "noul", "noul": .1}}}
	})
	ts := &TypeSafe{APIKey: "k", BaseURL: srv.URL, Client: &httpx.Client{BaseBackoff: 1}}
	if _, err := ts.Decide(context.Background(), Request{Questions: map[string]Question{"q": {Kind: KindNoul, Instructions: "x?"}}}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

// chatReply builds an OpenAI-compatible response with first-token logprobs.
func chatReply(content string, top map[string]float64) map[string]any {
	var lps []map[string]any
	for tok, p := range top {
		lps = append(lps, map[string]any{"token": tok, "logprob": math.Log(p)})
	}
	choice := map[string]any{"message": map[string]any{"content": content}}
	if top != nil {
		choice["logprobs"] = map[string]any{"content": []any{map[string]any{"token": content, "top_logprobs": lps}}}
	}
	return map[string]any{
		"model": "deepseek/deepseek-v4.1-flash", "provider": "Fake",
		"choices": []any{choice},
		"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 1, "cost": 0.00002},
	}
}

func TestLLMConstrainedLogprobs(t *testing.T) {
	var sawReasoningOff, sawPrompt bool
	srv := fakeServer(t, func(body map[string]any) (int, any) {
		r, _ := body["reasoning"].(map[string]any)
		sawReasoningOff = r["enabled"] == false
		msgs := body["messages"].([]any)
		user := msgs[1].(map[string]any)["content"].(string)
		sawPrompt = strings.Contains(user, "A) yes") && strings.Contains(user, "B) no")
		return 200, chatReply("A", map[string]float64{"A": .7, " B": .2, "Let": .1})
	})
	l := &LLMConstrained{APIKey: "k", BaseURL: srv.URL, Model: "deepseek/deepseek-v4.1-flash"}
	resp, err := l.Decide(context.Background(), Request{State: map[string]string{"msg": "pode ser"},
		Questions: map[string]Question{"confirmed": {Kind: KindNoul, Instructions: "Did the user confirm?"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !sawReasoningOff || !sawPrompt {
		t.Errorf("reasoningOff=%v prompt=%v", sawReasoningOff, sawPrompt)
	}
	a := resp.Answers["confirmed"]
	// "Let" is not a label; mass is renormalized over A and B: .7/.9.
	if a.Source != ProbLogprobs || math.Abs(a.Noul-.7/.9) > 1e-9 {
		t.Errorf("answer = %+v", a)
	}
	if resp.Provider != "Fake" || resp.Usage.CostUSD != 0.00002 {
		t.Errorf("provider/usage = %q %+v", resp.Provider, resp.Usage)
	}
}

func TestLLMConstrainedFallsBackToPoint(t *testing.T) {
	cases := map[string]map[string]float64{
		"no logprobs":         nil,
		"little label mass":   {"B": .3, "The": .7},
		"logprobs other only": {"Hmm": 1},
	}
	for name, top := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fakeServer(t, func(map[string]any) (int, any) { return 200, chatReply("B)", top) })
			l := &LLMConstrained{APIKey: "k", BaseURL: srv.URL}
			resp, err := l.Decide(context.Background(), Request{Questions: map[string]Question{"q": choiceQ("x", "y")}})
			if err != nil {
				t.Fatal(err)
			}
			a := resp.Answers["q"]
			if a.Source != ProbPoint || a.Choice != "y" || a.Probabilities["y"] != 1 {
				t.Errorf("answer = %+v", a)
			}
		})
	}
}

func TestLLMConstrainedRejectsNonLabel(t *testing.T) {
	srv := fakeServer(t, func(map[string]any) (int, any) { return 200, chatReply("I think yes", nil) })
	l := &LLMConstrained{APIKey: "k", BaseURL: srv.URL}
	_, err := l.Decide(context.Background(), Request{Questions: map[string]Question{"q": choiceQ("x", "y")}})
	if err == nil || !strings.Contains(err.Error(), "not one of the labels") {
		t.Fatalf("want label error, got %v", err)
	}
}

func TestLLMConstrainedPinsProviders(t *testing.T) {
	var prov map[string]any
	srv := fakeServer(t, func(body map[string]any) (int, any) {
		prov, _ = body["provider"].(map[string]any)
		return 200, chatReply("A", map[string]float64{"A": 1})
	})
	l := &LLMConstrained{APIKey: "k", BaseURL: srv.URL, Providers: []string{"DeepSeek"}, RequireLogprobs: true}
	if _, err := l.Decide(context.Background(), Request{Questions: map[string]Question{"q": choiceQ("x", "y")}}); err != nil {
		t.Fatal(err)
	}
	order, _ := prov["order"].([]any)
	if len(order) != 1 || order[0] != "DeepSeek" || prov["allow_fallbacks"] != false || prov["require_parameters"] != true {
		t.Errorf("provider = %v", prov)
	}
}

func TestCleanLabel(t *testing.T) {
	for in, want := range map[string]string{"A": "A", " A": "A", "A)": "A", "A.": "A", "**A**": "A", "`B`": "B", "C:": "C"} {
		if got := cleanLabel(in); got != want {
			t.Errorf("cleanLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLabelsBeyondLetters(t *testing.T) {
	keys := make([]string, 30)
	for i := range keys {
		keys[i] = string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	ls := labelsFor(choiceQ(keys...))
	if ls.letterOK || ls.labels[29] != "30" {
		t.Errorf("letterOK=%v last=%q", ls.letterOK, ls.labels[29])
	}
}
