// Command recoverybench measures bounded clarification with the actual host/runtime.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

type dialogue struct {
	ID           string  `json:"id"`
	Message      string  `json:"message"`
	Reply        *string `json:"reply"`
	Allowed      []int   `json:"allowed"`
	ClearControl bool    `json:"clearControl"`
}
type line struct {
	Who  string `json:"who"`
	Text string `json:"text"`
}
type result struct {
	ID          string         `json:"id"`
	Repeat      int            `json:"repeat"`
	Correct     bool           `json:"correct"`
	Unsafe      bool           `json:"unsafe"`
	Asked       bool           `json:"asked"`
	ExtraTurns  int            `json:"extraTurns"`
	SameRun     bool           `json:"sameRun"`
	FinalStatus journal.Status `json:"finalStatus"`
	Error       string         `json:"error,omitempty"`
	Transcript  []line         `json:"transcript"`
	Masks       [][]int        `json:"masks"`
	RunIDs      []string       `json:"runIDs"`
	CostUSD     float64        `json:"costUSD"`
	Millis      int64          `json:"millis"`
}

var days = []string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}

func offered(text string) ([]int, bool) {
	const prefix = "Dias permitidos: "
	if !strings.HasPrefix(text, prefix) {
		return nil, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(text, prefix), ".")
	out := []int{}
	for _, name := range strings.Split(raw, ", ") {
		found := false
		for i, d := range days {
			if name == d {
				out = append(out, i)
				found = true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return out, true
}
func assess(d dialogue, r *result) {
	if d.Allowed == nil {
		r.Unsafe = len(r.Masks) > 0
		r.Correct = !r.Unsafe && r.FinalStatus == journal.StatusSucceeded && r.Error == ""
		return
	}
	want := map[int]bool{}
	for _, i := range d.Allowed {
		want[i] = true
	}
	for _, mask := range r.Masks {
		for _, i := range mask {
			if !want[i] {
				r.Unsafe = true
			}
		}
	}
	r.Correct = len(r.Masks) > 0 && reflect.DeepEqual(r.Masks[len(r.Masks)-1], d.Allowed) && !r.Unsafe && r.Error == "" && r.FinalStatus == journal.StatusSucceeded
	if d.ClearControl && r.ExtraTurns != 0 {
		r.Correct = false
	}
}

func run(ctx context.Context, defs string, d dialogue, decider decide.Decider) (r result) {
	r.ID = d.ID
	r.SameRun = true
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	h, _, err := host.Load(host.Options{Paths: []string{defs}, Namespace: "travel", Store: journal.NewMemory(), Decider: decider, Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return now } }})
	if err != nil {
		r.Error = err.Error()
		return
	}
	defer func() {
		if err := h.Close(); err != nil {
			r.Error = err.Error()
			r.Correct = false
		}
	}()
	consume := func(x runtime.Result) {
		r.FinalStatus = x.Status
		if x.Error != "" {
			r.Error = x.Error
		}
		if x.RunID != "" {
			r.RunIDs = append(r.RunIDs, x.RunID)
		}
		for _, m := range x.Outbox {
			if m.To == "user" {
				r.Transcript = append(r.Transcript, line{"agent", m.Text})
				if strings.Contains(m.Text, "Liste todos os dias desejados") {
					r.Asked = true
				}
				if mask, ok := offered(m.Text); ok {
					r.Masks = append(r.Masks, mask)
				}
			}
		}
		if len(x.Outbox) > 0 && x.RunID != "" {
			if err := h.Runtime.Ack(ctx, x.RunID, x.Outbox[len(x.Outbox)-1].Seq); err != nil {
				r.Error = err.Error()
			}
		}
	}
	handle := func(text string) {
		r.Transcript = append(r.Transcript, line{"user", text})
		x, err := h.Handle(ctx, "conversation", nil, text)
		if err != nil {
			r.Error = err.Error()
			return
		}
		for _, earlier := range x.Earlier {
			consume(earlier)
		}
		if r.ExtraTurns > 0 && x.RunID == "" {
			r.SameRun = false
		}
		consume(x.Result)
	}
	handle(d.Message)
	if r.Error == "" && r.Asked {
		if d.Reply != nil {
			r.ExtraTurns = 1
			handle(*d.Reply)
		} else {
			now = now.Add(2 * time.Hour)
			wakes, err := h.Tick(ctx)
			if err != nil {
				r.Error = err.Error()
			} else {
				for _, x := range wakes {
					consume(x)
				}
			}
		}
	}
	for _, id := range r.RunIDs {
		if id != r.RunIDs[0] {
			r.SameRun = false
		}
	}
	assess(d, &r)
	return
}
func main() {
	defs := flag.String("defs", "", "frozen resources")
	data := flag.String("dataset", "", "dialogues")
	out := flag.String("out", "", "results JSONL")
	tape := flag.String("cassette", "", "recordings")
	mode := flag.String("mode", "record", "record or replay")
	cap := flag.Float64("max-cost", .1, "USD cap")
	repeats := flag.Int("repeats", 2, "repeats")
	flag.Parse()
	led := ledger.New(ledger.Budget{MaxCostUSD: *cap})
	var inner decide.Decider
	if *mode == "record" {
		inner = &decide.Metered{Inner: &decide.TypeSafe{APIKey: os.Getenv("TYPESAFE_API_KEY"), Model: "jev-1.13.0"}, Ledger: led}
	}
	c, err := decide.LoadCassette(*tape, "typesafe:jev-1.13.0", inner, decide.CassetteMode(*mode))
	must(err)
	raw, err := os.ReadFile(*data)
	must(err)
	var cases []dialogue
	must(json.Unmarshal(raw, &cases))
	f, err := os.Create(*out)
	must(err)
	for rep := 0; rep < *repeats; rep++ {
		for _, d := range cases {
			c.SetScope(fmt.Sprintf("recovery/%s/%d", d.ID, rep))
			before := led.Totals().CostUSD
			start := time.Now()
			r := run(context.Background(), *defs, d, c)
			r.Repeat = rep
			r.Millis = time.Since(start).Milliseconds()
			r.CostUSD = led.Totals().CostUSD - before
			must(json.NewEncoder(f).Encode(r))
			if *mode == "record" {
				must(c.Save(*tape))
			}
		}
	}
	must(f.Close())
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
