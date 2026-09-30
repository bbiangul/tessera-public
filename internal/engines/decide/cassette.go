package decide

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bbiangul/tessera/internal/cassette"
)

// CassetteMode selects how a Cassette behaves.
type CassetteMode = cassette.Mode

const (
	// CassetteReplay answers only from recordings; a request that was never
	// recorded fails with ErrCassetteMiss. This is the CI mode: free,
	// deterministic, and every prompt change shows up as a failing test.
	CassetteReplay = cassette.Replay
	// CassetteRecord calls the inner backend and stores every exchange.
	CassetteRecord = cassette.Record
)

// ErrCassetteMiss means a request is not in the cassette. The error names the
// questions and the key but never the state, which may hold customer data;
// Describe prints the question text for review.
var ErrCassetteMiss = cassette.ErrMiss

// Cassette wraps a Decider with record/replay (RFC-0002, level 2). Requests
// are keyed by the target (backend + model) and the canonical JSON of the
// request.
type Cassette struct {
	Inner  Decider // required in record mode; unused in replay
	Target string  // e.g. "typesafe:jev-1.13.0"; part of the key
	Mode   CassetteMode
	c      *cassette.Cassette[*Response]
}

// Describe summarizes a request without its state: question ids, kinds and
// instructions. Instructions are written by authors, not customers.
func Describe(req Request) string {
	ids := make([]string, 0, len(req.Questions))
	for id := range req.Questions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parts := make([]string, len(ids))
	for i, id := range ids {
		q := req.Questions[id]
		parts[i] = fmt.Sprintf("%s (%s): %q", id, q.Kind, q.Instructions)
	}
	return "questions " + strings.Join(parts, "; ")
}

// LoadCassette reads a cassette file. A missing file is an empty cassette.
func LoadCassette(path, target string, inner Decider, mode CassetteMode) (*Cassette, error) {
	c, err := cassette.Load[*Response](path, target, mode)
	if err != nil {
		return nil, err
	}
	return &Cassette{Inner: inner, Target: target, Mode: mode, c: c}, nil
}

func (c *Cassette) Name() string {
	if c.Inner != nil {
		return c.Inner.Name()
	}
	return "cassette"
}

// Key returns the cassette key for a request and its canonical JSON.
func (c *Cassette) Key(req Request) (string, []byte, error) { return c.c.Key(req) }

func (c *Cassette) Decide(ctx context.Context, req Request) (*Response, error) {
	key, canon, err := c.c.Key(req)
	if err != nil {
		return nil, err
	}
	switch c.Mode {
	case CassetteReplay:
		resp, ok := c.c.Replay(key)
		if !ok {
			return nil, fmt.Errorf("%w (target %s, key %s): %s; if the change is intended, re-record (bench/README.md)",
				ErrCassetteMiss, c.Target, key, Describe(req))
		}
		cp := *resp
		return &cp, nil
	case CassetteRecord:
		if c.Inner == nil {
			return nil, errors.New("cassette: record mode needs an inner backend")
		}
		resp, err := c.Inner.Decide(ctx, req)
		if err != nil {
			return resp, err // failures are not recorded; replay must not fake errors
		}
		c.c.Put(key, canon, resp)
		return resp, nil
	}
	return nil, fmt.Errorf("cassette: unknown mode %q", c.Mode)
}

// Save writes this target's recordings to path, keeping every line that
// belongs to other targets, so several targets can share one cassette.
func (c *Cassette) Save(path string) error { return c.c.Save(path) }

// ResetReplay restarts replay from the first recording of every key.
func (c *Cassette) ResetReplay() { c.c.ResetReplay() }

// SetScope selects the explicit EvalSuite scenario partition between calls.
func (c *Cassette) SetScope(scope string) { c.c.SetScope(scope) }
