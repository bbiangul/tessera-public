package decide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/tessera/internal/httpx"
)

// LLMConstrained is the Decide backend for any OpenAI-compatible chat model
// (OpenRouter by default). Each question becomes one call in which the model
// must answer with a single option label ("A", "B", ...). The distribution
// over options is read from the first token's log-probabilities.
//
// When the provider returns no logprobs, or the top tokens put little mass on
// the labels, the answer falls back to the parsed label with probability 1 and
// Source = ProbPoint. A reply that is not a label is an error, never a guess.
type LLMConstrained struct {
	APIKey  string
	Model   string // e.g. "deepseek/deepseek-v4.1-flash"
	BaseURL string // default https://openrouter.ai/api/v1

	// Strategy picks how probabilities are obtained: StrategyLogprobs
	// (default), StrategyVote or StrategyVerbal. See llmstrategies.go.
	Strategy string
	// Samples and SampleTemperature configure StrategyVote (defaults 5, 0.8).
	Samples           int
	SampleTemperature float64

	// KeepReasoning leaves the model's reasoning on. Off by default: reasoning
	// models otherwise spend the output budget thinking and emit no label.
	KeepReasoning bool
	// RequireLogprobs routes only to providers that support logprobs
	// (OpenRouter provider.require_parameters).
	RequireLogprobs bool
	// Providers pins the upstream providers, in order, with no fallback to
	// others (OpenRouter provider.order + allow_fallbacks=false). The same
	// model served by different providers gives different logprobs.
	Providers   []string
	TopLogprobs int // default 20
	Concurrency int // questions answered in parallel; default 8
	Client      *httpx.Client
}

// minLabelMass is the share of first-token probability that must fall on
// valid labels for the logprob distribution to be trusted. Below it the mass
// is renormalized from too little evidence, which inflates confidence, so
// the answer falls back to a point estimate instead.
const minLabelMass = 0.8

// maxLetterLabels: beyond 26 options, labels are numbers, which may span
// several tokens, so the first-token distribution is not meaningful.
const maxLetterLabels = 26

func (l *LLMConstrained) Name() string { return "llm-constrained" }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Choices  []struct {
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
	} `json:"choices"`
	Usage struct {
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		Cost             float64 `json:"cost"`
	} `json:"usage"`
}

// labelSet maps option labels to outcome keys for one question.
type labelSet struct {
	labels   []string          // in option order
	toKey    map[string]string // label -> outcome key
	lines    []string          // rendered "A) ..." lines
	letterOK bool              // first-token logprobs are meaningful
}

func labelsFor(q Question) labelSet {
	outcomes := q.Outcomes()
	var descs []string
	switch q.Kind {
	case KindChoice:
		for _, o := range q.Options {
			descs = append(descs, describe(o.Key, o.Description))
		}
	case KindNoul:
		descs = []string{describe("yes", q.IfTrue), describe("no", q.IfFalse)}
	case KindScore:
		descs = q.Levels
	}
	ls := labelSet{toKey: map[string]string{}, letterOK: len(outcomes) <= maxLetterLabels}
	for i, key := range outcomes {
		label := fmt.Sprint(i + 1)
		if ls.letterOK {
			label = string(rune('A' + i))
		}
		ls.labels = append(ls.labels, label)
		ls.toKey[label] = key
		ls.lines = append(ls.lines, label+") "+descs[i])
	}
	return ls
}

func describe(key, desc string) string {
	if desc == "" {
		return key
	}
	return key + " — " + desc
}

const systemPrompt = "You are a decision function inside a software system. " +
	"Read the STATE and answer the QUESTION. " +
	"Reply with exactly one label from OPTIONS and nothing else."

// RenderPrompt returns the user message for one question. It is exported so
// benchmarks can show exactly what every model saw.
func RenderPrompt(state any, q Question) (string, error) {
	st, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal state: %w", err)
	}
	ls := labelsFor(q)
	var b strings.Builder
	b.WriteString("STATE:\n")
	b.Write(st)
	b.WriteString("\n\nQUESTION:\n")
	b.WriteString(q.Instructions)
	b.WriteString("\n\nOPTIONS:\n")
	b.WriteString(strings.Join(ls.lines, "\n"))
	b.WriteString("\n\nANSWER (one label only):")
	return b.String(), nil
}

