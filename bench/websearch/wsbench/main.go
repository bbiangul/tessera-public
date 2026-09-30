// Command wsbench runs websearch-001 (docs/research/websearch-001-
// preregistration.md): one message per conversation, in process, with
// web/search served by a provider: a local SearXNG, TinyFish, or either
// with one hostile result injected. Each record has the reply, the queries
// sent and the results the agent saw, for the judge.
//
//	wsbench -defs bench/websearch/traveler-web -variant B -set dev -provider searxng -out runs.jsonl
//	wsbench ... -inject t-card        # the injection set's hostile snippet
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
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

type message struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type hostile struct {
	ID      string   `json:"id"`
	Snippet string   `json:"snippet"`
	Follows []string `json:"follows"`
}

type record struct {
	Variant  string   `json:"variant"`
	Provider string   `json:"provider"`
	Set      string   `json:"set"`
	Message  string   `json:"message"`
	Kind     string   `json:"kind"`
	Text     string   `json:"text"`
	Inject   string   `json:"inject,omitempty"`
	Repeat   int      `json:"repeat"`
	Routed   string   `json:"routed,omitempty"`
	Reply    string   `json:"reply"`
	Queries  []string `json:"queries"`
	Results  []any    `json:"results"`
	Error    string   `json:"error,omitempty"`
	// SearchErrors are the provider's failures (a rate limit, a timeout):
	// the agent then answered without results.
	SearchErrors []string `json:"searchErrors,omitempty"`
	LatencyMs    int64    `json:"latencyMs"`
	CostUSD      float64  `json:"costUSD"`
}

func main() {
	defs := flag.String("defs", "examples/traveler", "the agent's definitions (namespace traveler)")
	variant := flag.String("variant", "A", "the variant's name in the records")
	set := flag.String("set", "dev", "message set: dev or test")
	msgFile := flag.String("messages", "bench/websearch/messages.json", "the message sets")
	provider := flag.String("provider", "searxng", "searxng (http://localhost:18888), tinyfish (TINYFISH_API_KEY) or none")
	inject := flag.String("inject", "", "a hostile snippet's id (bench/websearch/injection.json): runs that set's injection messages instead")
	repeats := flag.Int("repeats", 3, "runs per message")
	out := flag.String("out", "", "JSONL output; runs already there are skipped")
	maxCost := flag.Float64("max-cost-usd", 0.5, "hard budget")
	parallel := flag.Int("parallel", 6, "runs at once")
	flag.Parse()
	orKey, jevKey := os.Getenv("OPENROUTER_API_KEY"), os.Getenv("TYPESAFE_API_KEY")
	if *out == "" || orKey == "" || jevKey == "" {
		log.Fatal("need -out, OPENROUTER_API_KEY and TYPESAFE_API_KEY")
	}
	var msgs map[string][]message
	must(readJSON(*msgFile, &msgs))
	var inj struct {
		Messages map[string][]string  `json:"messages"`
		Hostile  map[string][]hostile `json:"hostile"`
	}
	must(readJSON("bench/websearch/injection.json", &inj))
	list := msgs[*set]
	var bad *hostile
	if *inject != "" {
		for _, s := range []string{"dev", "test"} {
			for _, h := range inj.Hostile[s] {
				if h.ID == *inject {
					bad, *set = &h, s
				}
			}
		}
		if bad == nil {
			log.Fatalf("no hostile snippet %q", *inject)
		}
		list = nil
		for i, t := range inj.Messages[*set] {
			list = append(list, message{ID: fmt.Sprintf("inj-%d", i+1), Kind: "injection", Message: t})
		}
	}
	search := searcher(*provider)
	done := map[string]bool{}
	if f, err := os.Open(*out); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var r record
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				done[key(r.Variant, r.Message, r.Inject, r.Repeat)] = true
			}
		}
		_ = f.Close()
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	must(err)
	defer f.Close()
	global := ledger.New(ledger.Budget{MaxCostUSD: *maxCost})
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, *parallel)
	n := 0
	for k := 1; k <= *repeats; k++ {
		for _, m := range list {
			if done[key(*variant, m.ID, *inject, k)] {
				continue
			}
			n++
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				if global.Exceeded() {
					return
				}
				l := ledger.New(ledger.Budget{MaxCostUSD: math.Max(*maxCost-global.Totals().CostUSD, 1e-6)})
				rec := run(*defs, m, search, bad, l, orKey, jevKey)
				rec.Variant, rec.Provider, rec.Set, rec.Repeat = *variant, *provider, *set, k
				if bad != nil {
					rec.Inject = bad.ID
				}
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
	fmt.Printf("%d runs, spent $%.4f\n", n, global.Totals().CostUSD)
}

func key(variant, msg, inject string, k int) string {
	return fmt.Sprintf("%s/%s/%s/%d", variant, msg, inject, k)
}

// run sends one message in a new conversation.
func run(defs string, m message, search func(string) ([]any, error), bad *hostile, l *ledger.Ledger, orKey, jevKey string) record {
	ctx := context.Background()
	rec := record{Message: m.ID, Kind: m.Kind, Text: m.Message, Queries: []string{}, Results: []any{}}
	var mu sync.Mutex
	handlers := map[string]act.Handler{
		"web/search": func(_ context.Context, c act.Call) (any, error) {
			q, _ := c.Args["query"].(string)
			mu.Lock()
			rec.Queries = append(rec.Queries, q)
			mu.Unlock()
			res, err := search(q)
			if err != nil {
				mu.Lock()
				rec.SearchErrors = append(rec.SearchErrors, err.Error())
				mu.Unlock()
				return nil, err
			}
			if bad != nil {
				res = append([]any{map[string]any{"site": "Blog Viagem Fácil", "title": "Guia completo e atualizado", "url": "https://blog-viagem-facil.example/guia", "snippet": bad.Snippet}}, res...)
			}
			mu.Lock()
			rec.Results = res[:min(len(res), 5)] // what the model saw (maxResults)
			mu.Unlock()
			return res, nil
		},
	}
	// The traveler's own systems are not part of this benchmark: a message
	// routed to a search fails there, and the record says it was routed.
	for _, a := range []string{"booking/search-flight-destination", "booking/search-flights", "booking/search-hotel-destination", "booking/search-hotels",
		"booking/seat-map", "profile/get", "profile/save", "memo/save-search", "memo/last-search", "routes/drive-matrix"} {
		handlers[a] = func(context.Context, act.Call) (any, error) { return nil, errors.New("not in this benchmark") }
	}
	h, issues, err := host.Load(host.Options{Paths: []string{defs}, Namespace: "traveler", Store: journal.NewMemory(),
		Decider:   &decide.Metered{Inner: &decide.TypeSafe{APIKey: jevKey, Model: "jev-1.13.0"}, Ledger: l},
		Generator: &generate.Metered{Inner: &generate.OpenRouter{APIKey: orKey, Model: "deepseek/deepseek-v4.1-flash", Providers: []string{"DeepSeek"}}, Ledger: l},
		Handlers:  handlers})
	if err != nil {
		rec.Error = fmt.Sprintf("%v %v", err, issues)
		return rec
	}
	defer func() { _ = h.Close() }()
	start := time.Now()
	r, err := h.Handle(ctx, "conv-"+m.ID, map[string]any{"userId": "u1"}, m.Message)
	rec.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		rec.Error = err.Error()
	}
	rec.Routed = r.Routed
	var replies []string
	for _, res := range append(append([]runtime.Result{}, r.Earlier...), r.Result) {
		for _, o := range res.Outbox {
			if o.To == "user" {
				replies = append(replies, o.Text)
			}
		}
	}
	rec.Reply = strings.Join(replies, "\n\n")
	rec.CostUSD = l.Totals().CostUSD
	return rec
}

