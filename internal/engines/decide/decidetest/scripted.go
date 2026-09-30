// Package decidetest provides a scripted Decider for unit tests of code that
// depends on the Decide engine (RFC-0002, level 1). It never calls a model.
package decidetest

import (
	"context"
	"fmt"
	"sync"

	"github.com/bbiangul/tessera/internal/engines/decide"
)

// Scripted answers each question id with a fixed distribution and records
// every request it receives.
type Scripted struct {
	// Answers maps question id to probabilities over the question's outcomes,
	// e.g. {"confirmed": {"true": .92, "false": .08}}.
	Answers map[string]map[string]float64
	// Err, when set, is returned for every call.
	Err error

	mu    sync.Mutex
	calls []decide.Request
}

func (s *Scripted) Name() string { return "scripted" }

// Decide returns the scripted answers. A question without a script is an
// error, so tests notice when code asks something unexpected.
func (s *Scripted) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	s.mu.Lock()
	s.calls = append(s.calls, req)
	s.mu.Unlock()
	if s.Err != nil {
		return nil, s.Err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	resp := &decide.Response{Answers: map[string]decide.Answer{}, Backend: s.Name(), Model: "scripted"}
	for id, q := range req.Questions {
		probs, ok := s.Answers[id]
		if !ok {
			return nil, fmt.Errorf("decidetest: no scripted answer for question %q", id)
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, fmt.Errorf("decidetest: question %q: %w", id, err)
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

// Calls returns the requests received so far.
func (s *Scripted) Calls() []decide.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]decide.Request(nil), s.calls...)
}
