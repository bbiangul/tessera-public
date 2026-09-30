//go:build integration

package host

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/runtime"
)

const sidePriorityExperiment = " An additional service or action distinct from the pending process must be handled as request, even when the same message also confirms the pending process. A question about whether such an extra service is possible may be question or request, but never answer. Resolve that side request before accepting a later unqualified confirmation. Changing the date, time, amount or option of THIS pending process remains answer, not a side request."

type interruptionProbe struct {
	inner   decide.Decider
	variant string
	req     decide.Request
	resp    *decide.Response
}

func (d *interruptionProbe) Name() string { return d.inner.Name() }
func (d *interruptionProbe) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if d.variant == "B" {
		q := req.Questions["turn"]
		q.Instructions += sidePriorityExperiment
		req.Questions["turn"] = q
	}
	d.req = req
	var err error
	d.resp, err = d.inner.Decide(ctx, req)
	return d.resp, err
}

type interruptionCase struct {
	ID      string `json:"id"`
	Pending string `json:"pending"`
	Message string `json:"message"`
	Want    string `json:"want"`
}
type interruptionRow struct {
	ID       string           `json:"id"`
	Variant  string           `json:"variant"`
	Repeat   int              `json:"repeat"`
	Want     string           `json:"want"`
	Got      string           `json:"got"`
	Correct  bool             `json:"correct"`
	Request  decide.Request   `json:"request"`
	Response *decide.Response `json:"response"`
	Error    string           `json:"error,omitempty"`
	Cost     float64          `json:"costUSD"`
	Latency  int64            `json:"latencyMs"`
}

func TestInterruptionExperiment(t *testing.T) {
	data, out := os.Getenv("INTERRUPTION_DATA"), os.Getenv("INTERRUPTION_OUT")
	if data == "" || out == "" || os.Getenv("TYPESAFE_API_KEY") == "" {
		t.Skip("explicit dataset/output and provider key required")
	}
	variants := os.Getenv("INTERRUPTION_VARIANTS")
	if variants == "" {
		variants = "A,B,C,D"
	}
	var cases []interruptionCase
	b, err := os.ReadFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(out, "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	global := ledger.New(ledger.Budget{MaxCostUSD: .1})
	inner := &decide.Metered{Inner: &decide.TypeSafe{APIKey: os.Getenv("TYPESAFE_API_KEY"), Model: "jev-1.13.0"}, Ledger: global}
	type job struct {
		c   interruptionCase
		rep int
	}
	var jobs []job
	for _, c := range cases {
		for rep := 1; rep <= 3; rep++ {
			jobs = append(jobs, job{c, rep})
		}
	}
	rand.New(rand.NewSource(5005)).Shuffle(len(jobs), func(i, j int) { jobs[i], jobs[j] = jobs[j], jobs[i] })
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, j := range jobs {
		sem <- struct{}{}
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			order := []string{"A", "B"}
			if j.rep%2 == 0 {
				order = []string{"B", "A"}
			}
			for _, v := range order {
				if !strings.Contains(variants, v) && !(v == "B" && strings.Contains(variants, "D")) {
					continue
				}
				local := ledger.New(ledger.Budget{MaxCostUSD: .01})
				probe := &interruptionProbe{inner: &decide.Metered{Inner: inner, Ledger: local}, variant: v}
				h := &Host{Runtime: &runtime.Runtime{}, opts: Options{Decider: probe}}
				start := time.Now()
				got, e := h.interruption(context.Background(), j.c.Pending, j.c.Message)
				row := interruptionRow{ID: j.c.ID, Variant: v, Repeat: j.rep, Want: j.c.Want, Got: got, Request: probe.req, Response: probe.resp, Cost: local.Totals().CostUSD, Latency: time.Since(start).Milliseconds()}
				if e != nil {
					row.Error = e.Error()
				}
				emit := func(r interruptionRow) {
					if r.Got == turnQuestion || r.Got == turnRequest {
						r.Got = "side"
					}
					r.Correct = r.Error == "" && r.Want == r.Got
					mu.Lock()
					defer mu.Unlock()
					if e := json.NewEncoder(f).Encode(r); e != nil {
						t.Error(e)
					}
				}
				emit(row)
				if (v == "A" && strings.Contains(variants, "C")) || (v == "B" && strings.Contains(variants, "D")) {
					row.Variant = "C"
					if v == "B" {
						row.Variant = "D"
					}
					if probe.resp != nil {
						a := probe.resp.Answers["turn"]
						if a.Probabilities[turnQuestion]+a.Probabilities[turnRequest] >= .6 {
							row.Got = turnQuestion
						}
					}
					emit(row)
				}
			}
		}(j)
	}
	wg.Wait()
	raw, err := json.MarshalIndent(global.Totals(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "ledger.json"), append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
