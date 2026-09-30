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

const preferencePriorityExperiment = " A message may both give a scheduling preference and ask an independent factual question (parking, address, insurance, price). In that case choose question, even if the question is not phrased as a prerequisite. Answer that information question first. Asking whether a different date/time is possible is still answer, not an information question."
const preferenceContextExperiment = " sideMessages contains earlier user messages already answered while waiting for this reply. Use their scheduling preferences to interpret a continuation such as permission to search those options; that continuation is answer. Do not answer their old factual questions again; classify only the current message."
const preferenceStopExperiment = "Using ONLY the current STATE.message, does the user want to stop or give up THIS pending rescheduling process, keeping their existing appointment unchanged? Explicit stop/leave it as it is/no longer want to change means true. Requesting cancellation of an appointment, especially another person or another appointment, is a different service and means false. A date correction, read-only search continuation, or factual question alone is false. Ignore earlier sideMessages for this decision."
const preferenceContinueExperiment = "Does the current message authorize consulting/searching scheduling options for this SAME process, including a brief acknowledgment referring to preferences in sideMessages? This is permission to READ availability only, never to book, reschedule, pay or cancel. True examples: go ahead and look, check those days, yes consult those options. False: factual questions, new services, cancellation, goodbye, changing an appointment now, or merely stating a day/time."
const preferenceInfoExperiment = "Does the message ask for factual information independent of the scheduling preference, such as parking, address, insurance, price or contact details? Answer true even when the message also supplies a preferred day/time, and even when the question is not a prerequisite. Asking for a different date/time or permission to search options is false. A cancellation or goodbye alone is false."

type preferenceProbe struct {
	inner   decide.Decider
	variant string
	side    []string
	req     decide.Request
	resp    *decide.Response
}

func (d *preferenceProbe) Name() string { return d.inner.Name() }
func (d *preferenceProbe) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if d.variant == "E" {
		q := req.Questions["turn"]
		q.Instructions += preferencePriorityExperiment
		req.Questions["turn"] = q
	}
	if d.variant == "G" || d.variant == "H" || d.variant == "I" || d.variant == "J" || d.variant == "K" {
		q := req.Questions["turn"]
		q.Instructions += preferenceContextExperiment
		req.Questions["turn"] = q
		st := req.State.(map[string]any)
		st["sideMessages"] = d.side
	}
	if d.variant == "F" || d.variant == "G" || d.variant == "H" || d.variant == "I" || d.variant == "J" || d.variant == "K" {
		req.Questions["information"] = decide.Question{Kind: decide.KindNoul, Instructions: preferenceInfoExperiment, IfTrue: "An independent factual question is present.", IfFalse: "No independent factual question."}
	}
	if d.variant == "H" || d.variant == "J" {
		req.Questions["continueSearch"] = decide.Question{Kind: decide.KindNoul, Instructions: preferenceContinueExperiment, IfTrue: "Asks to look up options for this same pending process.", IfFalse: "Not a read-only continuation."}
	}
	if d.variant == "I" || d.variant == "J" || d.variant == "K" {
		q := req.Questions["information"]
		q.Instructions += " Read ONLY the current STATE.message to answer this question. Ignore all questions in STATE.sideMessages: those older questions have already been answered. Their presence does not make the current message an information question."
		req.Questions["information"] = q
	}
	if d.variant == "K" {
		req.Questions["stopCurrent"] = decide.Question{Kind: decide.KindNoul, Instructions: preferenceStopExperiment, IfTrue: "Stop this rescheduling process without changing the existing appointment.", IfFalse: "Not a withdrawal from this process."}
	}
	d.req = req
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var err error
	d.resp, err = d.inner.Decide(ctx, req)
	return d.resp, err
}

type preferenceCase struct {
	ID      string   `json:"id"`
	Pending string   `json:"pending"`
	Message string   `json:"message"`
	Want    string   `json:"want"`
	Side    []string `json:"sideMessages,omitempty"`
}
type preferenceRow struct {
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

func TestPreferenceExperiment(t *testing.T) {
	data, out := os.Getenv("PREFERENCE_DATA"), os.Getenv("PREFERENCE_OUT")
	if data == "" || out == "" || os.Getenv("TYPESAFE_API_KEY") == "" {
		t.Skip("explicit dataset/output and provider key required")
	}
	variants := os.Getenv("PREFERENCE_VARIANTS")
	if variants == "" {
		variants = "A,E,F"
	}
	var cases []preferenceCase
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
		c   preferenceCase
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
			order := []string{"A", "E", "F", "G", "H", "I", "J", "K"}
			if j.rep%2 == 0 {
				order = []string{"K", "J", "I", "H", "G", "F", "E", "A"}
			}
			for _, v := range order {
				if !strings.Contains(variants, v) {
					continue
				}
				local := ledger.New(ledger.Budget{MaxCostUSD: .01})
				probe := &preferenceProbe{inner: &decide.Metered{Inner: inner, Ledger: local}, variant: v, side: j.c.Side}
				h := &Host{Runtime: &runtime.Runtime{}, opts: Options{Decider: probe}}
				start := time.Now()
				got, e := h.interruption(context.Background(), j.c.Pending, j.c.Message)
				if (v == "F" || v == "G" || v == "H" || v == "I" || v == "J" || v == "K") && e == nil && probe.resp.Answers["information"].Noul >= .8 {
					got = turnQuestion
				}
				if (v == "H" || v == "J") && e == nil && probe.resp.Answers["information"].Noul <= .2 && probe.resp.Answers["continueSearch"].Noul >= .8 && got != turnCancel && got != turnGoodbye {
					got = turnAnswer
				}
				if v == "K" && e == nil && probe.resp.Answers["stopCurrent"].Noul >= .8 {
					got = turnCancel
				}
				row := preferenceRow{ID: j.c.ID, Variant: v, Repeat: j.rep, Want: j.c.Want, Got: got, Request: probe.req, Response: probe.resp, Cost: local.Totals().CostUSD, Latency: time.Since(start).Milliseconds()}
				if e != nil {
					row.Error = e.Error()
				}
				emit := func(r preferenceRow) {
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
