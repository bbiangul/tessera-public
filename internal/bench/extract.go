package bench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/bbiangul/tessera/internal/engines/generate"
)

// Extraction benchmarks (Generate engine with a schema). A dataset is a task
// file (instructions + JSON Schema) and a JSONL file of cases with gold
// values; a gold value {"$any": [...]} accepts any listed value. Nested
// objects require the same keys and compare each value recursively.

// ExtractTask is what every case asks.
type ExtractTask struct {
	Instructions string         `json:"instructions"`
	Schema       map[string]any `json:"schema"`
}

// ExtractCase is one labelled input.
type ExtractCase struct {
	ID    string         `json:"id"`
	State map[string]any `json:"state"`
	Gold  map[string]any `json:"gold"`
}

// LoadExtract reads a task file and its cases.
func LoadExtract(taskPath, casesPath string) (ExtractTask, []ExtractCase, error) {
	var task ExtractTask
	data, err := os.ReadFile(taskPath)
	if err != nil {
		return task, nil, err
	}
	if err := json.Unmarshal(data, &task); err != nil {
		return task, nil, fmt.Errorf("%s: %w", taskPath, err)
	}
	f, err := os.Open(casesPath)
	if err != nil {
		return task, nil, err
	}
	defer f.Close()
	var cases []ExtractCase
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for n := 1; sc.Scan(); n++ {
		var c ExtractCase
		d := json.NewDecoder(strings.NewReader(sc.Text()))
		d.DisallowUnknownFields()
		if err := d.Decode(&c); err != nil {
			return task, nil, fmt.Errorf("%s:%d: %w", casesPath, n, err)
		}
		if c.ID == "" || seen[c.ID] {
			return task, nil, fmt.Errorf("%s:%d: missing or duplicate id %q", casesPath, n, c.ID)
		}
		seen[c.ID] = true
		cases = append(cases, c)
	}
	return task, cases, sc.Err()
}

// GenTarget is a Generate backend under test.
type GenTarget struct {
	Label     string
	Generator generate.Generator
}

// ExtractTrial is one case on one target.
type ExtractTrial struct {
	Target   string         `json:"target"`
	CaseID   string         `json:"caseId"`
	Repeat   int            `json:"repeat"`
	Value    map[string]any `json:"value,omitempty"`
	Wrong    []string       `json:"wrong,omitempty"` // fields that differ from gold
	Err      string         `json:"err,omitempty"`
	Attempts int            `json:"attempts"`
	Model    string         `json:"model,omitempty"`
	Provider string         `json:"provider,omitempty"`
	Latency  time.Duration  `json:"latency"`
	CostUSD  float64        `json:"costUSD"`
}

// RunExtract runs every case on every target.
func RunExtract(ctx context.Context, task ExtractTask, cases []ExtractCase, targets []GenTarget, repeats, concurrency int) []ExtractTrial {
	if repeats < 1 {
		repeats = 1
	}
	if concurrency < 1 {
		concurrency = 4
	}
	type job struct {
		t GenTarget
		c ExtractCase
		r int
	}
	var jobs []job
	for _, t := range targets {
		for r := range repeats {
			for _, c := range cases {
				jobs = append(jobs, job{t, c, r})
			}
		}
	}
	out := make([]ExtractTrial, len(jobs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			tr := ExtractTrial{Target: j.t.Label, CaseID: j.c.ID, Repeat: j.r}
			resp, err := j.t.Generator.Generate(ctx, generate.Request{Instructions: task.Instructions, State: j.c.State, Schema: task.Schema})
			if resp != nil {
				tr.Attempts, tr.Model, tr.Provider, tr.Latency, tr.CostUSD = resp.Attempts, resp.Model, resp.Provider, resp.Latency, resp.Usage.CostUSD
			}
			if err != nil {
				tr.Err = err.Error()
			} else {
				tr.Value, _ = resp.Value.(map[string]any)
				tr.Wrong = CompareFields(j.c.Gold, tr.Value)
			}
			out[i] = tr
		}()
	}
	wg.Wait()
	return out
}

// CompareFields returns the gold fields the value gets wrong, sorted.
func CompareFields(gold, got map[string]any) []string {
	var wrong []string
	for k, g := range gold {
		if !goldMatches(g, got[k]) {
			wrong = append(wrong, k)
		}
	}
	sort.Strings(wrong)
	return wrong
}

