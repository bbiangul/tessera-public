// Package decide implements the Decide engine: typed decisions with
// probabilities (RFC-0001, "Os quatro motores").
//
// A Request carries a state and a set of named questions. Each question is one
// of three kinds — Choice (one option from a closed set), Noul (yes/no, as the
// probability of yes) or Score (ordered levels). Every backend returns the same
// Answer shape, so callers never depend on which model answered.
//
// Probabilities are reported as the backend produced them. Whether they are
// calibrated is NOT something a backend can claim: Answer.Calibrated is only
// set from a measured calibration profile (see internal/bench). Until such a
// profile exists for a backend and model, it stays false.
package decide

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// Kind is the type of a question.
type Kind string

const (
	KindChoice Kind = "choice"
	KindNoul   Kind = "noul"
	KindScore  Kind = "score"
)

// Limits taken from the TypeSafe API, which are also sensible for any backend.
const (
	MaxChoiceOptions = 255
	MaxScoreLevels   = 10
)

// Option is one option of a Choice question. Options are ordered: the order
// is part of the question and backends must preserve it.
type Option struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
}

// Question is a single typed question.
type Question struct {
	Kind         Kind   `json:"kind"`
	Instructions string `json:"instructions"`

	// Options is required for KindChoice.
	Options []Option `json:"options,omitempty"`
	// IfTrue and IfFalse optionally describe what yes and no mean (KindNoul).
	IfTrue  string `json:"ifTrue,omitempty"`
	IfFalse string `json:"ifFalse,omitempty"`
	// Levels is required for KindScore, ordered from lowest to highest.
	Levels []string `json:"levels,omitempty"`
}

// Request is a set of named questions about one state.
type Request struct {
	// State is the content the questions are about. It must already be
	// filtered to what the questions need and masked for personal data.
	State     any                 `json:"state"`
	Questions map[string]Question `json:"questions"`
}

// ProbSource says where an answer's probabilities came from.
type ProbSource string

const (
	// ProbNative: the backend returns a distribution by design (TypeSafe).
	ProbNative ProbSource = "native"
	// ProbLogprobs: derived from token log-probabilities of an LLM.
	ProbLogprobs ProbSource = "logprobs"
	// ProbPoint: no distribution available; the chosen answer gets 1.0.
	ProbPoint ProbSource = "point"
	// ProbVote: share of votes over several sampled answers.
	ProbVote ProbSource = "vote"
	// ProbVerbal: a probability the model states in its reply.
	ProbVerbal ProbSource = "verbal"
)

// Answer is the typed answer to one question.
type Answer struct {
	Kind Kind `json:"kind"`

	// Choice is the most probable option key (KindChoice).
	Choice string `json:"choice,omitempty"`
	// Noul is the probability that the answer is yes (KindNoul).
	Noul float64 `json:"noul,omitempty"`
	// Score is the probability-weighted level index (KindScore).
	Score float64 `json:"score,omitempty"`

	// Probabilities maps option keys (choice), "true"/"false" (noul) or level
	// indexes as strings (score) to probabilities that sum to ~1.
	Probabilities map[string]float64 `json:"probabilities"`
	// Confidence is 1 minus the normalized entropy of Probabilities: 1 when
	// all mass is on one outcome, 0 when it is uniform.
	Confidence float64    `json:"confidence"`
	Source     ProbSource `json:"source"`
	// Calibrated is set only from a measured calibration profile.
	Calibrated bool `json:"calibrated"`
}

// Usage reports what a call consumed.
type Usage struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CostUSD      float64 `json:"costUSD"`
	// UnreportedCalls counts attempts that may have been billed without a
	// usage report; the ledger charges each at the estimate (#7).
	UnreportedCalls int `json:"unreportedCalls,omitempty"`
}

// Response is the result of a Decide call.
type Response struct {
	Answers map[string]Answer `json:"answers"`
	Backend string            `json:"backend"`
	// Model is the versioned model that answered, as reported by the backend.
	Model string `json:"model"`
	// Provider is the upstream serving provider, when the backend is a router.
	Provider string        `json:"provider,omitempty"`
	Usage    Usage         `json:"usage"`
	Latency  time.Duration `json:"latency"`
}

// Decider is the Decide engine contract. Implementations must be safe for
// concurrent use. When a call fails after the provider billed it (a malformed
// answer, a reply that is not a label), Decide returns the error together
// with a Response carrying the Usage, so the cost is still accounted.
type Decider interface {
	Decide(ctx context.Context, req Request) (*Response, error)
	// Name identifies the backend, e.g. "typesafe" or "llm-constrained".
	Name() string
}

// ErrInvalidRequest wraps every validation failure.
var ErrInvalidRequest = errors.New("invalid decide request")