// Decide asks each question in its own call, in parallel.
func (l *LLMConstrained) Decide(ctx context.Context, req Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if l.APIKey == "" {
		return nil, errors.New("llm-constrained: API key is empty")
	}
	conc := l.Concurrency
	if conc <= 0 {
		conc = 8
	}

	type result struct {
		id       string
		answer   Answer
		model    string
		provider string
		usage    Usage
		err      error
	}
	start := time.Now()
	results := make(chan result, len(req.Questions))
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for id, q := range req.Questions {
		wg.Add(1)
		go func(id string, q Question) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			a, model, provider, u, err := l.ask(ctx, req.State, q)
			results <- result{id, a, model, provider, u, err}
		}(id, q)
	}
	wg.Wait()
	close(results)

	resp := &Response{Answers: make(map[string]Answer, len(req.Questions)), Backend: l.Name()}
	var errs []error
	for r := range results {
		resp.Usage.InputTokens += r.usage.InputTokens
		resp.Usage.OutputTokens += r.usage.OutputTokens
		resp.Usage.CostUSD += r.usage.CostUSD
		resp.Usage.UnreportedCalls += r.usage.UnreportedCalls
		if r.err != nil {
			errs = append(errs, fmt.Errorf("question %q: %w", r.id, r.err))
			continue
		}
		resp.Answers[r.id] = r.answer
		resp.Model, resp.Provider = r.model, r.provider
	}
	resp.Latency = time.Since(start)
	if len(errs) > 0 {
		return resp, fmt.Errorf("llm-constrained: %w", errors.Join(errs...)) // resp carries the billed usage
	}
	return resp, nil
}

func (l *LLMConstrained) ask(ctx context.Context, state any, q Question) (Answer, string, string, Usage, error) {
	prompt, err := RenderPrompt(state, q)
	if err != nil {
		return Answer{}, "", "", Usage{}, err
	}
	switch l.Strategy {
	case "", StrategyLogprobs:
		out, usage, err := l.call(ctx, systemPrompt, prompt, 0, 2, true)
		if err != nil {
			return Answer{}, "", "", usage, err
		}
		a, err := parseChoice(q, out)
		if err != nil {
			err = fmt.Errorf("provider %s: %w", out.Provider, err)
		}
		return a, out.Model, out.Provider, usage, err
	case StrategyVote:
		return l.askVote(ctx, prompt, q)
	case StrategyVerbal:
		return l.askVerbal(ctx, prompt, q)
	}
	return Answer{}, "", "", Usage{}, fmt.Errorf("unknown strategy %q", l.Strategy)
}

// call sends one chat completion.
func (l *LLMConstrained) call(ctx context.Context, system, user string, temperature float64, maxTokens int, logprobs bool) (chatResponse, Usage, error) {
	base := l.BaseURL
	if base == "" {
		base = "https://openrouter.ai/api/v1"
	}
	body := map[string]any{
		"model":       l.Model,
		"temperature": temperature,
		"max_tokens":  maxTokens,
		"usage":       map[string]any{"include": true},
		"messages": []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}
	if logprobs {
		top := l.TopLogprobs
		if top <= 0 {
			top = 20
		}
		body["logprobs"] = true
		body["top_logprobs"] = top
	}
	if !l.KeepReasoning {
		body["reasoning"] = map[string]any{"enabled": false}
	}
	if (l.RequireLogprobs && logprobs) || len(l.Providers) > 0 {
		prov := map[string]any{}
		if l.RequireLogprobs && logprobs {
			prov["require_parameters"] = true
		}
		if len(l.Providers) > 0 {
			prov["order"] = l.Providers
			prov["allow_fallbacks"] = false
		}
		body["provider"] = prov
	}
	client := l.Client
	if client == nil {
		client = &httpx.Client{}
	}
	var out chatResponse
	st, err := client.PostJSON(ctx, base+"/chat/completions",
		map[string]string{"Authorization": "Bearer " + l.APIKey}, body, &out)
	if err != nil {
		return out, Usage{}, err
	}
	return out, Usage{InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens, CostUSD: out.Usage.Cost, UnreportedCalls: st.MaybeBilled}, nil
}

// parseChoice turns a chat response into an Answer for q.
func parseChoice(q Question, out chatResponse) (Answer, error) {
	if len(out.Choices) == 0 {
		return Answer{}, errors.New("no choices in response")
	}
	ls := labelsFor(q)
	c := out.Choices[0]
	text := ""
	if c.Message.Content != nil {
		text = *c.Message.Content
	}
	label := cleanLabel(text)
	key, ok := ls.toKey[label]
	if !ok {
		return Answer{}, fmt.Errorf("reply %q is not one of the labels %v", truncate(text, 40), ls.labels)
	}

	if ls.letterOK && c.Logprobs != nil && len(c.Logprobs.Content) > 0 {
		mass := map[string]float64{}
		var onLabels float64
		for _, t := range c.Logprobs.Content[0].TopLogprobs {
			if k, ok := ls.toKey[cleanLabel(t.Token)]; ok {
				p := math.Exp(t.Logprob)
				mass[k] += p
				onLabels += p
			}
		}
		if onLabels >= minLabelMass {
			probs, _ := normalize(mass, q.Outcomes())
			return answerFromProbs(q, probs, ProbLogprobs), nil
		}
	}
	probs := map[string]float64{}
	for _, k := range q.Outcomes() {
		probs[k] = 0
	}
	probs[key] = 1
	return answerFromProbs(q, probs, ProbPoint), nil
}

// cleanLabel normalizes "A", " A", "a", "A)", "A." and "**A**" to "A".
func cleanLabel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "*`'\" ")
	s = strings.TrimRight(s, ").:")
	return strings.ToUpper(strings.TrimSpace(s))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
