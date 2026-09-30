// Command escbench runs the escalation benchmark's Tessera agent
// (docs/research/escalation-000-preregistration.md): one question per
// conversation, in process, on an in-memory journal. A run records whether
// the coach was asked and the reply. The LLM competitors run in
// competitors.py; both write the same JSONL records.
//
//	escbench -set test -repeats 20 -out runs.jsonl [-defs bench/escalation/tessera]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

type question struct {
	ID    string `json:"id"`
	Set   string `json:"set"`
	Label string `json:"label"`
	Text  string `json:"text"`
}

type record struct {
	Competitor string  `json:"competitor"`
	Question   string  `json:"question"`
	Label      string  `json:"label"`
	Repeat     int     `json:"repeat"`
	Escalated  bool    `json:"escalated"`
	AskCalls   int     `json:"askCalls"`
	Reply      string  `json:"reply"`
	Error      string  `json:"error"`
	LatencyMs  int64   `json:"latencyMs"`
	Calls      int     `json:"calls"`
	CostUSD    float64 `json:"costUSD"`
	Routed     string  `json:"routed,omitempty"`
	// Handoff is the free mode's decision, when it was asked: each
	// option's probability.
	Handoff map[string]float64 `json:"handoff,omitempty"`
}

func main() {
	defs := flag.String("defs", "bench/escalation/tessera", "the agent's definitions (namespace brightpath)")
	name := flag.String("name", "tessera", "the competitor's name in the records")
	set := flag.String("set", "test", "question set")
	repeats := flag.Int("repeats", 20, "runs per question")
	out := flag.String("out", "", "JSONL output; runs already there are skipped")
	maxCost := flag.Float64("max-cost-usd", 1, "hard budget")
	parallel := flag.Int("parallel", 12, "runs at once")
	model := flag.String("model", "deepseek/deepseek-v4.1-flash@DeepSeek", "the generate model, @provider")
	convs := flag.String("conversations", "", "multi-turn mode: a conversations file (bench/escalation/conversations.json)")
	flag.Parse()
	orKey, jevKey := os.Getenv("OPENROUTER_API_KEY"), os.Getenv("TYPESAFE_API_KEY")
	if *out == "" || orKey == "" || jevKey == "" {
		log.Fatal("need -out, OPENROUTER_API_KEY and TYPESAFE_API_KEY")
	}
	var checklist map[string]any
	var qs struct{ Questions []question }
	must(readJSON("bench/escalation/checklist.json", &checklist))
	must(readJSON("bench/escalation/questions.json", &qs))
	m, prov, _ := strings.Cut(*model, "@")
	var providers []string
	if prov != "" {
		providers = []string{prov}
	}
	done := map[string]bool{}
	if f, err := os.Open(*out); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var r record
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				done[fmt.Sprintf("%s/%s/%d", r.Competitor, r.Question, r.Repeat)] = true
			}
		}
		_ = f.Close()
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	must(err)
	defer f.Close()

	global := ledger.New(ledger.Budget{MaxCostUSD: *maxCost})
	if *convs != "" {
		must(runConversations(*convs, *defs, *name, *repeats, *parallel, *maxCost, f, global, checklist, m, providers, orKey, jevKey))
		fmt.Printf("spent $%.4f\n", global.Totals().CostUSD)
		return
	}
	var mu sync.Mutex
	jobs := make(chan func())
	var wg sync.WaitGroup
	for range *parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				j()
			}
		}()
	}
	n := 0
	for k := 1; k <= *repeats; k++ {
		for _, q := range qs.Questions {
			if q.Set != *set || done[fmt.Sprintf("%s/%s/%d", *name, q.ID, k)] {
				continue
			}
			n++
			jobs <- func() {
				if global.Exceeded() {
					return
				}
				runL := ledger.New(ledger.Budget{MaxCostUSD: math.Max(*maxCost-global.Totals().CostUSD, 1e-6)})
				rec := run(*defs, q, checklist, runL, m, providers, orKey, jevKey)
				rec.Competitor, rec.Repeat = *name, k
				for _, e := range runL.Entries() {
					_ = global.Add(e)
				}
				b, _ := json.Marshal(rec)
				mu.Lock()
				_, _ = f.Write(append(b, '\n'))
				mu.Unlock()
			}
		}
	}
	close(jobs)
	wg.Wait()
	fmt.Printf("%d runs, spent $%.4f\n", n, global.Totals().CostUSD)
}

// convRecord is one multi-turn conversation's replies.
type convRecord struct {
	Competitor   string     `json:"competitor"`
	Conversation string     `json:"conversation"`
	Repeat       int        `json:"repeat"`
	Turns        []string   `json:"turns"`
	Replies      [][]string `json:"replies"`
	Errors       []string   `json:"errors,omitempty"`
	CostUSD      float64    `json:"costUSD"`
}