// Validate checks a request against the rules shared by all backends.
func (r Request) Validate() error {
	if len(r.Questions) == 0 {
		return fmt.Errorf("%w: no questions", ErrInvalidRequest)
	}
	for id, q := range r.Questions {
		if id == "" {
			return fmt.Errorf("%w: empty question id", ErrInvalidRequest)
		}
		if err := q.validate(); err != nil {
			return fmt.Errorf("%w: question %q: %v", ErrInvalidRequest, id, err)
		}
	}
	return nil
}

func (q Question) validate() error {
	if q.Instructions == "" {
		return errors.New("instructions are required")
	}
	switch q.Kind {
	case KindChoice:
		if len(q.Options) < 2 || len(q.Options) > MaxChoiceOptions {
			return fmt.Errorf("choice needs 2..%d options, got %d", MaxChoiceOptions, len(q.Options))
		}
		seen := make(map[string]bool, len(q.Options))
		for _, o := range q.Options {
			if o.Key == "" {
				return errors.New("option key is empty")
			}
			if seen[o.Key] {
				return fmt.Errorf("duplicate option key %q", o.Key)
			}
			seen[o.Key] = true
		}
	case KindNoul:
		if len(q.Options) > 0 || len(q.Levels) > 0 {
			return errors.New("noul takes no options or levels")
		}
	case KindScore:
		if len(q.Levels) < 2 || len(q.Levels) > MaxScoreLevels {
			return fmt.Errorf("score needs 2..%d levels, got %d", MaxScoreLevels, len(q.Levels))
		}
	default:
		return fmt.Errorf("unknown kind %q", q.Kind)
	}
	return nil
}

// Outcomes returns the ordered outcome keys a question's Probabilities use.
func (q Question) Outcomes() []string {
	switch q.Kind {
	case KindChoice:
		keys := make([]string, len(q.Options))
		for i, o := range q.Options {
			keys[i] = o.Key
		}
		return keys
	case KindNoul:
		return []string{"true", "false"}
	case KindScore:
		keys := make([]string, len(q.Levels))
		for i := range q.Levels {
			keys[i] = fmt.Sprint(i)
		}
		return keys
	}
	return nil
}

// Confidence returns 1 - H(p)/log(n) over the question's outcomes. Outcomes
// missing from probs count as zero.
func Confidence(probs map[string]float64, outcomes []string) float64 {
	n := len(outcomes)
	if n < 2 {
		return 1
	}
	var h float64
	for _, k := range outcomes {
		if p := probs[k]; p > 0 {
			h -= p * math.Log(p)
		}
	}
	c := 1 - h/math.Log(float64(n))
	return math.Max(0, math.Min(1, c))
}

// answerFromProbs builds an Answer for q from a distribution over its outcomes.
func answerFromProbs(q Question, probs map[string]float64, src ProbSource) Answer {
	outcomes := q.Outcomes()
	a := Answer{Kind: q.Kind, Probabilities: probs, Source: src, Confidence: Confidence(probs, outcomes)}
	switch q.Kind {
	case KindChoice:
		best, bestP := outcomes[0], -1.0
		for _, k := range outcomes { // ordered iteration makes ties deterministic
			if probs[k] > bestP {
				best, bestP = k, probs[k]
			}
		}
		a.Choice = best
	case KindNoul:
		a.Noul = probs["true"]
	case KindScore:
		for i, k := range outcomes {
			a.Score += float64(i) * probs[k]
		}
	}
	return a
}

// normalize rescales probs over outcomes to sum to 1. It reports false when
// there is no mass on any outcome.
func normalize(probs map[string]float64, outcomes []string) (map[string]float64, bool) {
	var total float64
	for _, k := range outcomes {
		total += probs[k]
	}
	if total <= 0 {
		return nil, false
	}
	out := make(map[string]float64, len(outcomes))
	for _, k := range outcomes {
		out[k] = probs[k] / total
	}
	return out, true
}

// AnswerFor builds a validated Answer for q from a distribution over its
// outcomes. It is used by test doubles and replay; backends use the same
// logic internally.
func AnswerFor(q Question, probs map[string]float64, src ProbSource) (Answer, error) {
	for k := range probs {
		found := false
		for _, o := range q.Outcomes() {
			if o == k {
				found = true
				break
			}
		}
		if !found {
			return Answer{}, fmt.Errorf("outcome %q is not one of %v", k, q.Outcomes())
		}
	}
	norm, ok := normalize(probs, q.Outcomes())
	if !ok {
		return Answer{}, errors.New("no probability mass on the question's outcomes")
	}
	return answerFromProbs(q, norm, src), nil
}
