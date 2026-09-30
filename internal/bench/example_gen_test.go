package bench

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
)

// The example benchmark's recording and baseline are made by a scripted,
// deterministic decider: no network, no key. It is right on most cases and
// wrong on two, so the gate has something to catch. Regenerate with
// GEN_EXAMPLE_BENCH=1 go test ./internal/bench -run TestGenerateExampleBench.

const (
	exampleTarget   = "example:scripted"
	exampleDataset  = "../../bench/datasets/travel-decisions-v1.jsonl"
	exampleCassette = "../../bench/cassettes/example-travel-v1.jsonl"
	exampleBaseline = "../../bench/baselines/example-travel-v1/trials.jsonl"
)

// scripted answers from the case's own words, as a fixed table.
type scripted struct{}

func (scripted) Name() string { return "example" }
func (scripted) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	b, _ := json.Marshal(req.State)
	s := string(b)
	out := &decide.Response{Answers: map[string]decide.Answer{}, Backend: "example", Model: "scripted"}
	for id, q := range req.Questions {
		pick := map[string]string{
			"obrigado": "false", "a mala": "true", "até mais": "false",
			"sim, pode": "true", "não, espera": "false", "o seguro": "true", // wrong: a qualified yes
			"voo pra Natal": "search-flights", "hotel em Recife": "search-hotels", "voo e hotel": "quote-trip",
			"clima":    "search-flights", // wrong: out of scope
			"da manhã": "LATAM 07:10", "qualquer um": "NONE",
		}
		pred, sure := "", 0.9
		for k, v := range pick {
			if strings.Contains(s, k) {
				pred = v
				if k == "o seguro" || k == "clima" {
					sure = 0.55 // its two errors, unsure, as a calibrated model is
				}
			}
		}
		probs := map[string]float64{}
		if q.Kind == decide.KindNoul {
			p := sure
			if pred == "false" {
				p = 1 - sure
			}
			probs["true"], probs["false"] = p, 1-p
			out.Answers[id] = decide.Answer{Kind: q.Kind, Noul: p, Probabilities: probs, Source: decide.ProbNative}
			continue
		}
		for _, o := range q.Options {
			probs[o.Key] = (1 - sure) / float64(len(q.Options)-1)
		}
		probs[pred] = sure
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		out.Answers[id] = a
	}
	return out, nil
}

func TestGenerateExampleBench(t *testing.T) {
	if os.Getenv("GEN_EXAMPLE_BENCH") != "1" {
		t.Skip("set GEN_EXAMPLE_BENCH=1 to regenerate")
	}
	cases, err := LoadDataset(exampleDataset)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(exampleCassette)
	cas, err := decide.LoadCassette(exampleCassette, exampleTarget, scripted{}, decide.CassetteRecord)
	if err != nil {
		t.Fatal(err)
	}
	trials := Run(context.Background(), cases, []Target{{Label: exampleTarget, Decider: cas}}, Options{Repeats: 3})
	if err := cas.Save(exampleCassette); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(exampleBaseline)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, tr := range trials {
		tr.Latency = 0
		if err := enc.Encode(tr); err != nil {
			t.Fatal(err)
		}
	}
}
