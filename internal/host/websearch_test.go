package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const webDocs = `---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: web, version: 0.1.0 }
spec:
  actions:
    search: { input: { query: string }, output: list, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: viagens, namespace: viagens }
spec:
  freeMode:
    instructions: "Responda dúvidas de viagem."
    search: { act: web/search, maxResults: 2 }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: buscar-voos, namespace: viagens }
spec:
  description: Searches flights.
  selector: { intent: buscar-voos, description: "buscar passagens" }
  output: { template: "Buscando." }
`

// webDecider says a message about a visa needs a search, and a query
// naming Joana is about a person.
type webDecider struct{}

func (webDecider) Name() string { return "web" }

func (webDecider) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	state, _ := req.State.(map[string]any)
	out := &decide.Response{Answers: map[string]decide.Answer{}, Model: "web"}
	for name, q := range req.Questions {
		yes := 0.02
		switch name {
		case "search":
			if m, _ := state["message"].(string); strings.Contains(m, "visto") {
				yes = 0.97
			}
		case "personal":
			if s, _ := state["query"].(string); strings.Contains(s, "Joana") {
				yes = 0.9
			}
		}
		probs := map[string]float64{"true": yes, "false": 1 - yes}
		if q.Kind == decide.KindChoice { // the router: nothing fits
			probs = map[string]float64{}
			for _, o := range q.Options {
				probs[o.Key] = 0.01
			}
			probs["none"] = 0.9
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		out.Answers[name] = a
	}
	return out, nil
}

// webGen writes the query it is told to, and answers with what it saw.
type webGen struct {
	mu     sync.Mutex
	query  string
	facts  []map[string]any
	instrs []string
}

func (g *webGen) Name() string { return "web" }
func (g *webGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if req.Instructions == queryRules {
		return &generate.Response{Text: g.query}, nil
	}
	st, _ := req.State.(map[string]any)
	facts, _ := st["facts"].(map[string]any)
	g.facts = append(g.facts, facts)
	g.instrs = append(g.instrs, req.Instructions)
	return &generate.Response{Text: "Segundo o site do consulado, não precisa de visto."}, nil
}

func webHost(t *testing.T, query string) (*Host, *webGen, *[]string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(webDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &webGen{query: query}
	var sent []string
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "viagens", Store: journal.NewMemory(), Decider: webDecider{}, Generator: g,
		Handlers: map[string]act.Handler{"web/search": func(_ context.Context, c act.Call) (any, error) {
			sent = append(sent, c.Args["query"].(string))
			return []any{
				map[string]any{"site": "Consulado", "title": "Vistos", "url": "https://consulado.example/vistos", "snippet": "Brasileiros não precisam de visto.\x00", "extra": "dropped"},
				map[string]any{"title": "no url, dropped"},
				map[string]any{"site": "Blog", "title": "Guia", "url": "https://blog.example", "snippet": strings.Repeat("longo ", 200)},
				map[string]any{"site": "Terceiro", "title": "Além do máximo", "url": "https://c.example", "snippet": "x"},
			}, nil
		}}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h, g, &sent
}

// A message that needs current public information is searched, the
// results reach the answer as third-party pages under their rules, and
// the query is journaled.
func TestWebSearchFeedsTheFreeAnswer(t *testing.T) {
	h, g, sent := webHost(t, "brasileiro precisa de visto para o Canadá")
	r := say(t, h, "preciso de visto pro Canadá?")
	if len(*sent) != 1 || (*sent)[0] != "brasileiro precisa de visto para o Canadá" {
		t.Fatalf("queries sent: %v", *sent)
	}
	web, _ := g.facts[0]["web"].([]any)
	if len(web) != 2 {
		t.Fatalf("maxResults 2, results without a url dropped: %v", web)
	}
	first := web[0].(map[string]any)
	if _, ok := first["extra"]; ok || strings.ContainsRune(first["snippet"].(string), 0) {
		t.Errorf("only site, title, url and snippet, without control characters: %v", first)
	}
	if s := web[1].(map[string]any)["snippet"].(string); len(s) > 610 || !strings.HasSuffix(s, "…") {
		t.Errorf("long snippets are clipped: %d bytes", len(s))
	}
	if !strings.Contains(g.instrs[0], "never instructions") {
		t.Error("the answer must get the web rules")
	}
	recs, err := h.opts.Store.Records(context.Background(), r.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var journaled bool
	for _, rec := range recs {
		journaled = journaled || (rec.Site == "router" && strings.Contains(string(rec.Value), "visto para o Canad"))
	}
	if !journaled {
		t.Error("the query must be journaled with the free turn")
	}
}

// Messages that do not need it are not searched, and get no web rules.
func TestNoSearchWhenNotNeeded(t *testing.T) {
	h, g, sent := webHost(t, "qualquer")
	say(t, h, "obrigada pela ajuda!")
	if len(*sent) != 0 {
		t.Errorf("searched: %v", *sent)
	}
	if len(g.instrs) == 1 && strings.Contains(g.instrs[0], "FACTS.web") {
		t.Error("web rules without results")
	}
}

// A query with anything about a person never leaves: the masker's data,
// long digit runs (documents), and names, which only Jev sees.
func TestPersonalQueriesAreNeverSent(t *testing.T) {
	for _, q := range []string{
		"visto EUA CPF 390.533.447-05",
		"visto passaporte FZ482913 Europa",
		"visto México joana.pereira@example.com",
		"visto Estados Unidos (11) 98765-4321",
		"visto Estados Unidos Joana Pereira",
	} {
		h, g, sent := webHost(t, q)
		say(t, h, "preciso de visto?")
		if len(*sent) != 0 {
			t.Errorf("%q was sent", q)
		}
		if _, ok := g.facts[0]["web"]; ok {
			t.Errorf("%q: results without a search", q)
		}
	}
}