func goldMatches(gold, got any) bool {
	if m, ok := gold.(map[string]any); ok {
		if alts, ok := m["$any"].([]any); ok {
			for _, a := range alts {
				if goldMatches(a, got) {
					return true
				}
			}
			return false
		}
		actual, ok := got.(map[string]any)
		if !ok || len(actual) != len(m) {
			return false
		}
		for key, expected := range m {
			value, present := actual[key]
			if !present || !goldMatches(expected, value) {
				return false
			}
		}
		return true
	}
	switch g := gold.(type) {
	case nil:
		return got == nil
	case string:
		s, ok := got.(string)
		return ok && fold(s) == fold(g)
	case float64: // JSON numbers from the dataset
		switch v := got.(type) {
		case int64:
			return float64(v) == g
		case float64:
			return v == g
		}
		return false
	case bool:
		return got == g
	case []any: // compared as multisets: order is not information here
		l, ok := got.([]any)
		if !ok || len(l) != len(g) {
			return false
		}
		key := func(xs []any) []string {
			out := make([]string, len(xs))
			for i, x := range xs {
				out[i] = fmt.Sprint(normalizeNumber(x))
			}
			sort.Strings(out)
			return out
		}
		a, b := key(g), key(l)
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	return false
}

func normalizeNumber(x any) any {
	if i, ok := x.(int64); ok {
		return float64(i)
	}
	return x
}

// fold compares place names the way a person would: case, accents and
// surrounding space do not matter.
func fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	return strings.ToLower(strings.TrimSpace(out))
}

// ExtractSummary aggregates one target.
type ExtractSummary struct {
	Target       string             `json:"target"`
	Trials       int                `json:"trials"`
	Errors       int                `json:"errors"` // no valid object
	Repaired     int                `json:"repaired"`
	RecordAcc    float64            `json:"recordAccuracy"` // every field right, over all trials
	RecordLo     float64            `json:"recordLo"`
	RecordHi     float64            `json:"recordHi"`
	FieldAcc     map[string]float64 `json:"fieldAccuracy"`
	LatencyP50   time.Duration      `json:"latencyP50"`
	LatencyP95   time.Duration      `json:"latencyP95"`
	CostPerCase  float64            `json:"costPerCase"`
	WrongByCase  map[string]int     `json:"wrongByCase"`
	Models       []string           `json:"models"`
	FailingCases []string           `json:"failingCases"`
}

// SummarizeExtract computes a target's summary from its trials.
func SummarizeExtract(target string, ts []ExtractTrial, gold map[string]map[string]any) ExtractSummary {
	s := ExtractSummary{Target: target, FieldAcc: map[string]float64{}, WrongByCase: map[string]int{}}
	fieldRight := map[string]int{}
	var lat []time.Duration
	var cost float64
	right := 0
	models := map[string]bool{}
	for _, t := range ts {
		if t.Target != target {
			continue
		}
		s.Trials++
		cost += t.CostUSD
		lat = append(lat, t.Latency)
		if t.Attempts > 1 {
			s.Repaired++
		}
		if t.Model != "" {
			models[t.Model+"@"+t.Provider] = true
		}
		if t.Err != "" {
			s.Errors++
			s.WrongByCase[t.CaseID]++
			continue
		}
		wrong := map[string]bool{}
		for _, f := range t.Wrong {
			wrong[f] = true
		}
		for f := range gold[t.CaseID] {
			if !wrong[f] {
				fieldRight[f]++
			}
		}
		if len(t.Wrong) == 0 {
			right++
		} else {
			s.WrongByCase[t.CaseID]++
		}
	}
	if s.Trials == 0 {
		return s
	}
	s.RecordAcc = float64(right) / float64(s.Trials)
	ci := Wilson(right, s.Trials, 1.96)
	s.RecordLo, s.RecordHi = ci[0], ci[1]
	for f, n := range fieldRight {
		s.FieldAcc[f] = float64(n) / float64(s.Trials)
	}
	for _, g := range gold {
		for f := range g {
			if _, ok := s.FieldAcc[f]; !ok {
				s.FieldAcc[f] = 0
			}
		}
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	s.LatencyP50, s.LatencyP95 = durationPercentile(lat, 0.5), durationPercentile(lat, 0.95)
	s.CostPerCase = cost / float64(s.Trials)
	for m := range models {
		s.Models = append(s.Models, m)
	}
	sort.Strings(s.Models)
	for id := range s.WrongByCase {
		s.FailingCases = append(s.FailingCases, id)
	}
	sort.Strings(s.FailingCases)
	return s
}

func durationPercentile(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	pos := q * float64(len(sorted)-1)
	lo := int(pos)
	if lo+1 >= len(sorted) {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo] + time.Duration(frac*float64(sorted[lo+1]-sorted[lo]))
}