// runConversations plays each conversation's turns in order, in one
// conversation, acknowledging every reply as a channel does.
func runConversations(path, defs, name string, repeats, parallel int, maxCost float64, f *os.File, global *ledger.Ledger, checklist map[string]any, model string, providers []string, orKey, jevKey string) error {
	var cf struct {
		Conversations []struct {
			ID    string   `json:"id"`
			Turns []string `json:"turns"`
		} `json:"conversations"`
	}
	if err := readJSON(path, &cf); err != nil {
		return err
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, parallel)
	for k := 1; k <= repeats; k++ {
		for _, c := range cf.Conversations {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				l := ledger.New(ledger.Budget{MaxCostUSD: math.Max(maxCost-global.Totals().CostUSD, 1e-6)})
				rec := convRecord{Competitor: name, Conversation: c.ID, Repeat: k, Turns: c.Turns}
				ctx := context.Background()
				h, issues, err := host.Load(host.Options{Paths: []string{defs}, Namespace: "brightpath", Store: journal.NewMemory(),
					Decider:   &decide.Metered{Inner: &decide.TypeSafe{APIKey: jevKey, Model: "jev-1.13.0"}, Ledger: l},
					Generator: &generate.Metered{Inner: &generate.OpenRouter{APIKey: orKey, Model: model, Providers: providers}, Ledger: l},
					Handlers: map[string]act.Handler{
						"checklist/read": func(context.Context, act.Call) (any, error) { return checklist, nil },
					}})
				if err != nil {
					rec.Errors = append(rec.Errors, fmt.Sprintf("%v %v", err, issues))
				} else {
					for _, t := range c.Turns {
						r, err := h.Handle(ctx, "owner-"+c.ID, map[string]any{"role": "owner", "name": "Dana"}, t)
						var out []string
						if err != nil {
							rec.Errors = append(rec.Errors, err.Error())
						}
						for _, res := range append(append([]runtime.Result{}, r.Earlier...), r.Result) {
							for _, m := range res.Outbox {
								if m.To == "user" {
									out = append(out, m.Text)
								}
							}
							if n := len(res.Outbox); n > 0 && res.Outbox[n-1].Seq > 0 {
								_ = h.Runtime.Ack(ctx, res.RunID, res.Outbox[n-1].Seq)
							}
						}
						rec.Replies = append(rec.Replies, out)
					}
					_ = h.Close()
				}
				rec.CostUSD = l.Totals().CostUSD
				for _, e := range l.Entries() {
					_ = global.Add(e)
				}
				b, _ := json.Marshal(rec)
				mu.Lock()
				_, _ = f.Write(append(b, '\n'))
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	return nil
}

// run asks one question in a new conversation.
func run(defs string, q question, checklist map[string]any, l *ledger.Ledger, model string, providers []string, orKey, jevKey string) record {
	ctx := context.Background()
	rec := record{Question: q.ID, Label: q.Label}
	h, issues, err := host.Load(host.Options{Paths: []string{defs}, Namespace: "brightpath", Store: journal.NewMemory(),
		Decider:   &decide.Metered{Inner: &decide.TypeSafe{APIKey: jevKey, Model: "jev-1.13.0"}, Ledger: l},
		Generator: &generate.Metered{Inner: &generate.OpenRouter{APIKey: orKey, Model: model, Providers: providers}, Ledger: l},
		Handlers: map[string]act.Handler{
			"checklist/read": func(context.Context, act.Call) (any, error) { return checklist, nil },
		}})
	if err != nil {
		rec.Error = fmt.Sprintf("%v %v", err, issues)
		return rec
	}
	defer func() { _ = h.Close() }()
	start := time.Now()
	r, err := h.Handle(ctx, "owner-1", map[string]any{"role": "owner"}, q.Text)
	rec.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		rec.Error = err.Error()
	}
	rec.Routed, rec.Handoff = r.Routed, r.Handoff
	var replies []string
	for _, res := range append(append([]runtime.Result{}, r.Earlier...), r.Result) {
		if res.Status == journal.StatusWaitingForHuman {
			rec.Escalated = true
		}
		for _, m := range res.Outbox {
			switch {
			case m.To == "user":
				replies = append(replies, m.Text)
			case strings.HasPrefix(m.To, "queue:coach"):
				rec.Escalated = true
			}
		}
	}
	if rec.Escalated {
		rec.AskCalls = 1
	}
	rec.Reply = strings.Join(replies, "\n\n")
	t := l.Totals()
	rec.CostUSD = t.CostUSD
	rec.Calls = len(l.Entries())
	return rec
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