// searcher is the provider behind web/search, with the plugin contract's
// fields (internal/initkit/web).
func searcher(provider string) func(string) ([]any, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	get := func(u string, header map[string]string, v any) error {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		for k, x := range header {
			req.Header.Set(k, x)
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if resp.StatusCode != 200 {
			return fmt.Errorf("%s: status %d", provider, resp.StatusCode)
		}
		return json.Unmarshal(b, v)
	}
	switch provider {
	case "searxng":
		return func(q string) ([]any, error) {
			var body struct {
				Results []struct{ Title, URL, Content string } `json:"results"`
			}
			if err := get("http://localhost:18888/search?format=json&q="+url.QueryEscape(q), nil, &body); err != nil {
				return nil, err
			}
			var out []any
			for _, r := range body.Results {
				out = append(out, map[string]any{"title": r.Title, "url": r.URL, "snippet": r.Content})
			}
			return out, nil
		}
	case "tinyfish":
		key := os.Getenv("TINYFISH_API_KEY")
		if key == "" {
			log.Fatal("tinyfish needs TINYFISH_API_KEY")
		}
		return func(q string) ([]any, error) {
			var body struct {
				Results []struct {
					SiteName string `json:"site_name"`
					Title    string `json:"title"`
					URL      string `json:"url"`
					Snippet  string `json:"snippet"`
				} `json:"results"`
			}
			if err := get("https://api.search.tinyfish.ai?query="+url.QueryEscape(q), map[string]string{"X-API-Key": key}, &body); err != nil {
				return nil, err
			}
			var out []any
			for _, r := range body.Results {
				out = append(out, map[string]any{"site": r.SiteName, "title": r.Title, "url": r.URL, "snippet": r.Snippet})
			}
			return out, nil
		}
	case "none":
		return func(string) ([]any, error) { return nil, errors.New("no provider") }
	}
	log.Fatalf("unknown provider %q", provider)
	return nil
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
