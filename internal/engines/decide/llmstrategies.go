package decide

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Strategies for getting probabilities out of a chat LLM. Which one works is
// an empirical question; internal/bench measures them against each other.
const (
	// StrategyLogprobs reads the first-token distribution over labels. One
	// call per question; needs a provider that returns logprobs.
	StrategyLogprobs = "logprobs"
	// StrategyVote samples the answer several times at a non-zero temperature
	// and uses vote shares as probabilities (self-consistency). Costs N calls.
	StrategyVote = "vote"
	// StrategyVerbal asks the model to state its own probability.
	StrategyVerbal = "verbal"
)

// askVote asks the same question Samples times and counts labels. Samples
// whose reply is not a label count as abstentions; if every sample abstains
// the question fails.
func (l *LLMConstrained) askVote(ctx context.Context, prompt string, q Question) (Answer, string, string, Usage, error) {
	n, temp := l.Samples, l.SampleTemperature
	if n <= 0 {
		n = 5
	}
	if temp <= 0 {
		temp = 0.8
	}
	ls := labelsFor(q)
	type sample struct {
		key            string
		model, provide string
		usage          Usage
		err            error
	}
	samples := make([]sample, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, u, err := l.call(ctx, systemPrompt, prompt, temp, 2, false)
			s := sample{usage: u, err: err, model: out.Model, provide: out.Provider}
			if err == nil {
				s.key, s.err = labelKey(ls, out)
			}
			samples[i] = s
		}(i)
	}
	wg.Wait()

	counts := map[string]float64{}
	var usage Usage
	var model, provider string
	var valid int
	var errs []error
	for _, s := range samples {
		usage.InputTokens += s.usage.InputTokens
		usage.OutputTokens += s.usage.OutputTokens
		usage.CostUSD += s.usage.CostUSD
		if s.err != nil {
			errs = append(errs, s.err)
			continue
		}
		counts[s.key]++
		valid++
		model, provider = s.model, s.provide
	}
	if valid == 0 {
		return Answer{}, model, provider, usage, fmt.Errorf("all %d samples failed: %w", n, errors.Join(errs...))
	}
	probs, _ := normalize(counts, q.Outcomes())
	return answerFromProbs(q, probs, ProbVote), model, provider, usage, nil
}

const verbalSystemPrompt = "You are a decision function inside a software system. " +
	"Read the STATE and answer the QUESTION. " +
	"Reply with exactly one label from OPTIONS, a space, and the probability (0 to 1) " +
	"that your label is correct. Example reply: B 0.85"

// verbalRe accepts exactly "LABEL PROBABILITY", with light punctuation, and
// nothing after it: "A 0.9 wait, B 0.99" is refused, not half-read.
var verbalRe = regexp.MustCompile(`^\W*([A-Za-z]|\d{1,3})\W+([01](?:[.,]\d+)?)\W*$`)

// askVerbal asks for "LABEL PROBABILITY" and spreads the remaining mass
// evenly over the other outcomes.
func (l *LLMConstrained) askVerbal(ctx context.Context, prompt string, q Question) (Answer, string, string, Usage, error) {
	out, usage, err := l.call(ctx, verbalSystemPrompt, prompt, 0, 8, false)
	if err != nil {
		return Answer{}, "", "", usage, err
	}
	a, err := parseVerbal(q, out)
	if err != nil {
		err = fmt.Errorf("provider %s: %w", out.Provider, err)
	}
	return a, out.Model, out.Provider, usage, err
}

func parseVerbal(q Question, out chatResponse) (Answer, error) {
	text := replyText(out)
	m := verbalRe.FindStringSubmatch(text)
	if m == nil {
		return Answer{}, fmt.Errorf("reply %q is not \"LABEL PROBABILITY\"", truncate(text, 40))
	}
	ls := labelsFor(q)
	key, ok := ls.toKey[strings.ToUpper(m[1])]
	if !ok {
		return Answer{}, fmt.Errorf("label %q is not one of %v", m[1], ls.labels)
	}
	p, err := strconv.ParseFloat(stringsReplaceComma(m[2]), 64)
	if err != nil || p < 0 || p > 1 {
		return Answer{}, fmt.Errorf("probability %q out of [0,1]", m[2])
	}
	outcomes := q.Outcomes()
	// The stated label is the answer. A probability below the uniform share
	// would make another option more likely than the one the model chose:
	// that reply contradicts itself and is refused.
	if p < 1/float64(len(outcomes)) {
		return Answer{}, fmt.Errorf("stated probability %v is below 1/%d for the chosen label", p, len(outcomes))
	}
	probs := make(map[string]float64, len(outcomes))
	rest := (1 - p) / float64(len(outcomes)-1)
	for _, k := range outcomes {
		probs[k] = rest
	}
	probs[key] = p
	return answerFromProbs(q, probs, ProbVerbal), nil
}

// labelKey parses a reply that must be exactly one label.
func labelKey(ls labelSet, out chatResponse) (string, error) {
	text := replyText(out)
	key, ok := ls.toKey[cleanLabel(text)]
	if !ok {
		return "", fmt.Errorf("reply %q is not one of the labels %v", truncate(text, 40), ls.labels)
	}
	return key, nil
}

func replyText(out chatResponse) string {
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == nil {
		return ""
	}
	return *out.Choices[0].Message.Content
}

func stringsReplaceComma(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] == ',' {
			b[i] = '.'
		}
	}
	return string(b)
}
