package generate

import (
	"context"
	"errors"
	"fmt"

	"github.com/bbiangul/tessera/internal/cassette"
)

// Cassette wraps a Generator with record/replay (RFC-0002, level 2), keyed
// by the target and the canonical request.
type Cassette struct {
	Inner  Generator // required in record mode
	Target string    // e.g. "openrouter:deepseek/deepseek-v4.1-flash"
	Mode   cassette.Mode
	c      *cassette.Cassette[*Response]
}

// LoadCassette reads a cassette file; a missing file is empty.
func LoadCassette(path, target string, inner Generator, mode cassette.Mode) (*Cassette, error) {
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

func (c *Cassette) Generate(ctx context.Context, req Request) (*Response, error) {
	key, canon, err := c.c.Key(req)
	if err != nil {
		return nil, err
	}
	switch c.Mode {
	case cassette.Replay:
		resp, ok := c.c.Replay(key)
		if !ok {
			// The state may hold customer data; the instructions do not.
			return nil, fmt.Errorf("%w (target %s, key %s): instructions %q; if the change is intended, re-record",
				cassette.ErrMiss, c.Target, key, truncate(req.Instructions, 120))
		}
		cp := *resp
		cp.Value = plain(cp.Value) // json.Number, as a live reply is parsed
		return &cp, nil
	case cassette.Record:
		if c.Inner == nil {
			return nil, errors.New("cassette: record mode needs an inner backend")
		}
		resp, err := c.Inner.Generate(ctx, req)
		if err != nil {
			return resp, err
		}
		c.c.Put(key, canon, resp)
		return resp, nil
	}
	return nil, fmt.Errorf("cassette: unknown mode %q", c.Mode)
}

// Save writes this target's recordings, keeping other targets' lines.
func (c *Cassette) Save(path string) error { return c.c.Save(path) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ResetReplay restarts replay from the first recording of every key.
func (c *Cassette) ResetReplay() { c.c.ResetReplay() }

// SetScope selects the explicit EvalSuite scenario partition between calls.
func (c *Cassette) SetScope(scope string) { c.c.SetScope(scope) }
