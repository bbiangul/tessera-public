// Package cassette records model calls and replays them (RFC-0002, level
// 2): free, offline and deterministic tests, where a changed prompt shows up
// as a miss instead of a silent new answer.
//
// A cassette is a JSONL file; each line holds a target (backend and model),
// an optional scenario scope, a key (hash of target, scope and canonical request), the request and the
// response. Several targets, and several engines, can share one file.
package cassette

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
)

// Mode selects how a cassette behaves.
type Mode string

const (
	// Replay answers only from recordings; anything else is a miss.
	Replay Mode = "replay"
	// Record calls the real backend and stores every exchange.
	Record Mode = "record"
)

// ErrMiss means a request was never recorded.
var ErrMiss = errors.New("cassette miss: request not recorded")

// Entry is one line of a cassette file.
type Entry[R any] struct {
	Key      string          `json:"key"`
	Target   string          `json:"target"`
	Scope    string          `json:"scope,omitempty"`
	Request  json.RawMessage `json:"request"`
	Response R               `json:"response"`
}

// Cassette holds one target's recordings.
type Cassette[R any] struct {
	Target string
	Mode   Mode

	mu     sync.Mutex
	scope  string
	scoped bool // this target contains scoped recordings; disable legacy fallback
	// hits counts replays per key, so repeated identical requests replay
	// the n-th recording made for them.
	hits  map[string]int
	multi map[string][]Entry[R]
	fresh map[string]bool // keys recorded in this session
}

// Load reads a cassette file; a missing file is an empty cassette. Lines of
// other targets are ignored (and kept by Save).
func Load[R any](path, target string, mode Mode) (*Cassette[R], error) {
	if mode != Replay && mode != Record {
		return nil, fmt.Errorf("cassette: unknown mode %q", mode)
	}
	c := &Cassette[R]{Target: target, Mode: mode, hits: map[string]int{}, multi: map[string][]Entry[R]{}, fresh: map[string]bool{}}
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	for i, raw := range lines {
		var head struct {
			Target string `json:"target"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			return nil, fmt.Errorf("cassette %s line %d: %w", path, i+1, err)
		}
		if head.Target != target {
			continue
		}
		var e Entry[R]
		// Numbers in free-form values stay json.Number, so an int
		// recorded is an int replayed (the backends decode the same way).
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err := d.Decode(&e); err != nil {
			return nil, fmt.Errorf("cassette %s line %d: %w", path, i+1, err)
		}
		c.scoped = c.scoped || e.Scope != ""
		c.multi[e.Key] = append(c.multi[e.Key], e)
	}
	return c, nil
}

func readLines(path string) ([]json.RawMessage, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []json.RawMessage
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		out = append(out, append(json.RawMessage(nil), sc.Bytes()...))
	}
	return out, sc.Err()
}

// Key returns the target/scope/request key and canonical request JSON.
// Scope never changes the request returned here or sent to the provider.
func (c *Cassette[R]) Key(req any) (string, []byte, error) {
	canon, err := json.Marshal(req)
	if err != nil {
		return "", nil, err
	}
	c.mu.Lock()
	scope := c.scope
	if c.Mode == Replay && !c.scoped {
		scope = ""
	}
	c.mu.Unlock()
	prefix := c.Target + "\n"
	if scope != "" {
		encoded, err := json.Marshal(scope)
		if err != nil {
			return "", nil, err
		}
		prefix += string(encoded) + "\n"
	}
	sum := sha256.Sum256(append([]byte(prefix), canon...))
	return hex.EncodeToString(sum[:16]), canon, nil
}

// Replay returns the next recording for a key. Asking more often than
// the recording did is a miss, never a repeat of an old answer: a
// regression that calls the model more must show up.
func (c *Cassette[R]) Replay(key string) (R, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	recs := c.multi[key]
	n := c.hits[key]
	var zero R
	if n >= len(recs) {
		return zero, false
	}
	c.hits[key]++
	return recs[n].Response, true
}

// SetScope selects an explicit recording/replay partition and resets replay
// positions. Call it between scenarios, never while calls are in flight.
// Wholly legacy targets retain their unscoped replay behavior; a scoped
// target never falls back to recordings belonging to another partition.
func (c *Cassette[R]) SetScope(scope string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scope = scope
	c.hits = map[string]int{}
}

// ResetReplay starts replaying every key from its first recording again,
// e.g. at the start of each scenario, so scenarios do not depend on the
// order they run in.
func (c *Cassette[R]) ResetReplay() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits = map[string]int{}
}

// Put records an exchange. Recording replaces what the file held for the
// key; it never mixes old and new answers.
func (c *Cassette[R]) Put(key string, canon []byte, resp R) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scoped = c.scoped || c.scope != ""
	if !c.fresh[key] {
		c.fresh[key] = true
		c.multi[key] = nil
	}
	c.multi[key] = append(c.multi[key], Entry[R]{Key: key, Target: c.Target, Scope: c.scope, Request: canon, Response: resp})
}

// Save writes this target's recordings to path, keeping every line of
// other targets as it was. Lines are sorted by target and key so the file
// diffs cleanly; the write is atomic.
func (c *Cassette[R]) Save(path string) error {
	existing, err := readLines(path)
	if err != nil {
		return err
	}
	type line struct {
		target, key string
		raw         json.RawMessage
	}
	var all []line
	for i, raw := range existing {
		var head struct {
			Target string `json:"target"`
			Key    string `json:"key"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			return fmt.Errorf("cassette %s line %d: %w", path, i+1, err)
		}
		if head.Target != c.Target {
			all = append(all, line{head.Target, head.Key, raw})
		}
	}
	c.mu.Lock()
	for _, recs := range c.multi {
		for _, e := range recs {
			raw, err := json.Marshal(e)
			if err != nil {
				c.mu.Unlock()
				return err
			}
			all = append(all, line{e.Target, e.Key, raw})
		}
	}
	c.mu.Unlock()
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].target != all[j].target {
			return all[i].target < all[j].target
		}
		return all[i].key < all[j].key
	})
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, l := range all {
		if _, err := w.Write(append(l.raw, '\n')); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
