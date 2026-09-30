// Package bench measures Decide backends against labelled datasets.
//
// The rule it enforces: every backend receives the exact same question text.
// Differences in results must come from the models, not from prompt effort.
package bench

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/bbiangul/tessera/internal/engines/decide"
)

// Case is one labelled decision.
type Case struct {
	ID       string          `json:"id"`
	Task     string          `json:"task"`
	State    any             `json:"state"`
	Question decide.Question `json:"question"`
	// Gold is the correct outcome key: an option key (choice), "true"/"false"
	// (noul) or a level index as a string (score).
	Gold string `json:"gold"`
}

// LoadDataset reads a JSONL dataset and validates every case.
func LoadDataset(path string) ([]Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadDataset(f)
}

// ReadDataset parses JSONL from r.
func ReadDataset(r io.Reader) ([]Case, error) {
	var cases []Case
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var c Case
		dec := json.NewDecoder(bytesReader(sc.Bytes()))
		dec.DisallowUnknownFields() // loud failure beats a silently ignored field
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if err := c.validate(); err != nil {
			return nil, fmt.Errorf("line %d (%s): %w", line, c.ID, err)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("line %d: duplicate id %q", line, c.ID)
		}
		seen[c.ID] = true
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("dataset is empty")
	}
	return cases, nil
}

func (c Case) validate() error {
	if c.ID == "" || c.Task == "" {
		return fmt.Errorf("id and task are required")
	}
	req := decide.Request{State: c.State, Questions: map[string]decide.Question{"q": c.Question}}
	if err := req.Validate(); err != nil {
		return err
	}
	if !slices.Contains(c.Question.Outcomes(), c.Gold) {
		return fmt.Errorf("gold %q is not an outcome of the question %v", c.Gold, c.Question.Outcomes())
	}
	return nil
}
